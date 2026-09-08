// Package upload 实现 quickserve 的文件上传处理：原始请求体（PUT/curl -T）
// 与 multipart 表单（浏览器/curl -F）统一支持，内置路径清洗、体积上限、
// Token 鉴权、单 IP 限速与覆盖保护。
package upload

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// copyBufferSize 是流式拷贝的缓冲大小，通过 sync.Pool 在并发上传间复用。
const copyBufferSize = 256 << 10 // 256 KiB

// bufPool 复用拷贝缓冲，降低高并发上传的分配压力。
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, copyBufferSize)
		return &b
	},
}

// Config 是上传 handler 的依赖注入点，便于测试与复用。
type Config struct {
	Root         string        // 上传根目录（绝对路径）
	MaxBytes     int64         // 单文件体积上限；0 表示不限制
	Token        string        // 非空时要求 Bearer / ?token= / X-Auth-Token 鉴权
	RatePerMin   int           // 单 IP 每分钟上传请求数上限；0 不限速
	WriteTimeout time.Duration // 慢速上传整体超时（0 用默认 15 分钟）
}

// handler 持有上传处理的可变状态。
type handler struct {
	cfg      Config
	limiter  *rateLimiter
	root     string
	timeout  time.Duration
	inFlight sync.WaitGroup // 优雅关停时等待进行中的写盘
}

// New 构造上传 handler。root 会被转为绝对路径。
func New(cfg Config) (*handler, error) {
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve upload root: %w", err)
	}
	h := &handler{cfg: cfg, root: abs}
	if cfg.RatePerMin > 0 {
		h.limiter = newRateLimiter(cfg.RatePerMin, time.Minute)
	}
	h.timeout = cfg.WriteTimeout
	if h.timeout == 0 {
		h.timeout = 15 * time.Minute
	}
	return h, nil
}

// Wait 等待所有进行中的上传落盘，供优雅关停调用。
func (h *handler) Wait() { h.inFlight.Wait() }

// Timeout 返回慢速上传整体超时，供 server 包配置 http.Server。
func (h *handler) Timeout() time.Duration { return h.timeout }

// ServeHTTP 实现 http.Handler；方法检查由外层路由保证，这里只处理 POST/PUT。
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, HEAD, POST, PUT, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg.Token != "" && !authorized(r, h.cfg.Token) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="quickserve"`)
		http.Error(w, "unauthorized: valid token required", http.StatusUnauthorized)
		return
	}
	if h.limiter != nil {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !h.limiter.Allow(ip, time.Now()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded; retry later", http.StatusTooManyRequests)
			return
		}
	}
	if r.URL.Path == "" || r.URL.Path == "/" {
		if isMultipart(r) {
			// POST / 走浏览器表单：文件名由 part 自带，不依赖 URL。
			h.uploadMultipart(w, r)
			return
		}
		http.Error(w, "target file path required, e.g. POST /files/name.txt", http.StatusBadRequest)
		return
	}
	h.uploadSingle(w, r)
}

// authorized 校验 Bearer 头、X-Auth-Token 头或 ?token= 查询参数（恒定时间比较）。
func authorized(r *http.Request, token string) bool {
	const prefix = "Bearer "
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, prefix) {
		return subtleCompare(strings.TrimPrefix(h, prefix), token)
	}
	if x := r.Header.Get("X-Auth-Token"); x != "" {
		return subtleCompare(x, token)
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return subtleCompare(q, token)
	}
	return false
}

// subtleCompare 恒定时间字符串比较，避免时序侧信道。
func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// isMultipart 判断请求是否为 multipart 表单上传。
func isMultipart(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "multipart/form-data")
}

// uploadSingle 处理原始请求体上传（curl -T / PUT / curl --data-binary）。
func (h *handler) uploadSingle(w http.ResponseWriter, r *http.Request) {
	rel, status, msg := h.safeRelPath(r.URL.Path)
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	full := filepath.Join(h.root, filepath.FromSlash(rel))
	if err := h.checkOverwrite(full); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if h.cfg.MaxBytes > 0 && r.ContentLength > h.cfg.MaxBytes {
		// Content-Length 已知超限：不读 body，直接拒绝并断连。
		w.Header().Set("Connection", "close")
		http.Error(w, fmt.Sprintf("upload exceeds limit (%d > %d bytes)", r.ContentLength, h.cfg.MaxBytes),
			http.StatusRequestEntityTooLarge)
		return
	}
	h.inFlight.Add(1)
	defer h.inFlight.Done()

	var limited io.Reader = r.Body
	if h.cfg.MaxBytes > 0 {
		limited = io.LimitReader(r.Body, h.cfg.MaxBytes+1)
	}
	n, err := h.writeAtomic(full, limited)
	if err != nil {
		http.Error(w, err.Error(), statusFor(err))
		return
	}
	respondCreated(w, rel, n)
}

// uploadMultipart 流式处理 multipart 表单：每个 part 即到即写，不缓存全量。
func (h *handler) uploadMultipart(w http.ResponseWriter, r *http.Request) {
	h.inFlight.Add(1)
	defer h.inFlight.Done()

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expected multipart/form-data: "+err.Error(), http.StatusBadRequest)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/plain; charset=utf-8")
	hdr.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusCreated)

	var saved int
	flusher, _ := w.(http.Flusher)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fmt.Fprintf(w, "error: read part: %v\n", err)
			return
		}
		if part.FileName() == "" {
			// 普通表单字段，流式丢弃其内容。
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		name := sanitizeFilename(part.FileName())
		if name == "" {
			fmt.Fprintf(w, "skip: empty or unsafe filename %q\n", part.FileName())
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		rel, status, msg := h.safeRelPath("/" + name)
		if status != 0 {
			fmt.Fprintf(w, "skip: %s\n", msg)
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		full := filepath.Join(h.root, filepath.FromSlash(rel))
		if err := h.checkOverwrite(full); err != nil {
			fmt.Fprintf(w, "skip %s: %s\n", name, err)
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}
		// 每个文件单独限额，防单个超大文件占满整包配额。
		var limited io.Reader = part
		if h.cfg.MaxBytes > 0 {
			limited = io.LimitReader(part, h.cfg.MaxBytes+1)
		}
		n, err := h.writeAtomic(full, limited)
		_ = part.Close()
		if err != nil {
			fmt.Fprintf(w, "error %s: %v\n", name, err)
			return
		}
		saved++
		fmt.Fprintf(w, "saved %s (%d bytes)\n", rel, n)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if saved == 0 {
		// 状态码已发 201，用正文明确提示没有文件被保存。
		fmt.Fprintln(w, "no files saved")
	}
}

// safeRelPath 将 URL 路径转为安全的相对路径，失败时返回对应 HTTP 状态码。
func (h *handler) safeRelPath(urlPath string) (rel string, status int, msg string) {
	clean, err := sanitizeRelPath(urlPath)
	if errors.Is(err, errBadSegment) {
		if strings.Contains(urlPath, "..") {
			return "", http.StatusForbidden, "path may not contain .."
		}
		return "", http.StatusBadRequest, "invalid path segment"
	}
	if err != nil {
		return "", http.StatusInternalServerError, "sanitize path: " + err.Error()
	}
	return clean, 0, ""
}

// checkOverwrite 覆盖保护：目标已存在且非空则拒绝（409）。
func (h *handler) checkOverwrite(full string) error {
	info, err := os.Stat(full)
	if err == nil && info.Size() > 0 {
		return errors.New("file already exists (refusing to overwrite)")
	}
	return nil
}

// statusFor 将写盘错误映射为 HTTP 状态码。
func statusFor(err error) int {
	switch {
	case errors.Is(err, errTooLarge):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

var errTooLarge = errors.New("upload exceeds size limit")

// writeAtomic 将 r 的内容写到 full：先写同目录临时文件，成功后 rename。
// 任一步失败都会清理临时文件，不留下半截文件；超过 MaxBytes 报 413。
func (h *handler) writeAtomic(full string, r io.Reader) (int64, error) {
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".qs-upload-*")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	bufp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufp)

	n, copyErr := io.CopyBuffer(tmp, r, *bufp)
	if copyErr == nil && h.cfg.MaxBytes > 0 && n > h.cfg.MaxBytes {
		copyErr = errTooLarge
	}
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	if copyErr == nil {
		copyErr = tmp.Close()
	}
	if copyErr != nil {
		return n, copyErr
	}
	if err := os.Rename(tmpName, full); err != nil {
		// Windows 上目标已存在时 rename 失败，补充检查给出明确错误。
		if _, statErr := os.Stat(full); statErr == nil {
			return n, errors.New("file already exists (refusing to overwrite)")
		}
		return n, fmt.Errorf("finalize upload: %w", err)
	}
	cleanup = false
	return n, nil
}

// respondCreated 输出 201 响应体。
func respondCreated(w http.ResponseWriter, rel string, n int64) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "uploaded %s (%d bytes)\n", rel, n)
}

// DrainBody 丢弃并关闭请求体，帮助连接复用（keep-alive）。
func DrainBody(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
}

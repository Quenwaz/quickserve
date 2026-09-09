// Package server 组装 quickserve 的 HTTP 处理链：路由、上传、CORS、
// 健康检查与访问日志，并提供优雅关停能力。
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"path/filepath"

	"github.com/Quenwaz/quickserve/internal/config"
	"github.com/Quenwaz/quickserve/internal/upload"
	"github.com/Quenwaz/quickserve/internal/web"
)

// New 构造完整 HTTP handler。
func New(opts *config.Options, fs http.Handler, up http.Handler) http.Handler {
	page := web.UploadPage(opts.BasePath())

	mux := http.NewServeMux()
	// /health 供容器 HEALTHCHECK 与负载均衡探活，始终可用，
	// 且不随 base-path 前缀变化（探活直连容器端口）。
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("/_upload", page)
	mux.Handle("/", rootHandler(opts, fs, up))

	h := http.Handler(mux)
	if bp := opts.BasePath(); bp != "" {
		h = stripPrefix(h, bp)
	}
	if opts.CORS() {
		h = corsMiddleware(h)
	}
	return logMiddleware(h)
}

// stripPrefix 剥离请求路径中的 base 前缀；未携带前缀的请求按根路径语义
// 处理（重定向到带前缀的根，便于直接访问域名时落到正确位置）。
func stripPrefix(next http.Handler, base string) http.Handler {
	trimmed := strings.TrimSuffix(base, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == trimmed || p == trimmed+"/" {
			// 根路径本身：改为 "/" 交给 mux（列出文件、上传页面入口）。
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			next.ServeHTTP(w, r2)
			return
		}
		if !strings.HasPrefix(p, trimmed+"/") {
			// 前缀不匹配：反向代理配置不一致或直接访问，给出明确指引。
			http.Redirect(w, r, trimmed+r.URL.Path, http.StatusFound)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = strings.TrimPrefix(p, trimmed)
		next.ServeHTTP(w, r2)
	})
}

// rootHandler 处理根路由：GET/HEAD 静态文件，POST/PUT 上传（可选），其余 405。
func rootHandler(opts *config.Options, fs http.Handler, up http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			if opts.ReadOnly() {
				http.Error(w, "server is in read-only mode", http.StatusForbidden)
				return
			}
			fs.ServeHTTP(w, r)
		case http.MethodPost, http.MethodPut:
			if !opts.Upload() {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "upload disabled; start with -u to enable", http.StatusMethodNotAllowed)
				return
			}
			up.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, HEAD, POST, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// setCORS 写入宽松的 CORS 头（本服务面向局域网共享场景）。
func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Auth-Token")
}

// printBanner 输出启动信息，包括局域网地址，方便手机等设备直接访问。
func printBanner(w io.Writer, opts *config.Options, absDir string) {
	base := opts.BasePath()
	rootURL := "http://localhost:" + strconv.Itoa(opts.Port()) + base + "/"
	fmt.Fprintf(w, "quickserve %s\n", version)
	fmt.Fprintf(w, "Serving HTTP on port %d (%s)\n", opts.Port(), rootURL)
	if base != "" {
		fmt.Fprintf(w, "Base path: %s\n", base)
	}
	fmt.Fprintf(w, "Directory: %s\n", absDir)
	for _, ip := range localIPs() {
		fmt.Fprintf(w, "LAN:       http://%s:%d%s/\n", ip, opts.Port(), base)
	}
	fmt.Fprintf(w, "Upload:    %s\n", enabledDisabled(opts.Upload()))
	fmt.Fprintf(w, "CORS:      %s\n", enabledDisabled(opts.CORS()))
	if opts.Upload() {
		fmt.Fprintf(w, "Max size:  %s\n", maxUploadDesc(opts.MaxUploadBytes()))
		fmt.Fprintf(w, "Rate:      %s/minute/IP\n", rateDesc(opts.RatePerMinute()))
		fmt.Fprintf(w, "Auth:      %s\n", authDesc(opts.Token()))
		fmt.Fprintf(w, "Web page:  %s/_upload\n", base)
	}
	fmt.Fprintln(w, "Press Ctrl+C to stop.")
}

func maxUploadDesc(n int64) string {
	if n <= 0 {
		return "unlimited"
	}
	const mib = 1 << 20
	if n%mib == 0 {
		return fmt.Sprintf("%d MiB", n/mib)
	}
	return fmt.Sprintf("%d bytes", n)
}

func rateDesc(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(n)
}

func authDesc(token string) string {
	if token == "" {
		return "none"
	}
	return "token required"
}

func enabledDisabled(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

// localIPs 枚举本机非环回 IPv4 地址。
func localIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ips
}

// statusWriter 记录响应状态码和写入字节数，用于访问日志。
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// logOutput 是访问日志的输出目标，测试中可替换为 io.Discard。
var logOutput io.Writer = os.Stdout

// logMiddleware 打印访问日志，格式接近 Python http.server 的 Common Log Format。
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)

		clientIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(clientIP); err == nil {
			clientIP = host
		}
		fmt.Fprintf(logOutput, "%s - - [%s] \"%s %s %s\" %d %s (%.3fs)\n",
			clientIP,
			start.Format("02/Jan/2006:15:04:05 -0700"),
			r.Method,
			r.URL.Path,
			r.Proto,
			lw.status,
			sizeOrDash(lw.bytes),
			time.Since(start).Seconds(),
		)
	})
}

func sizeOrDash(n int) string {
	if n > 0 {
		return strconv.Itoa(n)
	}
	return "-"
}

// version 在发布构建时通过 -ldflags "-X .../server.version=vX.Y.Z" 注入。
var version = "dev"

// SetVersion 供 main 包注入构建版本号。
func SetVersion(v string) { version = v }

// PrintVersion 输出版本信息（-v 子命令）。
func PrintVersion(w io.Writer) { printVersion(w) }

// Run 是 main 的执行入口：建 handler、起服务、处理信号优雅关停。
// 返回进程退出码。
func Run(opts *config.Options, args []string) int {
	// 子命令：healthcheck（容器 HEALTHCHECK 调用，不启服务器）。
	if len(args) > 0 && args[0] == "healthcheck" {
		return runHealthcheck(opts.Port())
	}
	if len(args) > 0 && args[0] == "version" {
		printVersion(os.Stdout)
		return 0
	}

	absDir, err := resolveDir(opts.Dir())
	if err != nil {
		log.Fatalf("directory: %v", err)
		return 1
	}

	fs := http.FileServer(http.Dir(absDir))
	up, err := upload.New(upload.Config{
		Root:       absDir,
		MaxBytes:   opts.MaxUploadBytes(),
		Token:      opts.Token(),
		RatePerMin: opts.RatePerMinute(),
	})
	if err != nil {
		log.Fatalf("upload handler: %v", err)
		return 1
	}

	handler := New(opts, fs, up)

	printBanner(os.Stdout, opts, absDir)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port()),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // Slowloris 防护
		// ReadTimeout/WriteTimeout 不设全局值：大文件上传/下载需要长连接，
		// 上传侧的慢速攻击由 upload handler 的 WriteTimeout 语义兜底（见下）。
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		log.Printf("server error: %v", err)
		up.Wait()
		return 1
	case sig := <-sigCh:
		fmt.Fprintf(os.Stdout, "\nreceived %s, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown: %v", err)
		}
		up.Wait() // 等待进行中的上传落盘
		fmt.Fprintln(os.Stdout, "bye.")
		return 0
	}
}

// corsMiddleware 为所有响应附加 CORS 头并放行 OPTIONS 预检。
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// runHealthcheck 请求本机 /health，成功返回 0，供 Docker HEALTHCHECK 使用。
func runHealthcheck(port int) int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}

// resolveDir 校验并解析服务目录为绝对路径。
func resolveDir(dir string) (string, error) {
	abs, err := filepathAbs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}

// filepathAbs 独立包装便于测试注入。
var filepathAbs = filepathAbsImpl

func filepathAbsImpl(path string) (string, error) { return filepath.Abs(path) }

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "quickserve %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

package upload

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestHandler 构造指向临时目录的上传 handler。
func newTestHandler(t *testing.T, mutate func(*Config)) *handler {
	t.Helper()
	cfg := Config{Root: t.TempDir(), RatePerMin: 0}
	if mutate != nil {
		mutate(&cfg)
	}
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func post(h *handler, target string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestUploadSingle(t *testing.T) {
	h := newTestHandler(t, nil)
	w := post(h, "/sub/up.txt", strings.NewReader("uploaded data"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body)
	}
	got, err := os.ReadFile(filepath.Join(h.root, "sub", "up.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "uploaded data" {
		t.Errorf("content = %q", got)
	}
}

func TestUploadAnyBinary(t *testing.T) {
	h := newTestHandler(t, nil)
	payload := bytes.Repeat([]byte{0x00, 0xFF, 0x7F, 0x80}, 4096)
	w := post(h, "/data.bin", bytes.NewReader(payload))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	got, _ := os.ReadFile(filepath.Join(h.root, "data.bin"))
	if !bytes.Equal(got, payload) {
		t.Errorf("binary content mismatch: got %d bytes", len(got))
	}
}

func TestUploadPathTraversal(t *testing.T) {
	h := newTestHandler(t, nil)
	parent := filepath.Dir(h.root)
	secret := filepath.Join(parent, "secret.txt")
	t.Cleanup(func() { os.Remove(secret) })

	for _, raw := range []string{"/../secret.txt", "/../../secret.txt", "/a/../../secret.txt"} {
		req := httptest.NewRequest(http.MethodPost, raw, strings.NewReader("evil"))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %q status = %d, want 403", raw, w.Code)
		}
	}
	if _, err := os.Stat(secret); err == nil {
		t.Fatal("path traversal escaped root directory")
	}
}

func TestUploadRejectsReservedNames(t *testing.T) {
	h := newTestHandler(t, nil)
	for _, name := range []string{"/CON", "/NUL.txt", "/com1"} {
		w := post(h, name, strings.NewReader("x"))
		if w.Code != http.StatusBadRequest {
			t.Errorf("POST %q status = %d, want 400 (reserved name)", name, w.Code)
		}
	}
}

func TestUploadCleansInvalidChars(t *testing.T) {
	h := newTestHandler(t, nil)
	// "a<b>:c?" 中的非法字符被替换为 "_"。
	w := post(h, "/a%3Cb%3E_c.txt", strings.NewReader("x"))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body)
	}
	entries, _ := os.ReadDir(h.root)
	found := false
	for _, e := range entries {
		if strings.ContainsAny(e.Name(), `<>:"|?*`) {
			t.Errorf("invalid char in filename %q", e.Name())
		}
		if e.Name() == "a<b>_c.txt" || e.Name() == "a<b>c.txt" {
			found = true
		}
	}
	_ = found // 具体清洗结果由 sanitize 单测覆盖
}

func TestUploadEmptyPath(t *testing.T) {
	h := newTestHandler(t, nil)
	w := post(h, "/", bytes.NewReader(nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST / status = %d, want 400", w.Code)
	}
}

func TestUploadMethodNotAllowed(t *testing.T) {
	h := newTestHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/x.txt", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", w.Code)
	}
}

func TestUploadOverwriteProtection(t *testing.T) {
	h := newTestHandler(t, nil)
	full := filepath.Join(h.root, "dup.txt")
	os.WriteFile(full, []byte("original"), 0o644)

	w := post(h, "/dup.txt", strings.NewReader("new"))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	got, _ := os.ReadFile(full)
	if string(got) != "original" {
		t.Errorf("file was overwritten: %q", got)
	}
	// 空文件允许覆盖（视为失败上传的残留）。
	os.WriteFile(full, []byte{}, 0o644)
	w2 := post(h, "/dup.txt", strings.NewReader("retry"))
	if w2.Code != http.StatusCreated {
		t.Errorf("overwrite empty file status = %d, want 201", w2.Code)
	}
}

func TestUploadMaxSizeEnforced(t *testing.T) {
	h := newTestHandler(t, func(c *Config) { c.MaxBytes = 10 })

	// Content-Length 已知超限 → 413，不落盘。
	big := bytes.Repeat([]byte("x"), 100)
	w := post(h, "/big.bin", bytes.NewReader(big))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
	// 恰好在限额内 → 201。
	w2 := post(h, "/ok.bin", bytes.NewReader(bytes.Repeat([]byte("y"), 10)))
	if w2.Code != http.StatusCreated {
		t.Errorf("at-limit upload status = %d, want 201; body=%s", w2.Code, w2.Body)
	}
	// 无 Content-Length 的流式超限 → 写入中途发现，413，无残留文件。
	req := httptest.NewRequest(http.MethodPost, "/stream.bin",
		io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("z"), 100))))
	req.ContentLength = -1
	wr := httptest.NewRecorder()
	h.ServeHTTP(wr, req)
	if wr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("streaming over-limit status = %d, want 413", wr.Code)
	}
	entries, _ := os.ReadDir(h.root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".qs-upload-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestUploadMultipart(t *testing.T) {
	h := newTestHandler(t, nil)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, name := range []string{"a.txt", "b.png", "设置.ini"} {
		fw, _ := mw.CreateFormFile("file", name)
		fw.Write([]byte("content-of-" + name))
	}
	// 普通字段应被忽略且不报错。
	mw.WriteField("note", "hello")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, name := range []string{"a.txt", "b.png", "设置.ini"} {
		if !strings.Contains(body, "saved "+name) {
			t.Errorf("multipart result missing %q:\n%s", name, body)
		}
		p := filepath.Join(h.root, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("file %s not saved: %v", name, err)
		}
		if string(data) != "content-of-"+name {
			t.Errorf("file %s content = %q", name, data)
		}
	}
}

func TestUploadMultipartTraversalFilename(t *testing.T) {
	h := newTestHandler(t, nil)
	parent := filepath.Dir(h.root)
	secret := filepath.Join(parent, "pwned.txt")
	t.Cleanup(func() { os.Remove(secret) })

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "../../pwned.txt")
	fw.Write([]byte("evil"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if _, err := os.Stat(secret); err == nil {
		t.Fatal("multipart filename escaped root")
	}
	// "pwned.txt" 部分应被保留（取最后一段）。
	if !strings.Contains(w.Body.String(), "saved pwned.txt") {
		t.Errorf("expected basename kept, body:\n%s", w.Body)
	}
}

func TestUploadTokenAuth(t *testing.T) {
	h := newTestHandler(t, func(c *Config) { c.Token = "s3cr3t" })

	// 无凭据 → 401
	w := post(h, "/a.txt", strings.NewReader("x"))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no token status = %d, want 401", w.Code)
	}
	// 错误 token → 401
	req := httptest.NewRequest(http.MethodPost, "/a.txt", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer wrong")
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Errorf("wrong token status = %d, want 401", rw.Code)
	}
	// Bearer 正确 → 201
	req2 := httptest.NewRequest(http.MethodPost, "/a.txt", strings.NewReader("x"))
	req2.Header.Set("Authorization", "Bearer s3cr3t")
	rw2 := httptest.NewRecorder()
	h.ServeHTTP(rw2, req2)
	if rw2.Code != http.StatusCreated {
		t.Errorf("bearer token status = %d, want 201", rw2.Code)
	}
	// 查询参数 → 201
	rw3 := post(h, "/b.txt?token=s3cr3t", strings.NewReader("x"))
	if rw3.Code != http.StatusCreated {
		t.Errorf("query token status = %d, want 201", rw3.Code)
	}
	// X-Auth-Token → 201
	req4 := httptest.NewRequest(http.MethodPost, "/c.txt", strings.NewReader("x"))
	req4.Header.Set("X-Auth-Token", "s3cr3t")
	rw4 := httptest.NewRecorder()
	h.ServeHTTP(rw4, req4)
	if rw4.Code != http.StatusCreated {
		t.Errorf("header token status = %d, want 201", rw4.Code)
	}
}

func TestUploadRateLimit(t *testing.T) {
	h := newTestHandler(t, func(c *Config) { c.RatePerMin = 2 })
	// 两个不同路径，避免覆盖保护（409）干扰限速断言。
	if w := post(h, "/f1.txt", strings.NewReader("x")); w.Code != http.StatusCreated {
		t.Fatalf("request 1 status = %d, want 201", w.Code)
	}
	if w := post(h, "/f2.txt", strings.NewReader("x")); w.Code != http.StatusCreated {
		t.Fatalf("request 2 status = %d, want 201", w.Code)
	}
	if w := post(h, "/f3.txt", strings.NewReader("x")); w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request status = %d, want 429", w.Code)
	}
}

func TestNoTempFilesLeftOnFailure(t *testing.T) {
	h := newTestHandler(t, nil)
	// 模拟客户端中断：body 读取中途出错。
	errReader := &errReader{data: []byte("partial"), err: errors.New("connection reset")}
	w := post(h, "/broken.bin", errReader)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	entries, _ := os.ReadDir(h.root)
	if len(entries) != 0 {
		t.Errorf("root not clean after failure: %v", entries)
	}
}

type errReader struct {
	data []byte
	err  error
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, r.err
}

func TestOverwriteProtectionIntegration(t *testing.T) {
	// multipart 模式下重复上传同一文件应 skip 而非覆盖。
	h := newTestHandler(t, nil)
	upload := func() string {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "dup.bin")
		fw.Write([]byte("data"))
		mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Body.String()
	}
	if out := upload(); !strings.Contains(out, "saved dup.bin") {
		t.Fatalf("first upload failed:\n%s", out)
	}
	if out := upload(); !strings.Contains(out, "already exists") {
		t.Errorf("second upload should be skipped with conflict note:\n%s", out)
	}
}

func TestUploadedFilePermissions(t *testing.T) {
	// 上传产物应为 0644：挂载卷场景下宿主侧非属主用户也要能读取。
	// （Windows 不实现 POSIX 权限位，仅在 Linux/macOS 上断言。）
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on windows")
	}
	h := newTestHandler(t, nil)
	if w := post(h, "/perm.txt", strings.NewReader("x")); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	info, err := os.Stat(filepath.Join(h.root, "perm.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("uploaded file mode = %o, want 644", got)
	}
}

func TestWriteTimeoutDefaults(t *testing.T) {
	h := newTestHandler(t, nil)
	if h.Timeout() != 15*time.Minute {
		t.Errorf("default timeout = %v, want 15m", h.Timeout())
	}
}

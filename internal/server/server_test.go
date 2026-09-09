package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quenwaz/quickserve/internal/config"
	"github.com/Quenwaz/quickserve/internal/upload"
)

// newTestServer 创建一个服务 dir 目录的测试服务器。
// 访问日志输出被丢弃，避免污染测试输出。
func newTestServer(t *testing.T, dir string, opts *config.Options) *httptest.Server {
	t.Helper()
	logOutput = io.Discard
	t.Cleanup(func() { logOutput = os.Stdout })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	up, err := upload.New(upload.Config{
		Root:       abs,
		MaxBytes:   opts.MaxUploadBytes(),
		Token:      opts.Token(),
		RatePerMin: opts.RatePerMinute(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(opts, http.FileServer(http.Dir(abs)), up))
	t.Cleanup(ts.Close)
	return ts
}

func testOpts(mutate ...func(*config.Options)) *config.Options {
	o := config.NewOptions()
	for _, m := range mutate {
		m(&o)
	}
	return &o
}

func TestDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644)

	ts := newTestServer(t, dir, testOpts())
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello.txt") {
		t.Errorf("directory listing missing hello.txt:\n%s", body)
	}
}

func TestServeFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o644)

	ts := newTestServer(t, dir, testOpts())
	resp, err := http.Get(ts.URL + "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello world" {
		t.Errorf("body = %q, want %q", body, "hello world")
	}
}

func TestNotFound(t *testing.T) {
	ts := newTestServer(t, t.TempDir(), testOpts())
	resp, err := http.Get(ts.URL + "/no-such-file")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestUploadDisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "up.txt"), []byte("old"), 0o644)

	ts := newTestServer(t, dir, testOpts())
	resp, err := http.Post(ts.URL+"/up.txt", "text/plain", strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 when upload disabled", resp.StatusCode)
	}
	got, err := os.ReadFile(filepath.Join(dir, "up.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Errorf("file was modified with upload disabled: %q", got)
	}
}

func TestUploadEnabledEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServer(t, dir, testOpts(func(o *config.Options) { o.SetUpload() }))

	resp, err := http.Post(ts.URL+"/sub/up.txt", "text/plain", strings.NewReader("uploaded data"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, mustRead(resp))
	}
	got, err := os.ReadFile(filepath.Join(dir, "sub", "up.txt"))
	if err != nil {
		t.Fatalf("uploaded file not created: %v", err)
	}
	if string(got) != "uploaded data" {
		t.Errorf("uploaded content = %q, want %q", got, "uploaded data")
	}

	// 上传后应能立即下载
	resp2, err := http.Get(ts.URL + "/sub/up.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(resp2.Body)
	if string(body) != "uploaded data" {
		t.Errorf("download after upload = %q, want %q", body, "uploaded data")
	}
}

func TestBrowserMultipartEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServer(t, dir, testOpts(func(o *config.Options) { o.SetUpload() }))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "photo hello.jpg")
	fw.Write([]byte("jpeg-bytes-here"))
	mw.Close()

	resp, err := http.Post(ts.URL+"/", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, mustRead(resp))
	}
	if _, err := os.Stat(filepath.Join(dir, "photo hello.jpg")); err != nil {
		t.Errorf("multipart upload not saved: %v", err)
	}
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t, t.TempDir(), testOpts())
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestUploadPageServedAtUnderscoreUpload(t *testing.T) {
	ts := newTestServer(t, t.TempDir(), testOpts())
	resp, err := http.Get(ts.URL + "/_upload")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<!DOCTYPE html>") {
		t.Error("upload page not served")
	}
}

func TestReadOnlyMode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644)
	ts := newTestServer(t, dir, testOpts(func(o *config.Options) { o.SetReadOnly() }))

	resp, err := http.Get(ts.URL + "/f.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET in read-only mode status = %d, want 403", resp.StatusCode)
	}
	// 健康检查仍可用
	resp2, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("health in read-only mode status = %d, want 200", resp2.StatusCode)
	}
}

func TestCORSHeaders(t *testing.T) {
	ts := newTestServer(t, t.TempDir(), testOpts(func(o *config.Options) { o.SetCORS() }))

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}

	req, err := http.NewRequest(http.MethodOptions, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", resp2.StatusCode)
	}
}

func TestNoCORSHeadersByDefault(t *testing.T) {
	ts := newTestServer(t, t.TempDir(), testOpts())
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty when CORS disabled", got)
	}
}

func TestPrintBanner(t *testing.T) {
	var buf bytes.Buffer
	o := config.NewOptions()
	printBanner(&buf, &o, "/tmp")
	out := buf.String()
	for _, want := range []string{"Serving HTTP on port 8000", "Directory: /tmp", "Ctrl+C"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestBannerShowsUploadConfig(t *testing.T) {
	var buf bytes.Buffer
	o := config.NewOptions()
	o.SetUpload()
	o.SetToken("x")
	printBanner(&buf, &o, "/tmp")
	out := buf.String()
	for _, want := range []string{"Max size:", "Rate:", "Auth:", "token required", "/_upload"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestBasePathEndToEnd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("base"), 0o644)
	o := testOpts(func(op *config.Options) {
		op.SetUpload()
		op.SetBasePath("/apps")
	})
	ts := newTestServer(t, dir, o)

	// 带前缀访问：静态文件、上传页面、health 均可达
	resp, err := http.Get(ts.URL + "/apps/f.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "base" {
		t.Errorf("GET /apps/f.txt = %d %q, want 200 %q", resp.StatusCode, body, "base")
	}

	resp2, err := http.Get(ts.URL + "/apps/_upload")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("GET /apps/_upload = %d, want 200", resp2.StatusCode)
	}
	if !strings.Contains(string(page), `uploadURL = "/apps/"`) {
		t.Error("upload page missing injected base path")
	}

	resp3, err := http.Get(ts.URL + "/apps/health")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("GET /apps/health = %d, want 200", resp3.StatusCode)
	}

	// 不带前缀的 multipart 上传走 /apps/（页面注入的 URL）
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "bp.txt")
	fw.Write([]byte("basepath-upload"))
	mw.Close()
	resp4, err := http.Post(ts.URL+"/apps/", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusCreated {
		t.Errorf("POST /apps/ = %d, want 201; body=%s", resp4.StatusCode, mustRead(resp4))
	}
	if _, err := os.Stat(filepath.Join(dir, "bp.txt")); err != nil {
		t.Errorf("upload under base path not saved: %v", err)
	}

	// 根路径重定向到带前缀的根（反向代理不一致时的指引）
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp5, err := client.Get(ts.URL + "/f.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusFound {
		t.Errorf("GET /f.txt without prefix = %d, want 302", resp5.StatusCode)
	}
	if loc := resp5.Header.Get("Location"); loc != "/apps/f.txt" {
		t.Errorf("Location = %q, want /apps/f.txt", loc)
	}
}

func TestHealthAlwaysOnRoot(t *testing.T) {
	// /health 不随 base-path 变化：容器探活直连端口，必须在根路径可用。
	dir := t.TempDir()
	o := testOpts(func(op *config.Options) { op.SetBasePath("/apps") })
	ts := newTestServer(t, dir, o)
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health with base-path set = %d, want 200", resp.StatusCode)
	}
}

func mustRead(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

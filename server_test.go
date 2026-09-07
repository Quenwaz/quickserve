package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer 创建一个服务 dir 目录的测试服务器与客户端。
// 访问日志输出被丢弃，避免污染测试输出。
func newTestServer(t *testing.T, dir string, opts options) (*httptest.Server, string) {
	t.Helper()
	logOutput = io.Discard
	t.Cleanup(func() { logOutput = os.Stdout })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(newHandler(opts, abs))
	t.Cleanup(ts.Close)
	return ts, abs
}

func TestDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644)

	ts, _ := newTestServer(t, dir, options{})
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

	ts, _ := newTestServer(t, dir, options{})
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
	ts, _ := newTestServer(t, t.TempDir(), options{})
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
	// 先放置一个已存在的文件：FileServer 对存在文件的 POST 返回 405，
	// 对不存在路径的 POST 返回 404，两种都说明上传未生效。
	os.WriteFile(filepath.Join(dir, "up.txt"), []byte("old"), 0o644)

	ts, _ := newTestServer(t, dir, options{})
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

func TestUploadEnabled(t *testing.T) {
	dir := t.TempDir()
	ts, _ := newTestServer(t, dir, options{upload: true})
	resp, err := http.Post(ts.URL+"/sub/up.txt", "text/plain", strings.NewReader("uploaded data"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
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

func TestUploadPathTraversal(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "secret.txt")
	t.Cleanup(func() { os.Remove(secret) })

	h := handleUpload(dir)
	// 直接以解码后的原始路径调用 handler，绕过 HTTP 客户端的路径规范化，
	// 构造服务端可能收到的恶意路径。
	for _, raw := range []string{"/../secret.txt", "/../../secret.txt", "/a/../../secret.txt"} {
		req := httptest.NewRequest(http.MethodPost, raw, strings.NewReader("evil"))
		w := httptest.NewRecorder()
		h(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("POST %q status = %d, want 403", raw, w.Code)
		}
	}
	if _, err := os.Stat(secret); err == nil {
		t.Fatal("path traversal escaped root directory")
	}
}

func TestCORSHeaders(t *testing.T) {
	ts, _ := newTestServer(t, t.TempDir(), options{cors: true})

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
	ts, _ := newTestServer(t, t.TempDir(), options{})
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
	printBanner(&buf, options{port: 8000}, "/tmp")
	out := buf.String()
	for _, want := range []string{"Serving HTTP on port 8000", "Directory: /tmp", "Ctrl+C"} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q:\n%s", want, out)
		}
	}
}

func TestUploadMethodNotAllowed(t *testing.T) {
	h := handleUpload(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/x.txt", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET upload handler status = %d, want 405", w.Code)
	}
}

func TestUploadEmptyPath(t *testing.T) {
	h := handleUpload(t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST / status = %d, want 400", w.Code)
	}
}

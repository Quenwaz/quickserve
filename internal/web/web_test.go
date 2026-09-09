package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadPageServed(t *testing.T) {
	h := UploadPage("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	for _, want := range []string{"<!DOCTYPE html>", "FormData", "drop"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// 根路径部署时注入空串：上传 URL 应以 "/" 开头且无占位符残留。
	if !strings.Contains(body, `uploadURL = "/"`) {
		t.Errorf("root deployment uploadURL not injected:\n%s", body)
	}
	if strings.Contains(body, basePlaceholder) {
		t.Error("placeholder left in rendered page")
	}
}

func TestUploadPageWithBasePath(t *testing.T) {
	h := UploadPage("/apps")
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/", nil))
	body := w.Body.String()
	if !strings.Contains(body, `uploadURL = "/apps/"`) {
		t.Errorf("base path not injected into uploadURL:\n%s", strings.Split(body, "\n")[64])
	}
	if strings.Contains(body, basePlaceholder) {
		t.Error("placeholder left in rendered page")
	}
}

func TestUploadPageBasePathNormalization(t *testing.T) {
	// 无前导斜杠、尾斜杠都应被规范化。
	for _, in := range []string{"apps", "/apps/", "apps/"} {
		w := httptest.NewRecorder()
		UploadPage(in)(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if !strings.Contains(w.Body.String(), `uploadURL = "/apps/"`) {
			t.Errorf("UploadPage(%q) not normalized to /apps/", in)
		}
	}
}

func TestUploadPageRejectsPost(t *testing.T) {
	h := UploadPage("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// Package web 提供内置的浏览器上传页面（go:embed 编译进二进制，无外部依赖）。
package web

import (
	"embed"
	"net/http"
)

//go:embed static/index.html
var static embed.FS

// UploadPageHandler 返回浏览器上传页面（仅 GET/HEAD）。
func UploadPageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := static.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "page not available", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// 页面不含敏感信息，允许缓存。
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(data)
	}
}

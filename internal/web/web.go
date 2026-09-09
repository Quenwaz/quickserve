// Package web 提供内置的浏览器上传页面（go:embed 编译进二进制，无外部依赖）。
package web

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed static/index.html
var static embed.FS

// basePlaceholder 是页面中待替换的基础路径占位符。
const basePlaceholder = "__QS_BASE_PATH__"

// UploadPage 返回注入了 base path 的上传页面 handler（仅 GET/HEAD）。
func UploadPage(basePath string) http.HandlerFunc {
	data, err := static.ReadFile("static/index.html")
	if err != nil {
		// 编译期内嵌文件必然存在；此分支仅防御未来重构。
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "page not available", http.StatusInternalServerError)
		}
	}
	page := string(data)
	// 规范化注入值：以 "/" 开头、不以 "/" 结尾、空串表示根路径。
	bp := strings.TrimSuffix(strings.TrimPrefix(basePath, "/"), "/")
	if bp != "" {
		bp = "/" + bp
	}
	page = strings.ReplaceAll(page, basePlaceholder, bp)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// 页面不含敏感信息，允许缓存。
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write([]byte(page))
	}
}

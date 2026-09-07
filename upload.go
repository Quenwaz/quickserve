package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// handleUpload 返回一个处理 POST/PUT 上传的 handler，将请求体保存为
// URL 路径对应的文件（相对 root）。目录自动创建，路径穿越会被拒绝。
func handleUpload(root string) http.HandlerFunc {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			w.Header().Set("Allow", "GET, HEAD, POST, PUT, OPTIONS")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// 显式拒绝包含 ".." 路径段的上传，避免路径被静默规范化产生歧义
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if seg == ".." {
				http.Error(w, "path may not contain ..", http.StatusForbidden)
				return
			}
		}
		// 以 "/" 锚定后 Clean，纵深防御确保 ".." 无法逃出根目录
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "." {
			http.Error(w, "target file path required, e.g. POST /files/name.txt", http.StatusBadRequest)
			return
		}
		full := filepath.Join(absRoot, filepath.FromSlash(rel))
		if full != absRoot && !strings.HasPrefix(full, absRoot+string(os.PathSeparator)) {
			http.Error(w, "path escapes root directory", http.StatusForbidden)
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := os.Create(full)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		n, copyErr := io.Copy(f, r.Body)
		closeErr := f.Close()
		if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusInternalServerError)
			return
		}
		if closeErr != nil {
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "uploaded %s (%d bytes)\n", rel, n)
	}
}

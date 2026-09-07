package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"
)

// newHandler 组装完整的 HTTP 处理链：上传（可选）→ CORS（可选）→ 文件服务，外层包访问日志。
func newHandler(opts options, absDir string) http.Handler {
	fs := http.FileServer(http.Dir(absDir))
	upload := handleUpload(absDir)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			// 静态文件服务
		case http.MethodPost, http.MethodPut:
			if !opts.upload {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "upload disabled; start with -u to enable", http.StatusMethodNotAllowed)
				return
			}
			upload(w, r)
			return
		default:
			w.Header().Set("Allow", "GET, HEAD, POST, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if opts.cors {
			setCORS(w)
		}
		fs.ServeHTTP(w, r)
	})

	if opts.cors {
		// OPTIONS 预检请求在 CORS 开启时直接放行
		base := h
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				setCORS(w)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			base.ServeHTTP(w, r)
		})
	}
	return logMiddleware(h)
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// printBanner 输出启动信息，包括局域网地址，方便手机等设备直接访问。
func printBanner(w io.Writer, opts options, absDir string) {
	fmt.Fprintf(w, "quickserve %s\n", version)
	fmt.Fprintf(w, "Serving HTTP on port %d (http://localhost:%d/)\n", opts.port, opts.port)
	fmt.Fprintf(w, "Directory: %s\n", absDir)
	for _, ip := range localIPs() {
		fmt.Fprintf(w, "LAN:       http://%s:%d/\n", ip, opts.port)
	}
	fmt.Fprintf(w, "Upload:    %s\n", enabledDisabled(opts.upload))
	fmt.Fprintf(w, "CORS:      %s\n", enabledDisabled(opts.cors))
	fmt.Fprintln(w, "Press Ctrl+C to stop.")
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "quickserve %s (%s %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
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

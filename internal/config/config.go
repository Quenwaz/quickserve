// Package config 定义 quickserve 的命令行配置并负责解析与校验。
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ErrHelp / ErrVersion 表示解析到 -h / -v 后应打印信息并正常退出。
var (
	ErrHelp    = errors.New("help requested")
	ErrVersion = errors.New("version requested")
)

// 默认配置值。MaxUpload 限制单次上传体积；RatePerMinute 限制单 IP 每分钟上传请求数。
const (
	DefaultPort      = 8000
	DefaultMaxUpload = 1 << 30 // 1 GiB
	DefaultRateLimit = 60
)

// Options 是从命令行与环境变量解析出的服务器配置。
type Options struct {
	port     int
	dir      string
	upload   bool
	cors     bool
	token    string
	maxSize  int64
	ratePM   int
	readonly bool
	basePath string
}

// NewOptions 返回带默认值的配置，供测试等场景直接构造。
func NewOptions() Options {
	return Options{
		port:    DefaultPort,
		dir:     ".",
		maxSize: DefaultMaxUpload,
		ratePM:  DefaultRateLimit,
	}
}

// Port / Dir / Upload / CORS / Token / MaxUploadBytes / RatePerMinute / ReadOnly
// 返回配置的只读视图，避免调用方意外修改内部状态。
func (o *Options) Port() int             { return o.port }
func (o *Options) Dir() string           { return o.dir }
func (o *Options) Upload() bool          { return o.upload }
func (o *Options) CORS() bool            { return o.cors }
func (o *Options) Token() string         { return o.token }
func (o *Options) MaxUploadBytes() int64 { return o.maxSize }
func (o *Options) RatePerMinute() int    { return o.ratePM }
func (o *Options) ReadOnly() bool        { return o.readonly }

// BasePath 返回 URL 基础路径（如 "/apps"），空串表示部署在根路径。
func (o *Options) BasePath() string { return o.basePath }

// SetUpload / SetCORS / SetReadOnly / SetToken / SetBasePath
// 供测试与嵌入场景编程式构造配置。SetBasePath 忽略规范化错误（非法值存原样），
// 校验语义以 ParseArgs 为准。
func (o *Options) SetUpload()           { o.upload = true }
func (o *Options) SetCORS()             { o.cors = true }
func (o *Options) SetReadOnly()         { o.readonly = true }
func (o *Options) SetToken(t string)    { o.token = t }
func (o *Options) SetMaxBytes(n int64)  { o.maxSize = n }
func (o *Options) SetRatePerMin(n int)  { o.ratePM = n }
func (o *Options) SetBasePath(p string) { o.basePath, _ = NormalizeBasePath(p) }

// ParseArgs 解析命令行参数与环境变量。兼容 python -m http.server 的位置参数
// 风格：纯数字位置参数作为端口，其余位置参数作为目录。也支持 -port/-dir 等长选项。
// 每个选项都有对应的 QUICKSERVE_<NAME> 环境变量；命令行优先于环境变量。
func ParseArgs(args []string) (Options, error) {
	opts, err := parseEnv()
	if err != nil {
		return opts, err
	}
	return parseFlags(opts, args)
}

// parseFlags 在 env 基础上解析命令行参数。
func parseFlags(env Options, args []string) (Options, error) {
	opts := env
	var wantPort, wantDir, wantToken, wantMax, wantRate, wantBase bool
	var positional []string

	for _, a := range args {
		switch {
		case wantPort:
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 || n > 65535 {
				return opts, fmt.Errorf("invalid port %q", a)
			}
			opts.port = n
			wantPort = false
		case wantDir:
			opts.dir = a
			wantDir = false
		case wantToken:
			opts.token = a
			wantToken = false
		case wantMax:
			n, err := ParseSize(a)
			if err != nil {
				return opts, fmt.Errorf("invalid max-upload %q", a)
			}
			opts.maxSize = n
			wantMax = false
		case wantRate:
			n, err := strconv.Atoi(a)
			if err != nil || n < 0 {
				return opts, fmt.Errorf("invalid rate-limit %q", a)
			}
			opts.ratePM = n
			wantRate = false
		case wantBase:
			bp, err := NormalizeBasePath(a)
			if err != nil {
				return opts, fmt.Errorf("invalid base-path %q: %v", a, err)
			}
			opts.basePath = bp
			wantBase = false
		case a == "-p" || a == "-port" || a == "--port":
			wantPort = true
		case strings.HasPrefix(a, "-p=") || strings.HasPrefix(a, "-port=") || strings.HasPrefix(a, "--port="):
			n, err := strconv.Atoi(cutValue(a))
			if err != nil || n <= 0 || n > 65535 {
				return opts, fmt.Errorf("invalid port %q", cutValue(a))
			}
			opts.port = n
		case a == "-d" || a == "-dir" || a == "--dir" || a == "--directory":
			wantDir = true
		case strings.HasPrefix(a, "-d=") || strings.HasPrefix(a, "-dir=") || strings.HasPrefix(a, "--dir=") || strings.HasPrefix(a, "--directory="):
			opts.dir = cutValue(a)
		case a == "-u" || a == "-upload" || a == "--upload":
			opts.upload = true
		case a == "-c" || a == "-cors" || a == "--cors":
			opts.cors = true
		case a == "-ro" || a == "-read-only" || a == "--read-only":
			opts.readonly = true
		case a == "-t" || a == "-token" || a == "--token":
			wantToken = true
		case strings.HasPrefix(a, "-t=") || strings.HasPrefix(a, "-token=") || strings.HasPrefix(a, "--token="):
			opts.token = cutValue(a)
		case a == "-m" || a == "-max-upload" || a == "--max-upload":
			wantMax = true
		case strings.HasPrefix(a, "-m=") || strings.HasPrefix(a, "-max-upload=") || strings.HasPrefix(a, "--max-upload="):
			n, err := ParseSize(cutValue(a))
			if err != nil {
				return opts, fmt.Errorf("invalid max-upload %q", cutValue(a))
			}
			opts.maxSize = n
		case a == "-r" || a == "-rate-limit" || a == "--rate-limit":
			wantRate = true
		case strings.HasPrefix(a, "-r=") || strings.HasPrefix(a, "-rate-limit=") || strings.HasPrefix(a, "--rate-limit="):
			n, err := strconv.Atoi(cutValue(a))
			if err != nil || n < 0 {
				return opts, fmt.Errorf("invalid rate-limit %q", cutValue(a))
			}
			opts.ratePM = n
		case a == "-b" || a == "-base-path" || a == "--base-path":
			wantBase = true
		case strings.HasPrefix(a, "-b=") || strings.HasPrefix(a, "-base-path=") || strings.HasPrefix(a, "--base-path="):
			bp, err := NormalizeBasePath(cutValue(a))
			if err != nil {
				return opts, fmt.Errorf("invalid base-path %q: %v", cutValue(a), err)
			}
			opts.basePath = bp
		case a == "-v" || a == "-version" || a == "--version":
			return opts, ErrVersion
		case a == "-h" || a == "-help" || a == "--help":
			return opts, ErrHelp
		case strings.HasPrefix(a, "-"):
			return opts, fmt.Errorf("unknown option %q", a)
		default:
			positional = append(positional, a)
		}
	}
	if wantPort {
		return opts, errors.New("missing value for port option")
	}
	if wantDir {
		return opts, errors.New("missing value for dir option")
	}
	if wantToken {
		return opts, errors.New("missing value for token option")
	}
	if wantMax {
		return opts, errors.New("missing value for max-upload option")
	}
	if wantRate {
		return opts, errors.New("missing value for rate-limit option")
	}
	if wantBase {
		return opts, errors.New("missing value for base-path option")
	}
	for _, p := range positional {
		if n, err := strconv.Atoi(p); err == nil {
			opts.port = n
		} else if opts.dir == "." {
			opts.dir = p
		}
	}
	return opts, nil
}

// parseEnv 读取 QUICKSERVE_* 环境变量作为默认配置。
// 仅显式设置的非空变量生效；布尔值接受 1/true/yes/on（不区分大小写）。
func parseEnv() (Options, error) {
	opts := NewOptions()
	if v := os.Getenv("QUICKSERVE_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return opts, fmt.Errorf("invalid QUICKSERVE_PORT %q", v)
		}
		opts.port = n
	}
	if v := os.Getenv("QUICKSERVE_DIR"); v != "" {
		opts.dir = v
	}
	if v := os.Getenv("QUICKSERVE_UPLOAD"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return opts, fmt.Errorf("invalid QUICKSERVE_UPLOAD %q", v)
		}
		opts.upload = b
	}
	if v := os.Getenv("QUICKSERVE_CORS"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return opts, fmt.Errorf("invalid QUICKSERVE_CORS %q", v)
		}
		opts.cors = b
	}
	if v := os.Getenv("QUICKSERVE_TOKEN"); v != "" {
		opts.token = v
	}
	if v := os.Getenv("QUICKSERVE_MAX_UPLOAD"); v != "" {
		n, err := ParseSize(v)
		if err != nil {
			return opts, fmt.Errorf("invalid QUICKSERVE_MAX_UPLOAD %q", v)
		}
		opts.maxSize = n
	}
	if v := os.Getenv("QUICKSERVE_RATE_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return opts, fmt.Errorf("invalid QUICKSERVE_RATE_LIMIT %q", v)
		}
		opts.ratePM = n
	}
	if v := os.Getenv("QUICKSERVE_READ_ONLY"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return opts, fmt.Errorf("invalid QUICKSERVE_READ_ONLY %q", v)
		}
		opts.readonly = b
	}
	if v := os.Getenv("QUICKSERVE_BASE_PATH"); v != "" {
		bp, err := NormalizeBasePath(v)
		if err != nil {
			return opts, fmt.Errorf("invalid QUICKSERVE_BASE_PATH %q: %v", v, err)
		}
		opts.basePath = bp
	}
	return opts, nil
}

// parseBool 解析宽松的布尔环境变量值。
func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y":
		return true, nil
	case "0", "false", "no", "off", "n":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean: %q", v)
}

// NormalizeBasePath 规范化 URL 基础路径：确保以 "/" 开头、不以 "/" 结尾，
// 折叠空段；空串/"/" 均视为根路径（返回空串）。拒绝 "."/".." 段与空白
// 字符，防前缀歧义。
func NormalizeBasePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "", nil
	}
	if strings.ContainsAny(p, " \t\r\n") {
		return "", errors.New("whitespace is not allowed")
	}
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "", "/":
			continue // 折叠空段与前导斜杠
		case ".", "..":
			return "", fmt.Errorf("segment %q is not allowed", seg)
		default:
			parts = append(parts, seg)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return "/" + strings.Join(parts, "/"), nil
}

// ParseSize 解析人类可读的体积字符串，如 "100MB"、"512K"、"1G"，纯数字按字节。
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty size")
	}
	// 单位表按后缀长度降序匹配，避免 "K" 抢先命中 "KB" 的前缀。
	units := []struct {
		suffix string
		mul    int64
	}{
		{"GIB", 1 << 30}, {"MIB", 1 << 20}, {"KIB", 1 << 10},
		{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1},
	}
	upper := strings.ToUpper(s)
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			num := strings.TrimSpace(upper[:len(upper)-len(u.suffix)])
			n, err := strconv.ParseFloat(num, 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("bad size %q", s)
			}
			return int64(n * float64(u.mul)), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return n, nil
}

// cutValue 返回 --key=value 形式中 = 之后的部分。
func cutValue(s string) string {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[i+1:]
	}
	return s
}

// PrintUsage 输出帮助信息。
func PrintUsage(w io.Writer) {
	fmt.Fprint(w, `quickserve - a zero-dependency static file server with upload support

Usage:
  quickserve [port] [directory]
  quickserve [options]

Options:
  -p, -port <N>          listen port (default 8000)
  -d, -dir <path>        directory to serve (default ".")
  -u, -upload            enable file upload via POST/PUT (default off)
  -c, -cors              enable CORS headers (default off)
  -t, -token <secret>    require token for upload (Authorization: Bearer <secret> or ?token=<secret>)
  -m, -max-upload <size> max upload size per request (default 1GiB; 0 = unlimited; e.g. 100MB)
  -r, -rate-limit <N>    per-IP uploads per minute (default 60; 0 = unlimited)
  -b, -base-path <path>  URL base path when behind a reverse proxy sub-path (e.g. /apps)
  -ro, -read-only        disable listing/downloads too; only health endpoint stays up
  -v, -version           print version and exit
  -h, -help              show this help

Environment variables (command-line flags take precedence):
  QUICKSERVE_PORT, QUICKSERVE_DIR, QUICKSERVE_UPLOAD, QUICKSERVE_CORS,
  QUICKSERVE_TOKEN, QUICKSERVE_MAX_UPLOAD, QUICKSERVE_RATE_LIMIT,
  QUICKSERVE_BASE_PATH, QUICKSERVE_READ_ONLY

Examples:
  quickserve                      serve current dir on :8000
  quickserve 9999                 serve current dir on :9999
  quickserve 9999 ./public        serve ./public on :9999
  quickserve -u -p 9000           serve with upload enabled on :9000
  quickserve -u -t s3cr3t -m 500MB  upload with token auth and 500MB cap
  quickserve -u -b /apps          serve under http://host:8000/apps/ (reverse proxy)
  curl -T big.zip localhost:9000/big.zip    upload a file
`)
}

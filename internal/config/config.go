// Package config 定义 quickserve 的命令行配置并负责解析与校验。
package config

import (
	"errors"
	"fmt"
	"io"
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

// Options 是从命令行解析出的服务器配置。
type Options struct {
	port     int
	dir      string
	upload   bool
	cors     bool
	token    string
	maxSize  int64
	ratePM   int
	readonly bool
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

// SetUpload / SetCORS / SetReadOnly / SetToken 供测试与嵌入场景编程式构造配置。
func (o *Options) SetUpload()          { o.upload = true }
func (o *Options) SetCORS()            { o.cors = true }
func (o *Options) SetReadOnly()        { o.readonly = true }
func (o *Options) SetToken(t string)   { o.token = t }
func (o *Options) SetMaxBytes(n int64) { o.maxSize = n }
func (o *Options) SetRatePerMin(n int) { o.ratePM = n }

// ParseArgs 解析命令行参数。兼容 python -m http.server 的位置参数风格：
// 纯数字位置参数作为端口，其余位置参数作为目录。也支持 -port/-dir 等长选项。
func ParseArgs(args []string) (Options, error) {
	opts := NewOptions()
	var wantPort, wantDir, wantToken, wantMax, wantRate bool
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
	for _, p := range positional {
		if n, err := strconv.Atoi(p); err == nil {
			opts.port = n
		} else if opts.dir == "." {
			opts.dir = p
		}
	}
	return opts, nil
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
  -ro, -read-only        disable listing/downloads too; only health endpoint stays up
  -v, -version           print version and exit
  -h, -help              show this help

Examples:
  quickserve                      serve current dir on :8000
  quickserve 9999                 serve current dir on :9999
  quickserve 9999 ./public        serve ./public on :9999
  quickserve -u -p 9000           serve with upload enabled on :9000
  quickserve -u -t s3cr3t -m 500MB  upload with token auth and 500MB cap
  curl -T big.zip localhost:9000/big.zip    upload a file
`)
}

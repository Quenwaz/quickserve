package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// errHelp / errVersion 表示解析到 -h / -v 后应打印信息并正常退出。
var (
	errHelp    = errors.New("help requested")
	errVersion = errors.New("version requested")
)

// options 是从命令行解析出的服务器配置。
type options struct {
	port   int
	dir    string
	upload bool
	cors   bool
}

// parseArgs 解析命令行参数。兼容 python -m http.server 的位置参数风格：
// 纯数字位置参数作为端口，其余位置参数作为目录。也支持 -port/-dir 等长选项。
func parseArgs(args []string) (options, error) {
	opts := options{port: 8000, dir: "."}
	var wantPort, wantDir bool
	var positional []string

	for _, a := range args {
		switch {
		case wantPort:
			n, err := strconv.Atoi(a)
			if err != nil {
				return opts, fmt.Errorf("invalid port %q", a)
			}
			opts.port = n
			wantPort = false
		case wantDir:
			opts.dir = a
			wantDir = false
		case a == "-p" || a == "-port" || a == "--port":
			wantPort = true
		case strings.HasPrefix(a, "-p=") || strings.HasPrefix(a, "-port=") || strings.HasPrefix(a, "--port="):
			n, err := strconv.Atoi(cutValue(a))
			if err != nil {
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
		case a == "-v" || a == "-version" || a == "--version":
			return opts, errVersion
		case a == "-h" || a == "-help" || a == "--help":
			return opts, errHelp
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
	for _, p := range positional {
		if n, err := strconv.Atoi(p); err == nil {
			opts.port = n
		} else if opts.dir == "." {
			opts.dir = p
		}
	}
	return opts, nil
}

// cutValue 返回 --key=value 形式中 = 之后的部分。
func cutValue(s string) string {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[i+1:]
	}
	return s
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `quickserve - a zero-dependency static file server with upload support

Usage:
  quickserve [port] [directory]
  quickserve [options]

Options:
  -p, -port <N>      listen port (default 8000)
  -d, -dir <path>    directory to serve (default ".")
  -u, -upload        enable file upload via POST/PUT (default off)
  -c, -cors          enable CORS headers (default off)
  -v, -version       print version and exit
  -h, -help          show this help

Examples:
  quickserve                      serve current dir on :8000
  quickserve 9999                 serve current dir on :9999
  quickserve 9999 ./public        serve ./public on :9999
  quickserve -u -p 9000           serve with upload enabled on :9000
  curl -T big.zip localhost:9000/big.zip    upload a file
`)
}

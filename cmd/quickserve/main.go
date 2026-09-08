// Command quickserve 是一个零依赖的静态文件服务器，功能类似
// python -m http.server，额外支持文件上传、访问保护、CORS 与多平台发布。
//
// 本文件只负责参数解析与进程退出码，业务逻辑在 internal 包中。
package main

import (
	"fmt"
	"os"

	"github.com/Quenwaz/quickserve/internal/config"
	"github.com/Quenwaz/quickserve/internal/server"
)

// version 在发布构建时通过 -ldflags "-X github.com/.../server.version=vX.Y.Z" 注入。
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	opts, err := config.ParseArgs(os.Args[1:])
	switch {
	case err == config.ErrHelp:
		config.PrintUsage(os.Stdout)
		return 0
	case err == config.ErrVersion:
		server.SetVersion(version)
		server.PrintVersion(os.Stdout)
		return 0
	case err != nil:
		fmt.Fprintf(os.Stderr, "error: %v\n\n", err)
		config.PrintUsage(os.Stderr)
		return 1
	}
	server.SetVersion(version)
	return server.Run(&opts, os.Args[1:])
}

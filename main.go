// Package main 实现一个零依赖的静态文件服务器，功能类似
// python -m http.server，额外支持文件上传、CORS 与多平台发布。
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// version 在发布构建时通过 -ldflags "-X main.version=vX.Y.Z" 注入。
var version = "dev"

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err == errHelp {
		printUsage(os.Stdout)
		return
	}
	if err == errVersion {
		printVersion(os.Stdout)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n", err)
		printUsage(os.Stderr)
		os.Exit(1)
	}

	absDir, err := filepath.Abs(opts.dir)
	if err != nil {
		log.Fatalf("resolve directory: %v", err)
	}
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		log.Fatalf("directory does not exist: %s", absDir)
	}

	printBanner(os.Stdout, opts, absDir)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", opts.port),
		Handler: newHandler(opts, absDir),
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

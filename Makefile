# quickserve 常用任务
BINARY  := quickserve
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet fmt run docker docker-run clean help

build: ## 编译本机二进制
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/quickserve

test: ## 运行全部测试
	go test -race ./...

vet: ## 静态检查
	go vet ./...

fmt: ## 格式化
	gofmt -w .

run: ## 本机启动（开启上传）
	go run ./cmd/quickserve -u

docker: ## 构建镜像
	docker build -t quickserve:$(VERSION) --build-arg VERSION=$(VERSION) .

docker-run: ## 以安全默认值运行容器
	docker run --rm -p 8000:8000 -v $(PWD)/data:/data \
		--read-only --cap-drop ALL --security-opt no-new-privileges:true \
		quickserve:$(VERSION)

clean: ## 清理构建产物
	go clean
	rm -f $(BINARY)

help: ## 显示本帮助
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

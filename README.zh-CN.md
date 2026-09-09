# quickserve

[![CI](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml/badge.svg)](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Quenwaz/quickserve)](https://github.com/Quenwaz/quickserve/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | **简体中文**

一个零依赖的单文件静态 HTTP 服务器。功能和 `python -m http.server` 类似，但**不需要装 Python、能安全地接收任意类型文件上传、支持跨域**。纯 Go 标准库实现，整个项目没有一行第三方依赖。交付形态：约 7 MB 的单文件可执行程序，或约 6.5 MB 的 scratch 容器镜像。

## 它能帮你做什么

- **电脑上的文件，手机直接看**：启动后把打印出的局域网地址发给手机，浏览器直接预览图片、播放视频、查看文档，不用微信传一遍。
- **手机拍的照片/视频，传回电脑**：开 `-u` 上传开关，浏览器打开 `/_upload` 页面拖拽上传，或者任意设备命令行推文件。
- **临时给同事共享个安装包**：比网盘快，不压缩不限速，局域网内跑满带宽。
- **前端本地调试**：开 `-c` 一键解决跨域，静态页面、mock 数据随手起一个服务。

没有 Python 环境、不想装 Node、不想 `npm install` 一个静态服务器的时候，一个 10 MB 的 exe 双击就能用。

## 下载安装

**方式一**：去 [Releases 页面](https://github.com/Quenwaz/quickserve/releases) 下载对应平台的压缩包，解压即用，每个版本附带 SHA-256 校验文件：

| 系统 | 文件 |
|---|---|
| Windows x64 | `quickserve_<版本>_windows_amd64.zip` |
| macOS Apple Silicon | `quickserve_<版本>_darwin_arm64.zip` |
| macOS Intel | `quickserve_<版本>_darwin_amd64.zip` |
| Linux x64 | `quickserve_<版本>_linux_amd64.tar.gz` |
| Linux ARM（树莓派等） | `quickserve_<版本>_linux_arm64.tar.gz` |

**方式二**：已有 Go 环境也可以 `go install github.com/Quenwaz/quickserve@latest`

**方式三**：Docker

```sh
docker pull ghcr.io/Quenwaz/quickserve:latest
docker run --rm -p 8000:8000 -v "$PWD/data:/data" ghcr.io/Quenwaz/quickserve
```

## 三个最常用的场景

**1. 电脑 → 手机：局域网看文件**

```powershell
quickserve
```

启动后会打印：

```
quickserve v0.2.0
Serving HTTP on port 8000 (http://localhost:8000/)
Directory: C:\Users\you\Desktop\share
LAN:       http://192.168.1.5:8000/
Upload:    disabled
CORS:      disabled
Press Ctrl+C to stop.
```

手机连同一个 Wi-Fi，访问那个 `LAN:` 地址即可。

**2. 手机 → 电脑：接收文件**

```powershell
quickserve -u
```

手机浏览器打开局域网地址下的 `/_upload` 页面，拖拽上传、实时进度；或者任意设备命令行：

```sh
curl -T photo.jpg http://192.168.1.5:8000/photo.jpg   # 原始请求体
curl -F file=@photo.jpg http://192.168.1.5:8000/      # multipart 表单
```

**3. 前端调试要跨域**

```powershell
quickserve -c -p 9000
```

## 命令行参数

```
quickserve [端口] [目录]
quickserve [选项]

  -p, -port <N>          监听端口（默认 8000）
  -d, -dir <路径>        服务目录（默认当前目录）
  -u, -upload            开启文件上传，接受 POST/PUT（默认关闭）
  -c, -cors              开启 CORS 跨域响应头（默认关闭）
  -t, -token <密钥>      上传需要令牌（Authorization: Bearer / X-Auth-Token / ?token=）
  -m, -max-upload <大小> 单文件体积上限（默认 1GiB；0 不限；如 100MB）
  -r, -rate-limit <N>    单 IP 每分钟上传请求数上限（默认 60；0 不限）
  -b, -base-path <路径>  反向代理子路径部署时的 URL 基础路径（如 /apps）
  -ro, -read-only        连下载也关闭，仅保留 /health 探活
  -v, -version           打印版本号
  -h, -help              显示帮助

每个选项都有对应的 `QUICKSERVE_*` 环境变量（`QUICKSERVE_PORT`、
`QUICKSERVE_DIR`、`QUICKSERVE_UPLOAD`、`QUICKSERVE_CORS`、`QUICKSERVE_TOKEN`、
`QUICKSERVE_MAX_UPLOAD`、`QUICKSERVE_RATE_LIMIT`、`QUICKSERVE_BASE_PATH`、
`QUICKSERVE_READ_ONLY`）；命令行参数优先于环境变量。布尔值接受
`1/true/yes/on` 与 `0/false/no/off`。
```

端口支持位置参数写法，和 `python -m http.server 9999` 习惯一致：`quickserve 9999`、`quickserve 9999 d:\share`。

### 端点一览

| 路径 | 方法 | 用途 |
|---|---|---|
| `/` 及文件路径 | GET, HEAD | 静态文件与目录列表 |
| `/<路径>` | POST, PUT | 上传到指定路径（需 `-u`） |
| `/` | POST（multipart） | 浏览器表单上传（需 `-u`） |
| `/_upload` | GET | 内置上传页面（需 `-u`） |
| `/health` | GET | 存活探针，始终开启，不受 base-path 前缀影响 |

### 反向代理子路径部署

quickserve 被挂在子路径下时（如 nginx `location /apps/ { proxy_pass http://qs:8000; }`），
用 `-b/--base-path`（或 `QUICKSERVE_BASE_PATH`）声明前缀：

```sh
quickserve -u -b /apps
# 此时可从 http://host:8000/apps/ 访问，上传落点相对该前缀；
# 不带前缀的请求以 308 重定向到带前缀的地址（保留方法、请求体与查询串，
# 上传不会因此失败）；/health 在根路径与前缀下都直接应答
```

## 上传保护机制

上传默认关闭，显式开启后有以下多层防护：

- **路径穿越防护**：`..` 段直接拒绝（403）；文件名逐段清洗为安全基名；Windows 保留设备名（`CON`、`NUL`、`COM1`…）拒绝；非法字符替换为 `_`；超长名按 UTF-8 边界截断。
- **覆盖保护**：已存在的非空文件绝不被覆盖（409）。上传失败**不留半截文件**——内容先写临时文件、fsync 落盘，再原子 rename 到目标名。
- **体积上限**：`-m` 限制单文件大小（默认 1 GiB），超限返回 413；带 Content-Length 的请求在读入一个字节前就拒绝。
- **令牌鉴权**：`-t` 后上传必须携带 `Authorization: Bearer`、`X-Auth-Token` 头或 `?token=` 参数（恒定时间比较，防时序侧信道）。
- **限速**：`-r` 按客户端 IP 滑动窗口限流（默认 60 次/分钟），内存占用有界，伪造源地址也打不爆。
- **慢速攻击加固**：10 秒读头超时（Slowloris 防护）、空闲连接回收、停滞上传 15 分钟上限。
- **优雅关停**：SIGINT/SIGTERM 后停止接受新连接，等待进行中的上传落盘再退出。

## Docker

镜像采用两阶段构建：Go 以 `CGO_ENABLED=0` 编译出静态链接的二进制，然后拷入空的 **`scratch`** 基础镜像——无 shell、无包管理器、无 libc、连 CA 证书都没有，以 **UID 65532（非 root）** 运行，典型大小约 6.5 MB，攻击面比 distroless 更小。

```sh
docker build -t quickserve .
docker run --rm -p 8000:8000 -v "$PWD/data:/data" quickserve
```

[docker-compose.yml](docker-compose.yml) 附带安全加固：根文件系统只读、丢弃全部 capabilities、`no-new-privileges`、`/tmp` 用 tmpfs、内存/CPU 限额。容器原生暴露 `/health` 并内置 `HEALTHCHECK`——镜像里没有 curl 也能探活。

参数通过 command 传递，例如：

```yaml
command: ["-d", "/data", "-u", "-t", "s3cr3t", "-m", "500MB"]
```

## 项目结构

```
cmd/quickserve/       入口：参数解析、退出码
internal/config/      配置定义、解析与校验
internal/server/      路由、CORS、日志、生命周期、启动横幅
internal/upload/      上传处理、文件名清洗、限速器
internal/web/         内置上传页面（go:embed 编译进二进制）
```

## 从源码构建

```powershell
go build -o quickserve ./cmd/quickserve
go test -race ./...
gofmt -l . && go vet ./...
```

推送 `v*` 标签会自动触发 GitHub Actions：构建全平台二进制并发布 Release（[release 工作流](.github/workflows/release.yml)），同时发布多架构容器镜像到 GHCR（[docker-release 工作流](.github/workflows/docker-release.yml)）。

## 安全须知

- 上传**默认关闭**，需要显式 `-u` 开启，避免无意开放写入。
- 上传路径包含 `..` 会被拒绝（403），无法逃逸出服务目录。
- 局域网共享场景是明文 HTTP，**不要**在不信任的网络里共享敏感文件；需要时用 `-t` 防止陌生人写入。

## 许可证

[MIT](LICENSE) © Quenwaz

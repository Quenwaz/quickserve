# quickserve

[![CI](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml/badge.svg)](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Quenwaz/quickserve)](https://github.com/Quenwaz/quickserve/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

[English](README.md) | **简体中文**

一个零依赖的单文件静态 HTTP 服务器。功能和 `python -m http.server` 类似，但**不需要装 Python、能接收文件上传、支持跨域**。纯 Go 标准库实现，整个项目没有一行第三方依赖。

## 它能帮你做什么

- **电脑上的文件，手机直接看**：启动后把打印出的局域网地址发给手机，浏览器直接预览图片、播放视频、查看文档，不用微信传一遍。
- **手机拍的照片/视频，传回电脑**：开 `-u` 上传开关，手机浏览器打开页面上传，或者电脑开着等文件进来。
- **临时给同事共享个安装包**：比网盘快，不压缩不限速，局域网内跑满带宽。
- **前端本地调试**：开 `-c` 一键解决跨域，静态页面、mock 数据随手起一个服务。

没有 Python 环境、不想装 Node、不想 `npm install` 一个静态服务器的时候，一个 8MB 的 exe 双击就能用。

## 下载安装

去 [Releases 页面](https://github.com/Quenwaz/quickserve/releases) 下载对应平台的压缩包，解压即用，每个版本附带 SHA-256 校验文件：

| 系统 | 文件 |
|---|---|
| Windows x64 | `quickserve_<版本>_windows_amd64.zip` |
| macOS Apple Silicon | `quickserve_<版本>_darwin_arm64.zip` |
| macOS Intel | `quickserve_<版本>_darwin_amd64.zip` |
| Linux x64 | `quickserve_<版本>_linux_amd64.tar.gz` |
| Linux ARM（树莓派等） | `quickserve_<版本>_linux_arm64.tar.gz` |

已有 Go 环境也可以：`go install github.com/Quenwaz/quickserve@latest`

## 三个最常用的场景

**1. 电脑 → 手机：局域网看文件**

```powershell
quickserve
```

启动后会打印：

```
quickserve v0.1.0
Serving HTTP on port 8000 (http://localhost:8000/)
Directory: C:\Users\you\Desktop\share
LAN:       http://192.168.1.5:8000/
Upload:    disabled
Press Ctrl+C to stop.
```

手机连同一个 Wi-Fi，访问那个 `LAN:` 地址即可。

**2. 手机 → 电脑：接收文件**

```powershell
quickserve -u
```

手机浏览器打开局域网地址，用页面上传；或者任意设备命令行：

```sh
curl -T photo.jpg http://192.168.1.5:8000/photo.jpg
```

**3. 前端调试要跨域**

```powershell
quickserve -c -p 9000
```

## 命令行参数

```
quickserve [端口] [目录]
quickserve [选项]

  -p, -port <N>     监听端口（默认 8000）
  -d, -dir <路径>   服务目录（默认当前目录）
  -u, -upload       开启文件上传，接受 POST/PUT（默认关闭）
  -c, -cors         开启 CORS 跨域响应头（默认关闭）
  -v, -version      打印版本号
  -h, -help         显示帮助
```

端口支持位置参数写法，和 `python -m http.server 9999` 习惯一致：`quickserve 9999`、`quickserve 9999 d:\share`。

## 和 python -m http.server 对比

| | `python -m http.server` | quickserve |
|---|---|---|
| 运行环境 | 需要安装 Python | **无需任何环境**，单文件 |
| 文件上传 | ❌ 只能下载 | ✅ `POST` / `PUT` |
| CORS 跨域 | ❌ | ✅ `-c` 开启 |
| 启动时显示局域网地址 | ❌ 要自己 ipconfig | ✅ |
| 访问日志 | ✅ | ✅ 格式兼容 |

## 安全须知

- 上传**默认关闭**，需要显式 `-u` 开启，避免无意开放写入。
- 上传路径包含 `..` 会被拒绝（403），无法逃逸出服务目录；目录会按需自动创建。
- 和 `python -m http.server` 一样是明文 HTTP，**不要**在不信任的网络里共享敏感文件。

## 从源码构建

```powershell
go build -o quickserve .
go test ./...
```

推送 `v*` 标签会自动触发 GitHub Actions 构建全平台产物并发布 Release，见 [release 工作流](.github/workflows/release.yml)。

## 许可证

[MIT](LICENSE) © Quenwaz

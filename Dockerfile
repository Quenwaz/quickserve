# syntax=docker/dockerfile:1

# ---------- 构建阶段：官方 Go 工具链，静态编译 ----------
FROM golang:1.26-alpine AS build

WORKDIR /src

# 先拷贝依赖清单利用层缓存（本项目零第三方依赖，go mod download 秒过）
COPY go.mod go.sum* ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

# CGO_ENABLED=0 纯静态编译；-trimpath 去除本机路径；-ldflags 缩小体积并注入版本
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/quickserve ./cmd/quickserve

# ---------- 运行阶段：scratch，零基础镜像 —— 无 shell、无 libc、无证书 ----------
# 二进制为 CGO_ENABLED=0 纯静态链接，直接跑在空镜像上；
# 相比 distroless static 攻击面更小（连 CA/tzdata 都没有），体积也更小。
FROM scratch

# 元数据
LABEL org.opencontainers.image.title="quickserve" \
      org.opencontainers.image.description="Zero-dependency static file server with upload support" \
      org.opencontainers.image.source="https://github.com/Quenwaz/quickserve" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/quickserve /quickserve

# 非 root 用户运行（任意未映射到宿主用户的 UID）
USER 65532:65532

EXPOSE 8000

# 数据卷：服务/上传目录挂载点
VOLUME ["/data"]

# 内置探活子命令，无 curl/wget 依赖
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/quickserve", "healthcheck"]

ENTRYPOINT ["/quickserve"]
CMD ["-d", "/data", "-u"]

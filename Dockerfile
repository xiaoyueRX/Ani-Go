# ============================================================
# Ani-Go 多阶段 Docker 构建
# 全自动番剧追番管理系统
# ============================================================

# ---- Stage 1: 前端构建（强制使用宿主机原生架构，避免 QEMU 模拟 Node.js 极慢） ----
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: Go 后端构建（利用 Go 原生极速交叉编译，无需 QEMU） ----
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download
COPY . ./

# 从前端构建阶段拷贝 dist
COPY --from=frontend-builder /src/web/dist ./web/dist

# Go 原生交叉编译：amd64 / arm64 / armv7 等
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG APP_VERSION=v0.6.0
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=${APP_VERSION}" -trimpath -o /anigo .

# ---- Stage 3: 极简运行环境 ----
FROM alpine:3.21

# 时区 + CA 证书（HTTPS 请求需要）
RUN apk add --no-cache tzdata ca-certificates

ENV TZ=Asia/Shanghai \
    PORT=20001 \
    PUID=1000 \
    PGID=1000

WORKDIR /app
COPY --from=backend-builder /anigo /app/anigo

# 创建非 root 用户 (UID/GID 1000) 并预创建数据卷持久化目录 (/data/db, /data/media, /data/downloads)
RUN addgroup -g 1000 anigo && \
    adduser -u 1000 -G anigo -s /bin/sh -D anigo && \
    mkdir -p /data/db /data/media /data/downloads /app && \
    chown -R 1000:1000 /data /app && \
    chmod 755 /app/anigo

USER 1000:1000

EXPOSE 20001

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:20001/api/health || exit 1

ENTRYPOINT ["/app/anigo"]

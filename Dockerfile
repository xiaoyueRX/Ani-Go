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
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend-builder

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
ARG APP_VERSION=v0.5.2
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=${APP_VERSION}" -trimpath -o /anigo .

# ---- Stage 3: 极简运行环境 ----
FROM alpine:3.20

# 时区 + CA 证书（HTTPS 请求需要）
RUN apk add --no-cache tzdata ca-certificates

ENV TZ=Asia/Shanghai \
    PORT=20001

WORKDIR /app
COPY --from=backend-builder /anigo /app/anigo

EXPOSE 20001
ENTRYPOINT ["/app/anigo"]

# ============================================================
# Ani-Go 多阶段 Docker 构建
# 全自动番剧追番管理系统
# ============================================================

# ---- Stage 1: 前端构建 ----
FROM node:24-alpine AS frontend-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: Go 后端构建 ----
FROM golang:1.25-alpine AS backend-builder

# 安装 git（Go 模块可能需要）及基础编译工具
RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

# 从前端构建阶段拷贝 dist
COPY --from=frontend-builder /src/web/dist ./web/dist

# 静态编译（CGO_ENABLED=0，无 CGO 依赖）
# 支持多架构：amd64 / arm64 / arm
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /anigo .

# ---- Stage 3: 极简运行环境 ----
FROM alpine:3.22

# 时区 + CA 证书（HTTPS 请求需要）
RUN apk add --no-cache tzdata ca-certificates

ENV TZ=Asia/Shanghai
ENV PORT=20001

COPY --from=backend-builder /anigo /anigo

EXPOSE 20001
ENTRYPOINT ["/anigo"]

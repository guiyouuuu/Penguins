#!/usr/bin/env bash
# 企鹅棋全链路构建：前端 → 内嵌进 Go 单二进制
set -e
cd "$(dirname "$0")"

echo "==> 构建前端..."
cd frontend
npm install --silent
npm run build

echo "==> 同步前端内嵌资源..."
mkdir -p ../server/internal/web/dist
rsync -a --delete dist/ ../server/internal/web/dist/

echo "==> 构建 Go 服务器..."
cd ../server
go build -o penguin-chess-server ./cmd/api

echo "==> 完成！运行: cd server && ./penguin-chess-server (默认 :8080)"

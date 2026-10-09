#!/usr/bin/env bash
# 交叉编译出各平台的二进制，放到 dist/
set -euo pipefail
cd "$(dirname "$0")"

if command -v go >/dev/null 2>&1; then
  GO=go
elif [ -x "$PWD/.tools/go/bin/go" ]; then
  GO="$PWD/.tools/go/bin/go"      # 项目自带的工具链
else
  echo "找不到 Go。装一个（brew install go），或者把工具链解压到 .tools/go/"
  exit 1
fi

export CGO_ENABLED=0
export GOPROXY=off
export GOCACHE="$PWD/.tools/gocache"
export GOMODCACHE="$PWD/.tools/gomodcache"

mkdir -p dist

echo "用 $($GO version) 构建"
for target in linux/amd64 linux/arm64 darwin/arm64 darwin/amd64; do
  os="${target%%/*}"
  arch="${target##*/}"
  out="dist/imghost-${os}-${arch}"
  GOOS="$os" GOARCH="$arch" "$GO" build -trimpath -ldflags "-s -w" -o "$out" .
  echo "  ✓ $out"
done

cd dist && shasum -a 256 imghost-* > SHA256SUMS && cd ..
echo
echo "完成。校验和写入 dist/SHA256SUMS"

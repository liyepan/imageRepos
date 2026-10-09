#!/usr/bin/env bash
# 交叉编译出各平台的二进制，放到 dist/
set -euo pipefail
cd "$(dirname "$0")"

if ! command -v go >/dev/null 2>&1; then
  echo "找不到 Go。装一个：brew install go"
  exit 1
fi
GO=go

# 编译缓存就放在项目目录里，不往 ~/Library/Caches 或 /tmp 扔东西。
# 整个项目零第三方依赖，所以构建过程完全不需要联网。
export CGO_ENABLED=0
export GOCACHE="$PWD/.build-cache/go-build"
export GOMODCACHE="$PWD/.build-cache/go-mod"
mkdir -p "$GOCACHE" "$GOMODCACHE"

# 以后要是引入了外部包，记得先设：
#   export GOPROXY=https://goproxy.cn,direct    # 你这边 proxy.golang.org 不通

mkdir -p dist

echo "用 $($GO version) 构建"
for target in linux/amd64 linux/arm64 darwin/arm64 darwin/amd64; do
  os="${target%%/*}"
  arch="${target##*/}"
  out="dist/imageRepos-${os}-${arch}"
  GOOS="$os" GOARCH="$arch" "$GO" build -trimpath -ldflags "-s -w" -o "$out" .
  echo "  ✓ $out"
done

# macOS 用 shasum，Linux 用 sha256sum
if command -v sha256sum >/dev/null 2>&1; then
  (cd dist && sha256sum imageRepos-* > SHA256SUMS)
else
  (cd dist && shasum -a 256 imageRepos-* > SHA256SUMS)
fi
echo
echo "完成。校验和写入 dist/SHA256SUMS"

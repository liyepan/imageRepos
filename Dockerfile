# imghost —— 单文件图床
#
# 这个 Dockerfile 用 scratch 作基础镜像，不会从任何 registry 拉取东西。
# Docker Hub 被墙的环境下也能直接 build，镜像体积就是二进制本身。
#
#   docker build -t imghost:1.0.0 .
#
# 想从源码构建（需要能拉到 golang 镜像）用 Dockerfile.build。

FROM scratch

# 服务器架构：amd64（绝大多数 VPS）或 arm64（甲骨文 ARM、树莓派、M 系列 Mac）
ARG ARCH=amd64

COPY dist/imghost-linux-${ARCH} /imghost

ENV DATA_DIR=/data \
    ADDR=:8080

VOLUME ["/data"]
EXPOSE 8080

# 非 root 跑。挂载的目录需要先 chown 65532:65532，
# 详见 README 的「目录权限」一节；不想折腾就在 compose 里加 user: "0:0"
USER 65532:65532

ENTRYPOINT ["/imghost"]

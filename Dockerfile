# 发行版镜像。
#
# 基础镜像用 scratch —— 不需要从任何 registry 拉取，Docker Hub 不通的环境也能构建。
# 镜像里只有一个静态二进制，没有 shell、没有包管理器、没有多余的东西。
#
#   docker build -t imagerepos:latest .
#   docker save imagerepos:latest | gzip > imageRepos-docker.tar.gz
#
# 注意镜像名必须全小写，Docker 不接受大写字母的仓库名。

FROM scratch

# amd64 / arm64，构建时用 --build-arg ARCH=xxx 传。
# 不用 TARGETARCH 这个名字：它是 BuildKit 的保留参数，手动传值会打架。
ARG ARCH=amd64
COPY dist/imageRepos-linux-${ARCH} /imageRepos

# 兜底的架构标记。正常情况下镜像自身的 architecture 字段就是对的
# （构建时必须带 --platform），这个标签是第二道保险：
#   docker image inspect imagerepos:latest --format '{{index .Config.Labels "com.imagerepos.arch"}}'
LABEL org.opencontainers.image.title="imageRepos" \
      com.imagerepos.arch="${ARCH}"

# TZ 能生效是因为 main.go 里 import 了 time/tzdata，时区库编进二进制了
ENV DATA_DIR=/data \
    ADDR=:8080 \
    TZ=Asia/Shanghai

VOLUME ["/data"]
EXPOSE 8080

# 以 root 运行。
# 换成非 root（比如 user: "65532:65532"）的话，挂载的目录必须先 chown 给对应用户，
# 否则启动会直接失败并打印修复步骤。发行包优先保证「导入就能跑」。
ENTRYPOINT ["/imageRepos"]

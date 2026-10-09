# 发行版镜像。
#
# 基础镜像用 scratch —— 不需要从任何 registry 拉取，Docker Hub 不通的环境也能构建。
# 镜像里只有一个静态二进制，没有 shell、没有包管理器、没有多余的东西。
#
#   docker build -t imageRepos:latest .
#   docker save imageRepos:latest | gzip > imageRepos-docker.tar.gz

FROM scratch

# amd64 / arm64。buildx 会自动填 TARGETARCH，普通 docker build 用 --build-arg 传
ARG TARGETARCH=amd64
COPY dist/imageRepos-linux-${TARGETARCH} /imageRepos

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

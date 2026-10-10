# imageRepos

自托管的个人图床。**单个可执行文件、零第三方依赖、图片就存在你自己的目录里。**

上传后直接拿到直链，粘进 WordPress、Markdown 或任何编辑器就能用。
网页端支持 `⌘V` 粘贴截图，URL 自动进剪贴板 —— 从截图到贴进文章，全程不用碰文件管理器。

---

## 目录

- [特点](#特点)
- [安装](#安装)
- [跑起来](#跑起来)
- [常驻运行（systemd）](#常驻运行systemd)
- [挂域名和 HTTPS](#挂域名和-https可选)
- [在 WordPress / Markdown 里用](#在-wordpress--markdown-里用)
- [对接上传工具](#对接上传工具)
- [HTTP 接口](#http-接口)
- [环境变量](#环境变量)
- [数据与备份](#数据与备份)
- [开发](#开发)
- [已知限制](#已知限制)

---

## 特点

- **粘贴即上传** —— 网页里按 `⌘V`，剪贴板里的截图直接进图床，URL 自动进剪贴板
- **拖拽 / 多选上传**，一次最多 3 个并发
- **API 上传** —— PicGo、ShareX、Typora、uPic、curl 都能对接
- **单密码登录** —— 没有用户表、没有注册、没有找回密码
- **零依赖** —— 只用 Go 标准库，没有 `go.sum`，构建过程不联网
- **连数据库都没有** —— 元数据是一个 JSON 文件，备份就是拷目录
- **图片自动去重** —— 同一张图传两次只存一份
- **只认 magic bytes** —— 把 PHP 改名成 `.png` 也传不上去

编译出来约 7 MB，运行内存 20 MB 上下。前端打包在二进制里，运行时只需要那一个文件。

---

## 安装

三种方式，挑一种。

### 先确认你该下哪个文件

```bashuname -sm
```
| `uname -sm` 输出 | 下载 |
|---|---|
| `Linux x86_64` | `imageRepos-linux-amd64` |
| `Linux aarch64` | `imageRepos-linux-arm64` |
| `Darwin arm64` | `imageRepos-darwin-arm64`（Apple Silicon） |
| `Darwin x86_64` | `imageRepos-darwin-amd64`（Intel Mac） |
| Windows | 暂不支持 |

所有文件都在 [Releases](../../releases) 页，附 `SHA256SUMS` 可校验。

### 方式一：下载二进制直接跑（最快）

不需要 Go、不需要 Docker、不需要 root。

```bashchmod +x imageRepos-linux-amd64
PASSWORD=你的密码 ./imageRepos-linux-amd64
```
打开 <http://localhost:8080> 就是界面。

### 方式二：Docker

发行包里有两个架构的镜像 tar，**不需要联网拉任何 registry**（镜像是 `scratch` 基础，全部内容都在 tar 里）。

```bashuname -m
#   x86_64  → 用 docker-amd64 那个
#   aarch64 → 用 docker-arm64 那个

docker load -i imageRepos-docker-amd64.tar.gz
```
然后把发行包里的 `docker-compose.yml` 放到同一目录，改掉里面的 `PASSWORD`，启动：

```bashdocker compose up -d
```
> 如果启动报 `exec format error`，说明下成了另一个架构的包，换一个重新 `docker load` 即可
> （两个包共用 `imagerepos:latest` 这个 tag，后 load 的会覆盖前面的）。

### 方式三：从源码构建

需要 Go 1.22 或更高。

```bashgit clone <这个仓库的地址>
cd imageRepos
./build.sh          # 交叉编译出四个平台的二进制，放在 dist/
```
---

## 跑起来

最小启动只需要一个环境变量：

```bashPASSWORD=你的密码 ./imageRepos-linux-amd64
```
默认行为：

| | |
|---|---|
| 监听 | `:8080` |
| 数据目录 | `./data`（图片在 `./data/files/`） |
| 单文件上限 | 20 MB |

想改数据目录：

```bashPASSWORD=你的密码 DATA_DIR=/your/data/path ./imageRepos-linux-amd64
```
首次启动终端会打印一个 API Token，之后也可以在网页右上角的「API Token」按钮里查看。

---

## 常驻运行（systemd）

把二进制放到标准位置，数据放到 `/var/lib`：

```bashsudo install -m 755 imageRepos-linux-amd64 /usr/local/bin/imageRepos
sudo mkdir -p /var/lib/imageRepos
sudo chown 运行用户 /var/lib/imageRepos      # 换成实际跑这个服务的用户
```
新建 `/etc/systemd/system/imageRepos.service`：

```ini[Unit]
Description=imageRepos
After=network.target

[Service]
Type=simple
User=运行用户
Environment=PASSWORD=换成你的密码
Environment=DATA_DIR=/var/lib/imageRepos
Environment=ADDR=127.0.0.1:8080
# Environment=PUBLIC_BASE=https://img.example.com
ExecStart=/usr/local/bin/imageRepos
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```
```bashsudo chmod 600 /etc/systemd/system/imageRepos.service    # 里面有密码
sudo systemctl daemon-reload
sudo systemctl enable --now imageRepos
sudo systemctl status imageRepos
```
看日志：

```bashjournalctl -u imageRepos -f
```
升级就是换掉 `/usr/local/bin/imageRepos` 然后 `sudo systemctl restart imageRepos`。

> 上面把 `ADDR` 设成 `127.0.0.1:8080`，只监听本机，由反向代理对外。
> 想直接暴露端口就改成 `:8080`。

---

## 挂域名和 HTTPS（可选）

**Caddy**（自动申请证书，最省事）：

```img.example.com {
    reverse_proxy 127.0.0.1:8080
}
```
**Nginx**：

```nginxserver {
    listen 443 ssl http2;
    server_name img.example.com;

    ssl_certificate     /path/fullchain.pem;
    ssl_certificate_key /path/privkey.pem;

    client_max_body_size 30m;        # 要大于 MAX_MB，否则大图会被 Nginx 先挡掉

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```
`Host` 和 `X-Forwarded-Proto` 要传：程序靠它们推断外链地址、决定 Cookie 要不要加 `Secure`。
挂了域名之后建议把 `PUBLIC_BASE` 显式设上，这样外链地址不依赖请求头。

---

## 在 WordPress / Markdown 里用

1. 图床里粘贴或拖拽上传
2. URL 自动进剪贴板
3. WordPress 编辑器加「图片」区块 → **从 URL 插入** → 粘贴 → 回车

不需要任何插件。Markdown 同理，用返回的 `markdown` 字段即可。

---

## 对接上传工具

先在网页右上角点「API Token」拿到 Token。下面假设你的图床是 `https://img.example.com`。

### curl

```bashcurl -F "file=@photo.jpg" \
     -H "Authorization: Bearer 你的Token" \
     https://img.example.com/api/upload
```
只要 URL 纯文本：

```bashcurl -s -F "file=@photo.jpg" \
     -H "Authorization: Bearer 你的Token" \
     "https://img.example.com/api/upload?format=text"
```
### PicGo

装 **web-uploader**（自定义 Web 图床）插件，配置：

| 字段 | 值 |
|---|---|
| API 地址 | `https://img.example.com/api/upload` |
| POST 参数名 | `file` |
| 请求头 | `{"Authorization": "Bearer 你的Token"}` |
| JSON 路径 | `url` |

### ShareX

目标 → 自定义上传器：

| 字段 | 值 |
|---|---|
| 请求 URL | `https://img.example.com/api/upload` |
| 方法 | `POST` |
| 请求头 | `Authorization: Bearer 你的Token` |
| 表单字段 | `file` |
| 响应 URL | `$json:url$` |

### Typora

图像 → 上传服务选 **PicGo**；或使用自定义命令：

```bashcurl -s -F "file=@$1" -H "Authorization: Bearer 你的Token" "https://img.example.com/api/upload?format=text"
```
### uPic / iPic 等

选「自定义」图床，方法 `POST`，字段名 `file`，加一个 `Authorization` 请求头。

---

## HTTP 接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| `GET` | `/` | 登录 Cookie | 网页主界面 |
| `GET` | `/login` | 无 | 登录页 |
| `GET` | `/i/{文件名}` | **无** | 取图片，带一年强缓存 |
| `GET` | `/healthz` | 无 | 健康检查，返回 `ok` |
| `POST` | `/api/login` | 无 | 登录，body `{"password":"..."}` |
| `POST` | `/api/logout` | 无 | 退出 |
| `GET` | `/api/token` | 仅登录 Cookie | 查看 API Token |
| `GET` | `/api/list?q=&page=&size=` | 登录或 Token | 图片列表、搜索 |
| `POST` | `/api/upload` | 登录或 Token | 上传 |
| `POST` | `/api/delete` | 登录或 Token | 删除，参数 `name` |

Token 可以放三个地方，任选其一：`Authorization: Bearer xxx`、`X-Api-Token: xxx`、`?token=xxx`。

**上传接口三种用法**：

```bash# 1. multipart，字段名 file（也接受 files / image）
curl -F "file=@a.png" -H "Authorization: Bearer $TOKEN" .../api/upload

# 2. 裸 body，直接把图片字节当请求体
curl --data-binary @a.png -H "Content-Type: image/png" \
     -H "Authorization: Bearer $TOKEN" .../api/upload

# 3. 只要 URL 文本
curl -F "file=@a.png" -H "Authorization: Bearer $TOKEN" ".../api/upload?format=text"
```
**返回**（第一张的字段同时提到顶层，PicGo 直接取 `url` 就行）：

```json{
  "success": true,
  "count": 1,
  "url": "https://img.example.com/i/shot-88a7ac71.png",
  "markdown": "![shot.png](https://img.example.com/i/shot-88a7ac71.png)",
  "html": "<img src=\"https://img.example.com/i/shot-88a7ac71.png\" alt=\"shot.png\">",
  "name": "shot-88a7ac71.png",
  "original": "shot.png",
  "size": 12345,
  "width": 100,
  "height": 50,
  "mime": "image/png",
  "time": "2026-10-06T13:51:31+08:00",
  "files": [ { "...": "多文件时这里是全部结果" } ]
}
```
> `/i/{文件名}` 是**公开无鉴权**的，任何人拿到 URL 就能访问，这是它作为图床直链的前提。
> 所以别把不打算公开的东西传上来 —— 详情见[已知限制](#已知限制)。

---

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PASSWORD` | **必填** | 登录密码。没设会直接启动失败 |
| `API_TOKEN` | 自动生成 | 留空则首次启动生成一个，写进 `DATA_DIR/.apitoken` |
| `DATA_DIR` | `./data` | 数据根目录，图片在 `DATA_DIR/files/` |
| `ADDR` | `:8080` | 监听地址 |
| `PUBLIC_BASE` | 自动推断 | 外链域名前缀，如 `https://img.example.com` |
| `MAX_MB` | `20` | 单个文件大小上限 |

---

## 数据与备份

```DATA_DIR/
├── files/            ← 图片本体，文件名形如 shot-88a7ac71.png
├── index.json        ← 元数据（原始文件名、尺寸、哈希、上传时间）
├── .secret           ← 会话签名密钥（自动生成）
└── .apitoken         ← API Token（自动生成）
```
**备份就是打包这一个目录**：

```bashtar czf imageRepos-backup-$(date +%F).tar.gz -C /your/data/path .
```
`index.json` 是纯文本，可以直接看、直接 grep，坏了也能手工修。

文件名里的 `-88a7ac71` 是内容哈希的前 8 位，所以同一个文件重复上传会复用同一个 URL，不占两份空间。

> `.secret` 和 `.apitoken` 是密钥，别提交进版本库，也别进备份的公开位置。
> 仓库自带的 `.gitignore` 已经排除了 `data/` 和 `dist/data/`。

---

## 开发

```bash./build.sh          # 交叉编译 linux/amd64、linux/arm64、darwin/arm64、darwin/amd64
./e2e-test.sh       # 端到端回归测试，起临时实例，跑完自动清理
```
编译缓存放在项目里的 `.build-cache/`，不往 `~/Library/Caches` 或 `/tmp` 写东西。项目零第三方依赖，构建不联网。

代码结构：

| 文件 | 作用 |
|---|---|
| `main.go` | 配置、路由、启动、优雅关闭 |
| `auth.go` | 会话 Cookie、API Token、登录失败锁定 |
| `store.go` | JSON 索引，原子写入 |
| `handlers.go` | 所有 HTTP 处理逻辑 |
| `images.go` | magic bytes 嗅探、尺寸解析（含 WebP 手写解析） |
| `web/index.html` | 主界面，原生 JS，无框架 |
| `web/login.html` | 登录页 |

前端用 `//go:embed` 打进二进制，所以运行时只需要那一个可执行文件。

持续集成在 `.github/workflows/release.yml`：推 `main` 会更新滚动的 `latest` 发行版，推 `v*` 标签会发正式版本，两个架构的 Docker 镜像 tar 都在里面。

---

## 已知限制

这些是**故意不做**的，设计目标是「一个人用、几千张图以内」：

**功能上**

- 没有用户体系、注册、找回密码 —— 只有一个密码
- 没有配额和限流（只有登录失败 8 次锁 5 分钟）
- 没有图片压缩、转码、缩略图、水印 —— 原图原样存
- 没有相册 / 标签，只有一个平铺列表加按文件名搜索
- 不支持 SVG（能内嵌 JS，有 XSS 风险，故意拒掉）
- 不支持 Windows（没有交叉编译 Windows 目标）

**安全上**

- `/i/{文件名}` 公开无鉴权，**URL 即凭证**。拿到 URL 的人就能看图，且永久有效
- 文件名含内容哈希的前 8 位，所以**对内容已知的图片，URL 是可推导的**
- 没有「未发布」状态 —— 上传即公开
- 登录失败锁定是内存态，重启即清空
- 如果把服务直接暴露在公网，**用一个强密码**是最重要的一件事

**规模上**

- 索引全量载入内存。几千张没问题，上十万张该换数据库
- 上传是把文件读进内存再落盘，`MAX_MB` 设太大要留意内存

**验证情况**

- macOS (arm64) 与 Linux (aarch64, Debian 13) 已实际运行验证
- Linux x86_64 已经过交叉编译和 `GOOS=linux go vet`，理论上没问题（静态链接），但没有实机跑过

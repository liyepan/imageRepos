# imghost

一个只给自己用的图床。单文件、零依赖、本地存储、Docker 一条命令跑起来。

**为什么不用现成的？** 因为你写 WordPress 的真实流程是「截图 → 切浏览器 → 上传 → 复制链接 → 回编辑器」。
这个工具把它变成：**截图 → `⌘V` → 回编辑器直接粘 URL**。

---

## 特点

- **粘贴即上传** —— 网页里按 `⌘V`，剪贴板里的截图直接进图床，URL 自动进剪贴板
- **拖拽 / 多选上传**，一次最多 3 个并发
- **API 上传** —— PicGo、ShareX、Typora、uPic、curl 都能对接
- **单密码登录** —— 没有用户表、没有注册、没有找回密码
- **零依赖** —— 只用 Go 标准库，没有 go.sum，不需要联网拉包
- **SQLite 都不用** —— 元数据是一个 JSON 文件，备份就是拷目录
- **图片自动去重** —— 同一张图传两次只存一份
- **只认 magic bytes** —— 把 PHP 改名成 .png 传不上去

镜像基于 `scratch`，体积就是二进制本身（约 7 MB），内存占用 20 MB 上下。

---

## 一、在本机跑（macOS）

不用装 Go，`dist/` 里已经有编译好的二进制：

```bash
cd imageRepos
PASSWORD=你的密码 ./dist/imghost-darwin-arm64
```

然后打开 <http://localhost:8080>，输入密码登录。

数据（图片 + 索引）会写在当前目录的 `data/` 下。想换地方就加 `DATA_DIR`：

```bash
PASSWORD=你的密码 DATA_DIR=/Users/你/Pictures/imghost ./dist/imghost-darwin-arm64
```

第一次启动时终端会打印一个 API Token，也可以在网页右上角的「API Token」按钮里看。

---

## 二、部署到服务器（Docker）

### 1. 把项目传到服务器

```bash
# 在你的 Mac 上
scp -r imageRepos 你的用户名@服务器IP:~/
```

只需要 `dist/imghost-linux-amd64`、`Dockerfile`、`docker-compose.yml` 这几个文件，其余可以不传。

### 2. 建数据目录并授权

```bash
sudo mkdir -p /your/server/path
sudo chown -R 65532:65532 /your/server/path
```

> 容器里用的是非 root 用户（uid 65532）跑的，所以挂载的目录得先给它权限。
> 不想折腾就把 `docker-compose.yml` 里 `# user: "0:0"` 那行的注释去掉，用 root 跑。

### 3. 改配置

编辑 `docker-compose.yml`，至少改这几处：

- `PASSWORD` —— 换成你的密码
- `volumes` 左边的路径 —— 换成你刚才建的目录
- `ARCH` —— 服务器是 amd64 就保持不动；甲骨文 ARM / 树莓派 / M 系 Mac 改成 `arm64`

### 4. 构建并启动

```bash
docker compose up -d --build
docker compose logs -f
```

访问 `http://服务器IP:8080` 就能用了。

> **关于 Docker Hub**：默认的 `Dockerfile` 用 `scratch` 作基础镜像，**不联网也能 build**。
> 只有想从源码构建（`Dockerfile.build`）时才需要能访问 Docker Hub。

---

## 三、挂到域名和 HTTPS

你多半已经有 Nginx 或 Caddy 了。加一个反代就行。

**Caddy**（自动签证书，最省事）：

```
img.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

**Nginx**：

```nginx
server {
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

`X-Forwarded-Proto` 和 `Host` 一定要传，程序靠它们推断外链地址、决定 Cookie 要不要加 `Secure`。

挂好域名后，建议在 compose 里把 `PUBLIC_BASE` 填上（例如 `https://img.example.com`），这样外链地址就不依赖请求头了。

---

## 四、在 WordPress 里用

1. 图床里粘贴 / 拖拽上传
2. URL 会自动进剪贴板
3. WordPress 编辑器里加「图片」区块 → **从 URL 插入** → 粘贴 → 回车

不需要装任何插件。图片走的是你自己的域名，加载速度取决于你的服务器。

---

## 五、对接各种上传工具

先在网页右上角点「API Token」拿到 Token。

### curl

```bash
curl -F "file=@photo.jpg" \
     -H "Authorization: Bearer 你的Token" \
     https://img.example.com/api/upload
```

只想拿 URL 纯文本：

```bash
curl -s -F "file=@photo.jpg" \
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

- 请求 URL：`https://img.example.com/api/upload`
- 方法：`POST`
- 请求头：`Authorization: Bearer 你的Token`
- 表单字段：`file`
- 响应 URL：`$json:url$`
- 响应缩略图：`$json:url$`

### Typora

图像 → 上传服务选 **PicGo**，或者用自定义命令：

```bash
curl -s -F "file=@$1" -H "Authorization: Bearer 你的Token" "https://img.example.com/api/upload?format=text"
```

### uPic / iPic 等

选「自定义」图床，方法 `POST`，字段名 `file`，加一个 `Authorization` 请求头即可。

---

## 六、HTTP 接口

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| `GET` | `/` | 登录 Cookie | 网页主界面 |
| `GET` | `/login` | 无 | 登录页 |
| `GET` | `/i/{文件名}` | 无 | 取图片，带一年强缓存 |
| `GET` | `/healthz` | 无 | 健康检查，返回 `ok` |
| `POST` | `/api/login` | 无 | 登录，body `{"password":"..."}` |
| `POST` | `/api/logout` | 无 | 退出 |
| `GET` | `/api/token` | **仅登录 Cookie** | 查看 API Token |
| `GET` | `/api/list?q=&page=&size=` | 登录或 Token | 图片列表、搜索 |
| `POST` | `/api/upload` | 登录或 Token | 上传 |
| `POST` | `/api/delete` | 登录或 Token | 删除，参数 `name` |

Token 可以放在三个地方，任选其一：`Authorization: Bearer xxx`、`X-Api-Token: xxx`、`?token=xxx`。

**上传接口的三种用法**：

```bash
# 1. multipart，字段名 file（也接受 files / image）
curl -F "file=@a.png" -H "Authorization: Bearer $TOKEN" .../api/upload

# 2. 裸 body，直接把图片字节当请求体
curl --data-binary @a.png -H "Content-Type: image/png" \
     -H "Authorization: Bearer $TOKEN" .../api/upload

# 3. 只想要 URL 文本
curl -F "file=@a.png" -H "Authorization: Bearer $TOKEN" ".../api/upload?format=text"
```

**返回**（第一张的字段同时提到顶层，方便 PicGo 直接取 `url`）：

```json
{
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

---

## 七、环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PASSWORD` | **必填** | 登录密码。没设会直接启动失败 |
| `API_TOKEN` | 自动生成 | 留空则首次启动生成一个，写进 `DATA_DIR/.apitoken` |
| `DATA_DIR` | `./data` | 数据根目录，图片在 `DATA_DIR/files/` |
| `ADDR` | `:8080` | 监听地址 |
| `PUBLIC_BASE` | 自动推断 | 外链域名前缀，如 `https://img.example.com` |
| `MAX_MB` | `20` | 单个文件大小上限 |

---

## 八、数据与备份

```
DATA_DIR/
├── files/            ← 图片本体，文件名形如 shot-88a7ac71.png
├── index.json        ← 元数据（原始文件名、尺寸、哈希、时间）
├── .secret           ← 会话签名密钥（自动生成）
└── .apitoken         ← API Token（自动生成）
```

**备份 = 打包这一个目录**：

```bash
tar czf imghost-backup-$(date +%F).tar.gz -C /your/server/path .
```

索引是纯文本 JSON，可以直接看、直接 grep，坏了也能手工修。

文件名里的 `-88a7ac71` 是内容哈希的前 8 位，所以同一个文件重复上传会复用同一个 URL，不会占两份空间。

---

## 九、想改代码

```bash
./build.sh          # 交叉编译出 linux/amd64、linux/arm64、darwin/arm64、darwin/amd64
```

`build.sh` 会优先用系统的 `go`，找不到就用项目自带的 `.tools/go/bin/go`（已经下好放在这儿了）。

改完 Docker 镜像记得重新 build：`docker compose up -d --build`。

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

前端整个塞在 HTML 里用 `//go:embed` 打进二进制，所以部署只有一个文件。

---

## 十、已知限制

这些是**故意不做**的，因为你说了是自己用、几百张图：

- 没有用户体系、注册、找回密码 —— 只有一个密码
- 没有配额、限流（除了登录失败 8 次锁 5 分钟）
- 没有图片压缩、转码、缩略图、水印 —— 原图原样存
- 没有相册 / 标签，只有一个平铺列表 + 按文件名搜索
- 不支持 SVG（可以带 JS，有 XSS 风险，故意拒掉）
- 索引全量载入内存。几百张、几千张都没问题，上十万张该换数据库
- 上传是把文件读进内存再落盘的。`MAX_MB` 设太大要留意内存
- **Linux 二进制只做了交叉编译和 `GOOS=linux go vet`，没有在真实 Linux / Docker 里跑过**（这台 Mac 上没装 Docker）

#!/usr/bin/env bash
# imghost 端到端回归测试
#
#   ./e2e-test.sh
#
# 会在 .testdata/ 下起一个临时实例（端口 18080），跑完自动清理。
# 只依赖 curl、openssl 和 python3（只用标准库）。

cd "$(dirname "$0")" || exit 1

if ! command -v python3 >/dev/null 2>&1; then
  echo "需要 python3（只用标准库解析 JSON）"; exit 1
fi

BIN=./dist/imghost-darwin-arm64
[ -x "$BIN" ] || BIN=./dist/imghost-linux-amd64
[ -x "$BIN" ] || { echo "找不到二进制，先跑 ./build.sh"; exit 1; }

DATA=$PWD/.testdata
PORT=18080
BASEURL=http://127.0.0.1:$PORT
CJ=$DATA/cookies.txt

rm -rf "$DATA"; mkdir -p "$DATA/in"
export DATA_DIR="$DATA" ADDR=127.0.0.1:$PORT PASSWORD=test123 API_TOKEN=secret-token MAX_MB=1

# ---------- 造测试图 ----------
openssl base64 -d -A > "$DATA/in/shot.png" <<'B64'
iVBORw0KGgoAAAANSUhEUgAAAGQAAAAyCAIAAAAlV+npAAAAgUlEQVR4nO3SsREAIAwDscD+O8MK+V6qXf35vGHrrpeIVXhWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWIFYgViBWINbsfUiPAWPgQxDUAAAAAElFTkSuQmCC
B64
openssl base64 -d -A > "$DATA/in/photo.jpg" <<'B64'
/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAUDBAQEAwUEBAQFBQUGBwwIBwcHBw8LCwkMEQ8SEhEPERETFhwXExQaFRERGCEYGh0dHx8fExciJCIeJBweHx7/2wBDAQUFBQcGBw4ICA4eFBEUHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh4eHh7/wAARCABkAMgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDmaKKK/og/CQooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigD//2Q==
B64
openssl base64 -d -A > "$DATA/in/pic.webp" <<'B64'
UklGRlYAAABXRUJQVlA4IEoAAABQBACdASpAAEAAPm02mUkkIyKhIggAgA2JaQB2APwA/AAAIuML+46wAVQAAP7cUf//00UYUx9yb//7WgODVf5cf9UDCtgJgAAAAA==
B64
cp "$DATA/in/shot.png" "$DATA/in/same-again.png"
printf '<?php system($_GET[0]); ?>' > "$DATA/in/evil.png"
dd if=/dev/urandom of="$DATA/in/big.png" bs=1024 count=2048 2>/dev/null

# ---------- 起服务 ----------
"$BIN" > "$DATA/server.log" 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null; wait $PID 2>/dev/null' EXIT

for _ in $(seq 1 60); do curl -sf "$BASEURL/healthz" >/dev/null 2>&1 && break; sleep 0.2; done

pass=0; fail=0
chk() {
  if [ "$2" = "$3" ]; then echo "  [OK]   $1"; pass=$((pass+1));
  else echo "  [FAIL] $1  (期望: $2  实际: $3)"; fail=$((fail+1)); fi
}
code() { curl -s -o /dev/null -w "%{http_code}" "$@"; }
jget() { python3 -c "import sys,json;print(json.load(sys.stdin)$1)" 2>/dev/null || echo PARSE_ERR; }

echo "--- 1. 基础与鉴权 ---"
chk "healthz 可用"            200 "$(code $BASEURL/healthz)"
chk "未登录访问 / 跳登录页"    302 "$(code $BASEURL/)"
chk "登录页可访问"            200 "$(code $BASEURL/login)"
chk "没登录时上传被拒"        401 "$(code -X POST -F file=@$DATA/in/shot.png $BASEURL/api/upload)"
chk "错误密码被拒"            401 "$(code -X POST -H 'Content-Type: application/json' -d '{"password":"wrong"}' $BASEURL/api/login)"
chk "正确密码可登录"          200 "$(code -c $CJ -X POST -H 'Content-Type: application/json' -d '{"password":"test123"}' $BASEURL/api/login)"
chk "登录后 / 可访问"         200 "$(code -b $CJ $BASEURL/)"
chk "Token 不能反查 Token"     401 "$(code -H 'Authorization: Bearer secret-token' $BASEURL/api/token)"

echo "--- 2. 网页上传 ---"
R1=$(curl -s -b $CJ -F "file=@$DATA/in/shot.png" $BASEURL/api/upload)
chk "上传返回 success"        True "$(echo "$R1" | jget "['success']")"
chk "解析出 100x50"           100x50 "$(echo "$R1" | jget "['width']")x$(echo "$R1" | jget "['height']")"
chk "MIME 为 image/png"       image/png "$(echo "$R1" | jget "['mime']")"
NAME=$(echo "$R1" | jget "['name']")
URL=$(echo "$R1" | jget "['url']")
echo "       文件名: $NAME"
echo "       URL   : $URL"
chk "文件名保留原名"          shot "$(echo "$NAME" | cut -d- -f1)"

echo "--- 3. 图片访问 ---"
chk "图片能取回"              200 "$(code $URL)"
chk "Content-Type 正确"       image/png "$(curl -s -o /dev/null -w '%{content_type}' $URL)"
chk "一年强缓存"              yes "$(curl -sI $URL | grep -qi 'max-age=31536000, immutable' && echo yes || echo no)"
chk "带 nosniff"              nosniff "$(curl -sI $URL | grep -i 'x-content-type-options' | tr -d '\r' | cut -d' ' -f2)"
chk "不存在的图 404"          404 "$(code $BASEURL/i/nope.png)"
chk "路径穿越被挡"            404 "$(code --path-as-is "$BASEURL/i/..%2f..%2fetc%2fpasswd")"

echo "--- 4. 去重与格式校验 ---"
R2=$(curl -s -b $CJ -F "file=@$DATA/in/same-again.png" $BASEURL/api/upload)
chk "同内容重复上传复用文件"   "$NAME" "$(echo "$R2" | jget "['name']")"
chk "伪装成 png 的 php 被拒"   415 "$(code -b $CJ -F "file=@$DATA/in/evil.png" $BASEURL/api/upload)"
chk "超过大小限制被拒"         413 "$(code -b $CJ -F "file=@$DATA/in/big.png" $BASEURL/api/upload)"
R3=$(curl -s -b $CJ -F "file=@$DATA/in/pic.webp" $BASEURL/api/upload)
chk "webp 可上传"             image/webp "$(echo "$R3" | jget "['mime']")"
chk "webp 尺寸解析正确"        64x64 "$(echo "$R3" | jget "['width']")x$(echo "$R3" | jget "['height']")"

echo "--- 5. API 上传 ---"
chk "Bearer Token 可上传"     200 "$(code -H 'Authorization: Bearer secret-token' -F "file=@$DATA/in/photo.jpg" $BASEURL/api/upload)"
chk "错误 Token 被拒"         401 "$(code -H 'Authorization: Bearer wrong' -F "file=@$DATA/in/photo.jpg" $BASEURL/api/upload)"
chk "裸 body 上传"            200 "$(code -X POST -H 'Authorization: Bearer secret-token' -H 'Content-Type: image/png' --data-binary @$DATA/in/shot.png $BASEURL/api/upload)"
TXT=$(curl -s -H 'Authorization: Bearer secret-token' -F "file=@$DATA/in/shot.png" "$BASEURL/api/upload?format=text")
chk "?format=text 返回纯 URL"  http "$(echo "$TXT" | head -c4)"
chk "外链地址带正确端口"      yes "$(echo "$TXT" | grep -q '127.0.0.1:18080/i/' && echo yes || echo no)"

echo "--- 6. 列表 / 搜索 / 删除 ---"
L=$(curl -s -b $CJ "$BASEURL/api/list")
chk "列表有数据"              yes "$(echo "$L" | jget "['total']" | grep -qE '^[1-9]' && echo yes || echo no)"
echo "       共 $(echo "$L" | jget "['total']") 张"
chk "搜索能过滤"              1 "$(curl -s -b $CJ "$BASEURL/api/list?q=photo" | jget "['total']")"
chk "搜索无结果返回 0"        0 "$(curl -s -b $CJ "$BASEURL/api/list?q=zzzzz" | jget "['total']")"
chk "匿名不能列列表"          401 "$(code $BASEURL/api/list)"
chk "删除成功"                True "$(curl -s -b $CJ -X POST -F "name=$NAME" $BASEURL/api/delete | jget "['success']")"
chk "删掉后图片 404"          404 "$(code $URL)"
chk "重复删除返回 404"        404 "$(code -b $CJ -X POST -F "name=$NAME" $BASEURL/api/delete)"
chk "非法文件名被拒"          400 "$(code -b $CJ -X POST -F "name=../etc/passwd" $BASEURL/api/delete)"

echo "--- 7. 落盘情况 ---"
chk "图片写进 files/"         yes "$(ls $DATA/files | grep -qE '\.(png|jpg|jpeg|gif|webp|bmp|avif)$' && echo yes || echo no)"
chk "index.json 存在"         yes "$([ -s $DATA/index.json ] && echo yes || echo no)"
chk "磁盘文件数与索引一致"    "$(python3 -c "import json;print(len(json.load(open('$DATA/index.json'))))")" "$(ls $DATA/files | wc -l | tr -d ' ')"

echo "--- 8. 前端页面 ---"
IDX=$(curl -s -b $CJ $BASEURL/)
chk "首页有粘贴上传监听"      yes "$(echo "$IDX" | grep -q "addEventListener('paste'" && echo yes || echo no)"
chk "首页调用 /api/upload"    yes "$(echo "$IDX" | grep -q '/api/upload' && echo yes || echo no)"
chk "登录页调用 /api/login"   yes "$(curl -s $BASEURL/login | grep -q '/api/login' && echo yes || echo no)"

python3 -c "
import re
for src, dst in [('web/index.html','$DATA/index.js'), ('web/login.html','$DATA/login.js')]:
    html = open(src, encoding='utf-8').read()
    open(dst,'w',encoding='utf-8').write('\n'.join(re.findall(r'<script>(.*?)</script>', html, re.S)))
"
if command -v node >/dev/null 2>&1; then
  node --check $DATA/index.js 2>/dev/null && chk "首页 JS 语法" ok ok || chk "首页 JS 语法" ok 语法错误
  node --check $DATA/login.js 2>/dev/null && chk "登录页 JS 语法" ok ok || chk "登录页 JS 语法" ok 语法错误
else
  echo "  [跳过] JS 语法检查（没装 node）"
fi

echo
echo "=================== 通过 $pass   失败 $fail ==================="
if [ "$fail" != 0 ]; then echo "--- 服务器日志 ---"; tail -40 "$DATA/server.log"; fi
kill $PID 2>/dev/null
rm -rf "$DATA"
exit $fail

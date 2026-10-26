#!/bin/sh
# Cập nhật DCity trên server Ubuntu (dbx-linux@100.84.153.56, thư mục ~/dcity, https://dcity.dcompa.online).
# Build binary linux/amd64 từ HEAD, chép lên, build lại container app (database trong ~/dcity/data giữ nguyên).
# Chỉ khởi động lại khi không còn bàn nào đang mở (bàn chơi chỉ nằm trong bộ nhớ); thêm --force để bỏ qua.
set -eu
HOST=${DCITY_HOST:-dbx-linux@100.84.153.56}
SSH="ssh -o BatchMode=yes -o IdentitiesOnly=yes -i $HOME/.ssh/id_rsa"
cd "$(dirname "$0")/.."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
git archive HEAD | tar -x -C "$TMP"
(cd "$TMP" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dcity .)
if [ "${1:-}" != "--force" ]; then
  open=$(curl -s https://dcity.dcompa.online/api/games | python3 -c "import json,sys; print(sum((x.get('online') or {}).get('rooms',0) for x in json.load(sys.stdin)))")
  if [ "$open" != "0" ]; then echo "Còn $open bàn đang mở — thử lại sau hoặc dùng --force"; exit 1; fi
fi
scp -q -o BatchMode=yes -o IdentitiesOnly=yes -i "$HOME/.ssh/id_rsa" "$TMP/dcity" deploy/Dockerfile deploy/docker-compose.yml "$HOST:dcity/"
$SSH "$HOST" 'cd ~/dcity && docker compose up -d --build dcity-app && sleep 3 && docker ps --filter name=dcity --format "{{.Names}}\t{{.Status}}"'
curl -s -o /dev/null -w "https://dcity.dcompa.online %{http_code}\n" https://dcity.dcompa.online/healthz

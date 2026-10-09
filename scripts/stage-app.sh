#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCH="${1:?architecture}" MODE="${2:?permission mode}" OUT="${3:?output directory}"
case "$ARCH" in
  amd64|x86|x86_64) GOARCH=amd64; PATTERN='x86-64|x86_64' ;;
  arm64|arm|aarch64) GOARCH=arm64; PATTERN='ARM aarch64|ARM64|aarch64' ;;
  *) echo 'Unsupported architecture' >&2; exit 1 ;;
esac
case "$MODE" in
  standard) TAGS='' ;;
  full-ports) TAGS=full_ports ;;
  *) echo 'Unsupported permission mode' >&2; exit 1 ;;
esac
mkdir -p "$OUT/bin" "$OUT/etc" "$OUT/licenses"
VERSION="$(python3 "$ROOT/scripts/version.py")"
(cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -tags "$TAGS" -trimpath -buildvcs=false -ldflags="-s -w -X github.com/chenpingonline/nginx-web.BuildVersion=nginx-web-version/$VERSION/" -o "$OUT/bin/nginx-web-server" ./cmd/nginx-web)
"$ROOT/scripts/fetch-nginx.sh" "$GOARCH" "$OUT/bin/nginx" >/dev/null
for name in nginx-web-server nginx; do
  file "$OUT/bin/$name" | grep -Eq "$PATTERN" || { echo "Wrong architecture: $name" >&2; exit 1; }
done
file "$OUT/bin/nginx" | grep -Fq 'statically linked' || { echo 'Nginx must be statically linked' >&2; exit 1; }
grep -aFq 'nginx version: nginx/1.30.4' "$OUT/bin/nginx" || { echo 'Wrong Nginx version' >&2; exit 1; }
cp "$ROOT/third_party/nginx/mime.types" "$OUT/etc/mime.types"
cp "$ROOT/LICENSE" "$ROOT/NGINX_LICENSE" "$ROOT/NOTICE" "$ROOT/THIRD_PARTY_LICENSES.md" "$OUT/licenses/"
chmod 755 "$OUT/bin/"*

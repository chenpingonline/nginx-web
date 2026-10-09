#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(python3 "$ROOT/scripts/version.py")"
DIST="${NGINX_WEB_DIST:-$ROOT/dist}"
source "$ROOT/scripts/package-lib.sh"
if (( $# == 0 )); then
  case "$(uname -m)" in arm64|aarch64) set -- arm64 ;; x86_64|amd64) set -- amd64 ;; *) exit 1 ;; esac
fi
for arch in "$@"; do case "$arch" in amd64|arm64) ;; *) echo 'Use amd64 or arm64' >&2; exit 1 ;; esac; done
make --no-print-directory -C "$ROOT" FRONTEND_MODE=standalone PERMISSION_MODE=full-ports frontend-build
mkdir -p "$DIST"
for arch in "$@"; do
  WORK="$ROOT/.build/linux/$arch"
  rm -rf "$WORK"; mkdir -p "$WORK"
  "$ROOT/scripts/stage-app.sh" "$arch" full-ports "$WORK/app"
  cp "$ROOT/linux/"{install.sh,uninstall.sh,nginx-web.service,nginx-web.env.example,README.md} "$WORK/"
  cp "$ROOT/VERSION" "$WORK/VERSION"
  python3 - "$ROOT" "$WORK" <<'PYSOURCE'
import json,pathlib,subprocess,sys
root,work=map(pathlib.Path,sys.argv[1:])
def git(*args): return subprocess.check_output(['git',*args],cwd=root,text=True).strip()
(work/'CORE_SOURCE.json').write_text(json.dumps({'repository':'https://github.com/chenpingonline/nginx-web.git','commit':git('rev-parse','HEAD'),'version':(root/'VERSION').read_text().strip(),'dirty':bool(git('status','--porcelain'))},indent=2)+'\n')
PYSOURCE
  cp "$ROOT/third_party/nginx/$([[ $arch == amd64 ]] && echo x86_64 || echo arm64)/"{SOURCES.txt,SHA256SUMS.txt} "$WORK/app/licenses/"
  python3 - "$WORK" <<'PY'
import hashlib,pathlib,sys
root=pathlib.Path(sys.argv[1])
lines=[]
for p in sorted(root.rglob('*')):
    if p.is_file():
        with p.open('rb') as f: digest=hashlib.file_digest(f,'sha256').hexdigest()
        lines.append(f'{digest}  {p.relative_to(root)}\n')
(root/'SHA256SUMS').write_text(''.join(lines))
PY
  archive="$DIST/nginx-web-${VERSION}-linux-${arch}.tar.gz"
  create_archive "$WORK" "$archive"
  printf '%s  %s\n' "$(sha256_file "$archive")" "$(basename "$archive")" > "$archive.sha256"
  echo "Created $archive"
done

#!/usr/bin/env bash
set -euo pipefail
[[ $(id -u) == 0 ]] || { echo 'Run with sudo' >&2; exit 1; }
NO_START=0
case "${1:-}" in --no-start) NO_START=1 ;; '') ;; *) echo 'Usage: install.sh [--no-start]' >&2; exit 1 ;; esac
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for command_name in sha256sum file openssl getent useradd install; do command -v "$command_name" >/dev/null; done
if [[ $NO_START == 0 ]]; then
  command -v systemctl >/dev/null
  [[ -d /run/systemd/system ]] || { echo 'A running systemd is required; use --no-start to install files only' >&2; exit 1; }
fi
(cd "$ROOT" && sha256sum -c SHA256SUMS >/dev/null)
case "$(uname -m)" in aarch64|arm64) pattern='ARM aarch64|ARM64|aarch64' ;; x86_64|amd64) pattern='x86-64|x86_64' ;; *) exit 1 ;; esac
for name in nginx nginx-web-server; do file "$ROOT/app/bin/$name" | grep -Eq "$pattern"; done
if ! getent passwd nginx-web >/dev/null; then
  useradd --system --user-group --home-dir /var/lib/nginx-web --shell /usr/sbin/nologin nginx-web
fi
[[ $(id -u nginx-web) != 0 ]] || { echo 'nginx-web must be a non-root account' >&2; exit 1; }
getent group nginx-web >/dev/null
DEST=/usr/local/lib/nginx-web
[[ ! -L "$DEST" ]] || { echo 'Installation directory must not be a symlink' >&2; exit 1; }
if [[ $NO_START == 0 ]] && systemctl is-active --quiet nginx-web; then systemctl stop nginx-web; fi
install -d -m 755 "$DEST"
cp -a "$ROOT/app/." "$DEST/"
chown -R root:root "$DEST"
install -m 644 "$ROOT/VERSION" "$DEST/VERSION"
install -d -m 750 -o root -g nginx-web /etc/nginx-web
install -d -m 750 -o nginx-web -g nginx-web /var/lib/nginx-web
if [[ ! -e /etc/nginx-web/nginx-web.env ]]; then
  install -m 600 "$ROOT/nginx-web.env.example" /etc/nginx-web/nginx-web.env
fi
if [[ ! -e /etc/nginx-web/admin_password ]]; then
  umask 077
  openssl rand -base64 24 > /etc/nginx-web/admin_password
  chown root:nginx-web /etc/nginx-web/admin_password
  chmod 640 /etc/nginx-web/admin_password
fi
install -d -m 755 /etc/systemd/system
install -m 644 "$ROOT/nginx-web.service" /etc/systemd/system/nginx-web.service
if [[ $NO_START == 0 ]]; then
  systemctl daemon-reload
  systemctl enable --now nginx-web
fi
echo 'Installed nginx-web. Default administrator: admin; password file: /etc/nginx-web/admin_password'
echo 'Management port: 8080. Existing settings, password and application data were preserved.'

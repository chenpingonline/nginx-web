#!/usr/bin/env bash
set -euo pipefail
[[ $(id -u) == 0 ]] || { echo 'Run with sudo' >&2; exit 1; }
[[ $# == 0 ]] || { echo 'Usage: uninstall.sh (retains data and credentials)' >&2; exit 1; }
if [[ -d /run/systemd/system ]]; then
  systemctl disable --now nginx-web
fi
rm -f /etc/systemd/system/nginx-web.service
rm -rf /usr/local/lib/nginx-web
if [[ -d /run/systemd/system ]]; then systemctl daemon-reload; fi
echo 'Removed nginx-web program and service; retained /etc/nginx-web and /var/lib/nginx-web.'

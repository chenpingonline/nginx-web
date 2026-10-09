#!/usr/bin/env bash
# Shared archive creation for Linux and platform packaging repositories.
create_archive() {
  local source_dir="$1" output_file="$2"
  if tar --version 2>/dev/null | grep -q 'GNU tar'; then
    tar --sort=name --mtime='UTC 2026-09-02 00:00:00' --owner=0 --group=0 --numeric-owner -czf "$output_file" -C "$source_dir" .
  else
    find "$source_dir" -exec touch -h -t 202609020000.00 {} +
    (cd "$source_dir" && find . -print | LC_ALL=C sort | tar --no-recursion --uid 0 --gid 0 --uname root --gname root --no-acls --no-fflags --no-xattrs --no-mac-metadata -czf "$output_file" -T -)
  fi
}
sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

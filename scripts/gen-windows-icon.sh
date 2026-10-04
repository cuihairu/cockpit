#!/usr/bin/env bash
# Windows 图标链再生成：logo.svg → 多尺寸 .ico → rsrc .syso（exe 资源段）
#
# 产物：
#   packaging/windows/agent.ico           安装器/快捷方式图标（7 尺寸）
#   cmd/cockpit-agent/rsrc_windows_amd64.syso  go build windows/amd64 自动链接
#   cmd/cockpit-agent/rsrc_windows_arm64.syso  go build windows/arm64 自动链接
#
# 依赖：ImageMagick（magick，SVG 光栅化）+ rsrc（go run 拉取，免装）
# 注意：syso 按 <名>_<GOOS>_<GOARCH>.syso 命名才会被 go build 拾取，且只影响
# windows 目标（linux/darwin 构建忽略）；logo 变更后重跑本脚本并提交全部产物。
set -euo pipefail
cd "$(dirname "$0")/.."

SIZES=(16 24 32 48 64 128 256)
RSRC_VERSION=v0.10.2

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

ico_args=()
for s in "${SIZES[@]}"; do
  magick -background none web/public/logo.svg -resize "${s}x${s}" "$tmp/$s.png"
  ico_args+=("$tmp/$s.png")
done
magick "${ico_args[@]}" packaging/windows/agent.ico

for arch in amd64 arm64; do
  go run "github.com/akavel/rsrc@${RSRC_VERSION}" \
    -ico packaging/windows/agent.ico \
    -o "cmd/cockpit-agent/rsrc_windows_${arch}.syso"
done

echo "OK: agent.ico + rsrc_windows_{amd64,arm64}.syso 已再生成"

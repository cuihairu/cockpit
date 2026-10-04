#!/usr/bin/env bash
# Windows 图标全链再生（SVG → 多尺寸 ico → rsrc .syso）
# 用法：bash scripts/gen-windows-icon.sh
# 产物：
#   packaging/windows/agent.ico                 ← Inno Setup / 快捷方式 / 卸载条目
#   cmd/cockpit-agent/rsrc_windows_amd64.syso   ← GOOS=windows GOARCH=amd64
#   cmd/cockpit-agent/rsrc_windows_arm64.syso   ← GOOS=windows GOARCH=arm64
# 依赖：ImageMagick (magick)、go install github.com/akavel/rsrc@latest

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SVG="$ROOT/docs/public/logo.svg"
PACK_ICO="$ROOT/packaging/windows/agent.ico"
CMD_DIR="$ROOT/cmd/cockpit-agent"

command -v magick >/dev/null || { echo "need magick (ImageMagick 7)" >&2; exit 1; }
command -v rsrc >/dev/null || { echo "need rsrc (go install github.com/akavel/rsrc@latest)" >&2; exit 1; }

echo "→ 光栅化多尺寸 ico: $PACK_ICO"
magick "$SVG" -define icon:auto-resize=256,128,64,48,32,16 "$PACK_ICO"

echo "→ 生成 rsrc .syso (amd64)"
rsrc -ico "$PACK_ICO" -o "$CMD_DIR/rsrc_windows_amd64.syso"

echo "→ 生成 rsrc .syso (arm64)"
rsrc -ico "$PACK_ICO" -o "$CMD_DIR/rsrc_windows_arm64.syso"

echo "✓ 完成。在 CI 中直接用现成 syso 编译（GOOS=windows GOARCH=amd64/arm64 即可自动链接）。"
#!/usr/bin/env bash
# 打包 macOS 应用。
#
# wails build 之后额外做两件事：
#   1. 重新注册到 LaunchServices。macOS 按 bundle 标识缓存图标，图标改动后
#      不刷新的话 Dock / Finder 会继续显示旧图标。
#   2. 确认 iconfile.icns 存在且可被 iconutil 解析，避免交付一个没有图标的包。
set -euo pipefail

cd "$(dirname "$0")/.."
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}"

APP="build/bin/CLEAR.app"
RESOURCES="$APP/Contents/Resources"
LSREGISTER=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

echo "==> 构建"
wails build -clean

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "非 macOS，跳过图标检查"
  exit 0
fi

if [[ ! -f "$RESOURCES/iconfile.icns" ]]; then
  echo "!! 缺少 iconfile.icns" >&2
  exit 1
fi

echo "==> 校验图标"
if ! iconutil -c iconset "$RESOURCES/iconfile.icns" -o "/tmp/CLEAR-verify.iconset" 2>/dev/null; then
  echo "!! iconfile.icns 无法解析，图标会不显示" >&2
  exit 1
fi
rm -rf "/tmp/CLEAR-verify.iconset"
echo "    图标 OK"

echo "==> 刷新 LaunchServices 图标缓存"
touch "$APP"
[[ -x "$LSREGISTER" ]] && "$LSREGISTER" -f "$APP" || true

echo "==> 打包"
VERSION=$(python3 -c "import json;print(json.load(open('wails.json'))['info']['productVersion'])")
ARCH=$(uname -m)
ARCHIVE="dist/CLEAR-${VERSION}-darwin-${ARCH}.zip"
mkdir -p dist
rm -f "$ARCHIVE"
(cd build/bin && zip -q -r -y "../../$ARCHIVE" CLEAR.app)
ls -lh dist/

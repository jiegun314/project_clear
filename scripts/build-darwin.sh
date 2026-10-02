#!/usr/bin/env bash
# 打包 macOS 应用。
#
# wails build 之后额外做三件事：
#   1. 校验 Info.plist 合法。Wails 直接把 wails.json 里的文本塞进 plist 模板，
#      不做 XML 转义，值里出现裸 & 会产出非法 XML（Finder 的"显示简介"变乱码，
#      严格解析器直接报错）。这里用 plutil 当闸门。
#   2. 重新注册到 LaunchServices。macOS 按 bundle 标识缓存图标，图标改动后
#      不刷新的话 Dock / Finder 会继续显示旧图标。
#   3. 确认 iconfile.icns 存在且可被 iconutil 解析，避免交付一个没有图标的包。
set -euo pipefail

cd "$(dirname "$0")/.."
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}"
export npm_config_cache="${npm_config_cache:-$PWD/frontend/.npm-cache}"

APP="build/bin/CLEAR.app"
RESOURCES="$APP/Contents/Resources"
PLIST="$APP/Contents/Info.plist"
LSREGISTER=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister

echo "==> 构建"
wails build -clean

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "非 macOS，跳过图标与 plist 检查"
  exit 0
fi

echo "==> 校验 Info.plist"
if ! plutil -lint "$PLIST" >/dev/null; then
  echo "!! Info.plist 不是合法 plist，交付后"显示简介"等信息会损坏" >&2
  echo "!! 常见原因：wails.json 的 info 字段里有裸 & < > 等 XML 元字符，" >&2
  echo "!!         Wails 渲染模板时不转义。写成 &amp; 即可。" >&2
  exit 1
fi
# 反解一次，确认关键字段真的能被读出来，而不只是语法合法。
if ! plutil -extract CFBundleIdentifier raw -o /dev/null "$PLIST" 2>/dev/null; then
  echo "!! Info.plist 无法解析出 CFBundleIdentifier" >&2
  exit 1
fi
echo "    $(plutil -extract CFBundleIdentifier raw -o - "$PLIST") OK"

if [[ ! -f "$RESOURCES/iconfile.icns" ]]; then
  echo "!! 缺少 iconfile.icns" >&2
  exit 1
fi

echo "==> 校验图标"
VERIFY_ICONSET="$(mktemp -d /tmp/CLEAR-verify.XXXXXX)/CLEAR.iconset"
if ! iconutil -c iconset "$RESOURCES/iconfile.icns" -o "$VERIFY_ICONSET" 2>/dev/null; then
  echo "!! iconfile.icns 无法解析，图标会不显示" >&2
  exit 1
fi
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

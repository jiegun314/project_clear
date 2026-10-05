#!/usr/bin/env bash
# 打包 macOS 应用（仅 Apple Silicon / arm64）。
#
# 只发 arm64：通用二进制会把 Go 二进制复制一份，压缩包几乎翻倍
# （实测 18.8 MB → 9.1 MB），项目也不再声明支持 Intel Mac。
# 需要临时构建 Intel 版时用 `wails build -platform darwin/amd64`。
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

# `wails build -clean` deletes the whole build/bin directory, and the running
# program keeps its database and parameters there: build/bin/data/clear.db and
# build/bin/config/clear.yaml (README 第 7 节). Cleaning them away would destroy
# the user's data.
#
# They are moved to build/ — the parent of bin, so the same file system and the
# move is a rename even for a multi-gigabyte database — and put back afterwards.
# The trap covers a failed build and the early-exit paths, and rmdir only removes
# the stash when it is empty, so a failed move can never delete what it held.
BIN_DIR="build/bin"
STASH_DIR="build/.clear-build-stash"
RUNTIME_DIRS=(data config)

restore_runtime_data() {
  [[ -d "$STASH_DIR" ]] || return 0
  mkdir -p "$BIN_DIR"
  for name in "${RUNTIME_DIRS[@]}"; do
    if [[ -e "$STASH_DIR/$name" && ! -e "$BIN_DIR/$name" ]]; then
      mv "$STASH_DIR/$name" "$BIN_DIR/$name" || true
    fi
  done
  rmdir "$STASH_DIR" 2>/dev/null || true
  return 0
}

stash_runtime_data() {
  local moved=0
  for name in "${RUNTIME_DIRS[@]}"; do
    if [[ -e "$BIN_DIR/$name" ]]; then
      mkdir -p "$STASH_DIR"
      mv "$BIN_DIR/$name" "$STASH_DIR/$name"
      moved=1
    fi
  done
  [[ "$moved" == 1 ]] && echo "    运行数据已暂存到 ${STASH_DIR}（-clean 会清空 build/bin）"
  return 0
}

trap restore_runtime_data EXIT

# A stash left behind by an interrupted earlier run is returned before anything
# else, so its data is never forgotten.
restore_runtime_data
stash_runtime_data

echo "==> 构建 (darwin/arm64)"
wails build -platform darwin/arm64 -clean

# Hand the data back immediately, so the checks and the packaging below see the
# directory the way the application expects it.
restore_runtime_data
for name in "${RUNTIME_DIRS[@]}"; do
  if [[ -d "$STASH_DIR/$name" ]]; then
    echo "!! 运行数据未能归还，仍在 $STASH_DIR/$name" >&2
    echo "!! 请手动移回 $BIN_DIR/$name 后再运行程序" >&2
    exit 1
  fi
done

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
ARCH=arm64
ARCHIVE="dist/CLEAR-${VERSION}-darwin-${ARCH}.zip"
mkdir -p dist
rm -f "$ARCHIVE"
(cd build/bin && zip -q -r -y "../../$ARCHIVE" CLEAR.app)
ls -lh dist/

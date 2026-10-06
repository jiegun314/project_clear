#!/usr/bin/env bash
# 打包 Windows x64 版本。
#
# 与 build-darwin.sh 的区别：
#   * 不做 -clean。Windows 产物 CLEAR.exe 同样落在 build/bin，而那里还放着
#     运行数据（build/bin/data、build/bin/config）和 macOS 的 CLEAR.app；
#     -clean 会把整个 build/bin 删掉。这里刻意不加它，构建前照样做一次暂存/
#     归还，纯粹是为了将来有人顺手补上 -clean 时不至于毁掉用户数据。
#   * 产物要放进 zip 根目录下的 CLEAR/ 文件夹，并附一份面向使用者的
#     README.txt（Windows 用户拿到的是一个裸目录，macOS 那边有 .app 外壳）。
set -euo pipefail

cd "$(dirname "$0")/.."
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}"
export npm_config_cache="${npm_config_cache:-$PWD/frontend/.npm-cache}"

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
  [[ "$moved" == 1 ]] && echo "    运行数据已暂存到 ${STASH_DIR}"
  return 0
}

# 只装一个 EXIT trap：归还运行数据（构建中途失败也不能丢）并清理临时目录。
# WORK 必须先声明，否则 set -u 下 trap 在赋值前触发会报错。
WORK=""
trap '[[ -n "$WORK" ]] && rm -rf "$WORK"; restore_runtime_data' EXIT

restore_runtime_data
stash_runtime_data

echo "==> 构建 (windows/amd64)"
wails build -platform windows/amd64
restore_runtime_data
for name in "${RUNTIME_DIRS[@]}"; do
  if [[ -d "$STASH_DIR/$name" ]]; then
    echo "!! 运行数据未能归还，仍在 $STASH_DIR/$name" >&2
    exit 1
  fi
done

VERSION=$(python3 -c "import json;print(json.load(open('wails.json'))['info']['productVersion'])")
EXE="$BIN_DIR/CLEAR.exe"
[[ -f "$EXE" ]] || { echo "!! 没有生成 $EXE" >&2; exit 1; }

echo "==> 校验产物"
FILE_INFO=$(file -b "$EXE")
echo "    $FILE_INFO"
case "$FILE_INFO" in
  *PE32*x86-64*) ;;
  *) echo "!! $EXE 不是 64 位 Windows 可执行文件" >&2; exit 1;;
esac

echo "==> 组装发布目录"
WORK="$(mktemp -d)"
PKG="$WORK/CLEAR"
mkdir -p "$PKG"
cp "$EXE" "$PKG/"

# 面向使用者的说明。历史小节保留自上一版，本版新增内容写在最前面。
{
  printf 'CLEAR v%s（Windows x64）\n' "$VERSION"
  cat <<'BODY'
================================

运行
  双击 CLEAR.exe 即可。数据与参数会写在程序所在目录的 data\ 与 config\ 下，
  整个目录复制即可备份或迁移。

环境
  Windows 10 / 11，需要 Microsoft Edge WebView2 运行时（系统一般已预装；
  如提示缺少，请安装 Microsoft Edge WebView2 Runtime Evergreen 版）。

本版更新（v1.6.4）
  1. 修复"纯数据"导出（默认模式）里第 2 行周起始日期显示成整数的问题（重要）。导出的
     日期单元格被写成了文本，里面存的是日期的序列号（例如 46286），而数字格式对文本
     无效，所以显示出来就是一串数字。现在日期仍是日期，正常显示为 21-Sep。
     （"原文件格式"模式一直是对的。）另外这一行左边的周码现在也与原文件格式模式一致，
     是真正的数字而不是文本。
  2. 整合完成后，界面上显示的"整合时间"偶尔会与记录里的时间差一秒。原因是为了写记录和
     为了返回给界面各读了一次系统时钟，两次读数如果正好跨秒就会不一致。现在只读一次。

包含 v1.6.3 的改动
  1. 参数设定新增"表头显示"：可以选两行（周码 + 起始日期）或一行（只有周码，表头更
     紧凑）。这个参数以前只能手动编辑配置文件。
  2. 导入结果页面的"失败"标签改成与"成功"一致：浅灰底、文字用状态色（红/绿）。

包含 v1.6.2 的改动
  1. 修复 Windows 上导出必定报错的问题（重要）。导出时程序会先写好文件、再把它
     重命名到位，但在 Windows 上，只要文件还开着就不允许被替换——原来是程序自己
     的读取句柄没关。表现为：弹出"重命名失败/没有访问权限"的错误，而文件看起来是
     好的。实际后果比报错更严重：纯数据模式丢失条件格式（信号色），原文件格式模式
     则只得到一个没有数据的模板副本。现已修复。（macOS 不受影响。）
  2. 导入结果页面的"成功"标签：改为浅灰底、绿色文字，并在单元格里横向纵向居中。

包含 v1.6.1 的改动
  1. 参数设定新增"表头显示"：可以选两行（周码 + 起始日期）或一行（只有周码，表头更
     紧凑）。这个参数以前只能手动编辑配置文件。
  2. 导入结果页面的"失败"标签改成与"成功"一致：浅灰底、文字用状态色（红/绿）。

包含 v1.6.2 的改动
  1. 修复 Windows 上导出必定报错的问题（重要）。导出时程序会先写好文件、再把它
     重命名到位，但在 Windows 上，只要文件还开着就不允许被替换——原来是程序自己
     的读取句柄没关。表现为：弹出"重命名失败/没有访问权限"的错误，而文件看起来是
     好的。实际后果比报错更严重：纯数据模式丢失条件格式（信号色），原文件格式模式
     则只得到一个没有数据的模板副本。现已修复。（macOS 不受影响。）
  2. 导入结果页面的"成功"标签：改为浅灰底、绿色文字，并在单元格里横向纵向居中。

包含 v1.6.1 的改动
  1. 修复 Windows 上导出必定报错的问题（重要）。导出时程序会先写好文件、再把它
     重命名到位，但在 Windows 上，只要文件还开着就不允许被替换——原来是程序自己
     的读取句柄没关。表现为：弹出"重命名失败/没有访问权限"的错误，而文件看起来是
     好的。实际后果比报错更严重：纯数据模式丢失条件格式（信号色），原文件格式模式
     则只得到一个没有数据的模板副本。现已修复。（macOS 不受影响。）
  2. 导入结果页面的"成功"标签：改为浅灰底、绿色文字，并在单元格里横向纵向居中。

包含 v1.6.1 的改动
  本版没有功能变化，改的都是内部工程质量：改用 antd 新版属性写法、整理主窗口状态、
  补上自动化验收与持续集成。

包含 v1.6.0 的改动
  本版没有功能变化，改的都是内部工程质量，界面与操作与 v1.6.0 一致：
  1. 改用 antd 新版本的组件属性写法（提示框、分隔线、统计数值、弹窗遮罩），
     外观与行为不变，只是不再依赖已被标记弃用的写法。
  2. 主窗口的状态与行为做了整理，界面表现不变。
  3. 导入 / 添加 / 整合 / 导出与导出保真度现在有自动化验收，每次改动都会自动
     跑一遍完整流程，不再只靠人工验证。使用上没有遇到问题就不必特意升级。

包含 v1.6.0 的改动
  1. 导出文件修复（重要）。此前模板模式导出的工作表行号没有随表头重定位，
     文件会损坏；纯数据模式导出的文件则只有 1 行数据。现在两种模式都能导出
     完整数据，冻结窗格、筛选区域也随表头正确重定位。
  2. 整合后的批注不再丢失；打开旧数据库时会自动升级并留一份备份
     （data\clear.db.v0.bak），已有数据不受影响。
  3. 数据库里某一周的样式、批注或颜色数据损坏时，不再无声略过：运行日志会
     明确指出是哪一项被置空，便于把问题反馈回来定位。
  4. 参数设定窗口：数据加载中与加载失败不再是一片空白，保存失败或恢复默认
     失败都会给出提示；此前失败后窗口会一直打不开。
  5. 历史数据窗口不再反复重新查询；运行日志按时间倒序显示（最新在最上面），
     与"新日志置顶"的说明一致，不再把最早的一条排在最前面。

包含 v1.5.9 的改动
  1. 任务完成标记：导入、添加、整合、导出、清空完成后，左下角状态栏显示绿色的
     完成标记（此前会一直停在红色的"运行中"状态）。
  2. 历史数据窗口的导出也纳入同一套进度与完成标记，与工具栏导出表现一致；
     导出失败现在会正常提示，不再无声失败。
  3. 打开历史数据时增加全屏加载提示：点击"查看"的瞬间即出现加载动画，
     数据渲染完成后自动消失（数据量大时不再有"点了没反应"的空档）。

包含 v1.5.8 的调整
  需求文档移出版本管理；构建配置整理，确保全新克隆可直接编译。

包含 v1.5.7 的改动
  1. 运行日志改成表格形式：时间 | 类型 | 来源 | 内容 四栏，浅色细分隔线，
     条目很长时自动换行，时间始终停在自己那一栏。
  2. 日志按类型自动分类，筛选按钮显示每一类的条数。
  3. 日志内容中文化：来源栏不再出现 import/add，系统与库的英文报错先翻译成中文。
  4. 参数设定页面：底部按钮改为纯图标，各参数栏目之间用单色细横线区隔。

包含 v1.5.6 的改动
  整合数据表除 ITEM 列外，第二个 LOC 列也固定在左侧，向右滚动时两列始终可见。

包含 v1.5.5 的修复
  修复升级后导入报错：SQL logic error: table stg_row has no column named cf_colors。
  老数据库第一次打开程序时会自动补齐新增的列，导入/添加恢复正常，已有数据不受影响。

包含 v1.5.4 的功能
  1. 条件格式信号色：CalcOH / CalcOH2 这类行的红/黄/蓝/绿来自条件格式规则，
     现在会在导入时求值，窗口显示与导出都保留。
  2. 合并单元格内容逐行显示：MFG CLASS CODE 等合并单元格的内容补到被合并的每一行。
  3. WOS 行（第二列 LOC）的周数据保留一位小数。

说明
  Windows 版通过 windows/amd64 交叉编译产出，尚未在 Windows 真机上做过完整验收；
  如果遇到问题，请把 data\logs\ 下的日志发回以便定位。
BODY
} > "$PKG/README.txt"

echo "==> 打包"
mkdir -p dist
ARCHIVE="$PWD/dist/CLEAR-${VERSION}-windows-amd64.zip"
rm -f "$ARCHIVE"
(cd "$WORK" && zip -q -r -y "$ARCHIVE" CLEAR)
ls -lh "$ARCHIVE"

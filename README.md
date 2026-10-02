# CLEAR

> **C**onsolidation & **L**oading of **E**nterprise **A**nalytics for **R**eplenishment

CLEAR 读取 MPS 系统导出的 Excel 工作簿，筛选指定 LOC 的数据、跨文件合并，
并按周入库与导出，导出文件的样式、配色与注释与源文件保持一致。

技术方案：**Go + Wails v2 + Excelize**，前端 **React 19 + Ant Design 6 + Lucide**。

---

## 快速开始

```bash
# 依赖：Go 1.24+、Node 20+、Wails CLI v2
npm --prefix frontend install          # 安装前端依赖
wails dev                            # 开发模式（热更新）
wails build -clean                   # 打包，产物在 build/bin/
```

国内网络下 Go 模块下载缓慢时，可临时指定镜像：

```bash
GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct wails build
```

## 端到端验收

不带 UI 跑通整条流水线，并对导出文件做保真度审计：

```bash
go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e
go run ./tools/e2e -in raw_data/mps_data -out /tmp/clear-e2e -clean   # 干净重建模式
go test ./internal/mps/                                               # 条件格式与周码单测
```

## 数据流

```
导入 / 添加  ──▶  暂存表 stg_row          （可立即查看、可重复导入覆盖）
     │
     │ 整合
     ▼
  永久表 data_<周码>                      （如 data_2639，重复整合覆盖）
     │
     │ 导出
     ▼
  Excel 工作簿（模板改写 .xlsm / 干净重建 .xlsx）
```

- 周数据列以**周码**为唯一主键（`2639` = 2026 年第 39 周），日期由周码反算，
  读取与显示时再还原成源文件的第 55 / 56 两行。
- 跨文件合并要求 **A–O 索引表头与周码序列完全一致**，不一致的文件判为失败并跳过，
  不会静默对齐。
- 不同文件中完全相同的自然键（A–O 15 列）**全部保留**，并在界面与日志中标注来源文件。

## 关键实现说明

**条件格式是数据颜色的真正来源。** 这些工作簿每一数据行都带有自己的
`conditionalFormatting` 规则（把本行与相邻行比较，如 `(ROUND(P180,0)<P181)`），
且同一 `sqref` 下最多有 4 个独立块。excelize 的 `GetConditionalFormats` 以 sqref
为键做赋值，会丢掉其中 3 个，因此本项目改为**直接解析 sheet XML**，
把行号转换成相对锚点的偏移量保存，导出时再重锚到新行号并回写 zip。
`internal/mps/cfxml.go` 有对应单测。

**样式按签名去重存储。** 1516 行 × 35 列约 5.3 万个单元格，实际只有几百种不同样式；
签名 JSON 去重后存入 `cell_style` 表，导出时在目标工作簿中重建或复用。

**导出优先使用模板改写。** 以首个成功读取的源文件副本为模板，覆盖式写入数据区，
保留宏（`vbaProject.bin` 字节一致）、数据透视、切片器等全部部件；
`ReplaceParts` 对未改动的 zip 条目使用 `CreateRaw` 原样复制。
若模板不可用则回退为干净重建，只输出 MPS 表并移植源文件的 `dxfs`。

**读取走流式 XML 而非 `excelize.GetRows`。** 后者对每个单元格都要做数字格式解析，
在这类 4–8 MB 工作簿上单文件约 13 秒；改为流式解析后约 1.8 秒，
13 个文件的导入从 2 分 58 秒降到 24 秒（`internal/mps/values.go`）。

## 参数

首次启动会在程序同目录生成 `config/clear.yaml`（不可写时回退到 `~/.clear/config/`）：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `readColumns` | 20 | 从 P 列开始读取的周列数 |
| `locFilter` | WH_CNB | L 列（LOC）筛选值 |
| `pageSize` | 200 | 主界面每页行数 |
| `headerDisplay` | twoRow | 表头双行/单行显示 |
| `exportMode` | template | 模板改写 / 干净重建 |
| `exportDir` | — | 上次导出目录 |

## 目录结构

```
main.go / app.go          Wails 入口与前端绑定
internal/config           YAML 参数与默认文件自举
internal/logging          内存环形缓冲 + 按日落盘
internal/mps              读取、周码模型、条件格式、导出
internal/store            SQLite：暂存表 + 按周永久表
internal/service          导入/整合/导出的编排
frontend/src              React 界面（工具栏 / 数据表 / 历史 / 日志 / 状态栏）
tools/e2e                 端到端验收与保真度审计
```

## 已知限制

- **Wails v2 是单窗口框架**（`WindowCreate` 自 v3 起才提供）。需求中的
  “弹出单独窗口”因此实现为应用内的全屏遮罩层，功能与交互一致，但不是独立 OS 窗口。
- 表头不一致的文件按需求判为失败跳过，不会自动按周码对齐补列。

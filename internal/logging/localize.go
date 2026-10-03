package logging

import "regexp"

// 日志窗口是中文界面，但错误正文是产生它的库写的：标准库的 os 包、SQLite 驱动、
// excelize 都会给出英文片段（"open /x: no such file or directory"、
// "SQL logic error: table t has no column named c (1)"……）。
//
// localize 把这些常见片段改写成中文，避免面板里中英混排；没有命中规则的内容
// 原样保留——宁可留着英文，也不要被替换成看不出原因的套话。

type rewriteRule struct {
	re   *regexp.Regexp
	repl string
}

// fragmentRules 先翻译错误本体，后面的路径前缀才有中文尾巴可拼。
var fragmentRules = []rewriteRule{
	// 操作系统（POSIX / macOS）
	{regexp.MustCompile(`no such file or directory`), "文件或目录不存在"},
	{regexp.MustCompile(`no such process`), "进程不存在"},
	{regexp.MustCompile(`permission denied`), "没有访问权限"},
	{regexp.MustCompile(`operation not permitted`), "系统不允许该操作"},
	{regexp.MustCompile(`read-only file system`), "文件系统只读"},
	{regexp.MustCompile(`no space left on device`), "磁盘空间不足"},
	{regexp.MustCompile(`file name too long`), "文件名过长"},
	{regexp.MustCompile(`too many open files`), "同时打开的文件过多"},
	{regexp.MustCompile(`input/output error`), "磁盘读写错误"},
	{regexp.MustCompile(`is a directory`), "目标是一个目录"},
	{regexp.MustCompile(`not a directory`), "目标不是目录"},
	{regexp.MustCompile(`file exists`), "文件已存在"},
	{regexp.MustCompile(`broken pipe`), "管道已断开"},
	{regexp.MustCompile(`connection reset by peer`), "连接被对方重置"},
	{regexp.MustCompile(`context deadline exceeded`), "操作超时"},
	{regexp.MustCompile(`unexpected EOF`), "数据意外中断"},
	{regexp.MustCompile(`\bEOF\b`), "数据意外结束"},

	// Windows 的等价说法
	{regexp.MustCompile(`The system cannot find the file specified\.?`), "系统找不到指定的文件"},
	{regexp.MustCompile(`The system cannot find the path specified\.?`), "系统找不到指定的路径"},
	{regexp.MustCompile(`Access is denied\.?`), "访问被拒绝"},
	{regexp.MustCompile(`being used by another process\.?`), "文件正被另一个程序占用"},
	{regexp.MustCompile(`There is not enough space on the disk\.?`), "磁盘空间不足"},

	// 压缩包与 Excel（excelize / zipedit）
	{regexp.MustCompile(`zip: not a valid zip file`), "不是有效的 zip 压缩包"},
	{regexp.MustCompile(`not a valid zip file`), "不是有效的 zip 压缩包"},
	{regexp.MustCompile(`zip: checksum error`), "zip 压缩包校验和不匹配"},
	{regexp.MustCompile(`excelize:\s*`), "Excel 处理失败："},
	{regexp.MustCompile(`sheet (.+?) does not exist`), "工作表 $1 不存在"},
	{regexp.MustCompile(`sheet (.+?) already exists`), "工作表 $1 已存在"},

	// SQLite
	{regexp.MustCompile(`table (\S+) has no column named (\S+)`), "数据表 $1 缺少字段 $2"},
	{regexp.MustCompile(`no such table: (\S+)`), "数据表 $1 不存在"},
	{regexp.MustCompile(`no such column: (\S+)`), "字段 $1 不存在"},
	{regexp.MustCompile(`no such index: (\S+)`), "索引 $1 不存在"},
	{regexp.MustCompile(`UNIQUE constraint failed: (.+)`), "唯一性约束冲突：$1"},
	{regexp.MustCompile(`NOT NULL constraint failed: (\S+)`), "字段 $1 不能为空"},
	{regexp.MustCompile(`FOREIGN KEY constraint failed`), "外键约束失败"},
	{regexp.MustCompile(`constraint failed`), "约束校验失败"},
	{regexp.MustCompile(`database is locked`), "数据库正被其它进程占用"},
	{regexp.MustCompile(`database disk image is malformed`), "数据库文件已损坏"},
	{regexp.MustCompile(`disk I/O error`), "数据库磁盘读写错误"},
	{regexp.MustCompile(`SQL logic error: `), "SQL 错误："},
	{regexp.MustCompile(`SQL logic error`), "SQL 错误"},
}

// pathRules 把 "open X: …" 这类前缀改成中文动宾结构，放在 fragmentRules 之后。
var pathRules = []rewriteRule{
	{regexp.MustCompile(`\bopen (.+?): `), "打开 $1 失败："},
	{regexp.MustCompile(`\bmkdir (.+?): `), "创建目录 $1 失败："},
	{regexp.MustCompile(`\bremove (.+?): `), "删除 $1 失败："},
	{regexp.MustCompile(`\brename (.+?): `), "重命名 $1 失败："},
	{regexp.MustCompile(`\bstat (.+?): `), "读取 $1 的信息失败："},
	{regexp.MustCompile(`\bread (.+?): `), "读取 $1 失败："},
}

// 调用点自己写的 "打开失败: " 会和 pathRules 生成的 "打开 X 失败：" 叠在一起。
var mergedOpenPrefix = regexp.MustCompile(`打开失败[:：]\s*打开 (.+?) 失败[:：]`)

// localize rewrites the English fragments inside one log message.
func localize(msg string) string {
	if msg == "" {
		return msg
	}
	out := msg
	for _, r := range fragmentRules {
		out = r.re.ReplaceAllString(out, r.repl)
	}
	for _, r := range pathRules {
		out = r.re.ReplaceAllString(out, r.repl)
	}
	return mergedOpenPrefix.ReplaceAllString(out, "打开 $1 失败：")
}

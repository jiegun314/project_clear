package logging

import "testing"

func TestLocalizeRewritesLibraryErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "os path error behind our own prefix",
			in:   "2639.xlsm 打开失败: open /tmp/a.xlsm: no such file or directory",
			// The caller's own "打开失败: " prefix and the one derived from the
			// library error collapse into a single, non-repeated phrase.
			want: "2639.xlsm 打开 /tmp/a.xlsm 失败：文件或目录不存在",
		},
		{
			name: "sqlite missing column",
			in:   "写入失败: SQL logic error: table stg_row has no column named cf_colors (1)",
			want: "写入失败: SQL 错误：数据表 stg_row 缺少字段 cf_colors (1)",
		},
		{
			name: "sqlite missing table",
			in:   "no such table: data_2639",
			want: "数据表 data_2639 不存在",
		},
		{
			name: "excelize zip",
			in:   "打开失败: zip: not a valid zip file",
			want: "打开失败: 不是有效的 zip 压缩包",
		},
		{
			name: "windows wording",
			in:   "open C:\\tmp\\a.xlsm: The system cannot find the file specified.",
			want: "打开 C:\\tmp\\a.xlsm 失败：系统找不到指定的文件",
		},
		{
			name: "unknown text is kept verbatim",
			in:   "某个新出现的错误 xyzzy",
			want: "某个新出现的错误 xyzzy",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := localize(c.in); got != c.want {
				t.Fatalf("localize(%q)\n got: %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}

func TestLocalizeCollapsesDoubledOpenPrefix(t *testing.T) {
	got := localize("打开失败: 打开 /tmp/a.xlsm 失败：文件或目录不存在")
	if want := "打开 /tmp/a.xlsm 失败：文件或目录不存在"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoggerLocalizesBeforeItReachesTheUI(t *testing.T) {
	l := New("")
	l.Error("读取", "%s 打开失败: %v", "a.xlsm", errStub{})
	entries := l.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	want := "a.xlsm 打开 /tmp/a.xlsm 失败：文件或目录不存在"
	if entries[0].Message != want {
		t.Fatalf("got %q, want %q", entries[0].Message, want)
	}
}

// errStub mimics the *os.PathError text excelize hands back for a missing file.
type errStub struct{}

func (errStub) Error() string { return "open /tmp/a.xlsm: no such file or directory" }

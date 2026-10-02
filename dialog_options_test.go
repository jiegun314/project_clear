package main

import (
	"strings"
	"testing"
)

// 导入 / 添加 传进来的扩展名要变成对话框认识的 glob 列表；写错的写法（缺星号、
// 缺点、多余空格）都不该让过滤器失效。
func TestExcelPatterns(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"空列表", nil, []string{}},
		{"裸扩展名", []string{"xlsm", "xlsx"}, []string{"*.xlsm", "*.xlsx"}},
		{"带点与空格", []string{" .xlsm", "xlsx "}, []string{"*.xlsm", "*.xlsx"}},
		{"忽略空项", []string{"", "xlsm"}, []string{"*.xlsm"}},
	}
	for _, c := range cases {
		got := excelPatterns(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("%s: patterns = %v, want %v", c.name, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: patterns = %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func TestFileFilters(t *testing.T) {
	if got := fileFilters(nil); got != nil {
		t.Fatalf("fileFilters(nil) = %v, want nil（不过滤任何文件）", got)
	}
	got := fileFilters([]string{"xlsm", "xlsx"})
	if len(got) != 1 {
		t.Fatalf("fileFilters = %v, want one filter", got)
	}
	if got[0].Pattern != "*.xlsm;*.xlsx" {
		t.Errorf("pattern = %q, want %q", got[0].Pattern, "*.xlsm;*.xlsx")
	}
	if !strings.Contains(got[0].DisplayName, "*.xlsm") || !strings.Contains(got[0].DisplayName, "*.xlsx") {
		t.Errorf("display name = %q, want it to mention both patterns", got[0].DisplayName)
	}
}

// 选择文件夹时不能带文件过滤器，否则 Windows 的资源管理器对话框会把文件夹变灰。
func TestOpenDialogOptions(t *testing.T) {
	dir := openDialogOptions("选择包含 MPS 源文件的文件夹", "/tmp", false, nil)
	if dir.Title == "" || dir.DefaultDirectory != "/tmp" {
		t.Errorf("folder options = %+v", dir)
	}
	if dir.Filters != nil {
		t.Errorf("folder picker filters = %v, want none", dir.Filters)
	}

	files := openDialogOptions("选择 MPS 源文件（可多选）", "/tmp", true, []string{"xlsm", "xlsx"})
	if len(files.Filters) != 1 || files.Filters[0].Pattern != "*.xlsm;*.xlsx" {
		t.Errorf("file picker filters = %+v", files.Filters)
	}
}

func TestSaveDialogOptions(t *testing.T) {
	opts := saveDialogOptions("导出整合数据", "CLEAR_2639.xlsx", "/tmp", []string{"xlsx"})
	if opts.Title != "导出整合数据" || opts.DefaultFilename != "CLEAR_2639.xlsx" || opts.DefaultDirectory != "/tmp" {
		t.Errorf("save options = %+v", opts)
	}
	if len(opts.Filters) != 1 || opts.Filters[0].Pattern != "*.xlsx" {
		t.Errorf("save filters = %+v", opts.Filters)
	}
}

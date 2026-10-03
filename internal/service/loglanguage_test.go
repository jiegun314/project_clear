package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 日志窗口是中文界面：来源栏不能出现 import / add 这类机器名。
func TestImportLogsUseChineseSource(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import folder: %v", err)
	}
	entries := svc.log.Recent(50)
	if len(entries) == 0 {
		t.Fatal("导入没有写出任何日志")
	}
	var done bool
	for _, e := range entries {
		if e.Source == "import" || e.Source == "add" {
			t.Errorf("来源栏出现了机器名: %+v", e)
		}
		if strings.HasPrefix(e.Message, "完成：成功") {
			done = true
			if e.Source != "导入" {
				t.Errorf("完成行的来源 = %q, want 导入", e.Source)
			}
			if !strings.Contains(e.Message, "毫秒") {
				t.Errorf("耗时仍用英文单位: %q", e.Message)
			}
		}
	}
	if !done {
		t.Fatalf("没有找到导入完成行: %+v", entries)
	}
}

// 真正来自 excelize 的英文报错也要在进入面板前变成中文。
func TestBrokenWorkbookErrorIsChinese(t *testing.T) {
	svc, _ := newTestService(t)
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.xlsm")
	if err := os.WriteFile(bad, []byte("这不是一个工作簿"), 0o644); err != nil {
		t.Fatalf("write bogus workbook: %v", err)
	}
	// Every file failed to read, so the import reports an error — the point of
	// this test is the log line the user sees, not the returned value.
	if _, err := svc.ImportFolder(dir, nil); err == nil {
		t.Log("import reported no error; the log assertion below still applies")
	}
	var seen bool
	for _, e := range svc.log.Recent(50) {
		if !strings.Contains(e.Message, "broken.xlsm") {
			continue
		}
		seen = true
		if !strings.Contains(e.Message, "不是有效的 zip 压缩包") {
			t.Fatalf("英文错误未被翻译: %q", e.Message)
		}
		if strings.Contains(e.Message, "not a valid") || strings.Contains(e.Message, "zip:") {
			t.Fatalf("译文里仍残留英文: %q", e.Message)
		}
	}
	if !seen {
		t.Fatalf("没有看到 broken.xlsm 的报错日志: %+v", svc.log.Recent(50))
	}
}

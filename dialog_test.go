package main

import (
	"context"
	"testing"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// A Local import is one file, not a folder. The picker shape is named now; this
// test pins which native dialog each shape opens, so a swapped boolean cannot
// silently turn 导入本地 MPS into a folder chooser again.
func TestPickerShapes(t *testing.T) {
	oldCtx := dialogCtx
	dialogCtx = context.Background()
	defer func() { dialogCtx = oldCtx }()

	oldFile, oldMultiple, oldDir := openFileDialog, openMultipleFilesDialog, openDirectoryDialog
	defer func() {
		openFileDialog, openMultipleFilesDialog, openDirectoryDialog = oldFile, oldMultiple, oldDir
	}()

	var calls []string
	openFileDialog = func(context.Context, wr.OpenDialogOptions) (string, error) {
		calls = append(calls, "file")
		return "/in/local.xlsx", nil
	}
	openMultipleFilesDialog = func(context.Context, wr.OpenDialogOptions) ([]string, error) {
		calls = append(calls, "files")
		return []string{"/in/a.xlsm", "/in/b.xlsm"}, nil
	}
	openDirectoryDialog = func(context.Context, wr.OpenDialogOptions) (string, error) {
		calls = append(calls, "folder")
		return "/in", nil
	}

	got, err := selectOneFile("本地 MPS", "", []string{"xlsx"})
	if err != nil || len(got) != 1 || got[0] != "/in/local.xlsx" {
		t.Fatalf("selectOneFile = %v %v", got, err)
	}
	if len(calls) != 1 || calls[0] != "file" {
		t.Fatalf("selectOneFile calls = %v, want file", calls)
	}

	calls = nil
	if got, err := selectOneFolder("导入文件夹", ""); err != nil || len(got) != 1 || got[0] != "/in" {
		t.Fatalf("selectOneFolder = %v %v", got, err)
	}
	if len(calls) != 1 || calls[0] != "folder" {
		t.Fatalf("selectOneFolder calls = %v, want folder", calls)
	}

	calls = nil
	if got, err := selectManyFiles("添加文件", "", []string{"xlsm", "xlsx"}); err != nil || len(got) != 2 {
		t.Fatalf("selectManyFiles = %v %v", got, err)
	}
	if len(calls) != 1 || calls[0] != "files" {
		t.Fatalf("selectManyFiles calls = %v, want files", calls)
	}
}

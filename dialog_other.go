//go:build !darwin

package main

import (
	"context"
	"errors"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The macOS build talks to NSOpenPanel through the Objective-C shim in
// dialog_darwin.go, because Wails v2 opens its own panels from a background
// goroutine and AppKit drops those. Everywhere else the Wails dialogs work,
// so the pickers are thin wrappers around them.

// dialogCtx is the runtime context the dialogs need. startup() sets it before
// the window is shown, and only the window can call these pickers.
var dialogCtx context.Context

func setDialogContext(ctx context.Context) { dialogCtx = ctx }

func selectPaths(title, dir string, chooseFiles, multiple bool, filters []string) ([]string, error) {
	if dialogCtx == nil {
		return nil, errors.New("文件选择器尚未就绪，请稍后重试")
	}
	opts := openDialogOptions(title, dir, chooseFiles, filters)
	if !chooseFiles {
		picked, err := wr.OpenDirectoryDialog(dialogCtx, opts)
		if err != nil || picked == "" {
			return nil, err
		}
		return []string{picked}, nil
	}
	if multiple {
		return wr.OpenMultipleFilesDialog(dialogCtx, opts)
	}
	picked, err := wr.OpenFileDialog(dialogCtx, opts)
	if err != nil || picked == "" {
		return nil, err
	}
	return []string{picked}, nil
}

func selectSavePath(title, filename, dir string, filters []string) (string, error) {
	if dialogCtx == nil {
		return "", errors.New("保存面板尚未就绪，请稍后重试")
	}
	return wr.SaveFileDialog(dialogCtx, saveDialogOptions(title, filename, dir, filters))
}

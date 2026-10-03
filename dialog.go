package main

import (
	"context"
	"errors"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// File pickers are the same code on every platform: 导入文件夹 / 添加文件 / 导出
// all go through the Wails runtime dialogs, so there is no platform-specific
// file left in this project.
//
// Why this is not an Objective-C shim any more: until v1.5.2 macOS presented
// NSOpenPanel/NSSavePanel through dialog_darwin.m, because Wails v2 runs bound
// methods in a goroutine (`go func()` in the darwin frontend) and presents its
// dialogs as sheets on the window, and the panel used to flash and disappear.
// The real culprit turned out to be the application asking for key status while
// the sheet appeared (the startup WindowShow/WindowUnminimise calls removed in
// commit b829dd8). With that gone the built-in dialogs were verified stable on
// macOS — folder chooser, multi-file chooser with `*.xlsm;*.xlsx` filters and
// the save panel, each opened, cancelled and re-opened several times — so the
// shim was deleted and macOS no longer needs cgo/Objective-C of its own.

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

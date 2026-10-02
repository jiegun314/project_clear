//go:build !darwin

package main

import "errors"

// On platforms other than macOS the Wails dialogs work correctly, so the
// wrappers are thin pass-throughs and the Objective-C shim is not compiled.

func selectPaths(title, dir string, chooseFiles, multiple bool, filters []string) ([]string, error) {
	return nil, errors.New("当前平台的文件选择器尚未实现")
}

func selectSavePath(title, filename, dir string, filters []string) (string, error) {
	return "", errors.New("当前平台的保存面板尚未实现")
}

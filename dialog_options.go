package main

import (
	"strings"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The file pickers of both platforms ask for the same thing, so the option
// mapping lives here, free of build tags: the macOS build hands it to the
// NSOpenPanel shim, the other platforms to the Wails dialogs, and it stays
// unit-testable on every platform.

// excelPatterns turns "xlsm", ".xlsx" into the "*.xlsm;*.xlsx" globs the
// dialogs expect, ignoring empty entries.
func excelPatterns(exts []string) []string {
	patterns := make([]string, 0, len(exts))
	for _, e := range exts {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		patterns = append(patterns, "*"+e)
	}
	return patterns
}

// fileFilters is the filter list for a picker; nil means "every file".
func fileFilters(exts []string) []wr.FileFilter {
	patterns := excelPatterns(exts)
	if len(patterns) == 0 {
		return nil
	}
	return []wr.FileFilter{{
		DisplayName: "Excel 工作簿 (" + strings.Join(patterns, ", ") + ")",
		Pattern:     strings.Join(patterns, ";"),
	}}
}

// openDialogOptions builds the options for 导入文件夹 / 添加文件. A folder
// picker takes no filter, so the extension list is only used for files.
func openDialogOptions(title, dir string, chooseFiles bool, exts []string) wr.OpenDialogOptions {
	opts := wr.OpenDialogOptions{Title: title, DefaultDirectory: dir}
	if chooseFiles {
		opts.Filters = fileFilters(exts)
	}
	return opts
}

// saveDialogOptions builds the options for 导出.
func saveDialogOptions(title, filename, dir string, exts []string) wr.SaveDialogOptions {
	return wr.SaveDialogOptions{
		Title:            title,
		DefaultFilename:  filename,
		DefaultDirectory: dir,
		Filters:          fileFilters(exts),
	}
}

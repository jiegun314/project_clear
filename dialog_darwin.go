//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework UniformTypeIdentifiers

#include <stdlib.h>

// clearOpenPanel presents a folder/file chooser as a sheet on the app's key
// window. It must be called from the main thread, which is why it exists
// instead of Wails' own dialog: Wails v2 runs bound methods in a goroutine and
// presents NSOpenPanel straight from that thread, which AppKit does not
// tolerate (the panel appears and is immediately discarded).
// The result is delivered on the main thread through clearPanelResult.
void clearOpenPanel(const char *title, const char *dir, int allowFiles, int allowDirs,
                    int multiple, const char *filters, const char *buttonLabel);
void clearSavePanel(const char *title, const char *filename, const char *dir,
                    const char *filters, const char *buttonLabel);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"unsafe"
)

// panelResult is what the ObjC side hands back.
type panelResult struct {
	Paths    []string `json:"paths"`
	Canceled bool     `json:"canceled"`
	Error    string   `json:"error,omitempty"`
}

var (
	panelMu sync.Mutex
	panelCh chan panelResult
)

//export clearPanelResult
func clearPanelResult(cjson *C.char) {
	defer C.free(unsafe.Pointer(cjson))
	raw := C.GoString(cjson)

	var res panelResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		res = panelResult{Canceled: true, Error: "无法解析文件选择结果: " + err.Error()}
	}
	panelMu.Lock()
	ch := panelCh
	panelMu.Unlock()
	if ch != nil {
		select {
		case ch <- res:
		default:
		}
	}
}

// selectPaths shows a native chooser and blocks until the user finishes.
//
// chooseFiles selects between file and folder mode; multiple allows more than
// one selection. Cancelling returns a nil slice and no error.
func selectPaths(title, dir string, chooseFiles, multiple bool, filters []string) ([]string, error) {
	ch := make(chan panelResult, 1)
	panelMu.Lock()
	panelCh = ch
	panelMu.Unlock()
	defer func() {
		panelMu.Lock()
		panelCh = nil
		panelMu.Unlock()
	}()

	ct := C.CString(title)
	cd := C.CString(dir)
	cf := C.CString(strings.Join(filters, ";"))
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cd))
	defer C.free(unsafe.Pointer(cf))

	if chooseFiles {
		C.clearOpenPanel(ct, cd, 1, 0, boolToCInt(multiple), cf, nil)
	} else {
		C.clearOpenPanel(ct, cd, 0, 1, boolToCInt(multiple), cf, nil)
	}

	res := <-ch
	if res.Error != "" && !res.Canceled {
		return nil, errors.New(res.Error)
	}
	return res.Paths, nil
}

// selectSavePath shows a native save panel and returns the chosen path.
func selectSavePath(title, filename, dir string, filters []string) (string, error) {
	ch := make(chan panelResult, 1)
	panelMu.Lock()
	panelCh = ch
	panelMu.Unlock()
	defer func() {
		panelMu.Lock()
		panelCh = nil
		panelMu.Unlock()
	}()

	ct := C.CString(title)
	cn := C.CString(filename)
	cd := C.CString(dir)
	cf := C.CString(strings.Join(filters, ";"))
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(cd))
	defer C.free(unsafe.Pointer(cf))

	C.clearSavePanel(ct, cn, cd, cf, nil)

	res := <-ch
	if res.Error != "" && !res.Canceled {
		return "", errors.New(res.Error)
	}
	if res.Canceled || len(res.Paths) == 0 {
		return "", nil
	}
	return res.Paths[0], nil
}

func boolToCInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

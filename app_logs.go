package main

import (
	"context"

	"project_clear/internal/logging"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) streamLogs(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ch, cancel := a.log.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			wr.EventsEmit(a.ctx, "log:entry", e)
		}
	}
}

// ---------------------------------------------------------------- logs

// ReportFrontendError records a webview-side exception in the same log the
// status bar shows, so a rendering failure is diagnosable instead of blank.
func (a *App) ReportFrontendError(message, stack, source string) {
	defer a.recoverFault("ReportFrontendError", nil)
	if a.log == nil {
		return
	}
	where := source
	if where == "" {
		where = "界面"
	}
	a.log.Error(where, "前端异常: %s", message)
	if stack != "" {
		a.log.Error(where, "调用栈: %s", truncate(stack, 2000))
	}
}

// GetLogs returns the most recent log entries.
func (a *App) GetLogs(limit int) []logging.Entry {
	defer a.recoverFault("GetLogs", nil)
	_ = a.waitBoot()
	if a.log == nil {
		return nil
	}
	return a.log.Recent(limit)
}

// ClearLogs empties the in-memory log buffer.
func (a *App) ClearLogs() {
	defer a.recoverFault("ClearLogs", nil)
	if a.log != nil {
		a.log.Clear()
	}
}

// ---------------------------------------------------------------- windows
//
// Wails v2 is a single-window framework: there is no WindowCreate, and the
// runtime window calls only ever act on the calling window. The secondary
// views (history, settings, about) are therefore rendered as full-height
// overlays inside the main window rather than as separate OS windows.

// Quit shuts the application down.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + " …"
}

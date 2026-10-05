// Package main is the Wails binding layer. The exported methods on App are the
// surface the frontend calls: they convert arguments, delegate to
// internal/service and shape the result into internal/view. Keeping them that
// thin is what lets the business logic be tested without a webview, and it is
// also why Wails appears in this package and nowhere else.
//
// The files are split by what the frontend asks for, so a change to one screen
// touches one file:
//
//	app.go          the App object, startup and shutdown, error recovery
//	app_status.go   the status bar and the About window
//	app_staging.go  导入 / 添加 / 整合 / 清空
//	app_data.go     the data grid reads
//	app_export.go   导出, and revealing the result in Finder
//	app_settings.go 参数设定
//	app_logs.go     the log stream and its queries
//	app_dirs.go     remembering the folder last used in a dialog
//
// Behaviour that belongs to the application rather than to the screen that
// reaches it — the merge itself, the database, the export engines — lives in
// internal/service and below.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/service"
	"project_clear/internal/store"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is shown in the About window.
const (
	AppName    = "CLEAR"
	AppFull    = "Consolidation & Loading of Enterprise Analytics for Replenishment"
	AppVersion = "1.6.1"
)

// App is the object whose exported methods are bound to the frontend.
type App struct {
	ctx context.Context
	// runCtx belongs to the application and is cancelled on shutdown. Wails
	// never cancels the context it hands to OnStartup, so a child context we
	// own is the only way to stop the log stream goroutine.
	runCtx context.Context
	cancel context.CancelFunc

	cfg  *config.Store
	log  *logging.Logger
	db   *store.Store
	svc  *service.Service
	data string

	bootOnce sync.Once
	// shutdownOnce keeps the clean-up idempotent: Quit and OnShutdown can both
	// run, in either order.
	shutdownOnce sync.Once

	// bootDone closes when startup has finished, so a webview that loads
	// faster than the backend can wait for it instead of failing outright.
	bootDone chan struct{}
	bootErr  error
}

// NewApp builds the application object.
func NewApp() *App { return &App{bootDone: make(chan struct{})} }

// domReady fires when the webview finished loading the document.
func (a *App) domReady(ctx context.Context) {
	// Startup may have failed before the logger existed, and Wails fires
	// OnDomReady regardless: dereferencing a nil log here would panic inside
	// the message loop and take the process down with no diagnostics.
	if a.log == nil {
		return
	}
	w, h := wr.WindowGetSize(ctx)
	a.log.Info("界面", "页面加载完成，窗口尺寸 %dx%d", w, h)
}

// startup runs once when Wails brings the backend up.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.runCtx, a.cancel = context.WithCancel(ctx)
	// The non-macOS file pickers go through the Wails runtime, which needs the
	// context; on macOS this is a no-op.
	setDialogContext(ctx)
	a.bootOnce.Do(func() {
		a.bootErr = a.boot()
		close(a.bootDone)
	})
}

func (a *App) boot() error {
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	a.data = dataDir
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	// A database written by an older CLEAR may still sit in the bundle or in
	// ~/.clear; bring it over before anything opens an empty one.
	adopted := config.AdoptDataDir(dataDir)

	cfg, notes, err := config.NewStore()
	if err != nil {
		return err
	}
	a.cfg = cfg

	a.log = logging.New(filepath.Join(dataDir, "logs"))
	if adopted != "" {
		a.log.Success("启动", "已把旧位置的数据迁移到 %s（原目录 %s 保留未动）", dataDir, adopted)
	}
	for _, n := range notes {
		a.log.Info("启动", "%s", n)
	}

	db, err := store.Open(filepath.Join(dataDir, "clear.db"))
	if err != nil {
		return err
	}
	a.db = db
	a.svc = service.New(cfg, a.log, db, dataDir)

	a.log.Success("启动", "%s v%s 已就绪，数据库 %s", AppName, AppVersion, filepath.Join(dataDir, "clear.db"))

	// The window state is deliberately left alone here. Calling WindowShow or
	// WindowUnminimise after start-up can take the key-window status away at
	// exactly the moment a native panel is presented, which makes a file
	// dialog flash and close.
	go func() {
		time.Sleep(1200 * time.Millisecond)
		w, h := wr.WindowGetSize(a.ctx)
		a.log.Info("界面", "窗口尺寸 %dx%d", w, h)
	}()

	// Stream log entries to every open window so the log panel updates live.
	go a.streamLogs(a.runCtx)
	return nil
}

// shutdown stops the log stream and releases the database once any operation in
// progress has finished. Wails calls it from OnShutdown, and Quit calls it
// before asking the window to close, so it has to be safe to run twice and in
// either order. The context is the one Wails passes to OnShutdown; nothing here
// needs it.
func (a *App) shutdown(context.Context) {
	a.shutdownOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		if a.svc != nil {
			// An import or an export may still be running on its own goroutine.
			// Closing the database underneath it would fail the operation
			// halfway, so this waits for the current one to finish.
			a.svc.WaitIdle()
		}
		if a.db != nil {
			_ = a.db.Close()
		}
		// Nothing to flush: the logger writes and closes the daily file per
		// entry rather than buffering it.
	})
}

func (a *App) ready() error {
	if err := a.waitBoot(); err != nil {
		return err
	}
	if a.bootErr != nil {
		return a.bootErr
	}
	if a.svc == nil {
		return fmt.Errorf("应用尚未初始化")
	}
	return nil
}

// waitBoot blocks until startup has finished. Wails calls OnStartup and
// OnDomReady on separate goroutines, so the page can ask for data before the
// database is open; blocking briefly is what keeps the first render correct
// instead of leaving the window stuck on an empty state.
func (a *App) waitBoot() error {
	select {
	case <-a.bootDone:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("应用初始化超时，请重新启动 CLEAR")
	}
}

func (a *App) progress(stage string, done, total int) {
	wr.EventsEmit(a.ctx, "task:progress", map[string]any{
		"stage": stage, "done": done, "total": total,
	})
}

// recoverFault turns a panic inside a bound method into a returned error.
//
// Wails recovers a panicking bound method itself, but then calls the webview
// back with an empty string, which is not valid JSON. The promise never
// settles, so the window stays in its processing state with no message and the
// only way out is to kill the application. Returning an ordinary error instead
// lets the frontend report it and put its controls back.
//
// It must be deferred directly: recover only works from the deferred call
// itself.
func (a *App) recoverFault(where string, err *error) {
	r := recover()
	if r == nil {
		return
	}
	msg := fmt.Sprintf("%s 内部错误: %v", where, r)
	if a.log != nil {
		a.log.Error(where, "%s", msg)
		a.log.Error(where, "调用栈: %s", truncate(string(debug.Stack()), 2000))
	}
	if err != nil {
		*err = errors.New(msg)
	}
}

func (a *App) Quit() {
	// The quit is registered first so it still runs if the clean-up panics,
	// which would otherwise leave the window open with no way out.
	defer wr.Quit(a.ctx)
	defer a.recoverFault("Quit", nil)
	a.shutdown(a.ctx)
}

package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the artwork used by the macOS About panel. The Dock/Finder icon
// comes from build/appicon.png, which the packager turns into iconfile.icns.
//
//go:embed clear.png
var appIcon []byte

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "CLEAR - MPS 数据整合平台",
		Width:             1480,
		Height:            940,
		MinWidth:          1080,
		MinHeight:         680,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 244, G: 244, B: 245, A: 1},
		OnStartup:         app.startup,
		OnDomReady:        app.domReady,
		// Closing the window or quitting goes through here, so the database is
		// released and the log stream stops even when the user never presses
		// the 退出 button.
		OnShutdown: app.shutdown,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// 内置素材（见 easteregg.go）从二进制里取，磁盘上没有对应文件。
			Middleware: hiddenAssetsMiddleware,
		},
		Mac: &mac.Options{
			// The title bar is deliberately left as the native one. A hidden
			// inset bar makes the window stop behaving like a key window on
			// macOS, which both overlaps the traffic lights over the toolbar
			// and makes every NSOpenPanel sheet dismiss itself immediately.
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "CLEAR",
				Message: AppFull + "\n\nMPS 数据整合与补货分析平台\n版本 " + AppVersion,
				Icon:    appIcon,
			},
		},
		Bind: []any{app},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "CLEAR 启动失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "提示: 请先执行 `npm --prefix frontend install && npm --prefix frontend run build` 生成前端资源\n")
		os.Exit(1)
	}
}

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

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "CLEAR - MPS 数据整合平台",
		Width:            1480,
		Height:           940,
		MinWidth:         1080,
		MinHeight:        680,
		StartHidden:      false,
		HideWindowOnClose: false,
		BackgroundColour: &options.RGBA{R: 244, G: 244, B: 245, A: 1},
		OnStartup:        app.startup,
		AssetServer:      &assetserver.Options{Assets: assets},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "CLEAR",
				Message: AppFull + "\n\nMPS 数据整合与补货分析平台\n版本 " + AppVersion,
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

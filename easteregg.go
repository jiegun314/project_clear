package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// 这两个素材在编译期被塞进二进制，运行目录里不会多出任何文件：前端按下面的
// 私有路径从内存里取用，磁盘上没有副本，也不需要额外的读盘权限。
//
//go:embed raw_data/resource/easter_egg.png raw_data/resource/Small-dog-barking-sound-effect.mp3
var hiddenAssets embed.FS

// hiddenPrefix 是这些素材对外的路径前缀；它只由内置资源服务响应，
// frontend/dist 里并不存在同名文件，浏览器地址栏也不会暴露真实文件名以外的信息。
const hiddenPrefix = "/easteregg/"

// hiddenAssetsHandler serves the embedded assets as plain files.
func hiddenAssetsHandler() http.Handler {
	sub, err := fs.Sub(hiddenAssets, "raw_data/resource")
	if err != nil {
		// 只有 embed 布局被改动时才会走到这里；退化成一个只回 404 的处理器，
		// 不影响其它静态资源。
		return http.NotFoundHandler()
	}
	return http.StripPrefix(hiddenPrefix, http.FileServer(http.FS(sub)))
}

// hiddenAssetsMiddleware 在默认资源服务之前截获私有前缀，其余请求原样放行。
// 用中间件而不是往 frontend/dist 里塞文件，是为了让素材只存在于二进制中。
func hiddenAssetsMiddleware(next http.Handler) http.Handler {
	handler := hiddenAssetsHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, hiddenPrefix) {
			handler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

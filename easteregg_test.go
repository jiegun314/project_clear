package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 素材必须来自二进制内部：这里直接断言内嵌副本和仓库里的源文件逐字节相同，
// 换素材后忘记重新编译会在这一步暴露。
func TestHiddenAssetsMatchTheSourceFiles(t *testing.T) {
	names := []string{"easter_egg.png", "Small-dog-barking-sound-effect.mp3"}
	for _, name := range names {
		want, err := os.ReadFile(filepath.Join("raw_data", "resource", name))
		if err != nil {
			t.Fatalf("读取源文件 %s: %v", name, err)
		}
		got, err := hiddenAssets.ReadFile("raw_data/resource/" + name)
		if err != nil {
			t.Fatalf("读取内嵌素材 %s: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s 内嵌副本与源文件不一致（%d vs %d 字节）", name, len(got), len(want))
		}
		if len(got) == 0 {
			t.Fatalf("%s 是空文件", name)
		}
	}
}

func TestHiddenAssetsAreServed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := hiddenAssetsMiddleware(next)

	cases := []struct {
		path string
		typ  string
	}{
		{"/easteregg/easter_egg.png", "image/png"},
		{"/easteregg/Small-dog-barking-sound-effect.mp3", "audio/mpeg"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: 状态 %d，want 200", c.path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, c.typ) {
			t.Errorf("%s: Content-Type %q，want 前缀 %q", c.path, ct, c.typ)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s: 响应体为空", c.path)
		}
	}
}

func TestUnknownHiddenPathIsNotFound(t *testing.T) {
	handler := hiddenAssetsMiddleware(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/easteregg/nope.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态 %d，want 404", rec.Code)
	}
}

// 其它路径必须原样交给后面的资源服务，不能被这个中间件吃掉。
func TestOtherPathsFallThrough(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := hiddenAssetsMiddleware(next)

	for _, path := range []string{"/", "/assets/index.js", "/clear.png"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusTeapot {
			t.Errorf("%s: 状态 %d，want 418（应交给下一跳）", path, rec.Code)
		}
	}
}

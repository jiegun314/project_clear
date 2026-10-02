//go:build darwin

package main

import (
	"testing"
	"time"
)

// installPanelSink points the ObjC callback at a fresh channel for one test.
func installPanelSink(t *testing.T) chan panelResult {
	t.Helper()
	ch := make(chan panelResult, 1)
	panelMu.Lock()
	panelCh = ch
	panelMu.Unlock()
	t.Cleanup(func() {
		panelMu.Lock()
		panelCh = nil
		panelMu.Unlock()
	})
	return ch
}

func waitPanel(t *testing.T, ch chan panelResult) panelResult {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("面板没有回传结果")
		return panelResult{}
	}
}

// TestPanelResultHandOff is the regression test for the app quitting itself as
// soon as a file panel was dismissed. sendResult used to hand
// [out UTF8String] straight to clearPanelResult, which frees it; that pointer
// is NSString-internal storage, so free() tripped the macOS malloc guard and
// killed the process with SIGTRAP. The app's own log shows nothing, because a
// Go fatal error only reaches stderr, and stderr is invisible when the app is
// launched from Finder. A broken hand-off fails this test by taking the test
// binary down with it, which is exactly the signal we want.
func TestPanelResultHandOff(t *testing.T) {
	t.Run("取消", func(t *testing.T) {
		ch := installPanelSink(t)
		testSendResult(true, 0)

		got := waitPanel(t, ch)
		if !got.Canceled {
			t.Errorf("Canceled = false, 期望 true")
		}
		if len(got.Paths) != 0 {
			t.Errorf("Paths = %v, 期望为空", got.Paths)
		}
	})

	t.Run("确定", func(t *testing.T) {
		ch := installPanelSink(t)
		testSendResult(false, 3)

		got := waitPanel(t, ch)
		if got.Error != "" {
			t.Fatalf("回传解析失败: %s", got.Error)
		}
		if got.Canceled {
			t.Error("Canceled = true, 期望 false")
		}
		if len(got.Paths) != 3 {
			t.Fatalf("Paths 有 %d 条, 期望 3 条: %v", len(got.Paths), got.Paths)
		}
		// Non-ASCII paths must survive the UTF-8 hand-off intact.
		want := []string{"/tmp/测试 路径 0.xlsm", "/tmp/测试 路径 1.xlsm", "/tmp/测试 路径 2.xlsm"}
		for i, w := range want {
			if got.Paths[i] != w {
				t.Errorf("Paths[%d] = %q, 期望 %q", i, got.Paths[i], w)
			}
		}
	})

	t.Run("确定但无选择", func(t *testing.T) {
		ch := installPanelSink(t)
		testSendResult(false, 0)

		got := waitPanel(t, ch)
		if got.Canceled {
			t.Error("Canceled = true, 期望 false")
		}
		if len(got.Paths) != 0 {
			t.Errorf("Paths = %v, 期望为空", got.Paths)
		}
	})
}

// TestPanelResultEscaping covers the JSON hand-off for paths that need
// escaping. Those used to be encoded as ["path"] instead of "path", so every
// non-empty result failed to decode and the import silently received nothing.
func TestPanelResultEscaping(t *testing.T) {
	ch := installPanelSink(t)
	testSendTrickyPaths()

	got := waitPanel(t, ch)
	if got.Error != "" {
		t.Fatalf("回传解析失败: %s", got.Error)
	}
	want := []string{
		"/tmp/a\"b.xlsm",
		"/tmp/back\\slash.xlsm",
		"/tmp/tab\there.xlsm",
		"/tmp/换行\nhere.xlsm",
		"/tmp/plain.xlsm",
	}
	if len(got.Paths) != len(want) {
		t.Fatalf("Paths 有 %d 条, 期望 %d 条: %q", len(got.Paths), len(want), got.Paths)
	}
	for i, w := range want {
		if got.Paths[i] != w {
			t.Errorf("Paths[%d] = %q, 期望 %q", i, got.Paths[i], w)
		}
	}
}

// TestPanelResultManyTimes checks the hand-off does not accumulate corruption:
// sendResult runs on every dialog dismissal for the life of the process.
func TestPanelResultManyTimes(t *testing.T) {
	for i := 0; i < 200; i++ {
		ch := installPanelSink(t)
		testSendResult(false, 1)
		if got := waitPanel(t, ch); len(got.Paths) != 1 {
			t.Fatalf("第 %d 次: Paths = %v, 期望 1 条", i, got.Paths)
		}
		panelMu.Lock()
		panelCh = nil
		panelMu.Unlock()
	}
}

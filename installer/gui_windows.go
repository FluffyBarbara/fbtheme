//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
)

//go:embed ui/index.html
var indexHTML string

const (
	winW = 880
	winH = 560
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	dwmapi           = syscall.NewLazyDLL("dwmapi.dll")
	getDpiForSystem  = user32.NewProc("GetDpiForSystem")
	dwmSetWindowAttr = dwmapi.NewProc("DwmSetWindowAttribute")
	dwmaDarkMode     = uintptr(20)
	dwmaBorderColor  = uintptr(34)
	dwmaCaptionColor = uintptr(35)
	dwmaTextColor    = uintptr(36)
	colorCaption     = uint32(0x00050505) // COLORREF: 0x00BBGGRR
	colorBorder      = uint32(0x00262626)
	colorCaptionText = uint32(0x00E6E6E6)
)

func dpiScale() float64 {
	if getDpiForSystem.Find() != nil {
		return 1
	}
	dpi, _, _ := getDpiForSystem.Call()
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// Тёмный заголовок окна (Windows 10 20H1+, цвета — Windows 11).
func darkTitleBar(hwnd unsafe.Pointer) {
	if dwmSetWindowAttr.Find() != nil {
		return
	}
	set := func(attr uintptr, v uint32) {
		dwmSetWindowAttr.Call(uintptr(hwnd), attr, uintptr(unsafe.Pointer(&v)), 4)
	}
	set(dwmaDarkMode, 1)
	set(dwmaCaptionColor, colorCaption)
	set(dwmaBorderColor, colorBorder)
	set(dwmaTextColor, colorCaptionText)
}

type guiReporter struct {
	w       webview2.WebView
	answers chan bool
}

func (g *guiReporter) emit(fn string, args ...any) {
	raw, _ := json.Marshal(args)
	js := "window.ui && ui." + fn + ".apply(null," + string(raw) + ")"
	g.w.Dispatch(func() { g.w.Eval(js) })
}

func (g *guiReporter) Step(i int)                          { g.emit("step", i) }
func (g *guiReporter) Log(kind, msg string)                { g.emit("log", kind, msg) }
func (g *guiReporter) Progress(frac float64, label string) { g.emit("progress", frac, label) }

func (g *guiReporter) ConfirmDiscordInstall() bool {
	g.emit("ask", "discord")
	return <-g.answers
}

type scanChannel struct {
	Title string `json:"title"`
	BD    bool   `json:"bd"`
}

type scanResult struct {
	Version  string        `json:"version"`
	Channels []scanChannel `json:"channels"`
}

func runGUI() bool {
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		return false // нет WebView2 — работаем в консоли
	}

	os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF050505")
	scale := dpiScale()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(localAppData(), "TokyoMidnightBlack-Setup", "WebView2"),
		WindowOptions: webview2.WindowOptions{
			Title:  "TokyoMidnightBlack",
			Width:  uint(winW * scale),
			Height: uint(winH * scale),
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	defer w.Destroy()

	w.SetSize(int(winW*scale), int(winH*scale), webview2.HintFixed)
	darkTitleBar(w.Window())

	g := &guiReporter{w: w, answers: make(chan bool, 1)}
	var busy sync.Mutex

	run := func(job func(reporter, options) error, opt options) {
		if !busy.TryLock() {
			return
		}
		go func() {
			defer busy.Unlock()
			if err := job(g, opt); err != nil {
				g.emit("fail", err.Error())
				return
			}
			g.emit("done")
		}()
	}

	w.Bind("scan", func() scanResult {
		res := scanResult{Version: version, Channels: []scanChannel{}}
		for _, in := range findInstalls() {
			res.Channels = append(res.Channels, scanChannel{in.Title, hasBetterDiscord(in)})
		}
		return res
	})
	w.Bind("install", func(opt options) { run(doInstall, opt) })
	w.Bind("uninstall", func(opt options) { run(doUninstall, opt) })
	w.Bind("answer", func(yes bool) {
		select {
		case g.answers <- yes:
		default:
		}
	})
	w.Bind("closeApp", func() { w.Terminate() })

	w.SetHtml(indexHTML)
	w.Run()
	return true
}

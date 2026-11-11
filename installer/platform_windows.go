//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutCP    = kernel32.NewProc("SetConsoleOutputCP")
	setConsoleCP       = kernel32.NewProc("SetConsoleCP")
	getConsoleMode     = kernel32.NewProc("GetConsoleMode")
	setConsoleMode     = kernel32.NewProc("SetConsoleMode")
	setConsoleTitleW   = kernel32.NewProc("SetConsoleTitleW")
	createNoWindow     = uint32(0x08000000)
	enableVTProcessing = uint32(0x0004)
)

// UTF-8 и цвета в консоли. Возвращает false, если цвета не поддерживаются.
func setupConsole() bool {
	setConsoleOutCP.Call(65001)
	setConsoleCP.Call(65001)
	h := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if r, _, _ := getConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	r, _, _ := setConsoleMode.Call(uintptr(h), uintptr(mode|enableVTProcessing))
	return r != 0
}

func setTitle(t string) {
	p, _ := syscall.UTF16PtrFromString(t)
	setConsoleTitleW.Call(uintptr(unsafe.Pointer(p)))
}

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// Закрывает все процессы с этим именем. true — если что-то было запущено.
func killProcess(exe string) bool {
	out, _ := hidden(exec.Command("tasklist", "/FI", "IMAGENAME eq "+exe, "/NH")).Output()
	if !strings.Contains(strings.ToLower(string(out)), strings.ToLower(exe)) {
		return false
	}
	_ = hidden(exec.Command("taskkill", "/F", "/T", "/IM", exe)).Run()
	return true
}

func startDiscord(base, exe string) error {
	update := filepath.Join(base, "Update.exe")
	if _, err := os.Stat(update); err == nil {
		return hidden(exec.Command(update, "--processStart", exe)).Start()
	}
	return startDetached(filepath.Join(base, exe))
}

func startDetached(path string) error {
	return exec.Command(path).Start()
}

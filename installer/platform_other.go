//go:build !windows

// Заглушки, чтобы проверять логику установщика не на Windows.
package main

import "fmt"

func attachConsole()          {}
func setupConsole() bool      { return true }
func setTitle(string)         {}
func killProcess(string) bool { return false }

func startDiscord(base, exe string) error {
	fmt.Println("      (тест) запуск", exe, "из", base)
	return nil
}

func startDetached(path string) error {
	return fmt.Errorf("запуск %s поддерживается только в Windows", path)
}

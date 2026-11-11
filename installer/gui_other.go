//go:build !windows

package main

// Окно есть только в Windows.
func runGUI() bool { return false }

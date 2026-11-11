package main

// Запасной консольный режим — если в Windows нет WebView2.

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

var (
	cReset = "\x1b[0m"
	cBold  = "\x1b[1m"
	cDim   = "\x1b[90m"
	cWhite = "\x1b[97m"
	cGreen = "\x1b[92m"
	cRed   = "\x1b[91m"
	cYel   = "\x1b[93m"
)

var stdin *bufio.Reader

func readLine() string {
	if stdin == nil {
		stdin = bufio.NewReader(os.Stdin)
	}
	s, _ := stdin.ReadString('\n')
	return strings.TrimSpace(s)
}

type consoleReporter struct {
	steps []string
}

func (c *consoleReporter) Step(i int) {
	fmt.Printf("\n%s[%d/%d]%s %s%s%s\n", cDim, i+1, len(c.steps), cReset, cBold, c.steps[i], cReset)
}

func (c *consoleReporter) Log(kind, msg string) {
	mark := map[string]string{"ok": cGreen + "✓", "warn": cYel + "!", "info": cDim + "·"}[kind]
	fmt.Printf("\r\x1b[2K      %s%s %s\n", mark, cReset, msg)
}

func (c *consoleReporter) Progress(frac float64, label string) {
	if frac < 0 {
		fmt.Print("\r\x1b[2K")
		return
	}
	pct := int(frac * 100)
	bar := strings.Repeat("█", pct/5) + strings.Repeat("░", 20-pct/5)
	fmt.Printf("\r      %s%s%s %3d%%  %s", cWhite, bar, cReset, pct, label)
}

func (c *consoleReporter) ConfirmDiscordInstall() bool {
	fmt.Printf("\n      Нажми %sEnter%s, чтобы скачать и установить Discord, или закрой окно для отмены. ", cBold, cReset)
	readLine()
	return true
}

func pause() {
	fmt.Printf("\n%sНажми Enter, чтобы закрыть окно…%s", cDim, cReset)
	readLine()
}

func consoleFail(err error) {
	fmt.Printf("\n   %s✗ Ошибка:%s %v\n", cRed+cBold, cReset, err)
	fmt.Printf("   %sЕсли не получается — сделай скриншот этого окна и отправь автору.%s\n", cDim, cReset)
	pause()
	os.Exit(1)
}

func runConsole(startErr error) {
	attachConsole()
	if !setupConsole() {
		cReset, cBold, cDim, cWhite, cGreen, cRed, cYel = "", "", "", "", "", "", ""
	}
	setTitle("TokyoMidnightBlack — установщик")

	fmt.Println()
	fmt.Println(cWhite + cBold + "   ████████  ███    ███  ██████ " + cReset)
	fmt.Println(cWhite + cBold + "      ██     ████  ████  ██   ██" + cReset + "    TokyoMidnightBlack")
	fmt.Println(cWhite + cBold + "      ██     ██ ████ ██  ██████ " + cReset + cDim + "    установщик v" + version + cReset)
	fmt.Println(cWhite + cBold + "      ██     ██  ██  ██  ██   ██" + cReset)
	fmt.Println(cWhite + cBold + "      ██     ██      ██  ██████ " + cReset + cDim + "    © 2026 FluffyBarbara" + cReset)
	fmt.Println()

	if startErr != nil {
		consoleFail(startErr)
	}

	fmt.Printf("   %s1%s  Установить или обновить %s(Enter)%s\n", cBold, cReset, cDim, cReset)
	fmt.Printf("   %s2%s  Удалить BetterDiscord\n", cBold, cReset)
	fmt.Printf("\n   Выбор: ")

	opt := options{Updater: true, Launch: true}
	if readLine() == "2" {
		if err := doUninstall(&consoleReporter{steps: uninstallSteps}, opt); err != nil {
			consoleFail(err)
		}
		fmt.Printf("\n   %s✓ BetterDiscord отключён.%s\n", cGreen+cBold, cReset)
	} else {
		if err := doInstall(&consoleReporter{steps: installSteps}, opt); err != nil {
			consoleFail(err)
		}
		fmt.Printf("\n   %s✓ Готово!%s Тема включится сама, обновления будут приходить автоматически.\n", cGreen+cBold, cReset)
		fmt.Printf("   %sЕсли Discord обновится и BetterDiscord пропадёт — просто запусти установщик ещё раз.%s\n", cDim, cReset)
	}
	pause()
}

// TokyoMidnightBlack Setup — ставит BetterDiscord, тему и плагин автообновления.
//
// © 2026 FluffyBarbara. Все права защищены.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	version   = "1.0.0"
	userAgent = "TokyoMidnightBlack-Setup/" + version

	repoRaw     = "https://raw.githubusercontent.com/FluffyBarbara/fbtheme/main/"
	themeFile   = "TokyoMidnightBlack.theme.css"
	themeName   = "TokyoMidnightBlack"
	pluginFile  = "FBThemeUpdater.plugin.js"
	pluginName  = "FBThemeUpdater"
	discordDL   = "https://discord.com/api/downloads/distributions/app/installers/latest?channel=stable&platform=win&arch=x64"
	bdOfficial  = "https://betterdiscord.app/Download/betterdiscord.asar"
	bdGitHub    = "https://github.com/BetterDiscord/BetterDiscord/releases/latest/download/betterdiscord.asar"
	originalJS  = `module.exports = require("./core.asar");`
	waitInstall = 10 * time.Minute
)

// Варианты Discord.
type channel struct {
	Title string // для вывода
	Dir   string // папка в %LOCALAPPDATA%
	Exe   string // имя процесса
	Data  string // папка настроек BetterDiscord
}

var channels = []channel{
	{"Discord", "Discord", "Discord.exe", "stable"},
	{"Discord PTB", "DiscordPTB", "DiscordPTB.exe", "ptb"},
	{"Discord Canary", "DiscordCanary", "DiscordCanary.exe", "canary"},
}

type install struct {
	channel
	Base string // папка установки Discord
	Core string // папка discord_desktop_core
}

// ─── вывод ────────────────────────────────────────────────────────────────

var (
	cReset = "\x1b[0m"
	cBold  = "\x1b[1m"
	cDim   = "\x1b[90m"
	cWhite = "\x1b[97m"
	cGreen = "\x1b[92m"
	cRed   = "\x1b[91m"
	cYel   = "\x1b[93m"
)

func noColors() { cReset, cBold, cDim, cWhite, cGreen, cRed, cYel = "", "", "", "", "", "", "" }

func banner() {
	fmt.Println()
	fmt.Println(cWhite + cBold + "   ████████  ███    ███  ██████ " + cReset)
	fmt.Println(cWhite + cBold + "      ██     ████  ████  ██   ██" + cReset + "    TokyoMidnightBlack")
	fmt.Println(cWhite + cBold + "      ██     ██ ████ ██  ██████ " + cReset + cDim + "    установщик v" + version + cReset)
	fmt.Println(cWhite + cBold + "      ██     ██  ██  ██  ██   ██" + cReset)
	fmt.Println(cWhite + cBold + "      ██     ██      ██  ██████ " + cReset + cDim + "    © 2026 FluffyBarbara" + cReset)
	fmt.Println()
}

func step(n, total int, text string) {
	fmt.Printf("\n%s[%d/%d]%s %s%s%s\n", cDim, n, total, cReset, cBold, text, cReset)
}
func ok(format string, a ...any) {
	fmt.Printf("      %s✓%s %s\n", cGreen, cReset, fmt.Sprintf(format, a...))
}
func info(format string, a ...any) {
	fmt.Printf("      %s·%s %s\n", cDim, cReset, fmt.Sprintf(format, a...))
}
func warn(format string, a ...any) {
	fmt.Printf("      %s!%s %s\n", cYel, cReset, fmt.Sprintf(format, a...))
}

var stdin = bufio.NewReader(os.Stdin)

func readLine() string {
	s, _ := stdin.ReadString('\n')
	return strings.TrimSpace(s)
}

func pause() {
	fmt.Printf("\n%sНажми Enter, чтобы закрыть окно…%s", cDim, cReset)
	readLine()
}

func fail(err error) {
	fmt.Printf("\n   %s✗ Ошибка:%s %v\n", cRed+cBold, cReset, err)
	fmt.Printf("   %sЕсли не получается — сделай скриншот этого окна и отправь автору.%s\n", cDim, cReset)
	pause()
	os.Exit(1)
}

// ─── пути ─────────────────────────────────────────────────────────────────

func appData() string      { return os.Getenv("APPDATA") }
func localAppData() string { return os.Getenv("LOCALAPPDATA") }
func bdFolder() string     { return filepath.Join(appData(), "BetterDiscord") }
func bdAsar() string       { return filepath.Join(bdFolder(), "data", "betterdiscord.asar") }

// Сравнение версий вида "app-1.0.9187" и "discord_desktop_core-2".
func versionKey(name string) []int {
	name = name[strings.LastIndexAny(name, "-")+1:]
	var out []int
	for _, p := range strings.Split(name, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = -1
		}
		out = append(out, n)
	}
	return out
}

func newest(dir, prefix string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := versionKey(names[i]), versionKey(names[j])
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return len(a) > len(b)
	})
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func findCore(base string) string {
	app := newest(base, "app-")
	if app == "" {
		return ""
	}
	modules := filepath.Join(base, app, "modules")
	wrap := newest(modules, "discord_desktop_core")
	if wrap == "" {
		return ""
	}
	core := filepath.Join(modules, wrap, "discord_desktop_core")
	if _, err := os.Stat(filepath.Join(core, "core.asar")); err != nil {
		return ""
	}
	return core
}

func findInstalls() []install {
	var found []install
	for _, ch := range channels {
		bases := []string{filepath.Join(localAppData(), ch.Dir)}
		if pd := os.Getenv("PROGRAMDATA"); pd != "" {
			bases = append(bases, filepath.Join(pd, os.Getenv("USERNAME"), ch.Dir))
		}
		for _, base := range bases {
			if core := findCore(base); core != "" {
				found = append(found, install{ch, base, core})
				break
			}
		}
	}
	return found
}

// ─── сеть ─────────────────────────────────────────────────────────────────

var client = &http.Client{Timeout: 5 * time.Minute}

func download(url string, showProgress bool) ([]byte, http.Header, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, nil, fmt.Errorf("сервер ответил %s", resp.Status)
	}

	var r io.Reader = resp.Body
	if showProgress && resp.ContentLength > 0 {
		r = &progressReader{r: resp.Body, total: resp.ContentLength}
	}
	data, err := io.ReadAll(r)
	if showProgress {
		fmt.Print("\r\x1b[2K")
	}
	return data, resp.Header, err
}

type progressReader struct {
	r           io.Reader
	total, done int64
	last        int
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	pct := int(p.done * 100 / p.total)
	if pct != p.last {
		p.last = pct
		bar := strings.Repeat("█", pct/5) + strings.Repeat("░", 20-pct/5)
		fmt.Printf("\r      %s%s%s %3d%%  %.1f / %.1f МБ", cWhite, bar, cReset, pct,
			float64(p.done)/1048576, float64(p.total)/1048576)
	}
	return n, err
}

// ─── шаги установки ───────────────────────────────────────────────────────

func installDiscord() []install {
	warn("Discord не найден на этом компьютере.")
	fmt.Printf("\n      Нажми %sEnter%s, чтобы скачать и установить Discord, или закрой окно для отмены. ", cBold, cReset)
	readLine()

	info("Скачиваю установщик Discord…")
	data, _, err := download(discordDL, true)
	if err != nil {
		fail(fmt.Errorf("не удалось скачать Discord: %w", err))
	}
	setup := filepath.Join(os.TempDir(), "DiscordSetup.exe")
	if err := os.WriteFile(setup, data, 0o755); err != nil {
		fail(err)
	}
	if err := startDetached(setup); err != nil {
		fail(fmt.Errorf("не удалось запустить установщик Discord: %w", err))
	}
	ok("Установщик Discord запущен")
	info("Жду, пока Discord установится и запустится (до %d минут)…", int(waitInstall.Minutes()))

	deadline := time.Now().Add(waitInstall)
	for time.Now().Before(deadline) {
		if found := findInstalls(); len(found) > 0 {
			time.Sleep(8 * time.Second) // даём Discord дописать файлы
			ok("Discord установлен")
			return findInstalls()
		}
		time.Sleep(3 * time.Second)
	}
	fail(errors.New("Discord так и не установился. Установи его вручную и запусти установщик ещё раз"))
	return nil
}

func installBetterDiscord() {
	for _, dir := range []string{"data", "plugins", "themes"} {
		if err := os.MkdirAll(filepath.Join(bdFolder(), dir), 0o755); err != nil {
			fail(err)
		}
	}

	data, hdr, err := download(bdOfficial, true)
	source := "betterdiscord.app"
	if err != nil || len(data) < 100_000 {
		warn("Официальный сайт не ответил, качаю с GitHub…")
		data, hdr, err = download(bdGitHub, true)
		source = "GitHub"
	}
	if err != nil {
		fail(fmt.Errorf("не удалось скачать BetterDiscord: %w", err))
	}
	if len(data) < 100_000 {
		fail(errors.New("скачанный файл BetterDiscord повреждён"))
	}
	if err := os.WriteFile(bdAsar(), data, 0o644); err != nil {
		fail(fmt.Errorf("не удалось сохранить BetterDiscord: %w", err))
	}
	ver := hdr.Get("X-Bd-Version")
	if ver != "" {
		ver = " " + ver
	}
	ok("BetterDiscord%s скачан с %s", ver, source)
}

func inject(list []install) {
	asar := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(bdAsar())
	shim := fmt.Sprintf("require(\"%s\");\n%s", asar, originalJS)
	for _, in := range list {
		if err := os.WriteFile(filepath.Join(in.Core, "index.js"), []byte(shim), 0o644); err != nil {
			fail(fmt.Errorf("не удалось подключить BetterDiscord к %s: %w", in.Title, err))
		}
		ok("Подключён к %s", in.Title)
	}
}

func installAddons(list []install) {
	files := []struct{ file, dir, done string }{
		{themeFile, "themes", "Тема TokyoMidnightBlack установлена"},
		{pluginFile, "plugins", "Плагин автообновления установлен"},
	}
	for _, f := range files {
		data, _, err := download(repoRaw+f.file, false)
		if err != nil {
			fail(fmt.Errorf("не удалось скачать %s: %w", f.file, err))
		}
		if err := os.WriteFile(filepath.Join(bdFolder(), f.dir, f.file), data, 0o644); err != nil {
			fail(err)
		}
		ok("%s", f.done)
	}

	for _, in := range list {
		dir := filepath.Join(bdFolder(), "data", in.Data)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		enable(filepath.Join(dir, "themes.json"), themeName)
		enable(filepath.Join(dir, "plugins.json"), pluginName)
	}
	ok("Тема и плагин включены")
}

// Отмечает аддон включённым, сохраняя остальные настройки BetterDiscord.
func enable(path, name string) {
	state := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	state[name] = true
	out, _ := json.MarshalIndent(state, "", "    ")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fail(err)
	}
}

func closeDiscord(list []install) {
	for _, in := range list {
		if killProcess(in.Exe) {
			ok("%s закрыт", in.Title)
		} else {
			info("%s не был запущен", in.Title)
		}
	}
	time.Sleep(1500 * time.Millisecond)
}

func launch(list []install) {
	for _, in := range list {
		if err := startDiscord(in.Base, in.Exe); err != nil {
			warn("Не удалось запустить %s — открой его сам", in.Title)
			continue
		}
		ok("%s запускается", in.Title)
	}
}

// ─── сценарии ─────────────────────────────────────────────────────────────

func runInstall() {
	const total = 6

	step(1, total, "Ищу Discord")
	list := findInstalls()
	if len(list) == 0 {
		list = installDiscord()
	}
	for _, in := range list {
		ok("Найден %s", in.Title)
	}

	step(2, total, "Закрываю Discord")
	closeDiscord(list)

	step(3, total, "Скачиваю свежий BetterDiscord")
	installBetterDiscord()

	step(4, total, "Подключаю BetterDiscord")
	inject(list)

	step(5, total, "Ставлю тему и плагин автообновления")
	installAddons(list)

	step(6, total, "Запускаю Discord")
	launch(list)

	fmt.Printf("\n   %s✓ Готово!%s Тема включится сама, обновления будут приходить автоматически.\n", cGreen+cBold, cReset)
	fmt.Printf("   %sЕсли Discord обновится и BetterDiscord пропадёт — просто запусти установщик ещё раз.%s\n", cDim, cReset)
	pause()
}

func runUninstall() {
	const total = 3

	step(1, total, "Ищу Discord")
	list := findInstalls()
	if len(list) == 0 {
		fail(errors.New("Discord не найден"))
	}
	for _, in := range list {
		ok("Найден %s", in.Title)
	}

	step(2, total, "Отключаю BetterDiscord")
	closeDiscord(list)
	for _, in := range list {
		if err := os.WriteFile(filepath.Join(in.Core, "index.js"), []byte(originalJS), 0o644); err != nil {
			fail(err)
		}
		ok("Отключён от %s", in.Title)
	}
	info("Темы и плагины остались в %s", bdFolder())

	step(3, total, "Запускаю Discord")
	launch(list)

	fmt.Printf("\n   %s✓ BetterDiscord отключён.%s\n", cGreen+cBold, cReset)
	pause()
}

func main() {
	if !setupConsole() {
		noColors()
	}
	setTitle("TokyoMidnightBlack — установщик")
	banner()

	if appData() == "" || localAppData() == "" {
		fail(errors.New("не найдены папки пользователя Windows (APPDATA / LOCALAPPDATA)"))
	}

	fmt.Printf("   %s1%s  Установить или обновить %s(Enter)%s\n", cBold, cReset, cDim, cReset)
	fmt.Printf("   %s2%s  Удалить BetterDiscord\n", cBold, cReset)
	fmt.Printf("\n   Выбор: ")

	switch readLine() {
	case "2":
		runUninstall()
	default:
		runInstall()
	}
}

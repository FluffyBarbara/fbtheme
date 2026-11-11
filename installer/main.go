// TokyoMidnightBlack Setup — ставит BetterDiscord, тему и плагин автообновления.
//
// © 2026 FluffyBarbara. Все права защищены.
package main

import (
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
	version   = "1.1.0"
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

// reporter — то, через что установка рассказывает о ходе работы (окно или консоль).
type reporter interface {
	Step(i int)                          // начат шаг i (с нуля)
	Log(kind, msg string)                // kind: ok, info, warn
	Progress(frac float64, label string) // загрузка; frac < 0 — скрыть
	ConfirmDiscordInstall() bool         // Discord не найден — ставить?
}

type options struct {
	Updater bool `json:"updater"` // ставить плагин автообновления
	Launch  bool `json:"launch"`  // запустить Discord в конце
}

var (
	installSteps   = []string{"Ищу Discord", "Закрываю Discord", "Скачиваю свежий BetterDiscord", "Подключаю BetterDiscord", "Ставлю тему и плагин", "Запускаю Discord"}
	uninstallSteps = []string{"Ищу Discord", "Отключаю BetterDiscord", "Запускаю Discord"}
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

// ─── пути ─────────────────────────────────────────────────────────────────

func appData() string      { return os.Getenv("APPDATA") }
func localAppData() string { return os.Getenv("LOCALAPPDATA") }
func bdFolder() string     { return filepath.Join(appData(), "BetterDiscord") }
func bdAsar() string       { return filepath.Join(bdFolder(), "data", "betterdiscord.asar") }

// Сравнение версий вида "app-1.0.9187" и "discord_desktop_core-2".
func versionKey(name string) []int {
	name = name[strings.LastIndex(name, "-")+1:]
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

func hasBetterDiscord(in install) bool {
	raw, err := os.ReadFile(filepath.Join(in.Core, "index.js"))
	return err == nil && strings.Contains(string(raw), "betterdiscord.asar")
}

func titles(list []install) string {
	var t []string
	for _, in := range list {
		t = append(t, in.Title)
	}
	return strings.Join(t, ", ")
}

// ─── сеть ─────────────────────────────────────────────────────────────────

var client = &http.Client{Timeout: 5 * time.Minute}

func download(url string, r reporter) ([]byte, http.Header, error) {
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

	var body io.Reader = resp.Body
	if r != nil && resp.ContentLength > 0 {
		body = &progressReader{r: resp.Body, total: resp.ContentLength, rep: r, last: -1}
	}
	data, err := io.ReadAll(body)
	if r != nil {
		r.Progress(-1, "")
	}
	return data, resp.Header, err
}

type progressReader struct {
	r           io.Reader
	rep         reporter
	total, done int64
	last        int
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if pct := int(p.done * 100 / p.total); pct != p.last {
		p.last = pct
		p.rep.Progress(float64(p.done)/float64(p.total),
			fmt.Sprintf("%.1f из %.1f МБ", float64(p.done)/1048576, float64(p.total)/1048576))
	}
	return n, err
}

// ─── шаги ─────────────────────────────────────────────────────────────────

func installDiscord(r reporter) ([]install, error) {
	r.Log("warn", "Discord не найден")
	if !r.ConfirmDiscordInstall() {
		return nil, errors.New("установка отменена: Discord не найден")
	}

	r.Log("info", "Скачиваю Discord…")
	data, _, err := download(discordDL, r)
	if err != nil {
		return nil, fmt.Errorf("не удалось скачать Discord: %w", err)
	}
	setup := filepath.Join(os.TempDir(), "DiscordSetup.exe")
	if err := os.WriteFile(setup, data, 0o755); err != nil {
		return nil, err
	}
	if err := startDetached(setup); err != nil {
		return nil, fmt.Errorf("не удалось запустить установщик Discord: %w", err)
	}
	r.Log("info", "Жду, пока Discord установится…")

	deadline := time.Now().Add(waitInstall)
	for time.Now().Before(deadline) {
		if len(findInstalls()) > 0 {
			time.Sleep(8 * time.Second) // даём Discord дописать файлы
			r.Log("ok", "Discord установлен")
			return findInstalls(), nil
		}
		time.Sleep(3 * time.Second)
	}
	return nil, errors.New("Discord так и не установился. Установи его вручную и запусти установщик ещё раз")
}

func closeDiscord(r reporter, list []install) {
	var closed []string
	for _, in := range list {
		if killProcess(in.Exe) {
			closed = append(closed, in.Title)
		}
	}
	if len(closed) > 0 {
		r.Log("ok", "Закрыт: "+strings.Join(closed, ", "))
		time.Sleep(1500 * time.Millisecond)
	} else {
		r.Log("info", "Discord не был запущен")
	}
}

func installBetterDiscord(r reporter) error {
	for _, dir := range []string{"data", "plugins", "themes"} {
		if err := os.MkdirAll(filepath.Join(bdFolder(), dir), 0o755); err != nil {
			return err
		}
	}

	data, hdr, err := download(bdOfficial, r)
	source := "betterdiscord.app"
	if err != nil || len(data) < 100_000 {
		r.Log("info", "Официальный сайт не ответил, качаю с GitHub…")
		data, hdr, err = download(bdGitHub, r)
		source = "GitHub"
	}
	if err != nil {
		return fmt.Errorf("не удалось скачать BetterDiscord: %w", err)
	}
	if len(data) < 100_000 {
		return errors.New("скачанный файл BetterDiscord повреждён")
	}
	if err := os.WriteFile(bdAsar(), data, 0o644); err != nil {
		return fmt.Errorf("не удалось сохранить BetterDiscord: %w", err)
	}
	ver := hdr.Get("X-Bd-Version")
	if ver != "" {
		ver = " " + ver
	}
	r.Log("ok", fmt.Sprintf("BetterDiscord%s с %s", ver, source))
	return nil
}

func inject(r reporter, list []install) error {
	asar := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(bdAsar())
	shim := fmt.Sprintf("require(\"%s\");\n%s", asar, originalJS)
	for _, in := range list {
		if err := os.WriteFile(filepath.Join(in.Core, "index.js"), []byte(shim), 0o644); err != nil {
			return fmt.Errorf("не удалось подключить BetterDiscord к %s: %w", in.Title, err)
		}
	}
	r.Log("ok", "Подключён к "+titles(list))
	return nil
}

func installAddons(r reporter, list []install, opt options) error {
	type addon struct{ file, dir, name, done string }
	addons := []addon{{themeFile, "themes", themeName, "тема"}}
	if opt.Updater {
		addons = append(addons, addon{pluginFile, "plugins", pluginName, "автообновление"})
	}

	var done []string
	for _, a := range addons {
		data, _, err := download(repoRaw+a.file, nil)
		if err != nil {
			return fmt.Errorf("не удалось скачать %s: %w", a.file, err)
		}
		if err := os.WriteFile(filepath.Join(bdFolder(), a.dir, a.file), data, 0o644); err != nil {
			return err
		}
		for _, in := range list {
			dir := filepath.Join(bdFolder(), "data", in.Data)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := enable(filepath.Join(dir, a.dir+".json"), a.name); err != nil {
				return err
			}
		}
		done = append(done, a.done)
	}
	r.Log("ok", "Включены: "+strings.Join(done, " и "))
	return nil
}

// Отмечает аддон включённым, сохраняя остальные настройки BetterDiscord.
func enable(path, name string) error {
	state := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &state)
	}
	state[name] = true
	out, _ := json.MarshalIndent(state, "", "    ")
	return os.WriteFile(path, out, 0o644)
}

func launch(r reporter, list []install) {
	var started []string
	for _, in := range list {
		if err := startDiscord(in.Base, in.Exe); err != nil {
			r.Log("warn", "Не удалось запустить "+in.Title+" — открой его сам")
			continue
		}
		started = append(started, in.Title)
	}
	if len(started) > 0 {
		r.Log("ok", "Запускается: "+strings.Join(started, ", "))
	}
}

// ─── сценарии ─────────────────────────────────────────────────────────────

func doInstall(r reporter, opt options) error {
	r.Step(0)
	list := findInstalls()
	if len(list) == 0 {
		var err error
		if list, err = installDiscord(r); err != nil {
			return err
		}
	}
	r.Log("ok", "Найден: "+titles(list))

	r.Step(1)
	closeDiscord(r, list)

	r.Step(2)
	if err := installBetterDiscord(r); err != nil {
		return err
	}

	r.Step(3)
	if err := inject(r, list); err != nil {
		return err
	}

	r.Step(4)
	if err := installAddons(r, list, opt); err != nil {
		return err
	}

	r.Step(5)
	if opt.Launch {
		launch(r, list)
	} else {
		r.Log("info", "Запуск пропущен")
	}
	return nil
}

func doUninstall(r reporter, opt options) error {
	r.Step(0)
	list := findInstalls()
	if len(list) == 0 {
		return errors.New("Discord не найден")
	}
	r.Log("ok", "Найден: "+titles(list))

	r.Step(1)
	closeDiscord(r, list)
	for _, in := range list {
		if err := os.WriteFile(filepath.Join(in.Core, "index.js"), []byte(originalJS), 0o644); err != nil {
			return err
		}
	}
	r.Log("ok", "Отключён от "+titles(list))

	r.Step(2)
	if opt.Launch {
		launch(r, list)
	}
	return nil
}

func main() {
	if appData() == "" || localAppData() == "" {
		runConsole(errors.New("не найдены папки пользователя Windows (APPDATA / LOCALAPPDATA)"))
		return
	}
	if runGUI() {
		return
	}
	runConsole(nil)
}

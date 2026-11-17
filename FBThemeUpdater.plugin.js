/**
 * @name FBThemeUpdater
 * @author FluffyBarbara
 * @authorLink https://github.com/FluffyBarbara
 * @version 1.3.0
 * @description 🔄 Сам скачивает свежие версии TokyoMidnightBlack и следит, чтобы в Discord была включена чёрная тема.
 * @source https://github.com/FluffyBarbara/fbtheme
 * @website https://github.com/FluffyBarbara/fbtheme
 */

/*
 * © 2026 FluffyBarbara. Все права защищены.
 * Копирование, изменение и распространение кода без письменного разрешения
 * автора запрещено. Подробно: https://github.com/FluffyBarbara/fbtheme/blob/main/LICENSE
 */

"use strict";

const fs = require("fs");
const path = require("path");

const REPO = "FluffyBarbara/fbtheme";
const BRANCH = "main";
const CHECK_EVERY = 5 * 60 * 1000; // 5 минут

// Стандартная чёрная тема Discord («Оникс»). С цветными и светлой темами
// Discord красит интерфейс поверх TokyoMidnightBlack, и она выглядит неправильно.
const BLACK_THEME = "midnight";
const SWITCH_COOLDOWN = 15 * 1000;

// Что обновлять. always: true — скачать, даже если файла ещё нет.
// Плагин обновляет и сам себя, чтобы новые возможности приходили всем.
const FILES = [
  { file: "TokyoMidnightBlack.theme.css", name: "TokyoMidnightBlack", always: true, folder: () => BdApi.Themes.folder, enable: (n) => BdApi.Themes.enable(n) },
  { file: "FBThemeUpdater.plugin.js", name: "FBThemeUpdater", always: false, folder: () => BdApi.Plugins.folder },
];

module.exports = class FBThemeUpdater {
  constructor(meta) {
    this.meta = meta;
    this.timer = null;
    this.busy = false;
    this.observer = null;
    this.guardTimer = null;
    this.lastSwitch = 0;
    this.noticeShown = false;
  }

  start() {
    this.check();
    this.timer = setInterval(() => this.check(), CHECK_EVERY);

    // следим за сменой темы Discord (меняются классы и стиль у <html>)
    this.observer = new MutationObserver(() => this.scheduleGuard(800));
    this.observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "style"] });
    this.scheduleGuard(2500); // даём Discord догрузиться
  }

  stop() {
    clearInterval(this.timer);
    clearTimeout(this.guardTimer);
    this.observer?.disconnect();
    this.timer = this.observer = this.guardTimer = null;
  }

  // ─── чёрная тема Discord ──────────────────────────────────────────────

  get autoBlack() {
    return BdApi.Data.load(this.meta.name, "autoBlack") !== false;
  }

  set autoBlack(on) {
    BdApi.Data.save(this.meta.name, "autoBlack", on);
    if (on) this.scheduleGuard(0);
  }

  scheduleGuard(delay) {
    clearTimeout(this.guardTimer);
    this.guardTimer = setTimeout(() => this.guardTheme(), delay);
  }

  themeState() {
    const html = document.documentElement;
    const classes = [...html.classList];
    let gradient = false;
    try {
      gradient = !!BdApi.Webpack.getStore?.("ClientThemesBackgroundStore")?.gradientPreset;
    } catch {}
    const custom = gradient
      || classes.some((c) => c.startsWith("custom-theme"))
      || html.style.getPropertyValue("--custom-theme-background") !== "";
    const light = classes.includes("theme-light");
    return { custom, light, bad: custom || light };
  }

  findThemeActions() {
    const W = BdApi.Webpack;
    try {
      return W.getByKeys?.("saveClientTheme")
        ?? W.getModule?.((m) => typeof m?.saveClientTheme === "function", { searchExports: true })
        ?? null;
    } catch {
      return null;
    }
  }

  guardTheme() {
    if (!this.autoBlack) return;
    const state = this.themeState();
    if (!state.bad) return;
    if (Date.now() - this.lastSwitch < SWITCH_COOLDOWN) return;
    this.lastSwitch = Date.now();

    const actions = this.findThemeActions();
    if (actions) {
      try {
        // без backgroundGradientPresetId Discord убирает цветную тему
        actions.saveClientTheme({ theme: BLACK_THEME });
        BdApi.UI.showToast("🖤 Включила чёрную тему Discord — так TokyoMidnightBlack выглядит правильно", { type: "info", timeout: 6000 });
        return;
      } catch (err) {
        console.error(`[${this.meta.name}] не получилось сменить тему`, err);
      }
    }

    if (!this.noticeShown) {
      this.noticeShown = true;
      BdApi.UI.showNotice(
        "TokyoMidnightBlack: включи в Discord стандартную чёрную тему — Настройки → Внешний вид → Тема → «Оникс». С цветными и светлой темами Discord она выглядит неправильно.",
        { type: "warning" },
      );
    }
  }

  async get(url) {
    const request = BdApi.Net?.fetch ?? window.fetch.bind(window);
    const res = await request(url, {
      headers: { "Accept": "application/vnd.github+json", "User-Agent": "FBThemeUpdater" },
    });
    if (!res.ok) throw new Error(`${res.status} ${url}`);
    return res.text();
  }

  async check(manual = false) {
    if (this.busy) return;
    this.busy = true;

    try {
      const commit = JSON.parse(await this.get(`https://api.github.com/repos/${REPO}/commits/${BRANCH}`));
      const sha = commit.sha;
      const updated = [];

      for (const item of FILES) {
        const target = path.join(item.folder(), item.file);
        const exists = fs.existsSync(target);

        if (!exists && !item.always) continue;
        if (exists && BdApi.Data.load(this.meta.name, item.file) === sha) continue;

        // ссылка на конкретный коммит не кешируется — всегда свежая версия
        const text = await this.get(`https://raw.githubusercontent.com/${REPO}/${sha}/${item.file}`);
        const local = exists ? fs.readFileSync(target, "utf8") : null;

        // запоминаем до записи: после записи плагина BetterDiscord сразу его перезапустит
        BdApi.Data.save(this.meta.name, item.file, sha);

        if (local !== text) {
          updated.push(item.name);
          fs.writeFileSync(target, text, "utf8");
          if (!exists && item.enable) setTimeout(() => item.enable(item.name), 1500);
        }
      }

      if (updated.length) {
        BdApi.UI.showToast(`🔄 Обновлено: ${updated.join(", ")}`, { type: "success" });
      } else if (manual) {
        BdApi.UI.showToast("Темы уже последней версии", { type: "info" });
      }
    } catch (err) {
      console.error(`[${this.meta.name}]`, err);
      if (manual) BdApi.UI.showToast("Не получилось проверить обновления", { type: "error" });
    } finally {
      this.busy = false;
    }
  }

  getSettingsPanel() {
    const panel = document.createElement("div");
    panel.style.cssText = "display:flex;flex-direction:column;gap:12px;color:var(--text-normal);padding:8px 0";

    const toggle = document.createElement("label");
    toggle.style.cssText = "display:flex;align-items:center;gap:10px;cursor:pointer";
    const box = document.createElement("input");
    box.type = "checkbox";
    box.checked = this.autoBlack;
    box.style.cssText = "width:18px;height:18px;accent-color:#fff;cursor:pointer";
    box.onchange = () => { this.autoBlack = box.checked; };
    const label = document.createElement("span");
    label.textContent = "Автоматически включать чёрную тему Discord («Оникс»), если выбрана цветная или светлая";
    toggle.append(box, label);
    panel.append(toggle);

    const text = document.createElement("div");
    text.textContent = `Проверяю ${REPO} каждые ${CHECK_EVERY / 60000} минут. Новая версия скачивается и применяется сама.`;

    const button = document.createElement("button");
    button.textContent = "Проверить сейчас";
    button.style.cssText = "align-self:flex-start;padding:8px 16px;border:0;border-radius:8px;background:#fff;color:#000;font-weight:700;cursor:pointer";
    button.onclick = () => {
      // забыть сохранённые версии, чтобы точно сверить файлы заново
      for (const item of FILES) BdApi.Data.delete(this.meta.name, item.file);
      this.check(true);
    };

    panel.append(text, button);
    return panel;
  }
};

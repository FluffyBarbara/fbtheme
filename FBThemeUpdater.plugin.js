/**
 * @name FBThemeUpdater
 * @author FluffyBarbara
 * @authorLink https://github.com/FluffyBarbara
 * @version 1.1.0
 * @description 🔄 Сам скачивает свежие версии тем из репозитория FluffyBarbara/fbtheme и заменяет их в BetterDiscord.
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

// always: true — скачать, даже если темы ещё нет в папке.
const THEMES = [
  { file: "TokyoMidnightBlack.theme.css", name: "TokyoMidnightBlack", always: true },
];

module.exports = class FBThemeUpdater {
  constructor(meta) {
    this.meta = meta;
    this.timer = null;
    this.busy = false;
  }

  start() {
    this.check();
    this.timer = setInterval(() => this.check(), CHECK_EVERY);
  }

  stop() {
    clearInterval(this.timer);
    this.timer = null;
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

      for (const theme of THEMES) {
        const target = path.join(BdApi.Themes.folder, theme.file);
        const exists = fs.existsSync(target);

        if (!exists && !theme.always) continue;
        if (exists && BdApi.Data.load(this.meta.name, theme.file) === sha) continue;

        // ссылка на конкретный коммит не кешируется — всегда свежая версия
        const css = await this.get(`https://raw.githubusercontent.com/${REPO}/${sha}/${theme.file}`);
        const local = exists ? fs.readFileSync(target, "utf8") : null;

        if (local !== css) {
          fs.writeFileSync(target, css, "utf8");
          updated.push(theme.name);
          if (!exists) setTimeout(() => BdApi.Themes.enable(theme.name), 1500);
        }

        BdApi.Data.save(this.meta.name, theme.file, sha);
      }

      if (updated.length) {
        BdApi.UI.showToast(`🔄 Тема обновлена: ${updated.join(", ")}`, { type: "success" });
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

    const text = document.createElement("div");
    text.textContent = `Проверяю ${REPO} каждые ${CHECK_EVERY / 60000} минут. Новая версия скачивается и применяется сама.`;

    const button = document.createElement("button");
    button.textContent = "Проверить сейчас";
    button.style.cssText = "align-self:flex-start;padding:8px 16px;border:0;border-radius:8px;background:#fff;color:#000;font-weight:700;cursor:pointer";
    button.onclick = () => {
      // забыть сохранённые версии, чтобы точно сверить файлы заново
      for (const theme of THEMES) BdApi.Data.delete(this.meta.name, theme.file);
      this.check(true);
    };

    panel.append(text, button);
    return panel;
  }
};

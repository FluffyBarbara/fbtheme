/**
 * @name FBThemeUpdater
 * @author FluffyBarbara
 * @authorLink https://github.com/FluffyBarbara
 * @version 1.8.0
 * @description 🔄 Обновляет TokyoMidnightBlack, следит за чёрной темой Discord и подсвечивает роли их цветом.
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
  { file: "TokyoMidnightBlack.theme.css", name: "TokyoMidnightBlack", marker: "@name TokyoMidnightBlack", always: true, folder: () => BdApi.Themes.folder, enable: (n) => BdApi.Themes.enable(n) },
  { file: "FBThemeUpdater.plugin.js", name: "FBThemeUpdater", marker: "@name FBThemeUpdater", always: false, folder: () => BdApi.Plugins.folder },
];

// Откуда качать — по очереди, пока что-то не ответит. У части провайдеров
// GitHub не открывается, а зеркала jsDelivr работают.
const MIRRORS = [
  { host: "raw.githubusercontent.com", url: (f) => `https://raw.githubusercontent.com/${REPO}/${BRANCH}/${f}` },
  { host: "cdn.jsdelivr.net", url: (f) => `https://cdn.jsdelivr.net/gh/${REPO}@${BRANCH}/${f}` },
  { host: "fastly.jsdelivr.net", url: (f) => `https://fastly.jsdelivr.net/gh/${REPO}@${BRANCH}/${f}` },
  { host: "github.com", url: (f) => `https://github.com/${REPO}/raw/${BRANCH}/${f}` },
];
const TIMEOUT = 15 * 1000;

module.exports = class FBThemeUpdater {
  constructor(meta) {
    this.meta = meta;
    this.timer = null;
    this.busy = false;
    this.observer = null;
    this.guardTimer = null;
    this.lastSwitch = 0;
    this.noticeShown = false;
    this.roleObserver = null;
    this.roleTimer = null;
  }

  start() {
    this.check();
    this.timer = setInterval(() => this.check(), CHECK_EVERY);

    // следим за сменой темы Discord (меняются классы и стиль у <html>)
    this.observer = new MutationObserver(() => this.scheduleGuard(800));
    this.observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "style"] });
    this.scheduleGuard(2500); // даём Discord догрузиться

    if (this.roleColors) this.startRoles();
  }

  stop() {
    clearInterval(this.timer);
    clearTimeout(this.guardTimer);
    this.observer?.disconnect();
    this.timer = this.observer = this.guardTimer = null;
    this.stopRoles();
  }

  // ─── цвета ролей ──────────────────────────────────────────────────────
  // Плашкам ролей в профиле и заголовкам групп в списке участников
  // ставим класс и цвет роли в --tmb-role, а красит их уже тема.

  get roleColors() {
    return BdApi.Data.load(this.meta.name, "roleColors") !== false;
  }

  set roleColors(on) {
    BdApi.Data.save(this.meta.name, "roleColors", on);
    if (on) this.startRoles();
    else this.stopRoles();
  }

  startRoles() {
    if (this.roleObserver) return;
    this.roleObserver = new MutationObserver(() => this.queueRoles());
    this.roleObserver.observe(document.body, { childList: true, subtree: true });
    // раз в 5 с — подхватить изменившиеся счётчики в заголовках групп
    this.roleInterval = setInterval(() => this.queueRoles(), 5000);
    this.queueRoles();
  }

  stopRoles() {
    this.roleObserver?.disconnect();
    this.roleObserver = null;
    clearInterval(this.roleInterval);
    this.roleInterval = null;
    clearTimeout(this.roleTimer);
    this.roleTimer = null;
    for (const el of document.querySelectorAll(".tmb-role-pill, .tmb-role-header")) {
      el.classList.remove("tmb-role-pill", "tmb-role-header");
      el.style.removeProperty("--tmb-role");
      delete el.dataset.tmbName;
      delete el.dataset.tmbCount;
      delete el.dataset.tmbFlat;
    }
  }

  queueRoles() {
    if (this.roleTimer) return;
    // не чаще раза в 0.6 с и только когда открыт список участников
    this.roleTimer = setTimeout(() => {
      this.roleTimer = null;
      if (document.querySelector('[class*="membersGroup_"], [class*="roleCircle_"]')) this.paintRoles();
    }, 600);
  }

  static parseColor(color) {
    const m = color && color.match(/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?/);
    if (!m) return null;
    return { r: +m[1], g: +m[2], b: +m[3], a: m[4] === undefined ? 1 : +m[4] };
  }

  // цвет «настоящий»: заметный и не серый
  static isRoleColor(color) {
    const c = FBThemeUpdater.parseColor(color);
    return !!c && c.a > 0.3 && Math.max(c.r, c.g, c.b) - Math.min(c.r, c.g, c.b) > 30;
  }

  static mark(el, cls, color) {
    if (el.style.getPropertyValue("--tmb-role") !== color) el.style.setProperty("--tmb-role", color);
    if (!el.classList.contains(cls)) el.classList.add(cls);
  }

  static unmark(el, cls) {
    if (!el.classList.contains(cls)) return;
    el.classList.remove(cls);
    el.style.removeProperty("--tmb-role");
  }

  // Цвета ролей текущего сервера: название роли → цвет. Кэш на 3 секунды.
  guildRoleColors() {
    const now = Date.now();
    if (this.roleCache && now - this.roleCache.time < 3000) return this.roleCache.map;
    const map = new Map();
    try {
      const W = BdApi.Webpack;
      const guildId = W.getStore?.("SelectedGuildStore")?.getGuildId?.();
      if (guildId) {
        const roleStore = W.getStore?.("GuildRoleStore");
        const guildStore = W.getStore?.("GuildStore");
        const roles = roleStore?.getUnsafeMutableRoles?.(guildId)
          ?? roleStore?.getRolesSnapshot?.(guildId)
          ?? roleStore?.getRoles?.(guildId)
          ?? roleStore?.getManyRoles?.(guildId)
          ?? guildStore?.getRoles?.(guildId)
          ?? guildStore?.getGuild?.(guildId)?.roles
          ?? {};
        for (const role of Object.values(roles)) {
          const color = FBThemeUpdater.roleColor(role);
          if (role?.name && color) map.set(role.name.trim().toLowerCase(), color);
        }
      }
    } catch {}
    this.roleCache = { time: now, map };
    return map;
  }

  static roleColor(role) {
    if (typeof role?.colorString === "string" && role.colorString) return role.colorString;
    const num = role?.colors?.primary_color ?? role?.colors?.primaryColor ?? role?.color;
    if (typeof num === "number" && num > 0) return "#" + num.toString(16).padStart(6, "0");
    return null;
  }

  // Цвет, который у большинства участников группы (если группа не смешанная).
  static majorityColor(header) {
    let anchor = header;
    while (!anchor.nextElementSibling && anchor.parentElement?.children.length === 1) anchor = anchor.parentElement;

    const counts = new Map();
    let total = 0;
    for (let el = anchor.nextElementSibling; el && total < 15; el = el.nextElementSibling) {
      if (el.matches('[class*="membersGroup_"]') || el.querySelector('[class*="membersGroup_"]')) break;
      // цвет роли React ставит прямо в style — читаем его, без пересчёта стилей
      const colored = el.querySelector('[class*="username_"][style*="color"], [class*="name_"] [style*="color"]');
      const color = colored?.style.color;
      if (!color) continue;
      counts.set(color, (counts.get(color) || 0) + 1);
      total++;
    }

    let best = null;
    let bestCount = 0;
    for (const [color, count] of counts) {
      if (count > bestCount) [best, bestCount] = [color, count];
    }
    return best && FBThemeUpdater.isRoleColor(best) && bestCount >= Math.ceil(total * 0.6) ? best : null;
  }

  paintRoles() {
    try {
      // плашки ролей в профиле: точка роли → её плашка
      for (const dot of document.querySelectorAll('[class*="roleCircle_"]')) {
        let pill = dot.parentElement;
        if (pill && /roleRemoveButton/i.test(String(pill.className))) pill = pill.parentElement;
        if (!pill || pill.classList.contains("tmb-role-pill")) continue;
        const color = dot.style.backgroundColor;
        if (color) FBThemeUpdater.mark(pill, "tmb-role-pill", color);
      }

      // заголовки групп в списке участников: цвет роли из Discord по названию,
      // а если не нашёлся — цвет большинства ников в группе
      const roleColors = this.guildRoleColors();
      for (const header of document.querySelectorAll('[class*="membersGroup_"]')) {
        const nameEl = header.querySelector('[class*="membersGroupName_"]');
        let name;
        let count;
        if (nameEl) {
          name = nameEl.textContent.trim();
          count = (nameEl.parentElement?.textContent || "").replace(name, "").replace(/\D/g, "");
        } else {
          // старая разметка: «Название — 5» одним текстом (без скрытой подписи)
          const visible = [...header.childNodes].filter((n) => !/hiddenVisually/i.test(String(n.className || ""))).map((n) => n.textContent).join("");
          const m = visible.replace(/\s+/g, " ").trim().match(/^(.*?)\s*[—–-]\s*([\d\s]+)$/);
          name = (m ? m[1] : visible).trim();
          count = m ? m[2].replace(/\s/g, "") : "";
        }

        // уже раскрашен этим же названием и числом — ничего не пересчитываем
        if (header.classList.contains("tmb-role-header") && header.dataset.tmbName === name && header.dataset.tmbCount === count) continue;
        const color = roleColors.get(name.toLowerCase()) || FBThemeUpdater.majorityColor(header);

        if (color && name) {
          FBThemeUpdater.mark(header, "tmb-role-header", color);
          header.dataset.tmbName = name;
          header.dataset.tmbCount = count;
          if (nameEl) delete header.dataset.tmbFlat;
          else header.dataset.tmbFlat = "";
        } else {
          FBThemeUpdater.unmark(header, "tmb-role-header");
          delete header.dataset.tmbName;
          delete header.dataset.tmbCount;
          delete header.dataset.tmbFlat;
        }
      }
    } catch (err) {
      console.error(`[${this.meta.name}] подсветка ролей`, err);
    }
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

  async request(url, headers = {}) {
    if (BdApi.Net?.fetch) {
      return BdApi.Net.fetch(url, { headers: { "User-Agent": "FBThemeUpdater", ...headers }, timeout: TIMEOUT });
    }
    return window.fetch(url, { headers, cache: "no-store", signal: AbortSignal.timeout(TIMEOUT) });
  }

  // Скачивает файл с первого ответившего зеркала.
  // Возвращает { text } или { unchanged: true }, если файл не менялся (ответ 304).
  async download(item, localSize) {
    const errors = [];
    for (const mirror of MIRRORS) {
      const url = mirror.url(item.file);
      const cache = BdApi.Data.load(this.meta.name, "etag:" + url);
      const headers = cache && cache.size === localSize ? { "If-None-Match": cache.etag } : {};
      try {
        const res = await this.request(url, headers);
        if (res.status === 304) return { unchanged: true, host: mirror.host };
        if (!res.ok) throw new Error(`ответ ${res.status}`);
        const text = await res.text();
        if (!text.includes(item.marker)) throw new Error("пришёл не тот файл");
        const etag = res.headers?.get?.("etag");
        if (etag) BdApi.Data.save(this.meta.name, "etag:" + url, { etag, size: new TextEncoder().encode(text).length });
        return { text, host: mirror.host };
      } catch (err) {
        errors.push(`${mirror.host}: ${err?.message || err}`);
      }
    }
    const err = new Error("нет связи ни с GitHub, ни с зеркалами");
    err.details = errors;
    throw err;
  }

  async check(manual = false) {
    if (this.busy) return false;
    this.busy = true;

    try {
      const updated = [];

      for (const item of FILES) {
        const target = path.join(item.folder(), item.file);
        const exists = fs.existsSync(target);
        if (!exists && !item.always) continue;

        const localSize = exists ? fs.statSync(target).size : -1;
        const got = await this.download(item, localSize);
        if (got.unchanged) continue;

        const local = exists ? fs.readFileSync(target, "utf8") : null;
        if (local !== got.text) {
          updated.push(item.name);
          fs.writeFileSync(target, got.text, "utf8");
          if (!exists && item.enable) setTimeout(() => item.enable(item.name), 1500);
        }
      }

      if (updated.length) {
        BdApi.UI.showToast(`🔄 Обновлено: ${updated.join(", ")}`, { type: "success" });
      } else if (manual) {
        BdApi.UI.showToast("Тема уже последней версии", { type: "info" });
      }
      return true;
    } catch (err) {
      console.error(`[${this.meta.name}] ${err.message}`, ...(err.details || []));
      if (manual) {
        BdApi.UI.showToast("Не получилось проверить обновления: нет связи с GitHub. Подробности — Ctrl+Shift+I → Console", { type: "error", timeout: 8000 });
      }
      return false;
    } finally {
      this.busy = false;
    }
  }

  // Отчёт для автора: что плагин видит в этом Discord (без личных данных, кроме названий ролей).
  report() {
    const W = BdApi.Webpack;
    const out = { plugin: this.meta.version, betterdiscord: BdApi.version, time: new Date().toISOString() };
    try {
      const guildId = W.getStore?.("SelectedGuildStore")?.getGuildId?.();
      const roleStore = W.getStore?.("GuildRoleStore");
      const guildStore = W.getStore?.("GuildStore");
      out.stores = {
        getStore: typeof W.getStore,
        SelectedGuildStore: !!W.getStore?.("SelectedGuildStore"),
        guildSelected: !!guildId,
        GuildRoleStore: !!roleStore,
        GuildRoleStoreMethods: roleStore ? Object.getOwnPropertyNames(Object.getPrototypeOf(roleStore)).filter((k) => /role/i.test(k)) : [],
        GuildStoreRoleMethods: guildStore ? Object.getOwnPropertyNames(Object.getPrototypeOf(guildStore)).filter((k) => /role/i.test(k)) : [],
      };
      out.rolesFound = [...this.guildRoleColors().entries()].slice(0, 12);
    } catch (err) {
      out.storesError = String(err);
    }
    const dot = document.querySelector('[class*="roleCircle_"]');
    if (dot) out.rolePill = (dot.parentElement?.parentElement?.outerHTML || "").slice(0, 400);
    out.rolePillsPainted = document.querySelectorAll(".tmb-role-pill").length;
    const headers = [...document.querySelectorAll('[class*="membersGroup_"]')];
    out.headers = headers.length;
    out.headersPainted = headers.filter((h) => h.classList.contains("tmb-role-header")).length;
    const h = headers[0];
    if (h) {
      out.firstHeader = {
        tag: h.tagName,
        className: String(h.className),
        text: h.textContent?.slice(0, 60),
        html: h.outerHTML.slice(0, 400),
        parent: String(h.parentElement?.className || h.parentElement?.tagName),
        next: String(h.nextElementSibling?.className || h.nextElementSibling?.tagName || "нет"),
      };
    } else {
      out.memberListGuess = [...document.querySelectorAll('[class*="member"]')].slice(0, 5).map((e) => String(e.className).slice(0, 80));
    }
    return JSON.stringify(out, null, 2);
  }

  async copyReport() {
    const text = this.report();
    try {
      if (window.DiscordNative?.clipboard?.copy) window.DiscordNative.clipboard.copy(text);
      else await navigator.clipboard.writeText(text);
      BdApi.UI.showToast("📋 Отчёт скопирован — вставь его в чат автору", { type: "success" });
    } catch {
      console.log(`[${this.meta.name}] отчёт:\n` + text);
      BdApi.UI.showToast("Не получилось скопировать — отчёт в консоли (Ctrl+Shift+I)", { type: "error" });
    }
  }

  getSettingsPanel() {
    const panel = document.createElement("div");
    panel.style.cssText = "display:flex;flex-direction:column;gap:12px;color:var(--text-normal);padding:8px 0";

    const toggle = (text, key) => {
      const row = document.createElement("label");
      row.style.cssText = "display:flex;align-items:center;gap:10px;cursor:pointer";
      const box = document.createElement("input");
      box.type = "checkbox";
      box.checked = this[key];
      box.style.cssText = "width:18px;height:18px;accent-color:#fff;cursor:pointer;flex:none";
      box.onchange = () => { this[key] = box.checked; };
      const label = document.createElement("span");
      label.textContent = text;
      row.append(box, label);
      return row;
    };

    panel.append(
      toggle("Автоматически включать чёрную тему Discord («Оникс»), если выбрана цветная или светлая", "autoBlack"),
      toggle("Подсвечивать роли их цветом: плашки в профиле и заголовки групп участников", "roleColors"),
    );

    const text = document.createElement("div");
    text.textContent = `Проверяю обновления каждые ${CHECK_EVERY / 60000} минут: GitHub, а если он не открывается — зеркала jsDelivr. Новая версия скачивается и применяется сама.`;

    const button = document.createElement("button");
    button.textContent = "Проверить сейчас";
    button.style.cssText = "align-self:flex-start;padding:8px 16px;border:0;border-radius:8px;background:#fff;color:#000;font-weight:700;cursor:pointer";
    button.onclick = async () => {
      if (button.disabled) return;
      button.disabled = true;
      button.textContent = "Проверяю…";
      button.style.opacity = "0.6";
      await this.check(true);
      button.disabled = false;
      button.textContent = "Проверить сейчас";
      button.style.opacity = "1";
    };

    const reportBtn = document.createElement("button");
    reportBtn.textContent = "Скопировать отчёт для автора";
    reportBtn.style.cssText = "align-self:flex-start;padding:8px 16px;border:0;border-radius:8px;background:rgba(255,255,255,.08);color:var(--text-normal);font-weight:600;cursor:pointer";
    reportBtn.onclick = () => this.copyReport();

    panel.append(text, button, reportBtn);
    return panel;
  }
};

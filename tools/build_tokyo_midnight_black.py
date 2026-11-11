#!/usr/bin/env python3
"""Собирает TokyoMidnightBlack.theme.css.

Порядок в готовом файле:
  1. шапка BetterDiscord и шрифты
  2. настройки            — src/tokyo-midnight-black/settings.css
  3. база темы            — скачивается, зелёные цвета перекрашиваются в серые
  4. оформление и анимации — src/tokyo-midnight-black/extras.css
  5. лицензия базы

Запуск:
    python3 tools/build_tokyo_midnight_black.py            # скачать базу с GitHub
    python3 tools/build_tokyo_midnight_black.py ./папка     # взять локальные файлы
"""

import colorsys
import re
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "src" / "tokyo-midnight-black"
OUT = ROOT / "TokyoMidnightBlack.theme.css"
UPSTREAM = "https://raw.githubusercontent.com/SnajperEXE/TokyoMidnight/main/"

VERSION = "1.3.0"

# (файл, перекрашивать ли, заголовок раздела). Ссылки и игровые карточки
# остаются в фирменных цветах платформ.
SOURCES = [
    ("links.css", False, "Цветные ссылки платформ"),
    ("gamecards.css", False, "Карточки игр"),
    ("addons/memberlist.css", True, "Сворачиваемый список участников"),
    ("tokyo_midnight.css", True, "Основа"),
]

FONTS = [
    "https://fonts.googleapis.com/css2?family=Story+Script&display=swap",
    "https://fonts.googleapis.com/css2?family=Electrolize&display=swap",
    "https://fonts.googleapis.com/css2?family=Fredoka:wght@300..700&display=swap",
]

# Фильтры базы, которые красят элементы в мятный цвет.
FILTER_FIXES = {
    "filter: hue-rotate(160deg) saturate(2.5) brightness(1.3) !important;":
        "filter: grayscale(1) brightness(1.4) !important;",
    "filter: hue-rotate(90deg) saturate(0.5) brightness(1.0);":
        "filter: grayscale(1) brightness(1.1);",
}

HEADER = f"""/**
 * @name TokyoMidnightBlack
 * @author FluffyBarbara
 * @authorLink https://github.com/FluffyBarbara
 * @version {VERSION}
 * @description ⚫ Глубокий чёрный · ⚪ серебристые акценты · ✦ мигающие точки у каналов · ✨ плавные анимации
 * @source https://github.com/FluffyBarbara/fbtheme
 * @website https://github.com/FluffyBarbara/fbtheme
 */

/*
 *  ████████  ███    ███  ██████
 *     ██     ████  ████  ██   ██     TokyoMidnightBlack  v{VERSION}
 *     ██     ██ ████ ██  ██████      by FluffyBarbara
 *     ██     ██  ██  ██  ██   ██
 *     ██     ██      ██  ██████      Настройки — сразу ниже.
 */
"""


def section(title):
    line = "═" * 58
    return f"\n/* ╔{line}╗\n   ║  {title:<56}║\n   ╚{line}╝ */\n"


def to_gray(r, g, b):
    """Возвращает серый для зелёных/мятных и почти чёрных цветов, иначе None."""
    h, l, s = colorsys.rgb_to_hls(r / 255, g / 255, b / 255)
    hue = h * 360
    mx = max(r, g, b)
    if s >= 0.12 and 65 <= hue <= 200:
        # неон → серебристо-белый, тёмно-зелёный → тёмно-серый
        luma = 0.2126 * r + 0.7152 * g + 0.0722 * b
        return round(0.5 * mx + 0.5 * luma)
    if s >= 0.12 and mx < 70:
        return round((r + g + b) / 3)  # тёмно-синие подложки → чёрный
    return None


def fmt(v, a):
    if a is None or a >= 0.999:
        return f"#{v:02x}{v:02x}{v:02x}"
    return f"rgba({v}, {v}, {v}, {round(a, 3)})"


def hex_sub(m):
    s = m.group(1)
    if len(s) in (3, 4):
        s = "".join(c * 2 for c in s)
    r, g, b = (int(s[i:i + 2], 16) for i in (0, 2, 4))
    a = int(s[6:8], 16) / 255 if len(s) == 8 else None
    v = to_gray(r, g, b)
    return m.group(0) if v is None else fmt(v, a)


def rgb_sub(m):
    r, g, b = (round(float(x)) for x in m.group(2, 3, 4))
    a = m.group(5)
    v = to_gray(r, g, b)
    if v is None:
        return m.group(0)
    if a is None:
        return fmt(v, None)
    a = float(a[:-1]) / 100 if a.endswith("%") else float(a)
    return fmt(v, a)


def hsl_sub(m):
    h, s, l = float(m.group(2)), float(m.group(3)) / 100, float(m.group(4)) / 100
    r, g, b = (round(c * 255) for c in colorsys.hls_to_rgb(h / 360, l, s))
    v = to_gray(r, g, b)
    if v is None:
        return m.group(0)
    a = m.group(5)
    return fmt(v, float(a) if a is not None else None)


NUM = r"\s*(-?[\d.]+)\s*"
HEX_RE = re.compile(r"#([0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})\b")
RGB_RE = re.compile(rf"(rgba?)\({NUM},{NUM},{NUM}(?:,\s*([\d.]+%?)\s*)?\)")
HSL_RE = re.compile(rf"(hsla?)\({NUM},{NUM}%\s*,{NUM}%\s*(?:,\s*([\d.]+)\s*)?\)")


def recolor(css):
    css = HEX_RE.sub(hex_sub, css)
    css = RGB_RE.sub(rgb_sub, css)
    css = HSL_RE.sub(hsl_sub, css)
    for old, new in FILTER_FIXES.items():
        css = css.replace(old, new)
    return css


def strip_base(css):
    """Убирает из базы шапки-подписи и блок курсоров (курсор остаётся системным)."""
    css = re.sub(r"/\* @name.*?@description[^\n]*\n", "", css, flags=re.S)
    start = css.find("/* 1. Normal */")
    end = css.find('[class*="guildInviteContainer"]')
    if start != -1 and end > start:
        css = css[:start] + css[end:]
    return css


def load(name, local):
    if local:
        return (Path(local) / name).read_text(encoding="utf-8")
    with urllib.request.urlopen(UPSTREAM + name) as r:
        return r.read().decode("utf-8")


def license_notice():
    text = (SRC / "LICENSE-TokyoMidnight").read_text(encoding="utf-8").strip()
    body = "\n".join((" * " + l).rstrip() for l in text.splitlines())
    return "\n/*\n * Сторонний код в этом файле распространяется по лицензии:\n *\n" + body + "\n */\n"


def main():
    local = sys.argv[1] if len(sys.argv) > 1 else None
    parts = [HEADER, "\n".join(f'@import url("{u}");' for u in FONTS), ""]
    parts.append((SRC / "settings.css").read_text(encoding="utf-8"))
    for name, do_recolor, title in SOURCES:
        css = strip_base(load(name, local))
        parts.append(section(title))
        parts.append(recolor(css) if do_recolor else css)
    parts.append((SRC / "extras.css").read_text(encoding="utf-8"))
    parts.append(license_notice())
    OUT.write_text("\n".join(parts), encoding="utf-8")
    print(f"Готово: {OUT.relative_to(ROOT)}  ({OUT.stat().st_size // 1024} КБ)")


if __name__ == "__main__":
    main()

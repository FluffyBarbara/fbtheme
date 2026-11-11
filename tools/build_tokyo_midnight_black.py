#!/usr/bin/env python3
"""Собирает TokyoMidnightBlack.theme.css.

Берёт исходники TokyoMidnight (SnajperEXE, MIT), перекрашивает зашитые в код
зелёные/мятные и тёмно-синие цвета в оттенки серого, затем добавляет
чёрную палитру и мигающую точку из src/tokyo-midnight-black/black.css.

Запуск:
    python3 tools/build_tokyo_midnight_black.py            # скачать с GitHub
    python3 tools/build_tokyo_midnight_black.py ./папка     # взять локальные файлы
"""

import colorsys
import re
import sys
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
UPSTREAM = "https://raw.githubusercontent.com/SnajperEXE/TokyoMidnight/main/"
OUT = ROOT / "TokyoMidnightBlack.theme.css"
BLACK = ROOT / "src" / "tokyo-midnight-black" / "black.css"

# (файл, перекрашивать ли). Ссылки и игровые карточки остаются в фирменных
# цветах платформ — это отдельная фишка оригинальной темы.
SOURCES = [
    ("links.css", False),
    ("gamecards.css", False),
    ("addons/memberlist.css", True),
    ("tokyo_midnight.css", True),
]

FONTS = [
    "https://fonts.googleapis.com/css2?family=Story+Script&display=swap",
    "https://fonts.googleapis.com/css2?family=Electrolize&display=swap",
    "https://fonts.googleapis.com/css2?family=Fredoka:wght@300..700&display=swap",
]

# Фильтры оригинала, которые красят элементы в мятный цвет.
FILTER_FIXES = {
    "filter: hue-rotate(160deg) saturate(2.5) brightness(1.3) !important;":
        "filter: grayscale(1) brightness(1.4) !important;",
    "filter: hue-rotate(90deg) saturate(0.5) brightness(1.0);":
        "filter: grayscale(1) brightness(1.1);",
}

HEADER = """/**
 * @name TokyoMidnightBlack
 * @author FluffyBarbara
 * @version 1.1.0
 * @description Чёрно-белая версия TokyoMidnight: глубокий чёрный фон, серебристые акценты, мигающая белая точка у каналов и плавные анимации.
 * @source https://github.com/fluffybarbara/fbtheme
 */

/*
 * Основано на TokyoMidnight от SnajperEXE — https://github.com/SnajperEXE/TokyoMidnight
 *
 * MIT License
 * Copyright (c) 2026 SnajperEXE
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * Настройки цветов и точки — в самом конце файла, в блоке «ЧЁРНАЯ ПАЛИТРА».
 */
"""


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


def load(name, local):
    if local:
        return (Path(local) / name).read_text(encoding="utf-8")
    with urllib.request.urlopen(UPSTREAM + name) as r:
        return r.read().decode("utf-8")


def main():
    local = sys.argv[1] if len(sys.argv) > 1 else None
    parts = [HEADER, "\n".join(f'@import url("{u}");' for u in FONTS), ""]
    for name, do_recolor in SOURCES:
        css = load(name, local)
        parts.append(f"\n/* {'=' * 56}\n   TokyoMidnight: {name}\n   {'=' * 56} */\n")
        parts.append(recolor(css) if do_recolor else css)
    parts.append(f"\n/* {'=' * 56}\n   ЧЁРНАЯ ПАЛИТРА И МИГАЮЩАЯ ТОЧКА\n   {'=' * 56} */\n")
    parts.append(BLACK.read_text(encoding="utf-8"))
    OUT.write_text("\n".join(parts), encoding="utf-8")
    print(f"Готово: {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    main()

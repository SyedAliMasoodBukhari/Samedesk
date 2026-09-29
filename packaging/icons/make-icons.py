#!/usr/bin/env python3
"""Draws the SameDesk app icon (docs/images/logo.svg) at every size the
installers need. Run from the repository root: python3 packaging/icons/make-icons.py
Needs Pillow. The outputs are committed, so builds don't need Python."""
import os, subprocess, tempfile
from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
BLUE, WHITE = (65, 143, 204, 255), (255, 255, 255, 255)


def draw(px, margin=0.0):
    """The logo on a 96-unit grid, supersampled for smooth edges. `margin` shrinks
    the tile inside the canvas (macOS icons sit inside a transparent border)."""
    ss = 4
    big = px * ss
    img = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    tile = big * (1 - 2 * margin)
    off = big * margin
    u = tile / 96

    def rect(x, y, w, h, r, **kw):
        d.rounded_rectangle([off + x * u, off + y * u, off + (x + w) * u, off + (y + h) * u], radius=r * u, **kw)

    rect(0, 0, 96, 96, 24, fill=BLUE)
    rect(26, 31, 30, 38, 7, outline=WHITE, width=round(5 * u))
    rect(40, 22, 30, 38, 7, fill=BLUE, outline=WHITE, width=round(5 * u))
    return img.resize((px, px), Image.LANCZOS)


def main():
    # macOS: an .iconset with the standard ~10% transparent margin, then iconutil.
    with tempfile.TemporaryDirectory() as tmp:
        iconset = os.path.join(tmp, "SameDesk.iconset")
        os.mkdir(iconset)
        for pt in (16, 32, 128, 256, 512):
            for scale in (1, 2):
                name = f"icon_{pt}x{pt}{'@2x' if scale == 2 else ''}.png"
                draw(pt * scale, margin=0.1).save(os.path.join(iconset, name))
        subprocess.run(["iconutil", "-c", "icns", iconset, "-o", os.path.join(HERE, "samedesk.icns")], check=True)

    # Windows: one .ico holding every size Explorer and the taskbar ask for.
    sizes = [16, 20, 24, 32, 40, 48, 64, 128, 256]
    draw(256).save(os.path.join(HERE, "samedesk.ico"), sizes=[(s, s) for s in sizes])

    # Linux: hicolor PNGs.
    for s in (48, 128, 256, 512):
        draw(s).save(os.path.join(HERE, f"samedesk-{s}.png"))


if __name__ == "__main__":
    main()

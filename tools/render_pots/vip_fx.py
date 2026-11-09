"""Ảnh hiệu ứng chậu VIP (CC0, tự vẽ bằng PIL): vip_sparkle.png (ánh lấp lánh 4 cánh, 2× 48×48 → hiển thị 24×24)
và vip_halo.png (quầng sáng vàng mềm dưới chân chậu, 2× 240×80 → hiển thị 120×40), vip_badge.png (nhãn "VIP" nhỏ).

Dùng: python3 tools/render_pots/vip_fx.py public/img/toon/farm
"""
import math
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont


def sparkle(size=48):
    big = size * 4
    im = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    c = big / 2
    # quầng mềm
    glow = Image.new("L", (big, big), 0)
    ImageDraw.Draw(glow).ellipse((c - big * 0.22, c - big * 0.22, c + big * 0.22, c + big * 0.22), fill=150)
    glow = glow.filter(ImageFilter.GaussianBlur(big * 0.08))
    g = Image.new("RGBA", (big, big), (255, 236, 160, 0)); g.putalpha(glow); im.alpha_composite(g)
    # sao 4 cánh dài + 4 cánh ngắn chéo
    d = ImageDraw.Draw(im)
    for rot, long, w in ((0, 0.48, 0.07), (math.pi / 4, 0.26, 0.05)):
        pts = []
        for k in range(8):
            a = rot + k * math.pi / 4
            r = big * (long if k % 2 == 0 else w)
            pts.append((c + math.cos(a) * r, c + math.sin(a) * r))
        d.polygon(pts, fill=(255, 250, 225, 255))
    d.ellipse((c - big * 0.06, c - big * 0.06, c + big * 0.06, c + big * 0.06), fill=(255, 255, 255, 255))
    return im.resize((size, size), Image.LANCZOS)


def halo(w=240, h=80):
    big = Image.new("L", (w, h), 0)
    ImageDraw.Draw(big).ellipse((w * 0.12, h * 0.28, w * 0.88, h * 0.78), fill=210)
    big = big.filter(ImageFilter.GaussianBlur(10))
    im = Image.new("RGBA", (w, h), (255, 214, 90, 0))
    im.putalpha(big)
    return im


def badge(w=64, h=30):
    big = Image.new("RGBA", (w * 4, h * 4), (0, 0, 0, 0))
    d = ImageDraw.Draw(big)
    d.rounded_rectangle((4, 4, w * 4 - 4, h * 4 - 4), radius=h * 2 - 4, fill=(120, 40, 0, 255))
    d.rounded_rectangle((12, 12, w * 4 - 12, h * 4 - 12), radius=h * 2 - 12, fill=(255, 196, 40, 255))
    d.rounded_rectangle((12, 12, w * 4 - 12, h * 2), radius=h * 2 - 12, fill=(255, 230, 120, 255))
    try:
        font = ImageFont.truetype("/System/Library/Fonts/Supplemental/Arial Black.ttf", h * 2 + 10)
    except OSError:
        font = ImageFont.load_default()
    d.text((w * 2, h * 2 + 4), "VIP", fill=(110, 30, 0, 255), font=font, anchor="mm")
    return big.resize((w, h), Image.LANCZOS)


def main():
    out = Path(sys.argv[1])
    sparkle().save(out / "vip_sparkle.png", optimize=True)
    halo().save(out / "vip_halo.png", optimize=True)
    badge().save(out / "vip_badge.png", optimize=True)
    print("xong →", out)


if __name__ == "__main__":
    main()

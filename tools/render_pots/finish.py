"""Hậu kỳ ảnh chậu: thu nhỏ 480×528 → 240×264 (ảnh 2× của khung 120×132), viền tối mảnh kiểu toon, bóng mềm dưới đáy,
nén bảng 256 màu.

Dùng: python3 tools/render_pots/finish.py <thư mục ảnh thô> <thư mục ra>
"""
import json
import sys
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFilter

OUT_W, OUT_H = 240, 264
# Tâm đáy chậu (0,0,0) chiếu lên y = 1,15 / 1,364 của khung.
BASE_Y = round(OUT_H * 1.15 / 1.364)


def finish(src: Path, outline_hex: str, glow_hex: str | None = None) -> Image.Image:
    im = Image.open(src).convert("RGBA").resize((OUT_W, OUT_H), Image.LANCZOS)
    a = im.getchannel("A")
    # Viền: nở alpha 2 px (1 px khi hiển thị), tô màu viền, đặt sau chậu.
    grown = a.filter(ImageFilter.MaxFilter(5))
    line = Image.new("RGBA", im.size, outline_hex)
    line.putalpha(ImageChops.multiply(grown, Image.new("L", im.size, 230)))
    # Bóng mềm: elip theo bề ngang phần dưới chậu.
    bbox = a.point(lambda v: 255 if v > 40 else 0).getbbox()
    rows = [y for y in range(bbox[1], bbox[3]) if a.crop((0, y, OUT_W, y + 1)).getbbox()]
    bottom = rows[-1]
    strip = a.crop((0, bottom - 14, OUT_W, bottom)).point(lambda v: 255 if v > 40 else 0).getbbox()
    half = (strip[2] - strip[0]) / 2 * 1.25
    cx = OUT_W / 2
    shadow = Image.new("L", im.size, 0)
    ImageDraw.Draw(shadow).ellipse((cx - half, bottom - 10, cx + half, bottom + 10), fill=120)
    shadow = shadow.filter(ImageFilter.GaussianBlur(6))
    sh = Image.new("RGBA", im.size, (40, 60, 90, 0))
    sh.putalpha(shadow)
    out = Image.new("RGBA", im.size, (0, 0, 0, 0))
    out.alpha_composite(sh)
    if glow_hex:
        # Chậu VIP: quầng sáng màu mềm quanh viền (nở alpha rồi làm mờ), nằm sau viền tối.
        halo_a = a.filter(ImageFilter.MaxFilter(9)).filter(ImageFilter.GaussianBlur(7))
        halo = Image.new("RGBA", im.size, glow_hex)
        halo.putalpha(halo_a.point(lambda v: min(255, int(v * 0.95))))
        out.alpha_composite(halo)
    out.alpha_composite(line)
    out.alpha_composite(im)
    return out


def main():
    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
    dst.mkdir(parents=True, exist_ok=True)
    meta = json.loads((src / "meta.json").read_text())
    for pid, m in meta.items():
        # Bảng 256 màu: ~10 KB / ảnh thay vì ~65 KB, mắt thường gần như không thấy khác.
        im = finish(src / f"{pid}.png", m["outline"], m.get("glow"))
        im.quantize(colors=256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.FLOYDSTEINBERG).save(dst / f"{pid}.png", optimize=True)
    print("xong", len(meta), "ảnh →", dst)


if __name__ == "__main__":
    main()

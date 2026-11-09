"""Hậu kỳ ảnh Noel / NPC: thu ảnh thô 4× về 2×, viền tối mảnh kiểu toon, bóng mềm dưới chân, cắt sát hình (các khung cùng
nhóm — cây đèn a/b, NPC — cắt chung một khung để thay ảnh không lệch), nén bảng màu. Ghi noel.json: kích thước hiển thị
(1×) và điểm chân của từng ảnh.

Dùng: python3 tools/render_noel/finish.py <thư mục ảnh thô> <thư mục ra>
"""
import json
import sys
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageFilter

GROUPS = {"tree": ("tree_a", "tree_b"), "lights": ("lights_a", "lights_b"),
          "npc": tuple(f"npc_{k}_{i}" for k in ("santa", "farmer") for i in range(3))}
NO_SHADOW = {"wreath", "lights_a", "lights_b", "quest_mark"}


def group_of(pid):
    for g, ids in GROUPS.items():
        if pid in ids:
            return g
    return pid


def finish(im, outline_hex, origin, shadow):
    a = im.getchannel("A")
    out = Image.new("RGBA", im.size, (0, 0, 0, 0))
    if shadow:
        bbox = a.point(lambda v: 255 if v > 40 else 0).getbbox()
        half = (bbox[2] - bbox[0]) * 0.42
        ox, oy = origin
        sh = Image.new("L", im.size, 0)
        ImageDraw.Draw(sh).ellipse((ox - half, oy - half * 0.28, ox + half, oy + half * 0.28), fill=110)
        sh = sh.filter(ImageFilter.GaussianBlur(5))
        layer = Image.new("RGBA", im.size, (40, 60, 90, 0))
        layer.putalpha(sh)
        out.alpha_composite(layer)
    if outline_hex:
        grown = a.filter(ImageFilter.MaxFilter(5))
        line = Image.new("RGBA", im.size, outline_hex)
        line.putalpha(ImageChops.multiply(grown, Image.new("L", im.size, 225)))
        out.alpha_composite(line)
    out.alpha_composite(im)
    return out


def glow(im):
    """Quầng sáng quanh bóng đèn: điểm rất sáng làm mờ rồi cộng lên dưới hình."""
    px = im.convert("RGBA")
    r, g, b, a = px.split()
    mx = ImageChops.lighter(ImageChops.lighter(r, g), b)
    mn = ImageChops.darker(ImageChops.darker(r, g), b)
    sat = ImageChops.subtract(mx, mn)
    # Chỉ bóng đèn đang sáng: rất sáng và có màu (tuyết trắng, lá xanh không tính).
    bright = ImageChops.multiply(mx.point(lambda v: 255 if v > 235 else 0), sat.point(lambda v: 255 if v > 70 else 0))
    bright = ImageChops.multiply(bright, a)
    halo = bright.filter(ImageFilter.GaussianBlur(10)).point(lambda v: min(255, v * 3))
    col = Image.merge("RGBA", (r.filter(ImageFilter.GaussianBlur(10)), g.filter(ImageFilter.GaussianBlur(10)), b.filter(ImageFilter.GaussianBlur(10)), halo))
    base = Image.new("RGBA", im.size, (0, 0, 0, 0))
    base.alpha_composite(Image.merge("RGBA", (Image.new("L", im.size, 255), Image.new("L", im.size, 236), Image.new("L", im.size, 170), halo)))
    base.alpha_composite(px)
    return base


def main():
    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
    dst.mkdir(parents=True, exist_ok=True)
    meta = json.loads((src / "meta.json").read_text())
    done = {}
    for pid, m in meta.items():
        im = Image.open(src / f"{pid}.png").convert("RGBA")
        im = im.resize((im.width // 2, im.height // 2), Image.LANCZOS)
        origin = (m["origin"][0] / 2, m["origin"][1] / 2)
        if pid.startswith(("tree_", "lights_")):
            im = glow(im)
        im = finish(im, m["outline"], origin, pid not in NO_SHADOW)
        done[pid] = (im, origin, m)
    # Khung cắt chung cho từng nhóm.
    boxes = {}
    for pid, (im, _, m) in done.items():
        g = group_of(pid)
        b = (0, 0, im.width, im.height) if m["noCrop"] else im.getchannel("A").point(lambda v: 255 if v > 8 else 0).getbbox()
        if g in boxes:
            o = boxes[g]
            b = (min(o[0], b[0]), min(o[1], b[1]), max(o[2], b[2]), max(o[3], b[3]))
        boxes[g] = b
    info = {}
    for pid, (im, origin, m) in done.items():
        b = boxes[group_of(pid)]
        if not m["noCrop"]:
            b = (max(0, b[0] - 2) // 2 * 2, max(0, b[1] - 2) // 2 * 2, min(im.width, b[2] + 3) // 2 * 2, min(im.height, b[3] + 3) // 2 * 2)
        c = im.crop(b)
        c.quantize(colors=256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.FLOYDSTEINBERG).save(dst / f"{pid}.png", optimize=True)
        # Kích thước hiển thị = nửa ảnh (ảnh 2×); chân = gốc toạ độ trên ảnh đã cắt, tính theo px hiển thị.
        info[pid] = {"w": c.width // 2, "h": c.height // 2, "foot": [round((origin[0] - b[0]) / 2), round((origin[1] - b[1]) / 2)]}
    (dst / "noel.json").write_text(json.dumps(info, indent=1, ensure_ascii=False))
    print("xong", len(done), "ảnh →", dst)


if __name__ == "__main__":
    main()

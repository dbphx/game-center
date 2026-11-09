"""Hậu kỳ ảnh nhiệm vụ: cắt sát hình, viền tối mảnh kiểu toon (rương, mảnh), thu về cỡ 2× hiển thị, nén bảng màu.
Rương: rộng 192 px (hiển thị 96). Mảnh: 96×96 (hiển thị 48). Xe / đồ độc quyền: khung cố định 320×200 (hiển thị 160×100),
hình nằm giữa, chân xe sát mép dưới.

Dùng: python3 tools/render_quests/finish.py <thư mục ảnh thô> <public/img/quests>
"""
import json
import sys
from pathlib import Path

from PIL import Image, ImageChops, ImageFilter


def crop(im, pad=6):
    bbox = im.getchannel("A").point(lambda v: 255 if v > 8 else 0).getbbox()
    l, t, r, b = bbox
    return im.crop((max(0, l - pad), max(0, t - pad), min(im.width, r + pad), min(im.height, b + pad)))


def outline(im, hexcol, width=7):
    a = im.getchannel("A")
    grown = a.filter(ImageFilter.MaxFilter(width))
    line = Image.new("RGBA", im.size, hexcol)
    line.putalpha(ImageChops.multiply(grown, Image.new("L", im.size, 230)))
    out = Image.new("RGBA", im.size, (0, 0, 0, 0))
    out.alpha_composite(line)
    out.alpha_composite(im)
    return out


def fit(im, w, h):
    im = im.copy()
    im.thumbnail((w, h), Image.LANCZOS)
    return im


def save(im, path):
    q = im.quantize(colors=255, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.NONE)
    q.save(path, optimize=True)


def main(src, dst):
    src, dst = Path(src), Path(dst)
    (dst / "cars").mkdir(parents=True, exist_ok=True)
    meta = json.loads((src / "meta.json").read_text())
    man = {}
    for pid, m in meta.items():
        im = Image.open(src / f"{pid}.png").convert("RGBA")
        if m.get("car"):
            im = crop(im, 2)
            im = fit(im, 304, 188)
            canvas = Image.new("RGBA", (320, 200), (0, 0, 0, 0))
            canvas.alpha_composite(im, ((320 - im.width) // 2, 200 - 6 - im.height))
            name = pid[4:]
            save(canvas, dst / "cars" / f"{name}.png")
            man[f"cars/{name}"] = {"w": 160, "h": 100}
            continue
        im = crop(im, 10)
        im = outline(im, m["outline"] or "#1a1430")
        if pid.startswith("chest"):
            im = fit(im, 192, 999)
        else:
            im = fit(im, 96, 96)
            c = Image.new("RGBA", (96, 96), (0, 0, 0, 0))
            c.alpha_composite(im, ((96 - im.width) // 2, (96 - im.height) // 2))
            im = c
        save(im, dst / f"{pid}.png")
        man[pid] = {"w": im.width // 2, "h": im.height // 2}
    (dst / "manifest.json").write_text(json.dumps({"scale": 2, "source": "tools/render_quests (tự dựng, CC0)", "images": man}, ensure_ascii=False, indent=1) + "\n")
    (dst / "LICENSE.txt").write_text("Ảnh trong thư mục này do DCity tự dựng (tools/render_quests), phát hành CC0 1.0.\nXe dựng từ mô hình Trường đua — nguồn / giấy phép mô hình gốc: public/race/models/CREDITS.txt.\n")
    print("xong", len(man), "ảnh →", dst)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])

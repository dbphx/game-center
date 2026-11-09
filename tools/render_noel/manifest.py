"""Ghi manifest.json, CREDITS.csv, LICENSE.txt cho public/img/toon/farm/noel/ (chạy sau snow.py và finish.py)."""
import json
from pathlib import Path

from PIL import Image

DIR = Path(__file__).resolve().parents[2] / "public/img/toon/farm/noel"
USE = {
    "tree_a": "Cây thông Noel (bộ đèn a sáng), đặt thay cây đào trái cổng", "tree_b": "Cây thông Noel (bộ đèn b sáng), chồng lên tree_a nhấp nháy",
    "snowman": "Người tuyết, thay cây đào phải cổng", "gift_red": "Hộp quà đỏ", "gift_green": "Hộp quà xanh lá", "gift_blue": "Hộp quà xanh dương",
    "wreath": "Vòng hoa treo cổng / kho", "lights_a": "Dây đèn nháy lặp ngang 128 px (bộ a sáng)", "lights_b": "Dây đèn nháy (bộ b sáng)",
    "quest_mark": "Chấm ! vàng trên đầu NPC (có nhiệm vụ)",
    "flake_dot": "Bông tuyết tròn mềm (lớp xa / giữa)", "flake_star": "Bông tuyết tinh thể to (lớp gần)", "flake_star_s": "Bông tuyết tinh thể nhỏ",
}
for i in range(3):
    USE[f"npc_santa_{i}"] = f"NPC Ông già Noel kèm bao quà, khung {i} ({'đứng' if i == 0 else 'vẫy tay'})"
    USE[f"npc_farmer_{i}"] = f"NPC Bác nông dân (nón lá, giỏ rau) ngoài mùa Noel, khung {i} ({'đứng' if i == 0 else 'vẫy tay'})"


def main():
    meta = json.loads((DIR / "noel.json").read_text())
    images, rows = [], ["file,source,author,license,url,notes"]
    for f in sorted(DIR.glob("*.png")):
        k = f.stem
        w, h = Image.open(f).size
        if k in meta:
            src = "DCity — dựng 3D bằng three.js (tools/render_noel/noel.js), hậu kỳ Pillow"
            e = {"file": f.name, "w": meta[k]["w"], "h": meta[k]["h"], "anchor": "foot", "foot": meta[k]["foot"], "use": USE.get(k, k)}
        elif k.startswith("flake"):
            src = "DCity — vẽ bằng Pillow (tools/render_noel/snow.py)"
            e = {"file": f.name, "w": w, "h": h, "use": USE.get(k, k)}
        else:
            src = f"DCity — bản phủ tuyết dựng bằng Pillow từ ảnh gốc {k}.png (tools/render_noel/snow.py)"
            e = {"file": f.name, "w": w, "h": h, "replaces": k, "use": f"Bản mùa Noel của {k}.png (cùng kích thước ảnh gốc)"}
        e["license"] = "CC0 1.0"
        images.append(e)
        rows.append(f'{f.name},{src},DCity,CC0 1.0,—,"{e["use"]}"')
    (DIR / "manifest.json").write_text(json.dumps({
        "style": "Mùa Noel cho nông trại: tuyết phủ trên chính ảnh gốc (mái, tán cây, hàng rào, nền), đồ trang trí và NPC dựng 3D cùng góc nhìn ảnh farm3d; ảnh 2× (hiển thị bằng w × h)",
        "anchor_note": "foot = điểm chân theo px hiển thị; ảnh phủ tuyết giữ nguyên kích thước và neo của ảnh gốc",
        "images": images}, indent=1, ensure_ascii=False))
    (DIR / "CREDITS.csv").write_text("\n".join(rows) + "\n")
    (DIR / "LICENSE.txt").write_text("Ảnh mùa Noel nông trại DCity (public/img/toon/farm/noel/)\n\n"
                                     "Mọi ảnh trong thư mục này do DCity tự dựng: bản phủ tuyết ghép bằng Pillow từ ảnh CC0 của dự án, đồ trang trí\n"
                                     "và NPC là hình học tự dựng bằng three.js (MIT). Phát hành CC0 1.0 (Public Domain Dedication):\n"
                                     "http://creativecommons.org/publicdomain/zero/1.0/\n")
    print("xong manifest", len(images), "ảnh")


if __name__ == "__main__":
    main()

"""Dựng ảnh mùa Noel cho nông trại từ chính ảnh gốc (CC0, tự vẽ cho DCity): tuyết phủ trên mái / tán cây / hàng rào,
nền đất – cỏ phủ tuyết, đồi xa phủ tuyết, bông tuyết rơi. Ảnh ra cùng kích thước ảnh gốc nên chỉ cần đổi đường dẫn.

Dùng: python3 tools/render_noel/snow.py   (chạy ở gốc repo; ghi vào public/img/toon/farm/noel/)
"""
import math
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[2] / "public/img/toon"
OUT = ROOT / "farm/noel"

SNOW = np.array([248, 251, 255], float)
SNOW_SHADE = np.array([196, 214, 236], float)
LINE = np.array([92, 112, 146], float)


def load(rel):
    return np.asarray(Image.open(ROOT / rel).convert("RGBA")).astype(float)


def save(arr, name, colors=None):
    im = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8), "RGBA")
    if colors:
        im = im.quantize(colors=colors, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.FLOYDSTEINBERG)
    im.save(OUT / name, optimize=True)


def periodic_noise(w, h, cells, seed, blur):
    """Nhiễu mịn lặp liền hai chiều (ghép 3×3, làm mờ, cắt giữa)."""
    rnd = np.random.default_rng(seed)
    g = rnd.random((max(1, round(h / cells)), max(1, round(w / cells))))
    small = Image.fromarray((g * 255).astype(np.uint8), "L").resize((w, h), Image.BICUBIC)
    big = Image.new("L", (w * 3, h * 3))
    for i in range(3):
        for j in range(3):
            big.paste(small, (i * w, j * h))
    big = big.filter(ImageFilter.GaussianBlur(blur))
    a = np.asarray(big.crop((w, h, 2 * w, 2 * h))).astype(float) / 255
    return (a - a.min()) / max(1e-6, a.max() - a.min())


def wave(w, period, seed, amp=1.0):
    """Dao động mịn theo x, lặp đúng theo `period` (để ảnh lặp ngang không lộ đường nối)."""
    rnd = np.random.default_rng(seed)
    x = np.arange(w)
    v = np.zeros(w)
    for k in (1, 2, 3, 5, 8):
        v += math.sin(0) + np.sin(2 * math.pi * k * x / period + rnd.random() * 6.28) / k
    v = (v - v.min()) / max(1e-6, v.max() - v.min())
    return v * amp


def snowcap(rel, name, thick, seed=1, lip=None, periodic=False, clumps=0, alpha_min=150, steep=2.2, colors=256, max_y=1.0, solid_mask=None, src=None):
    """Tuyết đọng trên mặt hướng lên của hình: dải dày `thick` px (theo ảnh gốc) chạy theo mép trên, nhô lên `lip` px,
    bo tròn, có bóng xanh nhạt phía dưới và viền tối mảnh như nét toon của bộ hình."""
    src = load(rel) if src is None else src
    h, w = src.shape[:2]
    a = src[..., 3]
    solid = (a >= alpha_min) if solid_mask is None else solid_mask
    lip = lip if lip is not None else max(1, round(thick * 0.35))
    var = wave(w, w if periodic else w * 1.7, seed)
    t = thick * (0.7 + 0.6 * var)
    up = np.round(lip * (0.4 + 0.6 * wave(w, w if periodic else w * 1.3, seed + 7))).astype(int)
    # Độ sâu tính từ mép trên của từng đoạn đặc trong mỗi cột.
    depth = np.zeros((h, w))
    run = np.zeros(w)
    for y in range(h):
        run = np.where(solid[y], run + 1, 0)
        depth[y] = run
    # Chỉ mặt hướng lên mới đọng tuyết: pháp tuyến mép (gradient của mặt nạ đã làm mờ) phải nghiêng ít hơn độ dốc
    # `steep` (dy/dx); càng dốc dải tuyết càng mỏng, quá dốc (tường, thân cây, sườn đồi) thì không có.
    top = (depth == 1)
    mask = np.zeros((h, w), bool)
    rows = np.nonzero(solid.any(axis=1))[0]
    ymax = rows[0] + (rows[-1] - rows[0]) * max_y if len(rows) else h
    blur = np.asarray(Image.fromarray(solid.astype(np.uint8) * 255, "L").filter(ImageFilter.GaussianBlur(max(2, thick * 0.8)))).astype(float)
    gy, gx = np.gradient(blur)
    upness = gy / np.maximum(1e-6, np.hypot(gx, gy))
    cmin = 1 / math.sqrt(1 + steep * steep)
    ys, xs = np.nonzero(top)
    for y, x in zip(ys, xs):
        if y > ymax:
            continue
        f = (upness[y, x] - cmin) / max(1e-6, 1 - cmin)
        if f <= 0.05:
            continue
        f = min(1.0, f * 1.6)
        y0 = max(0, y - int(round(up[x] * f)))
        y1 = min(h, y + max(1, int(t[x] * f)))
        mask[y0:y1, x] = True
    # Cục tuyết rải trên tán (cây): chấm tròn ở nửa trên các điểm đặc.
    if clumps:
        rnd = np.random.default_rng(seed + 3)
        cand = np.argwhere(solid & (depth > thick * 1.5))
        cand = cand[cand[:, 0] < np.percentile(cand[:, 0], 55)] if len(cand) else cand
        im = Image.fromarray(mask.astype(np.uint8) * 255, "L")
        d = ImageDraw.Draw(im)
        for _ in range(clumps if len(cand) else 0):
            y, x = cand[rnd.integers(len(cand))]
            rx = thick * (0.5 + rnd.random() * 0.7)
            d.ellipse((x - rx * 1.4, y - rx * 0.6, x + rx * 1.4, y + rx * 0.7), fill=255)
        mask = np.asarray(im) > 0
    # Bo tròn: làm mờ rồi cắt ngưỡng; không cho tuyết bay lơ lửng xa hình.
    soft = np.asarray(Image.fromarray(mask.astype(np.uint8) * 255, "L").filter(ImageFilter.GaussianBlur(max(1, thick * 0.25)))) / 255
    grown = np.asarray(Image.fromarray(solid.astype(np.uint8) * 255, "L").filter(ImageFilter.MaxFilter(2 * lip + 1))) > 0
    m = np.clip((soft - 0.35) / 0.3, 0, 1) * grown
    if periodic:  # nối mép trái – phải
        m[:, 0] = m[:, -1] = np.maximum(m[:, 0], m[:, -1])
    # Bóng: phần dưới của dải sẫm xanh hơn.
    below = np.zeros_like(m)
    k = max(1, round(thick * 0.45))
    below[k:] = m[:-k]
    shade = np.clip(m - np.minimum(m, np.roll(m, -k, axis=0)), 0, 1)
    col = SNOW[None, None, :] * (1 - shade[..., None] * 0.9) + SNOW_SHADE[None, None, :] * (shade[..., None] * 0.9)
    # Bóng đổ nhẹ của tuyết xuống hình ngay dưới.
    out = src.copy()
    drop = np.clip(below - m, 0, 1) * solid
    out[..., :3] *= (1 - 0.22 * drop[..., None])
    # Viền tối: phần tuyết nằm ngoài hình gốc.
    edge = np.asarray(Image.fromarray((m * 255).astype(np.uint8), "L").filter(ImageFilter.MaxFilter(3))).astype(float) / 255
    edge = np.clip(edge - m, 0, 1) * (1 - solid)
    oa = out[..., 3:4] / 255
    ea = edge[..., None] * 0.75
    out[..., :3] = out[..., :3] * oa * (1 - ea) + LINE * ea
    out[..., 3] = np.maximum(out[..., 3], edge * 190)
    na = np.maximum(oa[..., 0] * (1 - ea[..., 0]) + ea[..., 0], 1e-6)
    out[..., :3] /= na[..., None]
    ma = m[..., None]
    out[..., :3] = out[..., :3] * (1 - ma) + col * ma
    out[..., 3] = np.maximum(out[..., 3], m * 255)
    if name is None:
        return out
    save(out, name, colors)
    return name


def hills(rel, name, thick, seed, whiten=0.5):
    """Ảnh trời + đồi (không trong suốt): chỉ đồi xanh được phủ tuyết (đỉnh đồi đọng tuyết, sườn trắng nhạt), trời nhạt
    sang màu trời đông."""
    src = load(rel)
    r, g, b = src[..., 0], src[..., 1], src[..., 2]
    green = (g > b + 4) & (g > r + 10)
    green = np.asarray(Image.fromarray(green.astype(np.uint8) * 255, "L").filter(ImageFilter.MedianFilter(5))) > 0
    base = src.copy()
    gm = np.asarray(Image.fromarray(green.astype(np.uint8) * 255, "L").filter(ImageFilter.GaussianBlur(1.5))).astype(float)[..., None] / 255
    base[..., :3] = base[..., :3] * (1 - gm * whiten) + np.array([232, 240, 250]) * gm * whiten
    sky = 1 - gm
    lum = base[..., :3].mean(axis=2, keepdims=True)
    base[..., :3] = base[..., :3] * (1 - sky * 0.35) + (lum * 0.6 + np.array([96, 112, 130]) * 0.45) * sky * 0.35
    # Hai lớp đồi (xa nhạt, gần đậm): mỗi lớp đọng tuyết trên đỉnh riêng.
    back = green & (r >= 150)
    front = green & (r < 150)
    out = snowcap(None, None, thick * 0.7, seed, periodic=True, solid_mask=back, src=base, steep=0.7)
    out = snowcap(None, None, thick, seed + 1, periodic=True, solid_mask=front, src=out, steep=0.7)
    out[..., 3] = 255
    Image.fromarray(np.clip(out[..., :3], 0, 255).astype(np.uint8), "RGB").save(OUT / name, optimize=True)
    return name


def frost(rel, name, cover, seed=1, cells=10, only_green=False, keep_alpha=True, sparkle=0.0, colors=256, darken=0.0,
          soft=0.06, opacity=1.0, whiten=0.0, rim_k=0.18):
    """Phủ tuyết loang trên ảnh lặp (nền đất / cỏ / viền ô ruộng): nhiễu lặp liền hai chiều, mép mềm, bóng xanh nhạt."""
    src = load(rel)
    h, w = src.shape[:2]
    n = periodic_noise(w, h, cells, seed, blur=max(2, cells * 0.45))
    thr = np.quantile(n, 1 - cover)
    m = np.clip((n - thr) / soft, 0, 1)
    m = m * m * (3 - 2 * m) * opacity
    if only_green:
        r, g, b = src[..., 0], src[..., 1], src[..., 2]
        green = np.clip((g - np.maximum(r, b) - 8) / 20, 0, 1)
        green = np.asarray(Image.fromarray((green * 255).astype(np.uint8), "L").filter(ImageFilter.GaussianBlur(1.2))) / 255
        m = m * green
    # Bóng mép dưới của mảng tuyết (ánh sáng từ trên).
    k = max(1, round(cells * 0.15))
    rim = np.clip(np.roll(m, k, axis=0) - m, 0, 1)
    shade = np.clip(m - np.roll(m, -k, axis=0), 0, 1)
    out = src.copy()
    out[..., :3] = out[..., :3] * (1 - darken) + np.array([210, 220, 235]) * darken
    if whiten:  # phủ đều một lớp tuyết mỏng (vẫn thấy nét ảnh gốc)
        wm = whiten * (np.clip((src[..., 1] - np.maximum(src[..., 0], src[..., 2]) + 30) / 40, 0, 1) if only_green else 1)
        wm = np.asarray(wm) * np.ones((h, w))
        out[..., :3] = out[..., :3] * (1 - wm[..., None]) + np.array([236, 243, 251]) * wm[..., None]
    out[..., :3] *= (1 - rim_k * rim[..., None])
    col = SNOW[None, None] * (1 - 0.8 * shade[..., None]) + SNOW_SHADE[None, None] * (0.8 * shade[..., None])
    out[..., :3] = out[..., :3] * (1 - m[..., None]) + col * m[..., None]
    if sparkle:
        rnd = np.random.default_rng(seed + 11)
        sp = (rnd.random((h, w)) < sparkle) & (m > 0.9)
        out[sp, :3] = 255
    if not keep_alpha:
        out[..., 3] = 255
    save(out, name, colors)
    return name


def flake(name, size, kind, seed=0):
    """Bông tuyết rơi: tròn mềm (xa) hoặc tinh thể 6 cánh (gần), vẽ ở 4× rồi thu nhỏ."""
    S = size * 4
    im = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    c = S / 2
    if kind == "dot":
        glow = Image.new("L", (S, S), 0)
        ImageDraw.Draw(glow).ellipse((S * 0.2, S * 0.2, S * 0.8, S * 0.8), fill=255)
        glow = glow.filter(ImageFilter.GaussianBlur(S * 0.12))
        # Quầng xanh nhạt để bông tuyết vẫn thấy được trên nền tuyết trắng.
        im = Image.new("RGBA", (S, S), (150, 178, 214, 0))
        im.putalpha(glow.point(lambda v: v * 0.55))
        core = Image.new("L", (S, S), 0)
        ImageDraw.Draw(core).ellipse((S * 0.3, S * 0.3, S * 0.7, S * 0.7), fill=255)
        im.paste((255, 255, 255, 255), (0, 0), core.filter(ImageFilter.GaussianBlur(S * 0.03)))
    else:
        wline = max(2, round(S * 0.07))
        for i in range(6):
            ang = i * math.pi / 3 + 0.26
            ex, ey = c + math.cos(ang) * S * 0.44, c + math.sin(ang) * S * 0.44
            # Viền xanh sẫm mờ để bông tuyết nổi trên nền sáng.
            d.line((c, c, ex, ey), fill=(120, 150, 190, 150), width=wline + 4)
        for i in range(6):
            ang = i * math.pi / 3 + 0.26
            ex, ey = c + math.cos(ang) * S * 0.44, c + math.sin(ang) * S * 0.44
            d.line((c, c, ex, ey), fill=(255, 255, 255, 255), width=wline)
            for f in (0.55, 0.78):
                bx, by = c + math.cos(ang) * S * 0.44 * f, c + math.sin(ang) * S * 0.44 * f
                for s in (-1, 1):
                    a2 = ang + s * 0.75
                    ln = S * 0.12 * (1.2 - f)
                    d.line((bx, by, bx + math.cos(a2) * ln * 1.4, by + math.sin(a2) * ln * 1.4), fill=(255, 255, 255, 255), width=max(1, wline - 1))
        d.ellipse((c - S * 0.08, c - S * 0.08, c + S * 0.08, c + S * 0.08), fill=(255, 255, 255, 255))
    im = im.resize((size, size), Image.LANCZOS)
    im.save(OUT / name, optimize=True)
    return name


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    made = []
    # Nhà, cổng, đống rơm, giếng (ảnh 3D) — tuyết dày trên mái.
    made += [snowcap("farm3d/hut_rice.png", "hut_rice.png", 16, 1, max_y=0.75),
             snowcap("farm3d/hut_seed.png", "hut_seed.png", 15, 2, max_y=0.75),
             snowcap("farm3d/pen_shed.png", "pen_shed.png", 15, 3, max_y=0.75),
             snowcap("farm3d/haystack.png", "haystack.png", 20, 4, max_y=0.6, steep=6),
             snowcap("farm3d/farm_gate.png", "farm_gate.png", 18, 5),
             snowcap("farm3d/well.png", "well.png", 10, 6),
             snowcap("farm3d/fence_side.png", "fence_side.png", 7, 7, steep=1.4),
             snowcap("farm3d/fence_side_full.png", "fence_side_full.png", 7, 8, steep=1.4)]
    # Cây: dải tuyết trên tán + vài cục tuyết.
    trees = {"tree_big": 12, "tree_dao": 10, "tree_mai": 10, "tree_chuoi": 10, "tree_da": 16, "tree_khe": 12,
             "tree_dua": 12, "tree_tre": 7, "tree_xoai": 12}
    for i, (k, t) in enumerate(trees.items()):
        made.append(snowcap(f"farm/{k}.png", f"{k}.png", t, 20 + i, clumps=0 if k == "tree_tre" else 7, max_y=0.7))
    # Hàng rào, bụi, viền cỏ lặp ngang.
    made += [snowcap("farm/fence_row.png", "fence_row.png", 6, 31, periodic=True, steep=1.2),
             snowcap("farm/fence_bamboo.png", "fence_bamboo.png", 6, 32, periodic=True, steep=1.2),
             snowcap("farm/fence_flower_store.png", "fence_flower_store.png", 6, 33, periodic=True, steep=1.2),
             snowcap("farm/fence_picket.png", "fence_picket.png", 6, 34, periodic=True, steep=1.2),
             snowcap("farm3d/fence_wood.png", "fence_wood.png", 7, 35, periodic=True, steep=1.2),
             snowcap("farm/fence_piece.png", "fence_piece.png", 5, 36, steep=1.2),
             snowcap("farm/bush_1.png", "bush_1.png", 9, 37, periodic=True),
             snowcap("farm/grass_edge.png", "grass_edge.png", 7, 38, periodic=True)]
    for side in ("fence_bamboo_side", "fence_flower_store_side", "fence_picket_side"):
        made.append(snowcap(f"farm/{side}.png", f"{side}.png", 5, 40, steep=1.2))
    # Đồi xa + đồng ruộng: tuyết trên đỉnh đồi, ruộng phủ tuyết loang.
    snowcap("farm/far_farmland.png", "far_tmp.png", 9, 41, periodic=True, colors=None)
    Image.open(OUT / "far_tmp.png").save(OUT / "far_tmp_in.png")
    made.append(frost_file("far_tmp_in.png", "far_farmland.png", 0.35, 42, cells=30, soft=0.35, whiten=0.5, opacity=0.7, rim_k=0))
    (OUT / "far_tmp.png").unlink()
    (OUT / "far_tmp_in.png").unlink()
    made.append(hills("farm/bg_sky_hills.png", "bg_sky_hills.png", 28, 43))
    # Nền: lối đất phủ tuyết loang, cỏ phủ sương tuyết, viền cỏ của ô ruộng đọng tuyết (lòng đất vẫn rõ để gieo).
    made += [frost("farm/dirt_smooth.png", "dirt_smooth.png", 0.62, 51, cells=48, sparkle=0.0015, soft=0.3, opacity=0.92, whiten=0.18, rim_k=0.08),
             frost("farm/grass_tile.png", "grass_tile.png", 0.6, 52, cells=24, sparkle=0.002, soft=0.3, opacity=0.9, whiten=0.15, rim_k=0.08),
             frost("farm/soil_plot_light.png", "soil_plot_light.png", 0.8, 53, cells=10, only_green=True, soft=0.2, rim_k=0.1),
             frost("farm/soil_plot_light_wet.png", "soil_plot_light_wet.png", 0.8, 53, cells=10, only_green=True, soft=0.2, rim_k=0.1),
             frost("farm/soil_plot_light_good.png", "soil_plot_light_good.png", 0.8, 53, cells=10, only_green=True, soft=0.2, rim_k=0.1),
             frost("farm/soil_plot_light_wet_good.png", "soil_plot_light_wet_good.png", 0.8, 53, cells=10, only_green=True, soft=0.2, rim_k=0.1),
             frost("farm/grass_plot.png", "grass_plot.png", 0.6, 54, cells=16, soft=0.3, opacity=0.9, whiten=0.15, rim_k=0.08)]
    # Mép trước hàng rào gỗ chuồng (dòng dưới) giữ như hàng rào trên.
    made += [flake("flake_dot.png", 16, "dot"), flake("flake_star.png", 28, "star"), flake("flake_star_s.png", 18, "star")]
    print("xong", len(made), "ảnh →", OUT)


def frost_file(src_name, name, cover, seed, **kw):
    """frost() trên ảnh trung gian trong thư mục noel/."""
    rel = Path("farm/noel") / src_name
    return frost(str(rel), name, cover, seed, **kw)


if __name__ == "__main__":
    main()

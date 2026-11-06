#!/usr/bin/env python3
"""Nướng sẵn bóng đổ (và màu pastel của nhà dân) vào ảnh đồ vật trên bản đồ thành phố.

Trước đây mỗi .prop trên bản đồ dùng CSS filter: drop-shadow(0 4px 3px rgba(40,60,20,.28)), nhà dân thêm
sepia/hue-rotate/saturate. Hơn 1.000 ảnh có filter làm mỗi lần vẽ lại (hoạt ảnh, xe, nhân vật) rất nặng trên điện thoại.
Script này tạo bản PNG đã có bóng (và đã tô màu) trong public/img/toon/cityshadow/ cùng manifest.json để city.js dùng
thay cho filter. Ảnh không có bản nướng sẵn vẫn dùng filter CSS như cũ.

Chạy lại sau khi thêm / đổi ảnh đồ vật:  python3 tools/bake_city_shadows.py
Ảnh sinh ra giữ nguồn và giấy phép của ảnh gốc (CC0).
"""
import json
import math
import os
import sys

import numpy as np
from PIL import Image, ImageFilter

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'public', 'img', 'toon')
OUT = os.path.join(ROOT, 'cityshadow')

# Bóng như CSS cũ: lệch 4, mờ 3 (σ = 1,5), màu rgba(40, 60, 20, .28) — đơn vị hiển thị trên bản đồ.
SHADOW_DY, SHADOW_SIGMA, SHADOW_RGB, SHADOW_A = 4.0, 1.5, (40, 60, 20), 0.28

# Màu pastel của nhà dân bộ 3D (giống các lớp .tint0..4 cũ): (sepia, hue-rotate độ, saturate).
TINTS = [(0.32, 0, 1.25), (0.3, -28, 1.35), (0.3, 28, 1.15), (0.28, -45, 1.2), (0.3, 12, 1.3)]

# Ảnh nông trại dùng làm đồ vật trên bản đồ (khoá 'farm/x' trong city.js).
FARM_PROPS = ['animal_chicken', 'animal_chicken_white', 'animal_cow', 'animal_horse', 'animal_pig', 'animal_sheep', 'barn',
              'bush_1', 'bush_2', 'bush_3', 'fence_piece', 'flowers', 'hay', 'pond', 'tree_big', 'tree_chuoi', 'tree_da',
              'tree_dao', 'tree_dua', 'tree_mai', 'tree_med', 'tree_round_1', 'tree_round_3', 'tree_tre', 'tree_xoai',
              'trough_water', 'wheelbarrow']


def sepia(a):
    b = 1 - a
    return np.array([[0.393 + 0.607 * b, 0.769 - 0.769 * b, 0.189 - 0.189 * b],
                     [0.349 - 0.349 * b, 0.686 + 0.314 * b, 0.168 - 0.168 * b],
                     [0.272 - 0.272 * b, 0.534 - 0.534 * b, 0.131 + 0.869 * b]])


def saturate(s):
    return np.array([[0.213 + 0.787 * s, 0.715 - 0.715 * s, 0.072 - 0.072 * s],
                     [0.213 - 0.213 * s, 0.715 + 0.285 * s, 0.072 - 0.072 * s],
                     [0.213 - 0.213 * s, 0.715 - 0.715 * s, 0.072 + 0.928 * s]])


def hue(deg):
    c, s = math.cos(math.radians(deg)), math.sin(math.radians(deg))
    return np.array([[0.213 + c * 0.787 - s * 0.213, 0.715 - c * 0.715 - s * 0.715, 0.072 - c * 0.072 + s * 0.928],
                     [0.213 - c * 0.213 + s * 0.143, 0.715 + c * 0.285 + s * 0.140, 0.072 - c * 0.072 - s * 0.283],
                     [0.213 - c * 0.213 - s * 0.787, 0.715 - c * 0.715 + s * 0.715, 0.072 + c * 0.928 + s * 0.072]])


def tint(img, t):
    """Tô màu như CSS filter: sepia → hue-rotate → saturate (mỗi bước kẹp 0..1, trên giá trị sRGB)."""
    a = np.asarray(img, dtype=np.float64) / 255.0
    rgb = a[..., :3]
    sep, deg, sat = t
    for m in (sepia(sep), hue(deg), saturate(sat)):
        rgb = np.clip(rgb @ m.T, 0, 1)
    out = np.concatenate([rgb, a[..., 3:]], axis=-1)
    return Image.fromarray(np.round(out * 255).astype(np.uint8), 'RGBA')


def with_shadow(img, unit):
    """Thêm bóng đổ dưới ảnh; unit = số đơn vị hiển thị mỗi điểm ảnh. Trả ảnh mới và phần lề đã thêm [trái, trên, phải, dưới]."""
    dy = SHADOW_DY / unit
    sigma = SHADOW_SIGMA / unit
    reach = int(math.ceil(sigma * 3))
    pad = [reach, max(0, reach - int(dy)), reach, int(math.ceil(dy)) + reach]
    w, h = img.size
    W, H = w + pad[0] + pad[2], h + pad[1] + pad[3]
    alpha = Image.new('L', (W, H), 0)
    alpha.paste(img.getchannel('A'), (pad[0], pad[1] + int(round(dy))))
    alpha = alpha.filter(ImageFilter.GaussianBlur(sigma))
    sh = np.asarray(alpha, dtype=np.float64) / 255.0 * SHADOW_A
    shadow = np.zeros((H, W, 4))
    shadow[..., 0], shadow[..., 1], shadow[..., 2] = SHADOW_RGB
    shadow[..., 3] = sh * 255
    base = Image.fromarray(np.round(shadow).astype(np.uint8), 'RGBA')
    top = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    top.paste(img, (pad[0], pad[1]))
    return Image.alpha_composite(base, top), pad


def glow(img, unit, sigma_units, rgb, alpha, keep):
    """Quầng sáng quanh hình (drop-shadow không lệch). keep: giữ hình ở trên, không thì chỉ còn quầng."""
    sigma = sigma_units / unit
    reach = int(math.ceil(sigma * 3))
    w, h = img.size
    W, H = w + 2 * reach, h + 2 * reach
    a = Image.new('L', (W, H), 0)
    a.paste(img.getchannel('A'), (reach, reach))
    a = np.asarray(a.filter(ImageFilter.GaussianBlur(sigma)), dtype=np.float64) / 255.0
    g = np.zeros((H, W, 4))
    g[..., 0], g[..., 1], g[..., 2] = rgb
    g[..., 3] = np.clip(a * alpha * 255, 0, 255)
    base = Image.fromarray(np.round(g).astype(np.uint8), 'RGBA')
    if keep:
        top = Image.new('RGBA', (W, H), (0, 0, 0, 0))
        top.paste(img, (reach, reach))
        base = Image.alpha_composite(base, top)
    return base, [reach, reach, reach, reach]


def quantize(img):
    """Nén PNG bảng màu giữ kênh trong suốt (như các ảnh nặng khác trong dự án); ảnh nhỏ thì giữ RGBA."""
    try:
        q = img.quantize(colors=256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.NONE)
        return q
    except Exception:
        return img


def main():
    os.makedirs(OUT, exist_ok=True)
    out = {}
    jobs = []
    m3 = json.load(open(os.path.join(ROOT, 'city3d', 'manifest.json')))
    for it in m3['images']:
        if it.get('frames') or it.get('tile') or it.get('anchor') == 'top-left':
            continue
        jobs.append(('city3d', it))
    # Ảnh bộ 2D cũ vẫn được dùng làm đồ vật khi bộ 3D không có ảnh thay (cờ, ...).
    for it in json.load(open(os.path.join(ROOT, 'city', 'manifest.json')))['images']:
        if not (it.get('frames') or it.get('tile') or it.get('anchor') == 'top-left'):
            jobs.append(('city', it))
    mf = {it['file']: it for it in json.load(open(os.path.join(ROOT, 'farm', 'manifest.json')))['images']}
    for n in FARM_PROPS:
        if n + '.png' in mf:
            jobs.append(('farm', mf[n + '.png']))
    for f, it in mf.items():  # ruộng ngoại ô: cây trồng các giai đoạn
        if f.startswith('crop_'):
            jobs.append(('farm', it))
    for d, it in jobs:
        path = os.path.join(ROOT, d, it['file'])
        if not os.path.exists(path):
            continue
        src = Image.open(path).convert('RGBA')
        unit = it['w'] * (it.get('scale') or 1) / src.size[0]
        variants = [('', None)]
        if d == 'city3d' and it['file'].startswith('house_'):
            variants += [('#t%d' % i, t) for i, t in enumerate(TINTS)]
        for suffix, t in variants:
            img = tint(src, t) if t else src
            baked, pad = with_shadow(img, unit)
            name = '%s__%s%s.png' % (d, it['file'][:-4], suffix.replace('#', '_'))
            quantize(baked).save(os.path.join(OUT, name), optimize=True)
            out['/img/toon/%s/%s%s' % (d, it['file'], suffix)] = {'file': name, 'nw': src.size[0], 'pad': pad}
    # Bóng đèn quanh biển CASINO lúc sáng (giống filter cũ brightness(1.4) drop-shadow(0 0 4px #ffd84a)), hiển thị 9×9.
    bulb = Image.open(os.path.join(ROOT, 'city', 'fx_bulb.png')).convert('RGBA')
    lit = np.asarray(bulb, dtype=np.float64)
    lit[..., :3] = np.clip(lit[..., :3] * 1.4, 0, 255)
    lit = Image.fromarray(np.round(lit).astype(np.uint8), 'RGBA')
    img, pad = glow(lit, 9 / bulb.size[0], 2.0, (255, 216, 74), 1.0, keep=True)
    img.save(os.path.join(OUT, 'city__fx_bulb_on.png'), optimize=True)
    out['/img/toon/city/fx_bulb.png#on'] = {'file': 'city__fx_bulb_on.png', 'nw': bulb.size[0], 'pad': pad}
    # Quầng sáng của biển CASINO (lớp riêng nằm dưới biển, nhấp nháy bằng opacity thay cho filter drop-shadow 14px).
    m3i = {it['file']: it for it in m3['images']}
    sign = Image.open(os.path.join(ROOT, 'city3d', 'sign_casino.png')).convert('RGBA')
    img, pad = glow(sign, m3i['sign_casino.png']['w'] / sign.size[0], 7.0, (255, 210, 60), 1.0, keep=False)
    img.save(os.path.join(OUT, 'city3d__sign_casino_glow.png'), optimize=True)
    out['/img/toon/city3d/sign_casino.png#glow'] = {'file': 'city3d__sign_casino_glow.png', 'nw': sign.size[0], 'pad': pad}
    json.dump({'note': 'Sinh bởi tools/bake_city_shadows.py — ảnh đồ vật bản đồ đã có bóng đổ (và màu nhà). Khoá: đường dẫn ảnh gốc '
                       '(thêm #tN cho màu nhà N). pad: lề thêm [trái, trên, phải, dưới] theo điểm ảnh gốc; nw: bề rộng ảnh gốc.',
               'images': out}, open(os.path.join(OUT, 'manifest.json'), 'w'), ensure_ascii=False, indent=0)
    print(len(out), 'ảnh')


if __name__ == '__main__':
    sys.exit(main())

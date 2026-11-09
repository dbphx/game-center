# Decal bông tuyết (đồ độc quyền decal_x_snow, DCity CC0): bông tuyết 6 cánh trắng viền xanh băng + vài bông nhỏ, nền trong.
import sys, math
from PIL import Image, ImageDraw, ImageFilter
out = sys.argv[1]
S = 4  # vẽ to rồi thu nhỏ cho mịn
W = 256 * S
img = Image.new('RGBA', (W, W), (0, 0, 0, 0))

def flake(cx, cy, r, w, col):
    d = ImageDraw.Draw(img)
    for k in range(6):
        a = math.pi / 2 + k * math.pi / 3
        ex, ey = cx + math.cos(a) * r, cy - math.sin(a) * r
        d.line([(cx, cy), (ex, ey)], fill=col, width=w)
        for f, L in ((0.45, 0.32), (0.72, 0.22)):
            bx, by = cx + math.cos(a) * r * f, cy - math.sin(a) * r * f
            for s in (-1, 1):
                b = a + s * math.pi / 4
                d.line([(bx, by), (bx + math.cos(b) * r * L, by - math.sin(b) * r * L)], fill=col, width=int(w * 0.8))
        d.ellipse([ex - w * 0.7, ey - w * 0.7, ex + w * 0.7, ey + w * 0.7], fill=col)
    d.regular_polygon((cx, cy, w * 1.6), 6, fill=col)

# Viền xanh băng mờ phía sau rồi bông trắng phía trước.
for (cx, cy, r) in ((128, 128, 104), (40, 44, 26), (214, 214, 30), (218, 50, 18), (44, 210, 16)):
    flake(cx * S, cy * S, r * S, int(max(3, r * 0.13) * S * 1.9), (95, 210, 255, 255))
glow = img.filter(ImageFilter.GaussianBlur(5 * S))
img2 = Image.new('RGBA', (W, W), (0, 0, 0, 0))
img = img2
for (cx, cy, r) in ((128, 128, 104), (40, 44, 26), (214, 214, 30), (218, 50, 18), (44, 210, 16)):
    flake(cx * S, cy * S, r * S, int(max(3, r * 0.13) * S), (255, 255, 255, 245))
res = Image.alpha_composite(glow, img).resize((256, 256), Image.LANCZOS)
res.quantize(colors=128, method=Image.Quantize.FASTOCTREE).save(f'{out}/decal_x_snow.png', optimize=True)
print('ok')

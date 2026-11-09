Dựng ảnh chậu tầng mây (public/img/toon/farm/pot_*.png) — ảnh tự dựng cho DCity, CC0.

1. Ở thư mục gốc repo: python3 -m http.server 8305 --bind 127.0.0.1
2. NODE_PATH=<thư mục có node_modules/playwright-core> node tools/render_pots/render.mjs http://127.0.0.1:8305 /tmp/pots_raw
   (dùng Chrome cài sẵn ở /Applications/Google Chrome.app)
3. python3 tools/render_pots/finish.py /tmp/pots_raw /tmp/pots_new   (thu nhỏ 2×, viền tối mảnh, bóng mềm)
4. Chép /tmp/pots_new/pot_*.png vào public/img/toon/farm/

Khung chung 120×132 (ảnh 240×264), máy quay trực giao nghiêng 25°, tâm mặt đất ở y = 32 (manifest plantY).
Đổi khung thì sửa cả public/farm/cloud.css (.cloud-art, .cloud-pot, .cloud-crop).
Mỗi chậu là một hàm trong pots.js (profile LatheGeometry + vật liệu + hoạ tiết vẽ canvas).

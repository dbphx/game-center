Dựng ảnh mùa Noel + NPC cho nông trại (public/img/toon/farm/noel/) — ảnh tự dựng cho DCity, CC0.

1. Bản phủ tuyết (từ chính ảnh gốc, cùng kích thước) + bông tuyết:
   python3 tools/render_noel/snow.py
2. Đồ trang trí 3D + NPC: ở thư mục gốc repo chạy static server
   python3 -m http.server 8306 --bind 127.0.0.1
   NODE_PATH=<thư mục có node_modules/playwright-core> node tools/render_noel/render.mjs http://127.0.0.1:8306 /tmp/noel_raw
   python3 tools/render_noel/finish.py /tmp/noel_raw /tmp/noel_fin     (thu về 2×, viền, bóng, cắt sát; ghi noel.json)
   chép /tmp/noel_fin/*.png + noel.json vào public/img/toon/farm/noel/
3. python3 tools/render_noel/manifest.py   (manifest.json, CREDITS.csv, LICENSE.txt)

1 đơn vị three.js = 100 px hiển thị, máy quay trực giao nghiêng 26°. Các khung cùng nhóm (tree_a/b, npc_*) cắt chung một khung.
Kích thước / điểm chân trong noel.json phải khớp bảng META ở public/farm/season.js.

Dựng ảnh cho bảng nhiệm vụ (public/img/quests/) — ảnh tự dựng cho DCity, CC0.
Rương ngày / tuần (đóng, mở), mảnh ghép 4 bậc (Thường lục · Hiếm lam · Sử thi tím · Độc quyền vàng), ảnh nhỏ các xe đua và
4 đồ độc quyền (chụp trên xe mẫu) dùng chính mô hình trong public/race/scene.js.

1. python3 tools/render_quests/serve.py 8307        (phục vụ gốc repo, thiếu thì lấy trong public/)
2. NODE_PATH=<thư mục có node_modules/playwright-core> node tools/render_quests/render.mjs http://127.0.0.1:8307 /tmp/quests_raw
3. python3 tools/render_quests/finish.py /tmp/quests_raw public/img/quests   (cắt sát, viền toon, thu về 2×, manifest.json, LICENSE.txt)

Xe: renderer của game không có kênh alpha nên vẽ hai lần trên nền đen / trắng rồi tách nền. Thêm xe mới vào CARS trong
quests.js rồi chạy lại. Ảnh xe 320×200 (hiển thị 160×100), rương rộng 192, mảnh 96×96 (hiển thị 48).

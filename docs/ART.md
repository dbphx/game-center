# Hướng dẫn hình ảnh DCity

## Nguyên tắc

1. **Chỉ dùng ảnh raster vẽ sẵn**, không vẽ hình minh hoạ bằng SVG/CSS. Người dùng muốn hình sinh động như game,
   không phải khối hình phẳng / 3D.
2. **Một phong cách thống nhất** cho cả thành phố — không trộn pixel art với tranh vector trong cùng một cảnh.
   **Định hướng hiện tại (người dùng chốt): toàn bộ game theo phong cách HOẠT HÌNH giống game "Avatar" (TeaMobi)** —
   tranh 2D tô màu mềm, tươi sáng, cây tán tròn, hàng rào gỗ, ruộng ô vuông, trời xanh mây trắng, góc nhìn 3/4 từ phía
   trước, nhân vật **chibi đầu to**. Bộ pixel Kenney hiện có là tạm thời, sẽ được thay. Không dùng pixel art cho phần mới.
3. **Nguồn mở**: ưu tiên CC0 (Kenney: https://kenney.nl/assets). Được dùng thêm **LPC / OpenGameArt CC-BY / CC-BY-SA / GPL**
   (người dùng đã đồng ý) với điều kiện **ghi công đầy đủ**: mỗi thư mục có `CREDITS.csv` / `LICENSE.txt` liệt kê tác giả,
   giấy phép, link gốc cho từng file, và trang `/credits.html` tổng hợp. Hình chỉnh sửa từ CC-BY-SA giữ cùng giấy phép.
   Ảnh do người dùng cung cấp thì để trong thư mục riêng và ghi rõ nguồn / quyền dùng.

## Đặt file

| Thư mục | Nội dung |
|---|---|
| `public/img/city/` | bản đồ thành phố: mặt đất, đường, vỉa hè, xe, cây, nước, cầu, toà nhà, đồ trang trí, nhân vật |
| `public/img/farm/` | nông trại: ô đất, cây theo giai đoạn, hàng rào, chuồng, con vật, ao, cá |
| `public/img/casino/` | sảnh casino: sàn, bàn, ghế, trang trí |

- Mỗi thư mục có `manifest.json` mô tả từng file: `{ file, w, h, mô tả, dùng cho, scale đề xuất, pixelated, tile }`
  và `LICENSE.txt`. Code đọc manifest thay vì đoán tên file.
- Chỉ chép những file thực sự dùng; giữ tổng dung lượng gọn. Tilesheet thì cắt thành file riêng hoặc ghi toạ độ trong manifest.
- Gói tải về giải nén trong thư mục tạm, chỉ lấy PNG; không chạy script trong gói.

## Hiển thị

- Pixel art: `image-rendering: pixelated;` và phóng to **số nguyên lần** (×2, ×3, ×4) để nét không bị nhoè.
- Nền lặp (cỏ, nhựa đường, nước, thảm): `background-image` + `background-size` bằng bội số nguyên của tile.
- Vật thể (xe, cây, con vật, nhân vật): `<img>` hoặc `<image>` đặt vị trí tuyệt đối; hướng / xoay bằng transform.
- Hoạt ảnh: đổi frame trong sprite sheet hoặc dịch chuyển (CSS / requestAnimationFrame), không vẽ lại bằng SVG.
  Đổi frame bằng `transform` của dải khung bên trong một khung `overflow: hidden` (chạy trên GPU), không animate
  `background-position`.
- Bản đồ thành phố không dùng CSS `filter` cho đồ vật (quá nặng khi có hàng nghìn ảnh): bóng đổ và màu nhà dân được
  nướng sẵn vào `public/img/toon/cityshadow/` bằng `python3 tools/bake_city_shadows.py` (có `manifest.json`, giữ nguồn
  và giấy phép ảnh gốc). **Thêm / đổi ảnh đồ vật thành phố thì chạy lại script**; ảnh chưa nướng vẫn dùng filter như cũ.

## Hiện trạng

Toàn game theo phong cách hoạt hình Avatar (bộ pixel cũ đã xoá, còn trong lịch sử git):

| Chỗ | Hình |
|---|---|
| `public/city.js` bản đồ | canvas + ảnh `public/img/toon/city/` (+ nông trại dùng `toon/farm/`) |
| `public/character.js` nhân vật | `public/img/characters/` — chibi hoạt hình (Kenney Toon Characters, đầu phóng to), 4 hướng, bước, nằm, nháy đỏ, avatar, toàn thân |
| `public/farm/` nông trại | `public/img/toon/farm/` |
| `public/tienlen/` sảnh casino + bàn chơi | `public/img/toon/casino/` |
| Bệnh viện | `public/img/toon/hospital/`, xe cứu thương ở `toon/city/` |
| `/credits.html` | tổng hợp ghi công từ manifest / CREDITS.csv / LICENSE.txt |
| `public/icons.js` | icon giao diện SVG nhỏ trong nút — ngoại lệ, giữ |
| `public/race/` | Trường đua 3D (three.js): mô hình GLB CC0 hoặc CC-BY **có ghi công** (người dùng đồng ý) — tên tác giả hiện trong garage và ghi ở `public/race/models/CREDITS.txt`; không dùng mô hình trích từ game thương mại. Siêu xe hạng cao (và xe độc quyền) dựng bằng code trong `public/race/hyper.js` (thân loft, vây lưỡi dao, cánh kính phát sáng, neon), garage hiện "Thiết kế: DCity" |

Bản đồ (`public/city.js`) đọc mọi ảnh qua bảng ánh xạ tên → file + điểm chân lấy từ manifest. Một bộ ảnh thay thế
(vd 3D ở `public/img/toon/city3d/`) chỉ cần manifest có `"replaces": "tên cũ"` cho từng ảnh (hoặc bảng `"map"`), neo chân
`"anchor": [x, y]` / `"foot"` và `"scale"` nếu cần; bật bằng `?art=city3d` hoặc `localStorage['city.art']`.
Cây phố (phượng, bằng lăng, bàng, sao), khu Công trường / Hồ câu cá, hiệu ứng động (`fx_*`) và biểu cảm (`emote_*`) ở
`public/img/toon/city/` đều vẽ bằng PIL (CC0); hoạt ảnh chỉ dùng CSS transform / opacity trên ảnh PNG.
Chậu tầng mây (`public/img/toon/farm/pot_*.png`, ảnh 2× 240×264 hiển thị 120×132, tâm mặt đất `plantY` = 32) dựng 3D bằng
three.js rồi xuất PNG (CC0): `tools/render_pots/` — chạy static server ở gốc repo (`python3 -m http.server 8305`), `node
tools/render_pots/render.mjs` (cần playwright-core + Chrome) rồi `python3 tools/render_pots/finish.py` (viền + bóng); xem README.txt ở đó.
Chậu Noel VIP (`pot_skin_vip_gift|snowman|pine.png`) dựng cùng script, không theo dáng chậu tròn (hộp quà bung nắp, người tuyết
đội mũ chóp chứa đất, xe trượt tuyết mạ vàng) nhưng giữ tâm mặt đất y = 32; `finish.py` thêm quầng sáng màu quanh viền (`glow`).
Dựng riêng: `node tools/render_pots/render.mjs <url> <thư mục> pot_skin_vip_gift,pot_skin_vip_snowman,pot_skin_vip_pine`.
Ánh lấp lánh / quầng chân / nhãn VIP (`vip_sparkle.png`, `vip_halo.png`, `vip_badge.png`) vẽ bằng `python3 tools/render_pots/vip_fx.py public/img/toon/farm`,
hoạt ảnh chỉ bằng CSS transform / opacity.
Mùa Noel nông trại (`public/img/toon/farm/noel/`, CC0): bản phủ tuyết của ảnh cảnh (cùng tên, cùng kích thước ảnh gốc —
`season.js` chỉ đổi đường dẫn) dựng bằng `python3 tools/render_noel/snow.py`; cây thông, người tuyết, quà, vòng hoa, dây đèn,
chấm "!" và NPC (Ông già Noel / Bác nông dân, 3 khung đứng – vẫy tay) dựng 3D bằng `tools/render_noel/` (render.mjs → finish.py →
manifest.py, xem README.txt ở đó); `noel.json` ghi kích thước hiển thị và điểm chân.

Nhiệm vụ (`public/img/quests/`, CC0 tự dựng): rương ngày / tuần (đóng, mở), mảnh ghép 4 bậc, 3 phụ kiện nâng cấp xe
(`part_piston` / `part_turbo_kit` / `part_tire_kit`, dựng lại riêng: `render.mjs … part_`), ảnh xe và đồ độc quyền chụp
từ mô hình trong game (nền trong suốt) — dựng bằng `tools/render_quests/` (xem README.txt ở đó).

Nguồn hiện tại đều CC0 (Kenney, OpenGameArt; phần tự ghép bằng PIL phát hành CC0).
Hạn chế: xe chỉ có ảnh nghiêng (chỉ chạy đường ngang); nhân vật hướng lên/xuống nhún thay vì bước; nằm = xoay 90°.

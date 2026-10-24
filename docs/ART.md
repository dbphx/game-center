# Hướng dẫn hình ảnh DCity

## Nguyên tắc

1. **Chỉ dùng ảnh raster vẽ sẵn**, không vẽ hình minh hoạ bằng SVG/CSS. Người dùng muốn hình sinh động như game,
   không phải khối hình phẳng / 3D.
2. **Một phong cách thống nhất** cho cả thành phố — không trộn pixel art với tranh vector trong cùng một cảnh.
   Định hướng nâng cấp (người dùng chọn): cảnh vật + nhân vật đi trên bản đồ theo phong cách **LPC** (pixel chi tiết,
   tile 32px, nhân vật 64px, 4 hướng); avatar / màn tạo nhân vật dùng **ảnh hoạt hình cỡ lớn** (Kenney Toon, CC0).
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

## Hiện trạng

| Chỗ | Hình |
|---|---|
| `public/city.js` bản đồ | canvas + ảnh `public/img/city/` (Kenney Roguelike Modern City / RPG Urban + vài hình tự vẽ cùng bảng màu: thuyền, cầu, biển CASINO, bệnh viện, xe cứu thương) |
| `public/character.js` nhân vật | sprite `public/img/characters/` (RPG Urban, 2 giới tính × 6 trang phục, 4 hướng, bước đi, nằm, nháy đỏ) |
| `public/tienlen/` sảnh casino + bàn chơi | `public/img/casino/` (Roguelike Indoors) |
| `public/farm/` nông trại | `public/img/farm/` (Tiny Farm / Tiny Town / RPG Urban / Fish Pack) |
| `public/icons.js` | icon giao diện SVG nhỏ trong nút — ngoại lệ, giữ |

Hình còn tạm (thiếu trong gói Kenney): heo (bò đổi màu), lúa (lúa mì), dưa hấu (bắp cải), dâu tây (bụi quả mọng), cá vẽ đơn giản.
Muốn thay thì vẽ pixel PNG cùng phong cách và cập nhật manifest.

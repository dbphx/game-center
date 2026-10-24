# Hướng dẫn hình ảnh DCity

## Nguyên tắc

1. **Chỉ dùng ảnh raster vẽ sẵn**, không vẽ hình minh hoạ bằng SVG/CSS. Người dùng muốn hình sinh động như game,
   không phải khối hình phẳng / 3D.
2. **Một phong cách thống nhất** cho cả thành phố — không trộn pixel art với tranh vector trong cùng một cảnh.
3. **Nguồn CC0** (Kenney: https://kenney.nl/assets). Ghi nguồn trong `LICENSE.txt` cạnh file ảnh.
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

## Hiện trạng (nợ cần thay)

| Chỗ | Đang là | Cần thay bằng |
|---|---|---|
| `public/city.js` | toà nhà, đường, sông, xe, cây vẽ SVG | tile + sprite Kenney |
| `public/character.js` | nhân vật chibi vẽ SVG (chính diện, avatar, nhìn từ trên) | sprite nhân vật theo giới tính / trang phục |
| `public/tienlen/` sảnh casino | thảm, bàn, ghế vẽ CSS (đã gom vào `art.table()` / `art.chair()`, tự dùng ảnh khi có manifest) | ảnh trong `public/img/casino/` |
| `public/farm/` | (đang làm) | ảnh trong `public/img/farm/` |
| `public/icons.js` | icon giao diện SVG | ngoại lệ — giữ, trừ khi người dùng yêu cầu |

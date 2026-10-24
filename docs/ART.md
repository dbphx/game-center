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

Nguồn hiện tại đều CC0 (Kenney, OpenGameArt; phần tự ghép bằng PIL phát hành CC0).
Hạn chế: xe chỉ có ảnh nghiêng (chỉ chạy đường ngang); nhân vật hướng lên/xuống nhún thay vì bước; nằm = xoay 90°.

# DCity — hướng dẫn cho Claude / agent

Thành phố game online (Go server + web thuần). Đọc README.md để biết cấu trúc, API, luật chơi.

## Quy tắc hình ảnh (BẮT BUỘC)

Người dùng **không muốn hình khối vẽ bằng SVG/CSS** (hình phẳng, khối 3D) cho bất kỳ cảnh vật nào.
Mọi hình minh hoạ — bản đồ, toà nhà, đường, xe, cây, sông, thuyền, bàn ghế casino, nông trại (ô đất, cây trồng,
chuồng, con vật, ao, cá), nhân vật — phải dùng **ảnh raster vẽ sẵn (PNG) từ bộ hình CC0** (Kenney), hiển thị bằng
`<img>` / `background-image` / `<image>`.

- Không viết thêm hình minh hoạ bằng `<svg>` path/rect/circle hay CSS gradient. Code SVG minh hoạ cũ
  (`public/city.js`, `public/character.js`, phần vẽ trong `public/tienlen/`, `public/farm/`) là **nợ cần thay** dần bằng ảnh.
- Ngoại lệ còn giữ: icon giao diện nhỏ trong nút/menu (`public/icons.js`) và logo — chưa có yêu cầu thay.
- Chi tiết nguồn ảnh, cách đặt file, manifest, phóng to pixel art: xem [docs/ART.md](docs/ART.md).

## Làm việc

- Test: `go test -race ./...`; kiểm tra giao diện bằng trình duyệt thật (playwright-core + Chrome), tự xem ảnh chụp.
- Comment và chữ hiển thị bằng tiếng Việt, theo phong cách code hiện có.
- Server dev của người dùng chạy ở cổng 8090 với `data/gamehub.db` — không tắt / ghi đè khi thử nghiệm; dùng cổng
  và database riêng trong thư mục tạm.

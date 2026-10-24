# Tiến Lên Miền Nam online

Game bài Tiến Lên Miền Nam chơi online qua trình duyệt, **2–4 người**. Thiếu người thì chủ phòng thêm máy.

- Server Go giữ bài và kiểm tra mọi nước đi, nên không ai xem được bài người khác.
- Giao diện web được nhúng thẳng vào binary, chỉ cần chạy một file.

## Chạy

```
go run .
```

Mở http://localhost:8080. Server in ra cả địa chỉ LAN (ví dụ `http://192.168.1.10:8080`) để bạn bè cùng mạng wifi vào được.
Đổi cổng: `PORT=3000 go run .` hoặc `go run . -addr :3000`.

Muốn chơi với bạn ở xa thì đưa server lên mạng: chạy `docker build -t tienlen . && docker run -p 8080:8080 tienlen` trên VPS,
hoặc deploy lên Fly.io / Render / Railway bằng Dockerfile có sẵn, hoặc mở tạm bằng tunnel (`cloudflared tunnel --url http://localhost:8080`, `ngrok http 8080`).

## Cách chơi

1. Nhập tên → **Tạo phòng mới**. Bạn nhận mã phòng 4 chữ và một link để gửi cho bạn bè.
2. Bạn bè mở link (hoặc nhập mã phòng) → **Vào phòng**.
3. Chủ phòng chọn **mức cược**, có thể **Thêm máy** hoặc **Mời ra**, rồi bấm **Bắt đầu** khi có 2–4 người.
4. Bấm vào lá bài để chọn, **Đánh** (Enter), **Bỏ lượt** (Space), **Gợi ý** để máy chọn giúp.

- Mỗi lượt có 30 giây, hết giờ thì máy đánh thay.
- Mất mạng hoặc tải lại trang thì vẫn giữ chỗ; trong lúc vắng mặt, máy đánh thay.
- Rời phòng giữa ván thì máy đánh nốt ván đó thay bạn.
- Ván đang chơi thì không vào thêm được; đợi hết ván.

## Luật

- Thứ tự: 3 < 4 < … < K < A < 2; chất ♠ < ♣ < ♦ < ♥.
- Bộ bài: rác, đôi, sám cô, sảnh (≥3 lá, không có 2), đôi thông (≥3 đôi liên tiếp), tứ quý.
- Chặn: cùng loại, cùng số lá, lá cao nhất lớn hơn.
- Chặt: 3 đôi thông chặt heo; tứ quý chặt heo, đôi heo, 3 đôi thông; 4 đôi thông chặt heo, đôi heo, 3 đôi thông, tứ quý.
- Bỏ lượt là mất quyền đánh tới hết vòng. Người về trước mà không ai chặn thì người kế tiếp được đánh.
- Mỗi người 13 lá; chơi ít hơn 4 người thì phần bài còn lại để riêng.
- Ván đầu: ai giữ lá nhỏ nhất (đủ 4 người là 3♠) đánh trước và phải đánh kèm lá đó. Các ván sau: người về nhất đánh trước.
- Tới trắng: tứ quý heo, sảnh rồng, 6 đôi, 5 đôi thông.

## Xu ảo và tiền cược

Xu chỉ để chơi cho vui, không nạp/rút tiền thật. Mỗi người vào phòng có **1.000 xu**. Chủ phòng chọn mức cược 10–500 xu/mức giữa các ván.
Mọi khoản dưới đây tính bằng *mức*, nhân với mức cược ra số xu:

| Khoản | Mức |
|---|---|
| Thứ hạng, 4 người | Nhất +3, Nhì +1, Ba −1, Bét −3 |
| Thứ hạng, 3 người | +2 / 0 / −2 |
| Thứ hạng, 2 người | +1 / −1 |
| Tới trắng | người thắng +2 × số người thua, mỗi người thua −2 |
| Heo đen ♠♣ / heo đỏ ♦♥ | 1 / 2 |
| 3 đôi thông / tứ quý / 4 đôi thông | 3 / 4 / 6 |

- **Chặt heo / chặt hàng**: người bị chặt trả người chặt theo bảng trên. **Chặt chồng**: người bị chặt sau trả luôn số tiền mình vừa ăn, nên người chặt cuối ăn trọn.
- **Thối**: hết ván, người về bét còn heo / hàng trên tay thì trả cho người về nhất.
- **Cóng**: khi có người về nhất mà ai đó chưa đánh lá nào thì người đó bị cóng: xếp bét, bị loại khỏi ván và cũng phải thối cả tay.
- Không đủ xu cho mức cược thì không vào ván được; bấm **nhận lại 1.000 xu** trong phòng chờ (số lần nhận lại hiện cho cả phòng thấy). Máy hết xu được cấp lại tự động.

## Cấu trúc

| Thư mục | Nội dung |
|---|---|
| `game/` | Luật, luồng ván, bot (Go) |
| `room/` | Phòng chơi: vào/ra, chủ phòng, máy, hẹn giờ lượt, mất kết nối |
| `main.go` | HTTP + WebSocket, nhúng `public/` |
| `public/` | Giao diện web (HTML/CSS/JS thuần) |

Giao thức WebSocket (`/ws`, JSON): client gửi `create`, `join`, `resume`, `addBot`, `kick`, `start`, `play`, `pass`, `hint`, `leave`;
server gửi `session`, `state` (riêng cho từng người, chỉ có bài của chính họ), `hint`, `error`, `left`.

## Test

```
go test -race ./...
```

Gồm test luật, giả lập 500 ván bot cho mỗi số người 2/3/4, test phòng chơi, và một ván trọn vẹn qua WebSocket thật.

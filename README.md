# Game Hub

Tổ hợp game online chơi qua trình duyệt, **dùng chung một ví xu** cho mọi game. Game đầu tiên: **Tiến Lên Miền Nam** (2–4 người, thiếu người thì thêm máy).

- Server Go giữ bài và kiểm tra mọi nước đi; giao diện web được nhúng thẳng vào binary.
- Tài khoản và ví lưu trong SQLite (driver Go thuần, không cần cgo), mọi thay đổi số dư đều ghi sổ cái.

## Chạy

```
go run .
```

Mở http://localhost:8080. Server in ra cả địa chỉ LAN để bạn bè cùng wifi vào được.

| Tuỳ chọn | Mặc định | |
|---|---|---|
| `-addr` / `PORT` | `:8080` | địa chỉ lắng nghe |
| `-db` / `DB_PATH` | `data/gamehub.db` | file SQLite lưu tài khoản và ví |

Đưa lên mạng để bạn ở xa chơi: `docker build -t gamehub . && docker run -p 8080:8080 -v gamehub-data:/data gamehub`
(nhớ gắn volume `/data` để không mất ví), hoặc deploy Dockerfile lên Fly.io / Render / Railway, hoặc mở tạm bằng tunnel
(`cloudflared tunnel --url http://localhost:8080`).

## Tài khoản và xu

- Lần đầu vào chỉ cần nhập tên: tạo **tài khoản khách**, trình duyệt nhớ tài khoản (localStorage). Mọi tab cùng trình duyệt là cùng một người,
  nên muốn tự thử 2 người trên một máy thì mở thêm cửa sổ ẩn danh hoặc trình duyệt khác.
- Tài khoản mới được **1.000 xu**, mỗi ngày nhận thêm **500 xu** (reset 0h giờ Việt Nam).
- Số dư không bao giờ âm: thua quá số đang có thì chỉ mất hết số đang có.
- Chơi với máy thì máy đóng vai nhà cái: thắng máy được xu thật vào ví, thua thì mất.
- Trang hub có lịch sử xu (sổ cái) để xem xu đến / đi từ đâu.
- Xu chỉ để chơi cho vui, không nạp / rút tiền thật.

## Tiến Lên Miền Nam

1. Ở hub chọn **Tiến Lên Miền Nam** → **Tạo phòng mới**, gửi link phòng cho bạn bè. Bạn bè mở link, tạo tài khoản (nếu chưa có) là vào thẳng phòng.
2. Chủ phòng chọn **mức cược**, **Thêm máy** / **Mời ra**, rồi **Bắt đầu** khi có 2–4 người. Ai không đủ xu cho mức cược thì không bắt đầu được.
3. Bấm lá bài để chọn, **Đánh** (Enter), **Bỏ lượt** (Space), **Gợi ý** để máy chọn giúp.

- Mỗi lượt 30 giây, hết giờ máy đánh thay. Mất mạng / tải lại trang vẫn giữ ghế (vào lại bằng link phòng), trong lúc vắng máy đánh thay.
- Rời phòng giữa ván thì máy đánh nốt ván, kết quả vẫn tính vào ví người rời.

### Luật

- Thứ tự: 3 < 4 < … < K < A < 2; chất ♠ < ♣ < ♦ < ♥.
- Bộ bài: rác, đôi, sám cô, sảnh (≥3 lá, không có 2), đôi thông (≥3 đôi liên tiếp), tứ quý.
- Chặn: cùng loại, cùng số lá, lá cao nhất lớn hơn.
- Chặt: 3 đôi thông chặt heo; tứ quý chặt heo, đôi heo, 3 đôi thông; 4 đôi thông chặt heo, đôi heo, 3 đôi thông, tứ quý.
- Bỏ lượt là mất quyền đánh tới hết vòng. Người về trước mà không ai chặn thì người kế tiếp được đánh.
- Mỗi người 13 lá; chơi ít hơn 4 người thì phần bài còn lại để riêng.
- Ván đầu: ai giữ lá nhỏ nhất (đủ 4 người là 3♠) đánh trước và phải đánh kèm lá đó. Các ván sau: người về nhất đánh trước.
- Tới trắng: tứ quý heo, sảnh rồng, 6 đôi, 5 đôi thông.

### Tiền cược

Mọi khoản tính bằng *mức*, nhân với mức cược (10–500 xu/mức, chủ phòng chọn) ra số xu:

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

## Cấu trúc

```
main.go                  khởi động server, đăng ký danh sách game
internal/wallet/         tài khoản khách, ví xu, sổ cái (SQLite)
internal/server/         API hub (/api/...), WebSocket từng game (/ws/<game>), file tĩnh
games/tienlen/           luật, luồng ván, tiền cược, bot
games/tienlen/online/    phòng chơi online: người vào/ra, máy, hẹn giờ, trả thưởng vào ví
public/                  trang hub (index.html, hub.js), phần dùng chung (account.js, common.css)
public/tienlen/          giao diện Tiến Lên
```

### Thêm một game mới

1. Viết logic và server của game trong `games/<id>/`. Game nhận người chơi qua WebSocket, xác thực bằng token tài khoản
   (xem `online.Handler` của Tiến Lên), và cộng / trừ xu qua `wallet.Settle(game, ghi chú, map[userID]số xu)` — một transaction, có ghi sổ cái.
2. Làm giao diện trong `public/<id>/`, dùng `/account.js` (`Account.me()`, `Account.token()`, `Account.requireLogin()`) và `/common.css`.
3. Đăng ký game trong `games()` ở `main.go` (tên, mô tả, đường dẫn trang, handler WebSocket, số người đang chơi). Trang hub tự hiện game mới.

### API

| | |
|---|---|
| `POST /api/guest {name}` | tạo tài khoản khách → `{token, user}` |
| `GET /api/me` | thông tin + số dư (header `Authorization: Bearer <token>`) |
| `POST /api/me {name}` | đổi tên |
| `POST /api/daily` | nhận thưởng ngày (409 nếu đã nhận) |
| `GET /api/history` | 30 giao dịch gần nhất |
| `GET /api/games` | danh sách game và số người đang chơi |
| `GET /api/config` | số xu tặng ban đầu, thưởng ngày |

WebSocket Tiến Lên (`/ws/tienlen`, JSON): client gửi `create {token}`, `join {code, token}`, `addBot`, `kick`, `setBet`, `start`, `play`, `pass`, `hint`, `leave`;
server gửi `session`, `state` (riêng từng người, chỉ có bài của chính họ), `hint`, `error`, `left`, `auth`.

## Test

```
go test -race ./...
```

Gồm: luật, tiền chặt / thối / cóng, giả lập 500 ván bot cho mỗi số người 2/3/4, phòng chơi, ví SQLite (thưởng ngày, số dư không âm,
rollback), API hub, và một ván trọn vẹn qua WebSocket thật có ghi kết quả vào ví.

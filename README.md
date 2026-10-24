# DCity

Thành phố game online chơi qua trình duyệt. Mỗi người có một **nhân vật** (giới tính, trang phục, level riêng) dạo quanh
bản đồ thành phố **nhìn từ trên cao xuống** (lưới phố, xe chạy, mái nhà đổ bóng, con sông chảy ngang giữa thành phố có cầu và thuyền);
mỗi khu là một game hoặc tiện ích, tất cả **dùng chung một ví xu**.

| Khu | |
|---|---|
| Casino | **Tiến Lên Miền Nam** (2–4 người, thiếu người thì thêm máy) — sảnh có bàn ghế để chọn bàn / tạo bàn |
| Bệnh viện | hồi máu, nằm viện khi hết HP |
| Ngân hàng | số dư, thưởng hằng ngày, lịch sử xu |
| Shop thời trang | mua quần áo, tóc, cánh theo level; mặc lại đồ đã mua |
| Quảng trường | thông tin nhân vật, level, XP |
| Nông trại | trồng trọt (6 ô miễn phí, mua thêm tới 48 ô), chuồng gia súc và ao cá theo đơn vị sức chứa, kho giống và kho sản phẩm |
| Công viên ven sông, bến thuyền | trang trí; đi qua cầu để sang bờ Nam |

- Điều khiển nhân vật: **WASD / phím mũi tên** đi tự do (không đi xuyên sông, toà nhà), **bấm chuột** vào chỗ bất kỳ
  để tự tìm đường tới, **E / Enter** khi đứng trước cửa để vào khu; điện thoại có cần điều khiển ảo.
  Thanh đi nhanh phía dưới cho nhân vật tự đi tới khu.
- Hình ảnh: pixel art CC0 (Kenney) — xem [docs/ART.md](docs/ART.md); không vẽ hình minh hoạ bằng SVG/CSS.

## Máu (HP) và Bệnh viện

- Nhân vật có 100 HP, tự hồi 1 HP/phút. **Bị xe tông −20 HP.**
- Hết HP → **nằm viện 5 phút** (bị đưa tới Bệnh viện, không đi / vào khu được), hồi dần tới đầy; trả 200 xu để xuất viện ngay.
  Khi khoẻ có thể chữa nhanh ở Bệnh viện: 2 xu mỗi HP thiếu.
- HP tối thiểu để vào khu (server cũng chặn): Shop 10, Casino 30, Nông trại 40; Quảng trường, Ngân hàng, Bệnh viện 0.
- Server Go giữ bài và kiểm tra mọi nước đi; giao diện web (HTML/CSS/JS thuần, hình vẽ SVG) được nhúng thẳng vào binary.
- Tài khoản, nhân vật và ví lưu trong SQLite (driver Go thuần, không cần cgo), mọi thay đổi số dư đều ghi sổ cái.

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

- Đăng ký bằng **tên đăng nhập + mật khẩu** (tên đăng nhập 3–20 ký tự: chữ không dấu, số, `_`, `.`; không phân biệt hoa thường; mật khẩu ≥ 6 ký tự, băm bcrypt).
  Mỗi lần đăng nhập là một phiên riêng, đăng nhập được trên nhiều máy; sai mật khẩu 5 lần trong 15 phút thì tạm khoá tên đăng nhập đó.
- Tài khoản khách từ bản cũ vẫn chơi tiếp được và có nút đặt tên đăng nhập + mật khẩu, giữ nguyên xu. Database cũ được nâng cấp tự động khi khởi động.
- Tài khoản mới được **1.000 xu**, mỗi ngày nhận thêm **500 xu** (reset 0h giờ Việt Nam).
- Số dư không bao giờ âm: thua quá số đang có thì chỉ mất hết số đang có.
- Chơi với máy thì máy đóng vai nhà cái: thắng máy được xu thật vào ví, thua thì mất.
- Ngân hàng có lịch sử xu (sổ cái) để xem xu đến / đi từ đâu.

## Nhân vật, level và trang phục

- Lúc đăng ký chọn **giới tính** (Nam / Nữ, **không đổi được về sau**) và một bộ **trang phục khởi đầu** (Thường ngày hoặc Thể thao).
- Mỗi tài khoản là một nhân vật có **level riêng**, lên level bằng XP kiếm được khi chơi.
  Tiến Lên: hết mỗi ván người về nhất +30 XP, người bét +10 XP (ở giữa chia đều).
- Level n cần tổng 50·n·(n−1) XP: level 2 ở 100 XP, level 3 ở 300, level 4 ở 600...
- **Shop thời trang**: trang phục khác phải **đủ level mới mua được, trả bằng xu**; đồ đã mua mặc lại lúc nào cũng được.

| Trang phục | Level | Giá |
|---|---|---|
| Thường ngày, Thể thao | 1 | miễn phí (khởi đầu) |
| Áo hoodie | 2 | 800 xu |
| Vest lịch lãm | 3 | 1.500 xu |
| Áo dài | 4 | 2.500 xu |
| Hoàng gia (có vương miện) | 6 | 5.000 xu |

- Shop còn bán **tóc** và **cánh** (dùng chung cả hai giới). Ai cũng có sẵn tóc mặc định; cánh có thể tháo ra bất cứ lúc nào.

| Tóc | Level | Giá | | Cánh | Level | Giá |
|---|---|---|---|---|---|---|
| Tóc mặc định | 1 | có sẵn | | Cánh thiên thần | 3 | 2.500 xu |
| Tóc dựng, Đuôi ngựa | 1 | 300 xu | | Cánh tiên | 4 | 3.500 xu |
| Tóc dài xoăn, Tóc búi | 2 | 700 xu | | Cánh bướm | 5 | 4.500 xu |
| Mohican | 3 | 1.200 xu | | Cánh dơi | 6 | 6.000 xu |
| Hai bím hồng | 4 | 2.000 xu | | Cánh rồng | 8 | 10.000 xu |

- Trong game, avatar của mỗi người là mặt nhân vật của họ kèm level.

## Nông trại

- 6 ô đất miễn phí, ô thứ 7 giá 10.000 xu, mỗi ô sau +2.000 xu, tối đa 48 ô. Cây có thời gian chín, level tối thiểu.
- **Chuồng** (heo 1, cừu / bò 2 đơn vị...) và **ao cá** (koi 3 đơn vị...): mặc định 5 đơn vị, mở rộng +5 tới 30.
- **Kho giống**: mua hạt theo số lượng (1–100 hạt/lần) cất vào kho; gieo hạt lấy từ kho, hết hạt thì tự mua 1 hạt.
  Sức chứa 50, mở rộng +25 tới 300, lần mở rộng thứ n giá 1.000 × n xu.
- **Kho sản phẩm**: thu hoạch cây và thu vật nuôi / cá đã lớn → vào kho (cộng XP lúc thu); bán ở kho mới được xu.
  Sức chứa 50, mở rộng +25 tới 500, lần thứ n giá 1.500 × n xu. Kho đầy thì không thu hoạch được.

| API (cần token, POST trả `{farm, user, gained?}`) | Body |
|---|---|
| `GET /api/farm/catalog` (không cần token), `GET /api/farm` | |
| `POST /api/farm/plots/buy` | |
| `POST /api/farm/seeds/buy` | `{crop, qty}` |
| `POST /api/farm/plant` | `{plot, crop}` |
| `POST /api/farm/harvest`, `/harvest-all` | `{plot}` / — |
| `POST /api/farm/{barn,pond}/buy`, `/{barn,pond}/expand` | |
| `POST /api/farm/{animals,fish}/buy` | `{kind}` |
| `POST /api/farm/{animals,fish}/collect` (vào kho), `/sell` (bán thẳng) | `{id}` |
| `POST /api/farm/products/sell` (qty 0 = bán hết món đó), `/products/sell-all` | `{item, qty}` / — |
| `POST /api/farm/storage/{seeds,products}/expand` | |

Mã lỗi: 400 dữ liệu sai, 402 không đủ xu, 403 thiếu level / HP, 404 không tìm thấy, 409 trùng / đầy / chưa chín, 423 đang nằm viện.

## Bầu Cua Tôm Cá

Một sòng chung trong khu Casino, máy làm nhà cái. Mỗi vòng 30 giây: **đặt cược 20 s → lắc 4 s → mở bát 6 s**.

- 6 mặt `bau, cua, tom, ca, ga, nai`; 3 xúc xắc. Chip 10 / 50 / 100 / 500 / 1.000 xu, tổng tối đa 20.000 xu
  một người một vòng; đặt nhiều lần, nhiều mặt được. Xu trừ ngay lúc đặt; huỷ cược (trong giờ cược) thì hoàn.
- Mặt đã đặt ra k lần (k ≥ 1) → nhận lại vốn + k × cược; không ra thì mất cược. Mỗi vòng có đặt +2 XP, có lời thêm +3.
- Kết quả quay và trả thưởng đúng một lần ở request đầu tiên sau giờ cược. Cược còn treo khi server tắt được hoàn
  lúc khởi động lại. Đặt / huỷ cần đủ 30 HP và không nằm viện (như Casino).

| API | Body |
|---|---|
| `GET /api/baucua/state` (token tuỳ chọn) | |
| `POST /api/baucua/bet` | `{face, amount}` |
| `POST /api/baucua/clear` | |

`state` trả `{round, phase: betting|rolling|result, endsAt, now (ms), dice (chỉ khi result), totals, players,
mine: {bets, total, won?, net?}, history (10 vòng, mới nhất trước), chips, maxTotal, faces, balance?}`.
POST trả cùng dạng. Mã lỗi: 400 mặt / chip sai, 402 không đủ xu, 409 hết giờ cược / vượt tối đa, 403 / 423 HP.

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
games/farm/              nông trại: ô đất, chuồng, ao, kho
games/baucua/            bầu cua: sòng chung, vòng theo đồng hồ, trả thưởng vào ví
public/                  bản đồ thành phố (index.html, hub.js, city.js), nhân vật (character.js),
                         phần dùng chung (account.js, icons.js, common.css)
public/tienlen/          giao diện Tiến Lên
```

### Thêm một game mới

1. Viết logic và server của game trong `games/<id>/`. Game nhận người chơi qua WebSocket, xác thực bằng token tài khoản
   (xem `online.Handler` của Tiến Lên), và cộng / trừ xu qua `wallet.Settle(game, ghi chú, map[userID]số xu)` — một transaction, có ghi sổ cái.
2. Làm giao diện trong `public/<id>/`, dùng `/account.js` (`Account.me()`, `Account.token()`, `Account.requireLogin()`), `/icons.js` (icon SVG, avatar) và `/common.css`.
3. Đăng ký game trong `games()` ở `main.go` (tên, mô tả, đường dẫn trang, handler WebSocket, số người đang chơi).
4. Đặt game vào một khu trên bản đồ: thêm khu trong `ZONES` / `NODES` / `EDGES` và hàm vẽ tòa nhà ở `public/city.js`
   (ví dụ thay Công trường hoặc Nông trại), rồi thêm bảng thông tin của khu trong `PANELS` ở `public/hub.js`.
5. Cộng XP qua `wallet.AwardXP(map[userID]xp)` để người chơi lên level.

### API

| | |
|---|---|
| `POST /api/register {username, password, name, gender, outfit}` | đăng ký + tạo nhân vật → `{token, user}` |
| `POST /api/login {username, password}` | đăng nhập → `{token, user}` |
| `POST /api/logout` | đăng xuất phiên hiện tại |
| `POST /api/password {old, new}` | đổi mật khẩu |
| `POST /api/credentials {username, password}` | tài khoản khách cũ đặt tên đăng nhập |
| `POST /api/character {outfit}` | mặc trang phục đã có |
| `POST /api/outfits/buy {outfit}` | mua trang phục (403 thiếu level, 402 thiếu xu, 409 đã có) |
| `POST /api/me/items/buy {kind, id}` | mua quần áo / tóc / cánh (`kind`: `outfit`, `hair`, `wings`) và mặc luôn; lỗi như trên, 400 món lạ |
| `POST /api/me/items/wear {kind, id}` | mặc / đeo món đã có (403 chưa có); tóc `default` luôn được, cánh `""` là tháo cánh |
| `GET /api/me` | thông tin + số dư, nhân vật (`outfit`, `owned`, `hair`, `ownedHair`, `wings`, `ownedWings`) (header `Authorization: Bearer <token>`) |
| `POST /api/me {name}` | đổi tên |
| `POST /api/daily` | nhận thưởng ngày (409 nếu đã nhận) |
| `GET /api/history` | 30 giao dịch gần nhất |
| `GET /api/games` | danh sách game và số người đang chơi |
| `GET /api/config` | số xu tặng ban đầu, thưởng ngày, danh mục trang phục (`catalog`), tóc (`hair`), cánh (`wings`) |

WebSocket Tiến Lên (`/ws/tienlen`, JSON): client gửi `create {token}`, `join {code, token}`, `addBot`, `kick`, `setBet`, `start`, `play`, `pass`, `hint`, `leave`;
server gửi `session`, `state` (riêng từng người, chỉ có bài của chính họ), `hint`, `error`, `left`, `auth`.

## Test

```
go test -race ./...
```

Gồm: luật, tiền chặt / thối / cóng, giả lập 500 ván bot cho mỗi số người 2/3/4, phòng chơi, ví SQLite (thưởng ngày, số dư không âm,
rollback), API hub, và một ván trọn vẹn qua WebSocket thật có ghi kết quả vào ví.

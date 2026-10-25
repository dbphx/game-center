# DCity

Thành phố game online chơi qua trình duyệt. Mỗi người có một **nhân vật** (giới tính, trang phục, level riêng) dạo quanh
bản đồ thành phố **nhìn từ trên cao xuống** (lưới phố, xe chạy, mái nhà đổ bóng, con sông chảy ngang giữa thành phố có cầu và thuyền);
mỗi khu là một game hoặc tiện ích, tất cả **dùng chung một ví xu**.

| Khu | |
|---|---|
| Casino | **Tiến Lên Miền Nam** (2–4 người, thiếu người thì thêm máy) — sảnh có bàn ghế để chọn bàn / tạo bàn |
| Bệnh viện | hồi máu, nằm viện khi hết HP |
| Ngân hàng | số dư, thưởng hằng ngày, lịch sử xu |
| Shop thời trang | mua quần áo, tóc, cánh, gương mặt theo level; mặc lại đồ đã mua |
| Quảng trường | thông tin nhân vật, level, XP |
| Nông trại | trồng trọt (6 ô miễn phí, mua thêm tới 48 ô), chuồng gia súc và ao cá theo đơn vị sức chứa, kho giống và kho sản phẩm |
| Công viên ven sông, bến thuyền | trang trí; đi qua cầu để sang bờ Nam |

- Điều khiển nhân vật: **WASD / phím mũi tên** đi tự do (không đi xuyên sông, toà nhà), **bấm chuột** vào chỗ bất kỳ
  để tự tìm đường tới, **E / Enter** khi đứng trước cửa để vào khu; điện thoại có cần điều khiển ảo.
  Thanh đi nhanh phía dưới cho nhân vật tự đi tới khu.
- Hình ảnh: pixel art CC0 (Kenney) — xem [docs/ART.md](docs/ART.md); không vẽ hình minh hoạ bằng SVG/CSS.

## Máu (HP) và Bệnh viện

- Nhân vật có 100 HP, tự hồi 1 HP/phút. **Bị xe tông −20 HP.**
- **Ngồi ghế** hồi máu nhanh gấp đôi (1 HP / 30 giây); bị xe tông thì đứng dậy, đang nằm viện thì không ngồi được.
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
| Đồng phục học sinh | 1 | 600 xu |
| Áo bà ba | 2 | 1.200 xu |
| Áo Tết | 3 | 2.000 xu |
| Hải tặc | 5 | 4.000 xu |
| Đồ cưới | 7 | 8.000 xu |
| Phi hành gia | 9 | 12.000 xu |

- Shop còn bán **tóc** và **cánh** (dùng chung cả hai giới). Ai cũng có sẵn tóc mặc định; cánh có thể tháo ra bất cứ lúc nào.

| Tóc | Level | Giá | | Cánh | Level | Giá |
|---|---|---|---|---|---|---|
| Tóc mặc định | 1 | có sẵn | | Cánh thiên thần | 3 | 2.500 xu |
| Tóc dựng, Đuôi ngựa | 1 | 300 xu | | Cánh tiên | 4 | 3.500 xu |
| Tóc dài xoăn, Tóc búi | 2 | 700 xu | | Cánh bướm | 5 | 4.500 xu |
| Mohican | 3 | 1.200 xu | | Cánh dơi | 6 | 6.000 xu |
| Hai bím hồng | 4 | 2.000 xu | | Cánh rồng | 8 | 10.000 xu |

- **Gương mặt** (dùng chung hai giới, ai cũng có sẵn mặt mặc định):

| Gương mặt | Level | Giá |
|---|---|---|
| Mặt mặc định | 1 | có sẵn |
| Cười tươi | 1 | 200 xu |
| Nháy mắt | 1 | 300 xu |
| Má hồng | 2 | 600 xu |
| Kính râm | 2 | 800 xu |
| Mèo con | 3 | 1.200 xu |
| Mắt lấp lánh | 4 | 2.000 xu |

- Trong game, avatar của mỗi người là mặt nhân vật của họ kèm level.

## Nông trại

- 6 ô đất miễn phí, ô thứ 7 giá 10.000 xu, mỗi ô sau +2.000 xu, tối đa 48 ô. Cây có thời gian chín, level tối thiểu.
- **Chuồng** (heo 1, cừu / bò 2 đơn vị) và **ao cá** (koi 3 đơn vị...): đều mở từ level 1, mặc định 5 đơn vị, mở rộng +5 tới 30.
- **Sân gia cầm**: gà, vịt (1 đơn vị) không ở chuồng mà đi quanh trang trại. Có sẵn cho mọi người, mặc định 10 đơn vị,
  mở rộng +5 tới 40, lần thứ n giá 1.000 × n xu. Gà vịt trong chuồng từ bản cũ được chuyển ra sân khi khởi động.
- **Cho ăn**: con chưa lớn đói lại sau 10 phút kể từ lúc mua / lần ăn trước. Cho ăn tốn 1/10 giá mua (ít nhất 5 xu),
  +1 XP và rút ngắn 25% thời gian còn lại tới lúc lớn.
- **Sản phẩm vật nuôi**: con đã lớn cứ mỗi chu kỳ cho 1 món (để lâu vẫn chỉ 1 món chờ lấy), lấy vào kho sản phẩm;
  hoặc **lấy thịt** (con vật mất, được thịt vào kho). Cá thu nguyên con như cũ.

| Con | Sản phẩm (chu kỳ, giá, XP) | Thịt (giá, XP) |
|---|---|---|
| Gà | Trứng gà — 10 phút, 60 xu, 2 XP | 400 xu, 5 XP |
| Vịt | Trứng vịt — 15 phút, 80 xu, 2 XP | 520 xu, 6 XP |
| Heo | — | 1.300 xu, 12 XP |
| Cừu | Lông cừu — 30 phút, 400 xu, 6 XP | 2.000 xu, 20 XP |
| Bò | Sữa bò — 20 phút, 300 xu, 5 XP | 3.400 xu, 35 XP |

- **Làm nhanh**: gieo hạt mọi ô trống (dùng hạt trong kho trước, thiếu thì mua tới khi hết xu), thu hoạch tất cả,
  cho mọi con đói ăn (tới khi hết xu), lấy hết trứng / sữa / lông, thu hết cá đã lớn (tới khi kho đầy).
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
| `POST /api/farm/{barn,pond}/buy`, `/{barn,yard,pond}/expand` | |
| `POST /api/farm/{animals,poultry,fish}/buy` (chuồng / sân / ao) | `{kind}` |
| `POST /api/farm/{animals,poultry,fish}/feed` (cho ăn), `/collect` (vật nuôi, gia cầm: lấy thịt; cá: vào kho), `/sell` (bán thẳng) | `{id}` |
| `POST /api/farm/{animals,poultry}/produce` (lấy trứng / sữa / lông vào kho) | `{id}` |
| `POST /api/farm/plant-all`, `/feed-all`, `/produce-all`, `/fish/collect-all` (làm nhanh; `gained.spent` = xu đã tiêu) | `{crop}` / — |
| `POST /api/farm/products/sell` (qty 0 = bán hết món đó), `/products/sell-all` | `{item, qty}` / — |
| `POST /api/farm/storage/{seeds,products}/expand` | |

Mã lỗi: 400 dữ liệu sai, 402 không đủ xu, 403 thiếu level / HP, 404 không tìm thấy, 409 trùng / đầy / chưa chín, 423 đang nằm viện.

## Bầu Cua Tôm Cá

Trong khu Casino. Có **sòng chung** (mã `chung`, máy luôn làm cái) và **bàn riêng** người chơi tự mở để chơi với
bạn (mã 6 ký tự, link mời `/baucua/?room=MÃ`). Mỗi bàn có xúc xắc và lịch sử riêng; mọi bàn chạy cùng nhịp
30 giây: **đặt cược 20 s → lắc 4 s → mở bát 6 s**.

- 6 mặt `bau, cua, tom, ca, ga, nai`; 3 xúc xắc. Chip 10 / 50 / 100 / 500 / 1.000 xu, tổng tối đa 20.000 xu
  một người một vòng; đặt nhiều lần, nhiều mặt được. Xu trừ ngay lúc đặt; huỷ cược (trong giờ cược) thì hoàn.
- Mặt đã đặt ra k lần (k ≥ 1) → nhận lại vốn + k × cược; không ra thì mất cược. Trả thưởng lúc mở bát.
  Mỗi vòng có đặt +2 XP, có lời thêm +3.
- **Làm cái** (chỉ bàn riêng): nộp quỹ ≥ 3.000 xu khi bàn chưa ai làm cái và (đang cược mà chưa có cược nào, hoặc
  lúc mở bát — áp dụng từ vòng sau). Người làm cái không được đặt cược; tổng cược cả bàn mỗi vòng ≤ quỹ / 3 để luôn
  đủ trả. Tiền thua vào quỹ, tiền thắng trả từ quỹ; người làm cái +2 XP mỗi vòng có cược, +3 nếu quỹ lời.
  Thôi làm cái: hoàn quỹ về ví (ghi lãi / lỗ); còn cược chưa chốt thì thôi sau khi chốt. Tự thôi khi quỹ < 300
  hoặc vắng mặt > 60 s. Không ai làm cái thì máy làm cái.
- Có mặt ở bàn = gọi `join` hoặc poll `state` có token trong 15 s gần nhất. Bàn riêng bỏ trống 5 phút thì đóng.
- Kết quả quay và trả thưởng đúng một lần (dưới khoá, một transaction). Khởi động lại server: cược treo và quỹ
  cái được hoàn, bàn riêng mất (chỉ sống trong bộ nhớ). Mở bàn / vào bàn / đặt cược / làm cái cần đủ 30 HP và
  không nằm viện (như Casino).

| API | Body |
|---|---|
| `GET /api/baucua/state?room=MÃ` (mặc định `chung`, token tuỳ chọn) | |
| `GET /api/baucua/rooms` | |
| `POST /api/baucua/rooms` → `{code}` | `{}` |
| `POST /api/baucua/join`, `/leave` | `{room}` |
| `POST /api/baucua/bet` | `{room, face, amount}` |
| `POST /api/baucua/clear`, `/unbank` | `{room}` |
| `POST /api/baucua/bank` | `{room, bankroll}` |

`state` trả `{round, phase: betting|rolling|result, endsAt, now (ms), dice (chỉ khi result), totals, players,
mine: {bets, total, won?, net?, banker, host}, history (10 vòng, mới nhất trước), chips, maxTotal, faces, balance?,
room: {code, host, banker: null|{name, gender, outfit, hair, wings, face, level, bankroll, start, net?, leaving},
members: [{name, …, level, total}], maxRoundTotal (0 = không giới hạn), roundTotal}, invite}`.
`rooms` trả `[{code, host, banker, members, phase}]`, sòng chung đứng đầu.
Mã lỗi: 400 mặt / chip / quỹ sai, 402 không đủ xu, 404 không có bàn, 409 hết giờ cược / vượt giới hạn / đang làm cái,
403 / 423 HP.

## Cờ Caro

Khu Casino, trang `/caro/`. Bàn 15×15, X đi trước, ai có 5 quân liền (ngang / dọc / chéo) trước thì thắng.

- **Luật bàn** (chủ bàn chọn): `block2` (mặc định, luật Việt Nam: đường đúng 5 quân bị quân đối thủ chặn cả hai đầu
  thì không tính; mép bàn không tính là chặn; 6 quân trở lên vẫn thắng) hoặc `free` (5 quân trở lên là thắng).
  Hết ô mà không ai thắng là hoà.
- **Lượt** 30 giây, hết giờ là thua ván; đầu hàng là thua; rời bàn giữa ván (hoặc mất kết nối quá 30 giây) là thua.
- **2 ghế**: người hoặc máy (`easy` / `normal` / `hard`). Bàn đủ 2 người thì người vào sau được xem.
- **Cược** 0, 10, 20, 50, 100, 200, 500, 1000 xu (0 = chơi vui): người thắng ăn mức cược của người thua; hoà không đổi;
  đánh với máy thì máy là nhà cái. Ghi sổ cái game `caro`. **XP**: thắng 20, hoà 12, thua 8 (cả bàn 0 xu).
- Ván sau người thua cầm X đi trước (hoà thì đổi bên). Cần cả hai bấm "chơi lại", hoặc chủ bàn bấm bắt đầu.
- Cần đủ máu vào Casino để ngồi ghế.

WebSocket `/ws/caro`, danh sách bàn `GET /api/caro/rooms`
→ `[{code, phase, bet, rule, players:[{name, gender, outfit, hair, wings, face, level, bot}], spectators, max}]`.

| Client → server | |
|---|---|
| `{t:"create", token, bet, rule}` / `{t:"join", token, code}` | mở / vào bàn (đủ người thì vào xem) |
| `{t:"addBot", level}`, `{t:"kick", id}`, `{t:"setBet", bet}`, `{t:"setRule", rule}`, `{t:"start"}` | chủ bàn, ngoài ván |
| `{t:"move", x, y}`, `{t:"resign"}`, `{t:"rematch"}`, `{t:"leave"}` | |

Server gửi `{t:"state", code, you, phase, size, board, turn, players:[{id, name, …, bot, botLevel, host, online, mark,
coins, ready}], last:{x,y}, winLine:[[x,y]…], result:{winner, reason, money:{id:delta}, xp:{id:n}}, bet, gameBet,
betOptions, rule, turnMs, spectators, moves}` (`board` là chuỗi 225 ký tự `.`/`x`/`o` theo hàng; `you` rỗng khi đang xem;
`reason`: `five` | `timeout` | `resign` | `left` | `draw`), cùng `{t:"session", code}`, `{t:"error", msg}`, `{t:"left"}`, `{t:"auth"}`.

## Công trường (ngoại ô)

Làm thuê theo ca để kiếm xu. Nhận ca cần ≥ 30 HP; mỗi ca trừ máu ngay, giao diện chơi mini-game trong thời gian ca
rồi nộp điểm (số mục tiêu hoàn thành, tối đa `seconds / 2`).

| Việc | Level | HP | Thời gian | Tiền công (0 → đủ điểm) | XP |
|---|---|---|---|---|---|
| Khuân gạch | 1 | 8 | 30 giây | 120 → 300 | 4 |
| Trộn vữa | 3 | 10 | 30 giây | 200 → 500 | 6 |
| Lái cần cẩu | 6 | 12 | 40 giây | 400 → 900 | 10 |

- Tiền công = `basePay + (maxPay − basePay) × điểm / số mục tiêu`. Mỗi ngày (giờ Việt Nam) kiếm tối đa 6.000 xu ở công trường.
- Chống gian lận: chỉ nộp được trong khung [hết giờ − 2 giây, hết giờ + 30 giây], mỗi ca một lần; quá hạn thì ca mất (máu đã trừ).
  Hết giờ một ca phải nghỉ 20 giây mới nhận ca mới; đang có ca thì không nhận ca khác. Không được làm tới 0 HP.

| API (cần đăng nhập) | Mô tả |
|---|---|
| `GET /api/site` | `{now, jobs, today:{earned, cap}, active, cooldownUntil, user}` (thời gian là unix ms) |
| `POST /api/site/start {job}` | nhận ca: trả như trên kèm `shift:{id, job, seconds, targets, seed, startedAt, endsAt}`; 403 thiếu level / máu, 409 đang có ca / đang nghỉ / hết suất ngày, 423 nằm viện |
| `POST /api/site/finish {shiftId, score}` | nộp ca: thêm `pay`, `xp`, `score`; 409 sớm / muộn / đã nộp, 400 điểm sai, 404 ca lạ |

## Hồ câu cá (ngoại ô)

Mua mồi, thả câu, chờ cá cắn câu (3–10 giây) rồi giật và kéo cần. Câu cá cần ≥ 10 HP. Kết quả mỗi lần thả do server quay sẵn,
giao diện chỉ biết lúc cá cắn câu (`biteAt`) và độ khó; giật trước `biteAt` thì cá chạy, quá `biteAt + 1,5 giây + 8 giây`
thì cá ăn mồi mất. Mỗi lần thả mất 1 mồi dù câu được hay không.

| Mồi | Giá | Tỉ lệ (rác / thường / khá / hiếm / quý / huyền thoại) |
|---|---|---|
| Giun | 10 | 8% / 55% / 27% / 8% / 1,9% / 0,1% |
| Tép | 40 | 5% / 42% / 33% / 15% / 4,7% / 0,3% |
| Mồi đặc biệt | 150 | 3% / 30% / 36% / 22% / 8,5% / 0,5% |

Cá: rô, diếc (thường) · chép, lóc (khá) · trê, chim (hiếm) · koi (quý) · Cá Vàng Huyền Thoại; giá = cỡ (cm) × giá mỗi cm,
mồi tốt cho cá to hơn. Vớt phải rác (dép cũ, lon) thì chỉ được 1 XP. Giỏ chứa tối đa 30 con, bán lấy xu.

| API (cần đăng nhập) | Mô tả |
|---|---|
| `GET /api/fishing` | `{now, baits:[{id,name,price,luck,qty}], basket:[{id,fishId,name,rarity,cm,price,caughtAt}], basketMax, record:{fishId:cm}, cast, fish, user}` |
| `POST /api/fishing/bait/buy {bait, qty}` | mua 1–50 mồi; 402 thiếu xu |
| `POST /api/fishing/cast {bait}` | thả câu: `cast:{id, bait, createdAt, biteAt, windowMs, expiresAt, difficulty 1–5}`; 409 đang thả / hết mồi / giỏ đầy |
| `POST /api/fishing/reel {castId, success}` | giật: `outcome` = `caught` \| `miss` \| `early` \| `late`, `message`, `caught:{fishId,name,rarity,cm,price,record}`, `gained:{xp}`; 404 lần thả lạ |
| `POST /api/fishing/sell {ids}` hoặc `{all:true}` | bán cá trong giỏ: `gained:{coins, count}`; 409 không có gì để bán |

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
| **Nhất ăn tất** | mỗi người còn lại trả người về nhất 1 (bị cóng trả 2); không có tiền nhì / ba / bét |
| **Tới bằng 3 bích** | bộ bài cuối người nhất đánh ra có 3♠ thì tiền nhất ăn tất ×2 (tiền chặt / thối không nhân) |
| Tới trắng | người thắng +2 × số người thua, mỗi người thua −2 |
| Heo đen ♠♣ / heo đỏ ♦♥ | 1 / 2 |
| 3 đôi thông / tứ quý / 4 đôi thông | 3 / 4 / 6 |

- **Chặt heo / chặt hàng**: người bị chặt trả người chặt theo bảng trên. **Chặt chồng**: người bị chặt sau trả luôn số tiền mình vừa ăn, nên người chặt cuối ăn trọn.
- **Thối**: hết ván, người về bét còn heo / hàng trên tay thì trả cho người về nhất.
- **Cóng**: khi có người về nhất mà ai đó chưa đánh lá nào thì người đó bị cóng: xếp bét, bị loại khỏi ván và cũng phải thối cả tay.
- State có `delta` (tiền nhất ăn tất, mức) và `winBy: {threeSpades, multiplier}` để giao diện hiện "Tới bằng 3 bích ×2".

## Cấu trúc

```
main.go                  khởi động server, đăng ký danh sách game
internal/wallet/         tài khoản khách, ví xu, sổ cái (SQLite)
internal/server/         API hub (/api/...), WebSocket từng game (/ws/<game>), file tĩnh
games/tienlen/           luật, luồng ván, tiền cược, bot
games/tienlen/online/    phòng chơi online: người vào/ra, máy, hẹn giờ, trả thưởng vào ví
games/farm/              nông trại: ô đất, chuồng, ao, kho
games/baucua/            bầu cua: sòng chung, vòng theo đồng hồ, trả thưởng vào ví
games/caro/              cờ caro 15×15: luật thắng, máy chơi, bàn online qua WebSocket
games/site/              công trường: ca làm thuê, tiền công theo điểm, giới hạn ngày
games/fishing/           hồ câu cá: mồi, thả câu (kết quả quay ở server), giỏ cá, kỷ lục
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
| `POST /api/me/items/buy {kind, id}` | mua quần áo / tóc / cánh / gương mặt (`kind`: `outfit`, `hair`, `wings`, `face`) và mặc luôn; lỗi như trên, 400 món lạ |
| `POST /api/me/items/wear {kind, id}` | mặc / đeo món đã có (403 chưa có); tóc / mặt `default` luôn được, cánh `""` là tháo cánh |
| `POST /api/me/sit {sit}` | ngồi ghế (`true`, giữ 60 giây — gửi lại ~30 giây một lần khi còn ngồi) / đứng dậy (`false`); user có `sitting`, `regenSeconds` (60 đứng, 30 ngồi); 423 khi nằm viện |
| `GET /api/me` | thông tin + số dư, nhân vật (`outfit`, `owned`, `hair`, `ownedHair`, `wings`, `ownedWings`, `face`, `ownedFace`) (header `Authorization: Bearer <token>`) |
| `POST /api/me {name}` | đổi tên |
| `POST /api/daily` | nhận thưởng ngày (409 nếu đã nhận) |
| `GET /api/history` | 30 giao dịch gần nhất |
| `GET /api/games` | danh sách game và số người đang chơi |
| `GET /api/config` | số xu tặng ban đầu, thưởng ngày, danh mục trang phục (`catalog`), tóc (`hair`), cánh (`wings`), gương mặt (`face`), `sitRegenMultiplier` (2) |

WebSocket Tiến Lên (`/ws/tienlen`, JSON): client gửi `create {token}`, `join {code, token}`, `addBot`, `kick`, `setBet`, `start`, `play`, `pass`, `hint`, `leave`;
server gửi `session`, `state` (riêng từng người, chỉ có bài của chính họ), `hint`, `error`, `left`, `auth`.

## Test

```
go test -race ./...
```

Gồm: luật, tiền chặt / thối / cóng, giả lập 500 ván bot cho mỗi số người 2/3/4, phòng chơi, ví SQLite (thưởng ngày, số dư không âm,
rollback), API hub, và một ván trọn vẹn qua WebSocket thật có ghi kết quả vào ví.

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
| `-google-client-id` / `GOOGLE_CLIENT_ID` | rỗng (tắt) | OAuth Client ID (loại Web) để bật "Đăng nhập với Google" |

Đưa lên mạng để bạn ở xa chơi: `docker build -t gamehub . && docker run -p 8080:8080 -v gamehub-data:/data gamehub`
(nhớ gắn volume `/data` để không mất ví), hoặc deploy Dockerfile lên Fly.io / Render / Railway, hoặc mở tạm bằng tunnel
(`cloudflared tunnel --url http://localhost:8080`).

## Tài khoản và xu

- Đăng ký bằng **tên đăng nhập + mật khẩu** (tên đăng nhập 3–20 ký tự: chữ không dấu, số, `_`, `.`; không phân biệt hoa thường; mật khẩu ≥ 6 ký tự, băm bcrypt).
  Mỗi lần đăng nhập là một phiên riêng, đăng nhập được trên nhiều máy; sai mật khẩu 5 lần trong 15 phút thì tạm khoá tên đăng nhập đó.
- Tài khoản khách từ bản cũ vẫn chơi tiếp được và có nút đặt tên đăng nhập + mật khẩu, giữ nguyên xu. Database cũ được nâng cấp tự động khi khởi động.
- **Đăng nhập với Google** (bật khi có `GOOGLE_CLIENT_ID`): server xác minh ID token của Google (chữ ký RS256 theo khoá
  công khai của Google, `aud` = Client ID, còn hạn, email đã xác minh). Google chưa liên kết thì được mời tạo nhân vật
  (vé đăng ký sống 10 phút) — tài khoản mới không có tên đăng nhập / mật khẩu, thêm sau được bằng nút đặt tên đăng nhập.
  Tài khoản cũ liên kết Google ở mục tài khoản để giữ nguyên xu, nông trại, đồ; mỗi tài khoản một Google và ngược lại.
  Bỏ liên kết chỉ được khi tài khoản đã có tên đăng nhập + mật khẩu. Tạo Client ID: Google Cloud Console → APIs &
  Services → Credentials → OAuth client ID (Web application), Authorized JavaScript origins thêm `https://<tên miền>`
  (và `http://localhost:8080` khi thử).
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
  Cây ngắn lãi / giờ cao cho người chơi liên tục; cây dài (4–12 giờ) cho người treo máy:

| Cây | Level | Hạt | Chín sau | Bán | XP |
|---|---|---|---|---|---|
| Rau muống | 1 | 30 | 1 phút | 52 | 1 |
| Cà rốt | 1 | 50 | 2 phút | 90 | 2 |
| Ngô | 1 | 80 | 5 phút | 160 | 4 |
| Cà chua | 2 | 120 | 10 phút | 260 | 6 |
| Khoai lang | 2 | 100 | 15 phút | 290 | 5 |
| Lúa | 2 | 150 | 20 phút | 360 | 8 |
| Bắp cải | 3 | 180 | 30 phút | 520 | 10 |
| Dâu tây | 3 | 300 | 45 phút | 750 | 15 |
| Ớt | 3 | 220 | 1 giờ | 820 | 14 |
| Cà tím | 4 | 260 | 25 phút | 700 | 9 |
| Dưa hấu | 4 | 500 | 2 giờ | 1.400 | 30 |
| Dứa | 5 | 400 | 1 giờ | 1.250 | 18 |
| Bí ngô | 5 | 600 | 4 giờ | 2.600 | 45 |
| Thanh long | 6 | 800 | 6 giờ | 4.400 | 70 |
| Nho | 7 | 1.000 | 3 giờ | 3.700 | 55 |
| Sầu riêng | 8 | 2.000 | 12 giờ | 12.800 | 150 |

- **Chuồng** (heo 1, cừu / bò 2 đơn vị) và **ao cá** (koi 3 đơn vị...): đều mở từ level 1, mặc định 5 đơn vị, mở rộng +5 tới 30.
  Chuồng 3.000 xu, mở rộng lần thứ n 20.000 × n; ao 2.500 xu, mở rộng lần thứ n 15.000 × n.
- **Sân gia cầm**: gà, vịt (1 đơn vị) không ở chuồng mà đi quanh trang trại. Có sẵn cho mọi người, mặc định 10 đơn vị,
  mở rộng +5 tới 40, lần thứ n giá 10.000 × n xu. Gà vịt trong chuồng từ bản cũ được chuyển ra sân khi khởi động.
- **Mỗi chu kỳ đói một lần**: mỗi con (chuồng, sân, ao) chỉ lớn và ra trứng / sữa / lông **trong lúc no**, và mỗi chu kỳ
  chỉ cần cho ăn **một lần** — ăn xong no tới hết chu kỳ. Con mới mua đang đói; bữa đầu no tới lúc lớn **và** ra lứa
  sản phẩm đầu (heo, cá: no tới lúc lớn). Sản phẩm có rồi thì chưa cần ăn; lấy xong thì đói, một bữa cho một lứa kế.
  Vd gà: mua → đói → ăn → lớn và đẻ trứng → lấy trứng → đói → ăn → đẻ trứng... Đói thì dừng (tiến độ giữ nguyên).
  Đã ăn cho chu kỳ này thì không cho ăn thêm được. Con no theo luật cũ (chưa đủ hết chu kỳ) được ăn một lần cho đủ.
- **Thức ăn** là **vật phẩm mua trước** ở cửa hàng, cất trong kho giống (mỗi phần chiếm 1 chỗ): Thóc cho gà vịt 15 xu,
  Cám gia súc 50 xu, Thức ăn cá 10 xu, Thức ăn cho chó 40 xu mỗi phần. Mỗi lần ăn 1 phần (bò, cừu, cá koi 2 phần); hết thức ăn thì không cho
  ăn được. Cho ăn +1 XP. Con đã lớn không cho sản phẩm (heo, cá) thì không cần ăn nữa.
- **Sản phẩm vật nuôi**: con đã lớn cứ mỗi chu kỳ (tính trong lúc no) cho 1 món (để lâu vẫn chỉ 1 món chờ lấy), lấy vào kho sản phẩm;
  hoặc **lấy thịt** (con vật mất, được thịt vào kho). Cá thu nguyên con như cũ.

| Con | Sản phẩm (chu kỳ, giá, XP) | Thịt (giá, XP) |
|---|---|---|
| Gà | Trứng gà — 10 phút, 60 xu, 2 XP | 400 xu, 5 XP |
| Vịt | Trứng vịt — 15 phút, 80 xu, 2 XP | 520 xu, 6 XP |
| Heo | — | 1.300 xu, 12 XP |
| Cừu | Lông cừu — 30 phút, 400 xu, 6 XP | 2.000 xu, 20 XP |
| Bò | Sữa bò — 20 phút, 300 xu, 5 XP | 3.400 xu, 35 XP |

- **Làm nhanh**: gieo hạt mọi ô trống (dùng hạt trong kho trước, thiếu thì mua tới khi hết xu), thu hoạch tất cả,
  cho mọi con đói ăn (tới khi hết thức ăn; `autoBuy` thì tự mua đúng phần thức ăn còn thiếu, trong giới hạn xu và
  chỗ trống kho giống — giao diện hỏi trước, vd "Thiếu 6 Cám gia súc, 4 Thóc — mua 315 xu rồi cho ăn?"), lấy hết trứng /
  sữa / lông, thu hết cá đã lớn, lấy thịt tất cả gia súc / gia cầm đã lớn (tới khi kho đầy; phím Shift+M).
- **Kho giống**: mua hạt theo số lượng (1–100 hạt/lần) cất vào kho; gieo hạt lấy từ kho, hết hạt thì tự mua 1 hạt.
  Sức chứa 50, mở rộng +25 tới 300, lần mở rộng thứ n giá 10.000 × n xu.
- **Kho sản phẩm**: thu hoạch cây và thu vật nuôi / cá đã lớn → vào kho (cộng XP lúc thu); bán ở kho mới được xu.
  Sức chứa 50, mở rộng +25 tới 500, lần thứ n giá 15.000 × n xu. Kho đầy thì không thu hoạch được.

| API (cần token, POST trả `{farm, user, gained?}`) | Body |
|---|---|
| `GET /api/farm/catalog` (không cần token), `GET /api/farm` | |
| `POST /api/farm/plots/buy` | |
| `POST /api/farm/seeds/buy` | `{crop, qty}` |
| `POST /api/farm/feeds/buy` (thức ăn vào kho giống) | `{feed, qty}` |
| `POST /api/farm/plant` | `{plot, crop}` |
| `POST /api/farm/harvest`, `/harvest-all` | `{plot}` / — |
| `POST /api/farm/{barn,pond}/buy`, `/{barn,yard,pond}/expand` | |
| `POST /api/farm/{animals,poultry,fish}/buy` (chuồng / sân / ao) | `{kind}` |
| `POST /api/farm/{animals,poultry,fish}/feed` (cho ăn), `/collect` (vật nuôi, gia cầm: lấy thịt; cá: vào kho), `/sell` (bán thẳng) | `{id}` |
| `POST /api/farm/{animals,poultry}/produce` (lấy trứng / sữa / lông vào kho) | `{id}` |
| `POST /api/farm/plant-all`, `/feed-all`, `/produce-all`, `/fish/collect-all`, `/animals/collect-all` (làm nhanh; `gained.spent` = xu đã tiêu, `gained.used` = thức ăn đã dùng, `gained.bought` = thức ăn tự mua thêm) | `{crop}` / `{autoBuy}` / — |
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

## Cờ Cá Ngựa

Khu Casino, trang `/cangua/`. 2–4 người (hoặc máy), màu theo ghế: `red`, `blue`, `green`, `yellow` (chiều kim đồng hồ),
mỗi người 4 ngựa.

- **Bàn**: đường chung 56 ô (mỗi màu một cạnh 14 ô). Màu thứ i xuất quân ở ô `14·i`; cửa chuồng đích là ô ngay trước
  ô xuất phát (`14·i − 1`, đi đủ một vòng). Chuồng đích có 6 bậc.
- **Đổ xúc xắc** (server quay): ra 1 hoặc 6 mới được xuất quân. Ra 6 được đổ thêm; ra 6 lần thứ 3 liên tiếp thì mất lượt.
  Không có nước đi thì bỏ lượt (ra 6 thì vẫn được đổ tiếp).
- **Đi** đúng số ô, không được vượt qua ngựa nào trên đường; đến đúng ô có ngựa đối thủ thì **đá** ngựa đó về chuồng;
  không đứng chồng lên ngựa mình. Phải dừng đúng cửa đích (không đi quá); từ cửa đổ k thì lên bậc k (không vượt ngựa
  mình); trong chuồng đích, ngựa ở bậc b chỉ lên khi đổ đúng b+1.
- **Thắng**: ai đủ 4 ngựa trong chuồng đích trước thì thắng, ván kết thúc ngay.
- **Lượt** 20 giây; hết giờ máy đổ / đi thay một bước (không xử thua). Rời bàn giữa ván hoặc mất kết nối quá 30 giây thì
  máy đánh thay tới hết ván (vẫn tính tiền); vào lại thì lấy lại quyền. Không còn ai có mặt thì máy chơi nốt ván ngay để chốt.
- **Máy** `easy` / `normal` / `hard`: ưu tiên đá ngựa, lên đích, xuất quân, tiến ngựa đi xa nhất; máy khó tránh đứng
  trong tầm 1–6 ô trước ngựa đối thủ.
- **Cược** 0, 10, 20, 50, 100, 200, 500, 1000 xu: người thắng ăn **mỗi** người thua một mức cược (máy là nhà cái).
  Ghi sổ cái game `cangua`. **XP**: thắng 25, người khác 10 (cả bàn 0 xu). Ván sau người thắng đi trước.
- Cần đủ máu vào Casino để ngồi ghế. Bàn đủ 4 người hoặc đang chơi thì người vào sau được xem. Chat bàn như Cờ Caro.

WebSocket `/ws/cangua`, danh sách bàn `GET /api/cangua/rooms`
→ `[{code, phase, bet, players:[{name, gender, outfit, hair, wings, face, level, bot, color}], spectators, max}]`.

| Client → server | |
|---|---|
| `{t:"create", token, bet}` / `{t:"join", token, code}` | mở / vào bàn (đủ người hoặc đang chơi thì vào xem) |
| `{t:"addBot", level}`, `{t:"kick", id}`, `{t:"setBet", bet}`, `{t:"start"}` | chủ bàn, ngoài ván (cần ≥ 2 người) |
| `{t:"roll"}`, `{t:"move", horse}` | tới lượt mình: đổ, rồi đi ngựa `horse` (0..3) trong `movable` |
| `{t:"rematch"}`, `{t:"leave"}`, `{t:"chat", text}` | |

Server gửi `{t:"state", code, you, phase, players:[{id, name, …, bot, botLevel, host, online, auto, color, coins, ready}],
turn (id người tới lượt), dice, rolled, sixes, movable:[horse…] (chỉ gửi cho người tới lượt sau khi đổ),
horses:{color:[pos×4]}, last:{color, horse, from, to, kicked:{color, horse}}, winner, result:{money:{id:delta}, xp:{id:n}},
bet, gameBet, betOptions, turnMs, spectators, chat}`. `pos`: `-1` trong chuồng, `0..55` ô đường chung, `100+b` bậc đích b.
`auto` = máy đang đánh thay. Cùng `{t:"chat", …}`, `{t:"session", code}`, `{t:"error", msg}`, `{t:"left"}`, `{t:"replaced"}`, `{t:"auth"}`.

## Đấu trường

Khu riêng, trang `/arena/`. Bắn pháo theo lượt trên địa hình phá huỷ được: canh **góc**, **lực** và **gió**.

- **2–4 ghế** (người hoặc máy `easy` / `normal` / `hard`), người vào sau khi đủ ghế / đang đánh thì xem. Cần 30 HP.
- **Sân** 1600×900, 5 map: Đồng cỏ, Sa mạc (gió mạnh), Núi tuyết, Núi lửa (núi giữa, gồ ghề), Bãi biển (vũng nước giữa = vực).
  Địa hình sinh ngẫu nhiên mỗi trận; chủ bàn chọn map hoặc `random`.
- **Vật lý** (server tính, giao diện chỉ diễn): trọng lực 400, vận tốc = lực × 9, gió −10..10 (gia tốc ngang × 12) đổi mỗi lượt.
  Nổ khoét hố tròn, sát thương giảm dần tới r + 20; tự trúng mình vẫn mất máu; đất dưới chân sập xuống vực là chết.
- **Lượt** 25 giây (hết giờ mất lượt), mỗi lượt đi được 120 đơn vị (không leo dốc quá 6/1). Tối đa 40 vòng, hết vòng ai nhiều
  % máu nhất thắng. Rời bàn / mất kết nối quá 30 giây giữa trận là thua (mất cược ngay).
- **Vũ khí mỗi trận**: Đạn thường (30, r 40, vô hạn), Đạn chùm (3 × 14, r 28, 2 lần), Bom tạ (55, r 65, 1 lần),
  Đạn dịch chuyển (bay tới đâu người bắn tới đó, 1 lần).
- **Trang bị vĩnh viễn** Súng / Nón / Áo giáp, cấp 0..10: sát thương +8%/cấp, máu tối đa 200 + 15/cấp nón, giảm 3% sát thương
  nhận mỗi cấp giáp. **Lực chiến** = 1000 + 120·súng + 80·nón + 100·giáp. Máy: dễ +0, thường +3, khó +6.
- **Cường hoá**: mua đá 5 cấp (Lục 100, Lam 300, Tím 800, Vàng 2000, Đỏ 5000 xu); mỗi lần tốn 1 đá + phí 50·(L+1) xu,
  tỉ lệ `95 − 10·L + 12·(tier−1)` (5..100%). Thất bại mất đá + phí và **tụt 1 cấp** (cấp 0 giữ 0), trừ khi dùng
  **Bùa giữ cấp** (2000 xu). **Bùa may mắn** (1500 xu) +15% tỉ lệ. Bùa đã chọn bị tiêu dù thành hay bại. Lên +7 / +10 thì
  trang bị biến đổi hình (`evolved`).
- **Cược** 0, 10, 20, 50, 100, 200, 500, 1000 xu: người thắng ăn cược của mọi người thua (máy là nhà cái) + tiền người bỏ trận;
  hoà không đổi. Ghi sổ cái game `arena`. **XP**: thắng 30, hoà 15, thua 10.

REST: `GET /api/arena/maps`; cần đăng nhập: `GET /api/arena/gear`, `POST /api/arena/stones/buy {tier, qty}`,
`POST /api/arena/charms/buy {id, qty}`, `POST /api/arena/gear/upgrade {slot, stone, luck, protect}` → `{gear, power, stats,
stones, stoneShop, charms, charmShop, fee, rates, luckBonus, balance, result?}` (400 sai tham số, 402 thiếu xu, 409 max / hết đá / hết bùa).

WebSocket `/ws/arena`, danh sách bàn `GET /api/arena/rooms`
→ `[{code, phase, bet, map, mapName, players:[...], spectators, max}]`.

| Client → server | |
|---|---|
| `{t:"create", token, bet, map}` / `{t:"join", token, code}` | mở / vào bàn |
| `{t:"addBot", level}`, `{t:"kick", id}`, `{t:"settings", bet?, map?}` (`setBet` / `setMap`), `{t:"start"}` | chủ bàn, ngoài trận |
| `{t:"move", x}`, `{t:"aim", angle, power}`, `{t:"fire", angle, power, weapon}` | trong lượt mình |
| `{t:"rematch"}` / `{t:"ready"}`, `{t:"leave"}`, `{t:"chat", text}` | |

Server gửi `state` (phase, map, maps, weapons, ground[1600], wind, turn, turnMs, round, maxRounds, moveLeft, players
[x, y, hp, maxHp, alive, facing, gear, power, items, lastAngle, lastPower…], result, bet, chat), `shot` (by, weapon, paths,
blasts, hits, deaths, tele — gửi trước state, `turn` = "" trong lúc đạn bay), `aim`, `chat`, `error`.

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

## Khu danh vọng

Bảng vàng top 10 người giàu nhất theo **tổng tài sản** (xu), tính lại tối đa 30 giây một lần:

- **Xu** trong ví.
- **Đồ thời trang** đã có (quần áo, tóc, cánh, gương mặt) theo giá shop; món miễn phí / không còn bán tính 0.
- **Nông trại** theo giá trong game: ô đất đã mua (6 ô miễn phí tính 0) + hạt của cây đang trồng; chuồng / ao (giá mua +
  các lần mở rộng; sân gia cầm chỉ tính mở rộng) và mở rộng kho; vật nuôi / cá đang nuôi theo giá mua;
  kho giống (hạt, thức ăn) theo giá mua, kho sản phẩm theo giá bán.

Bằng tổng thì ai tạo tài khoản trước đứng trên. Bảng chỉ có tên hiển thị và ngoại hình, không có tên đăng nhập.

| API (không cần đăng nhập) | Mô tả |
|---|---|
| `GET /api/fame` | `{updatedAt (ms), top:[{rank,id,name,level,gender,outfit,hair,wings,face,total,coins,items,farm}], me}`; `me` (cùng dạng một dòng, hạng thật trong mọi người chơi) chỉ có khi gửi token hợp lệ |

## Bạn bè, trộm nông trại và chó giữ nhà

- **Kết bạn**: mời theo tên đăng nhập (không phân biệt hoa thường) hoặc id; người kia chấp nhận / từ chối (người mời cũng
  rút lại được bằng decline); huỷ kết bạn xoá cả hai chiều. Tối đa 100 bạn. Hai người mời chéo nhau thì thành bạn luôn.
- **Trộm**: chỉ vào được nông trại của bạn bè, chỉ trộm ô cây đã chín. Mỗi ô chỉ ra 1 nông sản nên kẻ trộm lấy đúng
  nông sản đó vào kho sản phẩm của mình (kho đầy thì không trộm được); chủ thu hoạch ô bị trộm chỉ còn XP. Mỗi vụ một ô
  chỉ bị trộm thành công một lần, và mỗi người chỉ được **thử** mỗi ô một lần mỗi vụ (bị bắt cũng mất lượt thử).
  Mỗi người được thử 5 lần / ngày (giờ Việt Nam) trên một nhà.
- **Chó giữ nhà**: cho ăn 1 phần *Thức ăn cho chó* (40 xu, mua ở cửa hàng, cất kho giống) thì no **4 giờ**; chỉ cho ăn
  được khi đói hoặc còn no dưới 1 giờ. Chó đói không canh: trộm lấy được ngay. Chó no thì **đuổi theo** kẻ trộm
  (tốc độ 255, người chơi 230 đơn vị / giây): chạy ra tới cổng là thoát, lấy được đồ; bị chó chạm thì mất **50 máu**
  (hết máu thì nằm viện như bị xe đụng), không lấy được gì. Trong lúc đuổi ô đó khoá với người khác. Server kiểm tra:
  báo thoát sớm hơn 1,5 giây kể từ lúc bắt đầu, hoặc không báo kết quả trong 20 giây, đều tính là bị bắt.
  Chủ nhà xem nhật ký 50 lần trộm gần nhất trong `GET /api/farm` (`thefts`).

| API (cần đăng nhập) | Body | Mô tả |
|---|---|---|
| `GET /api/friends` | | `{friends:[{id,name,level,gender,outfit,hair,wings,face,online,ripe,dogGuarding}], incoming:[…], outgoing:[…]}`; `ripe` = số ô chín chưa bị trộm |
| `POST /api/friends/request` | `{username}` hoặc `{id}` | gửi lời mời (trùng / đã là bạn → 409); trả như GET |
| `POST /api/friends/accept`, `/decline`, `/remove` | `{id}` | trả như GET |
| `GET /api/friends/{id}/farm` | | `{owner, now, plots:[{index,crop,plantedAt,readyAt,ripe,stolen,tried,locked}], dog:{fedUntil,guarding,fullFor}, stealsLeft}`; không phải bạn → 403 |
| `POST /api/friends/{id}/steal` | `{plot}` | chó đói: `{result:"stolen", item, qty, hpLost:0, farm, user}`; chó canh: `{result:"chase", chase:{id,plot,item,qty,startedAt,dogSpeed,minMs,expiresAt}, …}` (ms); chưa chín / đã bị trộm / đã thử / đang có người trộm / hết lượt / kho đầy → 409 |
| `POST /api/friends/{id}/steal/finish` | `{chase, escaped}` | `{result:"stolen"\|"caught"\|"gone", item, qty, hpLost, farm, user}`; `gone` = chủ đã thu hoạch trong lúc đuổi; token sai → 404, đã dùng → 409 |
| `POST /api/farm/dog/feed` | | cho chó ăn; `GET /api/farm` có thêm `dog:{fedUntil,guarding,fullFor}` và `thefts:[{at,thief:{id,name},item,qty,caught,hpLost,chasing}]` |

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
| **Nhất ăn tất, đếm lá** | lúc người nhất hết bài, mỗi người còn lại trả người nhất số lá đang cầm (bị cóng trả 26 lá); không có tiền nhì / ba / bét |
| **Tới bằng 3 bích** | bộ bài cuối người nhất đánh ra có 3♠ thì tiền đếm lá ×2 (tiền chặt / thối không nhân) |
| Tới trắng | mỗi người thua trả 13 lá × 2 = 26 |
| Heo đen ♠♣ / heo đỏ ♦♥ | 1 / 2 |
| 3 đôi thông / tứ quý / 4 đôi thông | 3 / 4 / 6 |

- **Chặt heo / chặt hàng**: người bị chặt trả người chặt theo bảng trên. **Chặt chồng**: người bị chặt sau trả luôn số tiền mình vừa ăn, nên người chặt cuối ăn trọn.
- **Thối**: hết ván, người về bét còn heo / hàng trên tay thì trả cho người về nhất.
- **Cóng**: khi có người về nhất mà ai đó chưa đánh lá nào thì người đó bị cóng: xếp bét, bị loại khỏi ván và cũng phải thối cả tay.
- State có `delta` (tiền đếm lá, mức), `cardsLeft` (số lá bị tính từng người lúc người nhất về, cóng 26) và `winBy: {threeSpades, multiplier}` để giao diện hiện "An còn 5 lá trả 50 xu", "Tới bằng 3 bích ×2".

## Cấu trúc

```
main.go                  khởi động server, đăng ký danh sách game
internal/wallet/         tài khoản khách, ví xu, sổ cái (SQLite)
internal/server/         API hub (/api/...), WebSocket từng game (/ws/<game>), file tĩnh
internal/googleauth/     xác minh ID token "Đăng nhập với Google" (JWKS, RS256, chỉ thư viện chuẩn)
games/tienlen/           luật, luồng ván, tiền cược, bot
games/tienlen/online/    phòng chơi online: người vào/ra, máy, hẹn giờ, trả thưởng vào ví
games/farm/              nông trại: ô đất, chuồng, ao, kho
games/baucua/            bầu cua: sòng chung, vòng theo đồng hồ, trả thưởng vào ví
games/caro/              cờ caro 15×15: luật thắng, máy chơi, bàn online qua WebSocket
games/arena/             đấu trường: vật lý bắn pháo, map, máy ngắm, bàn online, trang bị + cường hoá (đá, bùa)
games/cangua/            cờ cá ngựa 2–4 người: luật đi / đá / lên đích, máy chơi, bàn online qua WebSocket
games/site/              công trường: ca làm thuê, tiền công theo điểm, giới hạn ngày
games/fishing/           hồ câu cá: mồi, thả câu (kết quả quay ở server), giỏ cá, kỷ lục
games/fame/              khu danh vọng: xếp hạng tổng tài sản (xu + đồ thời trang + nông trại)
games/friends/           bạn bè: lời mời, chấp nhận, huỷ; xem và trộm nông trại của bạn (luật trộm, chó ở games/farm/)
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

Token sai trả 401 `{"error", "code"}`: `code` là `"elsewhere"` khi tài khoản vừa đăng nhập ở nơi khác (phiên này bị đá ra),
`"invalid"` khi không có / đã đăng xuất. WebSocket các game gửi `{t:"auth", reason:"elsewhere"}` rồi ngắt kết nối đang mở.

| | |
|---|---|
| `POST /api/register {username, password, name, gender, outfit}` | đăng ký + tạo nhân vật → `{token, user}` |
| `POST /api/login {username, password}` | đăng nhập → `{token, user}`; **mỗi tài khoản một phiên**: phiên cũ bị đá ra |
| `POST /api/logout` | đăng xuất phiên hiện tại |
| `POST /api/password {old, new}` | đổi mật khẩu |
| `POST /api/credentials {username, password}` | tài khoản khách cũ / tạo bằng Google đặt tên đăng nhập + mật khẩu |
| `POST /api/login/google {credential}` | ID token từ nút Google → đã liên kết: `{token, user}`; chưa: `{needSignup, email, name, signup}` (401 token sai, 503 chưa bật) |
| `POST /api/register/google {signup, name, gender, outfit}` | tạo tài khoản từ vé đăng ký → `{token, user}` (401 vé sai / hết hạn, 409 Google đã liên kết) |
| `POST /api/me/google {credential}` / `DELETE /api/me/google` | liên kết / bỏ liên kết Google → `{user}` (409 đã gắn tài khoản khác, đã có Google khác, chưa có mật khẩu) |
| `POST /api/character {outfit}` | mặc trang phục đã có |
| `POST /api/outfits/buy {outfit}` | mua trang phục (403 thiếu level, 402 thiếu xu, 409 đã có) |
| `POST /api/me/items/buy {kind, id}` | mua quần áo / tóc / cánh / gương mặt (`kind`: `outfit`, `hair`, `wings`, `face`) và mặc luôn; lỗi như trên, 400 món lạ |
| `POST /api/me/items/wear {kind, id}` | mặc / đeo món đã có (403 chưa có); tóc / mặt `default` luôn được, cánh `""` là tháo cánh |
| `POST /api/me/sit {sit}` | ngồi ghế (`true`, giữ 60 giây — gửi lại ~30 giây một lần khi còn ngồi) / đứng dậy (`false`); user có `sitting`, `regenSeconds` (60 đứng, 30 ngồi); 423 khi nằm viện |
| `GET /api/me` | thông tin + số dư, nhân vật (`outfit`, `owned`, `hair`, `ownedHair`, `wings`, `ownedWings`, `face`, `ownedFace`), `google` (`{email}` hoặc `null`) (header `Authorization: Bearer <token>`) |
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

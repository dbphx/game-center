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
- Máu dưới **10 HP** → **nằm viện 5 phút** (bị đưa tới Bệnh viện, không đi / vào khu được) và **bị trừ viện phí 500.000 xu** mỗi lần (không đủ thì mất hết số đang có), hồi dần tới đầy; trả 200 xu để xuất viện ngay.
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

Bộ nhớ đệm của trình duyệt (và Cloudflare): js / css trong trang có `?v=<phiên bản>` được giữ 1 năm (đổi nội dung thì
URL đổi theo); ảnh, font giữ 1 ngày rồi hỏi lại bằng ETag; HTML, json luôn hỏi lại. Vì vậy **đổi nội dung một ảnh mà
giữ nguyên tên file thì người chơi có thể còn thấy ảnh cũ tối đa 1 ngày** — muốn thấy ngay thì đặt tên file mới.
Module JS: server tự gắn `?v=<phiên bản>` vào import tương đối (`./x.js`, `/race/x.js`) lúc khởi động, phiên bản file cha tính theo
nội dung đã viết lại, nên Cloudflare (ghi đè js không có `?v` thành giữ 12 giờ) không ghép được module cũ với module mới.
Ảnh lớn nên nén lại bằng bảng 256 màu giữ bóng mờ (sprite `pet3d_*` đã nén như vậy, nhẹ hơn khoảng 3 lần).

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
- **Thuế 5% mỗi ván**: thắng xu ở Tiến Lên, Cờ Caro, Cá Ngựa, Bầu Cua (mỗi vòng, kể cả quỹ cái khi lời), Đấu trường thì bị thu 5%
  tiền thắng ròng của ván (làm tròn xuống), kể cả khi thắng máy; người thua không bị thu thêm, bàn chơi vui 0 xu không thuế.
  Sổ cái có dòng riêng "Thuế 5% tiền thắng — …"; tiền thuế không về tay ai. Thưởng hạ boss ở Đấu trường không bị thuế.
- Ngân hàng có lịch sử xu (sổ cái) để xem xu đến / đi từ đâu.

## Nhân vật, level và trang phục

- Lúc đăng ký chọn **giới tính** (Nam / Nữ, **không đổi được về sau**) và một bộ **trang phục khởi đầu** (Thường ngày hoặc Thể thao).
- Mỗi tài khoản là một nhân vật có **level riêng**, lên level bằng XP kiếm được khi chơi.
  Tiến Lên: hết mỗi ván người về nhất +30 XP, người xếp bét +10 XP (ở giữa chia đều).
- Level n cần tổng 50·n·(n−1) XP: level 2 ở 100 XP, level 3 ở 300, level 4 ở 600...
- **Shop thời trang**: trang phục khác phải **đủ level mới mua được, trả bằng xu**; đồ đã mua mặc lại lúc nào cũng được.

| Trang phục | Level | Giá |
|---|---|---|
| Thường ngày, Thể thao | 1 | miễn phí (khởi đầu) |
| Áo hoodie | 4 | 2.400 xu |
| Vest lịch lãm | 6 | 4.500 xu |
| Áo dài | 8 | 7.500 xu |
| Hoàng gia (có vương miện) | 12 | 15.000 xu |
| Đồng phục học sinh | 1 | 1.800 xu |
| Áo bà ba | 4 | 3.600 xu |
| Áo Tết | 6 | 6.000 xu |
| Hải tặc | 10 | 12.000 xu |
| Đồ cưới | 14 | 24.000 xu |
| Phi hành gia | 18 | 36.000 xu |

- Shop còn bán **tóc** và **cánh** (dùng chung cả hai giới). Ai cũng có sẵn tóc mặc định; cánh có thể tháo ra bất cứ lúc nào.

| Tóc | Level | Giá | | Cánh | Level | Giá |
|---|---|---|---|---|---|---|
| Tóc mặc định | 1 | có sẵn | | Cánh thiên thần | 6 | 7.500 xu |
| Tóc dựng, Đuôi ngựa | 1 | 900 xu | | Cánh tiên | 8 | 10.500 xu |
| Tóc dài xoăn, Tóc búi | 4 | 2.100 xu | | Cánh bướm | 10 | 13.500 xu |
| Mohican | 6 | 3.600 xu | | Cánh dơi | 12 | 18.000 xu |
| Hai bím hồng | 8 | 6.000 xu | | Cánh rồng | 16 | 30.000 xu |

- **Gương mặt** (dùng chung hai giới, ai cũng có sẵn mặt mặc định):

| Gương mặt | Level | Giá |
|---|---|---|
| Mặt mặc định | 1 | có sẵn |
| Cười tươi | 1 | 600 xu |
| Nháy mắt | 1 | 900 xu |
| Má hồng | 4 | 1.800 xu |
| Kính râm | 4 | 2.400 xu |
| Mèo con | 6 | 3.600 xu |
| Mắt lấp lánh | 8 | 6.000 xu |

- Trong game, avatar của mỗi người là mặt nhân vật của họ kèm level.

## Nông trại

- 6 ô đất miễn phí, ô thứ 7 giá 30.000 xu, mỗi ô sau +6.000 xu (đủ 48 ô hết ~6,4 triệu, nhắm ~2 tháng chơi), tối đa 48 ô. Cây có thời gian chín, level tối thiểu.
  Cây ngắn lãi / giờ cao cho người chơi liên tục; cây dài (4–12 giờ) lãi mỗi vụ cao hơn nhưng phải canh zombie (xem bên dưới):

| Cây | Level | Hạt | Chín sau | Bán | XP |
|---|---|---|---|---|---|
| Rau muống | 1 | 45 | 1 phút | 60 | 1 |
| Cà rốt | 1 | 75 | 2 phút | 100 | 2 |
| Ngô | 1 | 120 | 5 phút | 160 | 4 |
| Cà chua | 4 | 180 | 10 phút | 260 | 6 |
| Khoai lang | 4 | 150 | 15 phút | 250 | 5 |
| Lúa | 4 | 225 | 20 phút | 340 | 8 |
| Bắp cải | 7 | 270 | 30 phút | 440 | 10 |
| Dâu tây | 7 | 450 | 45 phút | 680 | 15 |
| Ớt | 7 | 330 | 1 giờ | 620 | 14 |
| Cà tím | 10 | 390 | 25 phút | 610 | 9 |
| Dưa hấu | 10 | 750 | 2 giờ | 1.200 | 30 |
| Dứa | 13 | 600 | 1 giờ | 1.000 | 18 |
| Bí ngô | 13 | 900 | 4 giờ | 1.900 | 45 |
| Thanh long | 16 | 1.200 | 6 giờ | 2.900 | 70 |
| Nho | 20 | 1.500 | 3 giờ | 2.800 | 55 |
| Sầu riêng | 25 | 3.000 | 12 giờ | 8.000 | 150 |

- **Chuồng** (heo 1, cừu / bò 2 đơn vị) và **ao cá** (koi 3 đơn vị...): đều mở từ level 1, mặc định 5 đơn vị, mở rộng +5 tới 30.
  Chuồng 9.000 xu, mở rộng lần thứ n 60.000 × n; ao 7.500 xu, mở rộng lần thứ n 45.000 × n.
- **Sân gia cầm**: gà, vịt (1 đơn vị) không ở chuồng mà đi quanh trang trại. Có sẵn cho mọi người, mặc định 10 đơn vị,
  mở rộng +5 tới 40, lần thứ n giá 30.000 × n xu. Gà vịt trong chuồng từ bản cũ được chuyển ra sân khi khởi động.
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
- **Thuế bán**: bán trong kho sản phẩm hoặc bán thẳng con vật bị trừ thuế, tiền thuế không về ai: **nông sản** (cây trồng, cây mây)
  **10%**, **vật nuôi và cá** (trứng, sữa, lông, thịt, cá, con vật) **5%**. Thuế tính trên tổng tiền từng món mỗi lần bán,
  làm tròn xuống; `gained.coins` là tiền nhận sau thuế, `gained.tax` là thuế; catalog có `cropTax`, `stockTax`.
  Giá trị nông trại ở Khu danh vọng vẫn tính theo giá bán chưa thuế.

| Con | Level | Giá con | Lớn sau | Sản phẩm (chu kỳ, giá, XP) | Thịt / bán (giá, XP) |
|---|---|---|---|---|---|
| Gà | 1 | 200 | 1 giờ | Trứng gà — 30 phút, 90 xu, 2 XP | 380 xu, 5 XP |
| Vịt | 1 | 250 | 1,5 giờ | Trứng vịt — 45 phút, 150 xu, 2 XP | 550 xu, 6 XP |
| Heo | 5 | 600 | 4 giờ | — | 1.530 xu, 12 XP |
| Cừu (2 đơn vị) | 8 | 900 | 6 giờ | Lông cừu — 2 giờ, 1.070 xu, 6 XP | 3.960 xu, 20 XP |
| Bò (2 đơn vị) | 12 | 1.500 | 8 giờ | Sữa bò — 1,5 giờ, 930 xu, 5 XP | 6.060 xu, 35 XP |
| Cá rô phi | 1 | 100 | 45 phút | — | 230 xu, 3 XP |
| Cá trê | 4 | 250 | 2 giờ | — | 650 xu, 8 XP |
| Cá chép (2 đơn vị) | 8 | 500 | 4 giờ | — | 2.310 xu, 15 XP |
| Cá Koi (3 đơn vị) | 15 | 2.000 | 12 giờ | — | 11.600 xu, 50 XP |

- Cân bằng 2026-11: con đã lớn (hoặc sản phẩm đã chờ lấy) theo thời gian cũ vẫn lớn sau khi cập nhật, con lớn dở giữ số giây đã lớn
  (migration một lần, bảng `farm_migrations`).

- **Làm nhanh**: gieo hạt mọi ô trống (dùng hạt trong kho trước, thiếu thì mua tới khi hết xu), thu hoạch tất cả,
  cho mọi con đói ăn (tới khi hết thức ăn; `autoBuy` thì tự mua đúng phần thức ăn còn thiếu, trong giới hạn xu và
  chỗ trống kho giống — giao diện hỏi trước, vd "Thiếu 6 Cám gia súc, 4 Thóc — mua 315 xu rồi cho ăn?"), lấy hết trứng /
  sữa / lông, thu hết cá đã lớn, lấy thịt tất cả gia súc / gia cầm đã lớn (tới khi kho đầy; phím Shift+M).
- **Kho giống**: mua hạt theo số lượng (1–100 hạt/lần) cất vào kho; gieo hạt lấy từ kho, hết hạt thì tự mua 1 hạt.
  Sức chứa 50, mở rộng +25 tới 300, lần mở rộng thứ n giá 30.000 × n xu.
- **Kho sản phẩm**: thu hoạch cây và thu vật nuôi / cá đã lớn → vào kho (cộng XP lúc thu); bán ở kho mới được xu.
  Sức chứa 50, mở rộng +25 tới 500, lần thứ n giá 45.000 × n xu. Kho đầy thì không thu hoạch được.

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


### Đất tốt và zombie

- **Đất tốt**: mỗi ô nâng một lần, giá 30.000 xu, giữ vĩnh viễn. Cây gieo sau đó chín nhanh hơn 30% (`POST /api/farm/plot/upgrade {plot}`);
  cây đang trồng giữ nguyên giờ chín. Tiền nâng cấp tính vào giá trị nông trại ở Khu danh vọng.
- **Zombie**: đúng đầu mỗi giờ theo giờ server có một đợt chung cho mọi nông trại, chủ có 5 phút để đập. Mục tiêu chọn ngẫu nhiên trong
  các ô có cây gieo trước lúc đợt bắt đầu, gồm cả cây đã chín. Số zombie là `max(1, ceil(p × số cây))` với `p = 1 − 0,5^(1/12) ≈ 5,61%`,
  không vượt quá số cây; nhà không có cây thì không có zombie.
- **Không đập kịp**: hết 5 phút, zombie ăn cây (mất cây, mất hạt) với xác suất `p × số cây / số zombie`, ngược lại bỏ đi. Nhờ vậy người
  không canh mất khoảng 50% số cây sau mỗi 12 giờ, nhà to nhỏ như nhau, kể cả khi đang offline.
- **Đập** (`POST /api/farm/zombie/hit {plot}`): 1 nhát chết, trừ 2 máu, không có thưởng; được đập cả khi máu dưới 40, trừ lúc nằm viện. Chỉ chủ
  nông trại đập được. Hết máu ở nhát đập thì vào viện như các nguồn mất máu khác.
- **Vật nuôi và cá**: mỗi đầu giờ zombie tấn công riêng từng khu chuồng, sân, ao với cùng công thức (khu có con thì có ít nhất 1 zombie), tính
  từ đầu giờ đầu tiên sau khi có tính năng, không tính bù quá khứ. Chó không bị tấn công. Hết 5 phút zombie ăn mất con đó hoặc bỏ đi.
  Con đang bị tấn công không bán, thu hoạch, lấy sản phẩm được (thao tác hàng loạt bỏ qua), vẫn cho ăn được. Đập bằng
  `POST /api/farm/zombie/hit-stock {id}`, cùng luật 1 nhát, trừ 2 máu.
- **Zombie lựa mục tiêu** ở cả ruộng và khu vật nuôi: chọn cây / con gần trưởng thành nhất (tiến độ chặn ở 100%, đã chín hay đã lớn đều ở mức
  cao nhất), bằng tiến độ thì ngẫu nhiên. Tỉ lệ mất chung vẫn khoảng 50% sau 12 giờ, nhưng rủi ro dồn vào cây / con sắp lớn.
- Ô có zombie không thu hoạch được và không bị bạn bè trộm được. `GET /api/farm/zombie/status` trả `{count, until}` cho bản đồ thành phố, `count` tính cả vật nuôi.
- **Súng đậu**: mua một lần 399.000 xu (`POST /api/farm/gun/buy`). **Ba loại đạn**, mỗi lần mua 1–1.000 viên một loại
  (`POST /api/farm/gun/ammo {type, qty}`, `type` ∈ `normal` / `medium` / `high`, sai → 400); catalog `ammoTypes`, state `gun.ammo = {normal, medium, high}`:

| Loại | Giá / viên | Bắn zombie nhắm vào món có giá bán |
|---|---|---|
| Đạn thường `normal` | 100 | 300–1.999 xu (gà, vịt, heo, bí ngô...) |
| Đạn trung `medium` | 500 | 2.000–7.999 xu (bò, cừu, thanh long...) |
| Đạn cao `high` | 1.500 | từ 8.000 xu (sầu riêng, cá Koi...) |

  Từ giây 30 của đợt, súng tự diệt zombie, đợt cũ trước, kể cả khi chủ offline, không mất máu và không mất cây. Mỗi con dùng đúng loại
  theo bậc của món bị tấn công; hết loại đó thì dùng loại cao hơn **chỉ khi** viên đó không đắt hơn món, không bao giờ dùng loại thấp
  hơn. Món dưới 300 xu (rau ngắn ngày, cá rô phi...) hay không có viên phù hợp thì súng bỏ qua: không tốn đạn, zombie đó vẫn ăn khi hết
  hạn nếu không đập tay. Đập tay trước giây 30 thì không tốn đạn. Đạn mua trước khi chia loại (2.500 xu/viên) giữ 1:1 thành Đạn cao.
  Súng và đạn (theo giá từng loại) tính vào giá trị nông trại ở Khu danh vọng.
- Không có tiến trình nền: server tính bù các đợt đã qua mỗi khi nông trại được đọc, tối đa 720 đợt gần nhất, ngẫu nhiên có seed theo
  `(người chơi, đợt)`. Lần đầu thấy một nông trại thì không tính ngược, đợt đầu tiên là đầu giờ kế tiếp.

### Đậu thần và tầng mây

- Đủ 48 ô đất thì có **ô thứ 49**, chỉ để trồng đậu thần (`POST /api/farm/bean/plant`). Trả tiền hạt ngay, mỗi lúc một hạt, không huỷ và không
  trả tiền để chín nhanh. Hạt chín thì tầng kế tiếp mở vĩnh viễn và ô 49 trống lại, kể cả khi chủ offline.
- Trên mây mỗi tầng có 6 chỗ đặt chậu, chỉ đặt được loại chậu của tầng đó. Mua lần lượt từng chậu (`POST /api/farm/pot/buy {floor}`),
  không bán lại được. Trồng (`POST /api/farm/pot/plant {floor, slot}`) trừ thẳng tiền hạt bằng xu, không qua kho giống. Cây chín sau 12 giờ,
  thu hoạch (`POST /api/farm/pot/harvest {floor, slot}`) được 1 nông sản vào kho sản phẩm và XP; kho đầy thì cây vẫn còn.

| Tầng | Hạt đậu | Chờ | Giá chậu | Cây mây | Hạt | Bán | XP |
|---|---|---|---|---|---|---|---|
| 1 | 400.000 | 24 giờ | 200.000 | Dâu mây | 5.000 | 20.000 | 200 |
| 2 | 2.000.000 | 48 giờ | 1.000.000 | Lê sương | 20.000 | 70.000 | 300 |
| 3 | 6.000.000 | 72 giờ | 4.000.000 | Nho sao | 50.000 | 200.000 | 450 |
| 4 | 20.000.000 | 96 giờ | 20.000.000 | Đào tiên | 100.000 | 600.000 | 700 |

- Trồng đậu mở tầng cần đủ level: tầng 1 level 20, tầng 2 level 25, tầng 3 level 30, tầng 4 level 35 (thiếu → 403 "Cần cấp N"); tầng đã mở dùng bình thường.
- Zombie không tấn công ô 49 và cây mây. Bạn bè không trộm được, cũng không thấy ô 49 và tầng mây khi ghé thăm. Đất tốt không áp dụng cho chậu.
- Tiền hạt đậu, chậu và cây đang trồng trong chậu tính vào giá trị nông trại ở Khu danh vọng.
- **Kiểu dáng chậu** (chỉ để đẹp, không đổi cây, giá hay thời gian chín): Chậu của tầng và Gốm trắng miễn phí; Gốm hoa, Gỗ mộc
  40.000; Sứ thanh hoa 100.000; Ngọc lục bảo 200.000; Mây trắng 400.000; Pha lê 600.000; Vàng khảm đá 1.000.000 xu. Mua một lần
  dùng cho mọi chậu (`POST /api/farm/pot/skin/buy {skin}`), lắp cho từng chậu (`POST /api/farm/pot/skin/set {floor, slot, skin}`),
  không đổi cây đang trồng. Catalog có `potSkins`; mỗi chậu có `skin`, `cloud.skins` là các kiểu đã có. Tiền mua kiểu chậu tính
  vào giá trị nông trại.

### Mùa Noel và NPC trong vườn

Trang nông trại nạp thêm `public/farm/season.js` + `season.css` (lớp phủ, không sửa `farm.js`). `noelActive(date)` đúng trong
tháng 12, ngày 1–6/1 và riêng năm 2026 từ tháng 11 (giờ Việt Nam); `?season=noel` / `?season=off` ép bật / tắt khi thử.
Trong mùa Noel: `<body class="season-noel">`, tuyết rơi (canvas vẽ ảnh bông tuyết PNG, ba lớp, điện thoại ít bông hơn, tắt khi
`prefers-reduced-motion`), mái kho / cổng / cây / hàng rào / nền đất / ô ruộng đổi sang bản phủ tuyết cùng kích thước, dây đèn
nháy dọc hàng rào, cây thông + quà thay cây đào trái cổng, người tuyết thay cây đào phải cổng, vòng hoa trên cổng và hai kho;
tầng mây có đồi phủ tuyết, tuyết rơi, cây thông nhỏ + quà ở bờ mây đáy.
NPC luôn có (trừ khi đang thăm nhà bạn): `<div id="farm-npc" class="farm-npc has-quest" role="button">` đứng bên trái cổng —
Ông già Noel kèm bao quà trong mùa Noel, Bác nông dân (nón lá, giỏ rau) ngoài mùa; thỉnh thoảng vẫy tay; nhấn (hoặc Enter)
phát `document.dispatchEvent(new CustomEvent('farm:npc', {detail: {npc}}))`; class `has-quest` hiện chấm "!" vàng trên đầu.

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
- **Đổ 2 xúc xắc** (server quay), mỗi lượt đi một nước. Chỉ đổ 1 + 1, 1 + 6 hoặc 6 + 6 mới được xuất quân (ngựa ra ô
  xuất phát rồi dừng). Đổ đôi được đổ thêm; đôi lần thứ 3 liên tiếp thì mất lượt.
  Không có nước đi thì bỏ lượt (ra đôi thì vẫn được đổ tiếp).
- **Đi** đúng tổng 2 viên, không được vượt qua ngựa nào trên đường; đến đúng ô có ngựa đối thủ thì **đá** ngựa đó về chuồng;
  không đứng chồng lên ngựa mình. Đổ 1 + 1 được chọn đi 1 ô hoặc 2 ô. Phải đi đúng tổng, tổng dư thì ngựa đó không đi được. Từ cửa lên bậc bằng
  viên lớn hơn, bị ngựa mình chặn thì dùng viên nhỏ hơn; trong chuồng đích, ngựa ở bậc b lên khi có viên bằng b+1.
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
- **Chế độ boss** (đánh chung, 1–4 người + máy đồng đội, không cược): Vua Chuột `chuot` (dễ, 3500 máu/người), Robot Rác `robot`
  (vừa, 5000), Rồng Lửa `rong` (khó, 6500). Boss đứng yên mép phải, thân to, bắn một phát mạnh (350 / 500 / 700) sau mỗi vòng vào
  người gần nhất hoặc ngẫu nhiên; người chơi đứng ở 60% sân bên trái, không bắn trúng đồng đội. Hạ boss → mỗi người chơi thật
  nhận ngọc rơi (dễ 1–2 viên bậc I; vừa 2–3 viên bậc I–II; khó 3–4 viên bậc II–III) + 50 / 100 / 200 xu; mọi người chết hoặc
  hết 40 vòng là thua. Boss khó hơn: Thuỷ Quái `thuyquai` (`veryhard`, 8000 máu/người, 800, r 75, **2 phát mỗi lượt** vào hai
  người khác nhau nếu có; rơi 3–5 ngọc bậc II–III, ~50% bậc III; 400 xu) và Ma Vương `mavuong` (`hell`, 10000, 1000, r 85,
  2 phát/lượt + cứ lượt boss thứ 3 tung **Mưa lửa**: cầu lửa rơi thẳng trúng mọi người còn sống, 500 sát thương, vẫn qua giáp;
  rơi 4–6 ngọc, ~90% bậc III; 800 xu).
- **Sân** 1600×900, 5 map: Đồng cỏ, Sa mạc (gió mạnh), Núi tuyết, Núi lửa (núi giữa, gồ ghề), Bãi biển (vũng nước giữa = vực).
  Địa hình sinh ngẫu nhiên mỗi trận; chủ bàn chọn map hoặc `random`.
- **Vật lý** (server tính, giao diện chỉ diễn): trọng lực 400, vận tốc = lực × 9, gió −10..10 (gia tốc ngang × 12) đổi mỗi lượt.
  Nổ khoét hố tròn, sát thương giảm dần tới r + 20; tự trúng mình vẫn mất máu; đất dưới chân sập xuống vực là chết.
- **Lượt** 25 giây (hết giờ mất lượt), mỗi lượt đi được 120 đơn vị (không leo dốc quá 6/1). Tối đa 40 vòng, hết vòng ai nhiều
  % máu nhất thắng. Rời bàn / mất kết nối quá 30 giây giữa trận là thua (mất cược ngay).
- **Kỹ năng mỗi trận** (dùng không giới hạn): Đạn thường (300, r 40), Đạn chùm (3 × 140, r 28), Bom tạ (550, r 65),
  Đạn dịch chuyển (bay tới đâu người bắn tới đó).
- **Nộ + chiêu cuối**: mỗi người có nộ 0..100 (boss không có): +10 đầu mỗi lượt của mình, + sát thương gây cho người khác / 20,
  + sát thương nhận vào / 10. Đầy 100 thì bắn được `weapon:"ult"` — chiêu cuối của vũ khí đang trang bị (sát thương vẫn nhân Tấn công /
  chí mạng / Phòng thủ), bắn xong nộ về 0; chưa đủ nộ thì lỗi. Đại bác "Pháo kích" (3 × 450, r 60), Dép lào "Mưa dép" (5 × 220, r 30),
  Chổi chà "Quét sạch" (600, r 110), Cây lau nhà "Lau sàn" (500, r 70, đẩy đối thủ trong bán kính ra xa tâm nổ 80 theo chiều ngang —
  ra khỏi sân / rơi vực là chết; boss không bị đẩy), TV "Sóng nhiễu" (800, r 60, người trúng mất hết nộ), Tủ lạnh "Tủ lạnh rơi"
  (1100, r 90). Máy đủ nộ thì tung chiêu cuối.
- **Vũ khí** (mua bằng xu, mỗi cái có cấp 0..10 riêng, trang bị một cái; chỉ số gốc Tấn công / Phòng thủ / May mắn / Thể lực):
  Đại bác (miễn phí, 100/50/20/100), Dép lào (45000, 150/80/40/150), Chổi chà (90000, 220/120/60/220),
  Cây lau nhà (180000, 300/160/80/300), TV (360000, 400/240/100/400), Tủ lạnh (750000, 550/320/120/600). Chỉ số ở cấp L = `round(gốc × (1 + 0.1·L))` + ngọc khảm.
  **Chuyển cấp**: đưa cấp của món A sang món B cùng loại (A phải cao hơn B), A về +0; phí 3000 xu × cấp.
- **Nón** (thiên May mắn + Thể lực): Nón lá (miễn phí, 0/20/20/100), Mũ bảo hiểm (60000, 20/60/30/150),
  Mũ len (120000, 40/80/50/220), Nón cao bồi (240000, 70/100/80/320), Mũ phù thuỷ (450000, 120/120/100/500),
  Vương miện (900000, 160/180/130/800).
  **Áo giáp** (thiên Phòng thủ + Thể lực): Áo thun (miễn phí, 0/40/0/150), Áo mưa (60000, 0/80/10/200),
  Áo bóng đá (120000, 30/110/20/260), Áo phao (240000, 40/160/20/380), Giáp sắt (450000, 60/240/30/550),
  Giáp rồng (900000, 100/320/50/800).
  Món mua bằng xu cần đủ level: theo thứ tự giá từ rẻ tới đắt level 1 / 6 / 12 / 20 / 30 (chưa đủ → 403 "Cần cấp N…"; món đã có vẫn dùng).
  Mỗi món có cấp 0..10 và 3 ô khảm riêng, mua / dùng / chuyển cấp (trong cùng loại) như vũ khí; dùng 1 nón + 1 áo.
- **Chỉ số trong trận** = tổng vũ khí + nón + áo đang dùng (mỗi món ở cấp của nó, cộng ngọc của nó): sát thương × (1 + Tấn công/1000);
  giảm sát thương nhận = Phòng thủ/1000 (tối đa 60%); chí mạng = May mắn/10 % (× 1.5); máu tối đa = 2000 + Thể lực.
  Đồ mặc định +0 (Đại bác + Nón lá + Áo thun): 100/110/40/350 → 2350 máu, giảm 11%, chí mạng 4%, Lực chiến 11200.
- **Ngọc khảm** Tấn công (+30/60/100), Phòng thủ (+20/40/70), May mắn (+10/20/40), Thể lực (+50/100/180) bậc I–III, rơi từ boss.
  Mỗi vũ khí, nón và áo giáp có 3 ô, mở ở +3, +7, +10 theo cấp món đó; khảm đè thì ngọc cũ về kho, tháo miễn phí; cấp giảm
  (cường hoá hỏng, chuyển cấp đi) thì ngọc ở ô bị khoá tự về kho. Ngọc có hiệu lực = ngọc của vũ khí, nón, áo đang dùng
  (`gear.bonus`), cộng vào chỉ số chiến đấu và Lực chiến; ngọc ở món không dùng vẫn nằm đó nhưng không tính.
- **Trang bị** 3 ô: `gun` / `hat` / `armor` = vũ khí / nón / áo đang dùng; cường hoá ô nào là cường hoá món đang dùng ở ô đó.
  **Lực chiến** = 10000 + 2·(Tấn công + Phòng thủ + May mắn + Thể lực, gồm ngọc).
  Máy: dễ Đại bác + Nón lá + Áo thun +0; thường Đại bác + Mũ bảo hiểm + Áo mưa +3; khó Đại bác + Nón cao bồi + Áo phao +6.
  Dữ liệu cũ: cấp nón / áo trong `arena_gear` và ngọc ở ô "hat" / "armor" chuyển sang Nón lá / Áo thun lúc khởi động
  (một lần). Mọi món (vũ khí, nón, áo) lưu chung bảng `arena_arms` (id không trùng nhau), ô khảm ở `arena_sockets` theo id món.
  Khi giá / chỉ số đổi (bản ×10), món đã mua giữ nguyên, không hoàn hay thu thêm xu.
- **Cường hoá**: mua đá 5 cấp (Lục 500, Lam 1500, Tím 4000, Vàng 10000, Đỏ 25000 xu); mỗi lần tốn 1 đá + phí 100·(L+1) xu,
  tỉ lệ `80 − 10·L + 6·(tier−1)` (1..95%); lên +10 (từ +9) **không quá 5%**, kể cả Đá Đỏ + bùa may mắn. Thất bại mất đá + phí và **tụt 1 cấp** (cấp 0 giữ 0), trừ khi dùng
  **Bùa giữ cấp** (4000 xu). **Bùa may mắn** (3000 xu) +15% tỉ lệ. Bùa đã chọn bị tiêu dù thành hay bại. Lên +7 / +10 thì
  trang bị biến đổi hình (`evolved`).
- **Cược** 0, 10, 20, 50, 100, 200, 500, 1000 xu: người thắng ăn cược của mọi người thua (máy là nhà cái) + tiền người bỏ trận;
  hoà không đổi. Ghi sổ cái game `arena`. **XP**: thắng 30, hoà 15, thua 10.

REST: `GET /api/arena/maps`; cần đăng nhập: `GET /api/arena/gear`, `POST /api/arena/stones/buy {tier, qty}`,
`POST /api/arena/charms/buy {id, qty}`, `POST /api/arena/gear/upgrade {slot, stone, luck, protect}`,
`POST /api/arena/arms/buy {id}`, `POST /api/arena/arms/equip {id}`, `POST /api/arena/arms/transfer {from, to}`,
`POST /api/arena/items/buy {slot, id}`, `POST /api/arena/items/equip {slot, id}`, `POST /api/arena/items/transfer {slot, from, to}`
(`slot` "gun" / "hat" / "armor", bỏ trống = "gun"; `arms/*` là bản cũ cho vũ khí),
`POST /api/arena/gems/socket {slot?, item?, arm?, idx, kind, tier}`, `POST /api/arena/gems/unsocket {slot?, item?, arm?, idx}`
(`slot` bỏ trống = "gun"; món = `item` (hoặc `arm` như cũ), bỏ trống = món đang dùng của ô đó)
→ `{gear{arm, gun, hatId, hat, armorId, armor, bonus}, power, stats{dmgPct, maxHp, armorPct, atk, def, luck, sta},
arms[{id, name, price, desc, atk, def, luck, sta, eff{…}, owned, level, equipped, sockets[{idx, unlockAt, open, gem}]}],
hats[…], armors[…] (cùng định dạng arms), slotSockets{hat:[…], armor:[…]} (ô khảm nón / áo đang dùng),
gems[{kind, tier, name, value, qty}], transferFeePerLevel,
stones, stoneShop, charms, charmShop, fee, rates, luckBonus, balance, result?{…, item, arm?}, transfer?{slot, from, to, level}}`
(400 sai tham số / sai ô, 402 thiếu xu, 409 max / hết đá / hết bùa / đã có / chưa có / không chuyển được / ô khoá / hết ngọc).
Trong trận `players[].gear` có `hatId`, `armorId` (luôn điền id, mặc định Nón lá / Áo thun).
`GET /api/arena/bosses` → `[{id, name, level, hpPer, dmg, r, size, coins, shots, skill?, lootText}]`.
`GET /api/arena/players/{username}` (cần đăng nhập) — hồ sơ Đấu trường: `{username, name, gender, outfit, hair, wings, face, level,
gear, power, stats{atk, def, luck, sta, maxHp, dmgPct, armorPct, critPct}, weapon, hat, armor}`, mỗi món đang dùng
`{id, name, level, form (1 / 2 ở +7 / 3 ở +10), atk, def, luck, sta, eff, sockets[3]}`; không có số dư / đá / bùa / kho ngọc
(401 chưa đăng nhập, 404 không có người).

WebSocket `/ws/arena`, danh sách bàn `GET /api/arena/rooms`
→ `[{code, phase, bet, map, mapName, players:[...], spectators, max, mode, boss?, bossName?}]`.

| Client → server | |
|---|---|
| `{t:"create", token, bet, map, mode?:"boss", boss?}` / `{t:"join", token, code}` | mở / vào bàn |
| `{t:"addBot", level}`, `{t:"kick", id}`, `{t:"settings", bet?, map?, boss?}` (`setBet` / `setMap`), `{t:"start"}` | chủ bàn, ngoài trận |
| `{t:"move", x}`, `{t:"aim", angle, power}`, `{t:"fire", angle, power, weapon}` | trong lượt mình |
| `{t:"rematch"}` / `{t:"ready"}`, `{t:"leave"}`, `{t:"chat", text}` | |

Server gửi `state` (phase, map, maps, weapons, ground[1600], wind, turn, turnMs, round, maxRounds, moveLeft, players
[username (rỗng với máy), x, y, hp, maxHp, alive, facing, gear, power, items, rage, lastAngle, lastPower…], result{…, victory?, loot?}, bet, chat,
mode, boss{id, name, level, hp, maxHp, x, y, size, alive} | null, bosses?, rageMax, ults[{arm, id:"ult", name, desc, dmg, r, n}]),
`shot` (by, weapon, arm, paths, blasts, hits[{id, dmg, hp, crit?}], deaths, tele, pushes[{id, x, y}] (Lau sàn: vị trí sau khi
bị đẩy), rage{<id>: nộ sau phát bắn}, chiêu cuối thêm `ult` (id vũ khí) + `ultName` — gửi trước state, `turn` = "" trong lúc
đạn bay; boss bắn có `by` = `turn` = "boss"; boss nhiều phát gửi lần lượt từng tin `shot`, tin sau đến khi phát trước bay xong;
Mưa lửa là một tin `shot` có `skill:"rain"`, `skillName:"Mưa lửa"`, mỗi người một đường rơi thẳng đứng trong `paths`),
`loot` (`{drops: {<id người chơi>: [{kind, tier, name, value}]}}`, khi hạ boss), `aim`, `chat`, `error`.

## Trường đua

Đua kart 3D 2–6 xe (người hoặc máy), 3 vòng, cược xu. Cần ít nhất 30 HP để vào.

- **Phòng:** chủ phòng chọn chế độ (**Tốc độ** hoặc **Đạo cụ**), đường (mọi file trong `public/race/tracks/`) và mức cược 100 / 500 / 1.000 / 5.000 / 10.000 xu mỗi người, thêm máy,
  rồi bắt đầu; ai thiếu xu thì không bắt đầu được. Đếm ngược 3 giây rồi xuất phát.
- **Về đích:** server tự tính vòng theo các điểm kiểm tra của đường. Người nhất về thì những người khác có thêm 90 giây;
  hết giờ, ai chưa về xếp theo số vòng và điểm kiểm tra đã qua. Rời phòng hoặc mất kết nối quá 15 giây giữa cuộc đua thì tính bét.
- **Tiền:** người nhất nhận cược của mọi người, trừ thuế 5% tiền thắng; XP nhất +30, bét +10, giữa chia đều.
- **Chống gian lận:** mỗi máy tự mô phỏng xe và gửi vị trí ≈15 lần/giây; server bỏ gói vượt tốc (trần xe × nitro 1,3 × 1,1),
  gói dịch chuyển xa hơn trần cho phép, vòng nhanh hơn thời gian tối thiểu của đường, và không tính vòng nếu bỏ điểm kiểm tra.
  Sai 15 lần trong một cuộc đua thì bị loại (tính bét).
- **Máy đua** chạy ở server theo đường mẫu, tốc độ 85–97% trần xe của nó; ở chế độ Đạo cụ máy cũng nhặt hộp và dùng món.
- **Nitro:** drift nạp nitro, tốc độ nạp ×2 (`nitroCharge`); dùng thì ×1,3 tốc độ tối đa trong 2 giây.
- **Chế độ Đạo cụ:** hộp "?" trên đường (`itemBoxes` trong file đường), chạy qua (cách hộp ≤ 6 m) nhận ngẫu nhiên 1 món, giữ tối đa
  2 món; hộp tắt 3 giây sau khi bị nhặt. Món theo hạng: dẫn đầu chủ yếu dầu / khiên, xếp cuối dễ ra tên lửa đuổi / tăng tốc.
  Server mô phỏng đạn và dầu 20 lần/giây và quyết định trúng (≤ 3 m); bị trúng thì xoay 1,2 giây, tốc độ còn 30%.

  | Món | Tác dụng |
  |---|---|
  | Tên lửa (`missile`) | bay dọc đường về trước 70 m/s, tối đa 3 giây |
  | Tên lửa đuổi (`homing`) | đuổi xe đứng ngay trên mình một hạng, 55 m/s, tối đa 6 giây |
  | Vũng dầu (`oil`) | thả sau xe 4 m, có trong 20 giây (người thả không dính trong 1,5 giây đầu) |
  | Khiên (`shield`) | 5 giây, chặn 1 đòn |
  | Tăng tốc (`boost`) | ×1,25 tốc độ tối đa trong 1,5 giây |

| Xe (theo hạng) | Giá | Level |
|---|---|---|
| Xe Tập Lái | miễn phí | 1 |
| Sao Chổi · **Gió Lốc** (moto) · Tốc Hành · Tia Chớp · Chim Ưng · Rắn Lục | 60.000 · 90.000 · 150.000 · 240.000 · 450.000 · 750.000 | 3 · 5 · 7 · 9 · 12 · 15 |
| **Bão Tố** (moto) · Thiên Đỉnh · Bóng Ma · **Neon Rider** (moto) | 900.000 · 1.200.000 · 2.400.000 · 3.000.000 | 17 · 19 · 22 · 24 |
| Lốc Xoáy · Đỉnh Cao · Hoả Tiễn · Siêu Tân Tinh | 3.600.000 · 4.800.000 · 5.400.000 · 6.000.000 | 26 · 28 · 29 · 30 |

- Mua xe cần đủ level (trường `level` trong garage; chưa đủ → 403 "Cần cấp N để mua …"); xe đã có vẫn dùng và nâng cấp.
- **Xe / đồ độc quyền** (`exclusive: true` trong garage): Cực Quang `x_aurora` (xe), Băng Phong `x_frost` (moto) — chỉ số bằng
  hạng Siêu Tân Tinh — và Sơn ánh kim `paint_x_chroma`, Decal bông tuyết `decal_x_snow`, Neon cầu vồng `neon_x_rainbow`,
  Cánh gió phát sáng `spoiler_x_glow`. Không bán (mua → 409 "Chỉ ghép từ mảnh"), chỉ ghép từ mảnh nhiệm vụ (xem
  [Nhiệm vụ và mảnh xe](#nhiệm-vụ-và-mảnh-xe)).

- **Moto** (`kind: "moto"`, ô tô là `"car"`): tăng tốc +5%, bám đường −5% so với ô tô cùng hạng, không lắp được cánh gió (409).

- **Chỉ số:** xe khởi đầu cấp 0: tốc độ 30 m/s, tăng tốc 12 m/s², bám đường 0,70. Mỗi hạng xe cao hơn một chút; mỗi cấp nâng +2,5%;
  xe đắt nhất nâng tối đa hơn xe khởi đầu ≈20% mỗi chỉ số; không mẫu nào (kể cả moto) vượt trần đó.
- **Nâng cấp** 3 món, mỗi món 5 cấp: động cơ / ống xả (tốc độ), turbo (tăng tốc), lốp / mâm (bám đường). Giá lên cấp n =
  giá xe × 10% × n (xe khởi đầu tính như 30.000 xu).
- **Đồ trang trí** (không có chỉ số): cánh gió 10.000–80.000, đèn neon gầm 20.000–24.000, decal 10.000–100.000; sơn 8 màu miễn phí.

| API (`/api/race`, cần đăng nhập) | Body | |
|---|---|---|
| `GET /api/race/garage` | | `{cars:[{id,name,kind,price,owned,levels,stats,cosmetics}], current, items, upgradePrice, maxLevel, nitroBoost, nitroMs, nitroCharge}` |
| `POST /api/race/car/buy` · `/car/select` | `{car}` | mua (chọn luôn) / chọn xe |
| `POST /api/race/upgrade` | `{car, part}` | `part`: `engine` / `turbo` / `tires` |
| `POST /api/race/item/buy` | `{item}` | mua đồ trang trí |
| `POST /api/race/equip` | `{car, kind, item}` | `kind`: `spoiler` / `neon` / `paint` / `decal`; `item: ""` để tháo |
| `GET /api/race/rooms` | | phòng đang mở (không cần đăng nhập) |

Các POST trả `{garage, user}`; thiếu xu 402, chưa có xe / đã có / cấp tối đa 409, sai tham số 400, không có món 404.
WebSocket `/ws/race`: client gửi `create {token, bet, track, mode}`, `join {token, code}`, `addBot`, `kick {seat}`, `setBet {bet}`,
`setTrack {track}`, `setMode {mode}`, `start`, `leave`, `chat {text}`, `state {x, z, rot, speed, nitro, drift}` khi đua,
`pickup {box}` và `use` (chế độ Đạo cụ); server gửi `room` (phòng, ghế kèm xe và đồ đang lắp, `phase` lobby / countdown /
racing / over, `mode`, `boxes [{id,x,z,active}]`, `nitroCharge`, `startAt`, `now` theo ms), `cars {ms, list}` ≈15 lần/giây
(mỗi xe thêm `shield`, `boost`, `spin`), `finish {seat, place, timeMs}`, `result {places, money, tax, xp}` (mảng theo ghế),
`items {list}` (riêng người giữ), `box {id, active}`, `fx {id, kind, x, z, rot, owner, dead}`, `hit {seat, by, kind, blocked?}`, `error`. Đường đua đọc từ
`public/race/tracks/<id>.json` (cùng file giao diện dùng).

## Công trường (ngoại ô)

Làm thuê theo ca để kiếm xu. Nhận ca cần ≥ 30 HP; mỗi ca trừ máu ngay, giao diện chơi mini-game trong thời gian ca
rồi nộp điểm (số mục tiêu hoàn thành, tối đa `seconds / 2`).

| Việc | Level | HP | Thời gian | Tiền công (0 → đủ điểm) | XP |
|---|---|---|---|---|---|
| Khuân gạch | 1 | 8 | 30 giây | 120 → 300 | 4 |
| Trộn vữa | 6 | 10 | 30 giây | 200 → 500 | 6 |
| Lái cần cẩu | 12 | 12 | 40 giây | 400 → 900 | 10 |

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

## Nhiệm vụ và mảnh xe

Nhiệm vụ hệ thống nằm ở nông trại: nhận, đổi nhiệm vụ và lấy thưởng tại NPC trong vườn (mùa Noel 1/12–6/1 — năm 2026 từ
tháng 11 — là Ông già Noel, ngoài mùa là Bác nông dân). Thưởng là **mảnh ghép xe đua**. Gói `games/quests`.

- **Ngày** (reset 0:00 giờ VN): 5 nhiệm vụ khác loại, mỗi cái 1 mảnh + 20 XP; xong cả 5 → rương ngày 1 mảnh.
  **Tuần** (reset thứ Hai 0:00): 3 nhiệm vụ, mỗi cái 3 mảnh + 100 XP; xong cả 3 → rương tuần 1 mảnh độc quyền.
  Xong rồi phải bấm Nhận (tiến độ không vượt mục tiêu; đã nhận thì thôi cộng).
- Loại nhiệm vụ (ngày: cấp 1 → cấp 30 nội suy; tuần cố định): thu hoạch ô cây 10→30 / 150, gieo hạt 10→30 / —,
  lấy trứng–sữa–len 5→15 / 60, bán hoặc thu con vật–cá 2→6 / 25, hạ zombie (đập hoặc súng đậu) 2→5 / 20,
  thu hoạch tầng mây 1→3 / 10 (chỉ khi đã có chậu), thăm (mỗi bạn một lần / ngày) hoặc trộm vườn bạn 3 / 15,
  câu được cá (không tính rác) 5→15 / 60, hoàn thành cuộc đua (không bỏ cuộc) 2→4 / 15, về nhất 1 / 5.
- **Đổi nhiệm vụ** ngày chưa xong lấy loại khác (tiến độ về 0): lần đầu mỗi ngày miễn phí, sau đó 2.000 xu, tối đa 5 lần / ngày.
- **Mảnh**: mỗi phần thưởng rút bậc Thường 60% · Hiếm 28% · Sử thi 10% · Độc quyền 2%, trong bậc chia đều.

  | Bậc | Món | Mảnh cần |
  |---|---|---|
  | Thường | Sao Chổi, Gió Lốc, Tốc Hành, Tia Chớp, Chim Ưng | 6 |
  | Hiếm | Rắn Lục, Bão Tố, Thiên Đỉnh, Bóng Ma, Neon Rider | 8 |
  | Sử thi | Lốc Xoáy, Đỉnh Cao, Hoả Tiễn, Siêu Tân Tinh | 8 |
  | Độc quyền | xe Cực Quang `x_aurora`, moto Băng Phong `x_frost` · Sơn ánh kim, Decal bông tuyết, Neon cầu vồng, Cánh gió phát sáng | xe 5 · đồ 3 |

- **Ghép**: đủ mảnh và chưa có món → xe vào garage (cấp 0, sơn mặc định, không đổi xe đang chọn), **không cần đủ cấp**.
  **Đổi mảnh thừa** (đã có món, hoặc vượt số cần): 3 mảnh thừa cùng bậc → 1 mảnh ngẫu nhiên bậc trên (Sử thi → Độc quyền;
  Độc quyền không đổi). Không bán mảnh lấy xu.
- Các game báo sự kiện qua hook đặt trong `main.go` (`farm.OnEvent`, `friends.OnVisit`, `fishing.OnCatch`,
  `race.Options.OnEvent`); súng đậu ghi ngay trong giao dịch nông trại, còn lại ghi sau khi thao tác xong; lỗi ghi nhiệm vụ
  không làm hỏng thao tác gốc.

| API (`/api/quests`, cần đăng nhập) | Body | Trả |
|---|---|---|
| `GET /api/quests` | | `{daily[], weekly[], chest{daily{ready, claimed}, weekly{…}}, shards[{item, name, tier, kind, qty, need, owned}], rates, rerollCost, rerollsLeft, resetAt{day, week}, npc: "santa"\|"farmer", pending}` |
| `POST /api/quests/claim` | `{id}` (`d0`–`d4`, `w0`–`w2`) | `{quests, gained:[{item, name, tier, kind, qty}]}` |
| `POST /api/quests/chest` | `{kind}` `daily` / `weekly` | `{quests, gained}` |
| `POST /api/quests/reroll` | `{id}` (chỉ nhiệm vụ ngày) | `{quests, cost}` |
| `POST /api/quests/assemble` | `{item}` | `{quests, granted:{item, kind}}` |
| `POST /api/quests/exchange` | `{tier}` `common` / `rare` / `epic` | `{quests, gained}` |

Nhiệm vụ: `{id, kind, name, progress, target, reward{shards, xp}, done, claimed}`. Lỗi: 400 sai tham số, 402 thiếu xu,
409 chưa xong / đã nhận / hết lượt đổi / thiếu mảnh / đã có, 401 chưa đăng nhập.

## Khu danh vọng

Bảng vàng top 10 người giàu nhất theo **tổng tài sản** (xu), tính lại tối đa 30 giây một lần:

- **Xu** trong ví.
- **Đồ thời trang** đã có (quần áo, tóc, cánh, gương mặt) theo giá shop; món miễn phí / không còn bán tính 0.
- **Nông trại** theo giá trong game: ô đất đã mua (6 ô miễn phí tính 0) + hạt của cây đang trồng; chuồng / ao (giá mua +
  các lần mở rộng; sân gia cầm chỉ tính mở rộng) và mở rộng kho; vật nuôi / cá đang nuôi theo giá mua;
  kho giống (hạt, thức ăn) theo giá mua, kho sản phẩm theo giá bán; trên mây: tiền hạt đậu, chậu và hạt của cây trong chậu.

Bằng tổng thì ai tạo tài khoản trước đứng trên. Bảng chỉ có tên hiển thị và ngoại hình, không có tên đăng nhập.

| API (không cần đăng nhập) | Mô tả |
|---|---|
| `GET /api/fame` | `{updatedAt (ms), top:[{rank,id,name,level,gender,outfit,hair,wings,face,total,coins,items,farm}], me}`; `me` (cùng dạng một dòng, hạng thật trong mọi người chơi) chỉ có khi gửi token hợp lệ |

## Bạn bè, trộm nông trại và chó giữ nhà

- **Kết bạn**: mời theo tên đăng nhập (không phân biệt hoa thường) hoặc id; người kia chấp nhận / từ chối (người mời cũng
  rút lại được bằng decline); huỷ kết bạn xoá cả hai chiều. Tối đa 100 bạn. Hai người mời chéo nhau thì thành bạn luôn.
- **Trộm con**: cũng trộm được vật nuôi và cá đã lớn đang không có zombie (con non và chó thì không). Kẻ trộm nhận đúng món chủ nhận
  khi thu con đó (thịt hoặc cá) vào kho sản phẩm, không có XP; chủ mất con, chuồng hoặc ao trống lại một chỗ. Chung 5 lượt / ngày với
  trộm cây, mỗi người chỉ thử mỗi con một lần. Chó canh như với cây; chủ thu hoặc bán con đó trong lúc đuổi thì kẻ trộm thoát mà không có gì.
- **Trộm**: chỉ vào được nông trại của bạn bè, chỉ trộm ô cây đã chín. Mỗi ô chỉ ra 1 nông sản nên kẻ trộm lấy đúng
  nông sản đó vào kho sản phẩm của mình (kho đầy thì không trộm được); chủ thu hoạch ô bị trộm chỉ còn XP. Mỗi vụ một ô
  chỉ bị trộm thành công một lần, và mỗi người chỉ được **thử** mỗi ô một lần mỗi vụ (bị bắt cũng mất lượt thử).
  Mỗi người được thử 5 lần / ngày (giờ Việt Nam) trên một nhà.
- **Chó giữ nhà**: cho ăn 1 phần *Thức ăn cho chó* (40 xu, mua ở cửa hàng, cất kho giống) thì no thêm **4 giờ**, cộng dồn
  vào thời gian còn lại, tối đa **7 ngày** (42 phần). Chó đói không canh: trộm lấy được ngay. Chó no thì **đuổi theo** kẻ trộm
  (tốc độ 255, người chơi 230 đơn vị / giây): chạy ra tới cổng là thoát, lấy được đồ; bị chó chạm thì mất **50 máu**
  (hết máu thì nằm viện như bị xe đụng), không lấy được gì. Trong lúc đuổi ô đó khoá với người khác. Server kiểm tra:
  báo thoát sớm hơn 1,5 giây kể từ lúc bắt đầu, hoặc không báo kết quả trong 20 giây, đều tính là bị bắt.
  Chủ nhà xem nhật ký 50 lần trộm gần nhất trong `GET /api/farm` (`thefts`).

| API (cần đăng nhập) | Body | Mô tả |
|---|---|---|
| `GET /api/friends` | | `{friends:[{id,name,level,gender,outfit,hair,wings,face,online,ripe,dogGuarding}], incoming:[…], outgoing:[…]}`; `ripe` = số ô chín chưa bị trộm |
| `POST /api/friends/request` | `{username}` hoặc `{id}` | gửi lời mời (trùng / đã là bạn → 409); trả như GET |
| `POST /api/friends/accept`, `/decline`, `/remove` | `{id}` | trả như GET |
| `GET /api/friends/{id}/farm` | | `{owner, now, plots:[{index,crop,plantedAt,readyAt,ripe,stolen,tried,locked}], stock:[{id,place,kind,grown,tried,locked,zombie}], dog:{fedUntil,guarding,fullFor,maxFull}, stealsLeft}`; không phải bạn thì `stealsLeft` = 0 |
| `POST /api/friends/{id}/steal` | `{plot}` hoặc `{stock}` (có cả hai hoặc không có gì → 400) | chó đói: `{result:"stolen", item, qty, hpLost:0, farm, user}`; chó canh: `{result:"chase", chase:{id,plot,stock,item,qty,startedAt,dogSpeed,minMs,expiresAt}, …}` (ms); chưa chín / đã bị trộm / đã thử / đang có người trộm / hết lượt / kho đầy / con chưa lớn / có zombie → 409; con không tồn tại hoặc của nhà khác → 404; không phải bạn → 403 |
| `POST /api/friends/{id}/steal/finish` | `{chase, escaped}` | `{result:"stolen"\|"caught"\|"gone", item, qty, hpLost, farm, user}`; `gone` = chủ đã thu hoạch, thu hoặc bán con trong lúc đuổi; token sai → 404, đã dùng → 409 |
| `POST /api/farm/dog/feed` | | cho chó ăn, cộng dồn (no tối đa → 409); `GET /api/farm` có thêm `dog:{fedUntil,guarding,fullFor,maxFull}` và `thefts:[{at,thief:{id,name},item,qty,caught,hpLost,chasing}]` |

## Tiến Lên Miền Nam

1. Ở hub chọn **Tiến Lên Miền Nam** → **Tạo phòng mới**, gửi link phòng cho bạn bè. Bạn bè mở link, tạo tài khoản (nếu chưa có) là vào thẳng phòng.
2. Chủ phòng chọn **mức cược**, **Thêm máy** / **Mời ra**, rồi **Bắt đầu** khi có 2–4 người. Ai không đủ xu cho mức cược thì không bắt đầu được.
3. Bấm lá bài để chọn, **Đánh** (Enter), **Bỏ lượt** (Space), **Gợi ý** để máy chọn giúp.

- Mỗi lượt 30 giây, hết giờ máy đánh thay. Mất mạng / tải lại trang vẫn giữ ghế (vào lại bằng link phòng), trong lúc vắng máy đánh thay.
- Rời phòng giữa ván thì máy đánh nốt ván, kết quả vẫn tính vào ví người rời.
- Máy đánh chia tay bài thành ít bộ nhất, nhớ lá đã ra để biết bộ nào không ai chặn được, chặn người sắp hết bài; máy không xem bài người khác.

### Luật

- Thứ tự: 3 < 4 < … < K < A < 2; chất ♠ < ♣ < ♦ < ♥.
- Bộ bài: rác, đôi, sám cô, sảnh (≥3 lá, không có 2), đôi thông (≥3 đôi liên tiếp), tứ quý.
- Chặn: cùng loại, cùng số lá, lá cao nhất lớn hơn.
- Chặt: 3 đôi thông chặt heo; tứ quý chặt heo, đôi heo, 3 đôi thông; 4 đôi thông chặt heo, đôi heo, 3 đôi thông, tứ quý.
- Bỏ lượt là mất quyền đánh tới hết vòng.
- **Có người về nhất (hết bài) là ván dừng ngay**, những người còn lại không đánh tiếp. Xếp hạng: người nhất, rồi
  người còn ít lá hơn; bằng lá thì người ngồi gần sau người nhất (theo chiều đánh) xếp trước; người bị cóng ở cuối.
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
- **Thối**: hết ván, **mọi người thua** còn heo / hàng trên tay đều trả cho người về nhất (mỗi người một khoản, kể cả người bị cóng).
- **Cóng**: khi có người về nhất mà ai đó chưa đánh lá nào thì người đó bị cóng: xếp bét, trả 26 lá và cũng phải thối heo / hàng.
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
games/race/              trường đua: garage (xe, nâng cấp, đồ), phòng đua, kiểm gói vị trí, máy đua
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

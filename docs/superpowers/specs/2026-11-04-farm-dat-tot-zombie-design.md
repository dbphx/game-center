# Nông trại: đất tốt và zombie

## Mục tiêu

Thêm hai cơ chế cho nông trại:

- **Đất tốt:** chỗ tiêu xu dài hạn, cây chín nhanh hơn.
- **Zombie:** mỗi giờ có một đợt tấn công chung toàn server, người chơi phải canh để đập, không đập kịp thì mất cây.

Thành công khi: đợt tấn công chạy đúng giờ kể cả khi server khởi động lại hoặc người chơi offline, không có cách né
zombie ngoài việc đập, và người không bảo vệ mất khoảng **50% số cây sau mỗi 12 giờ**, nhà to nhỏ như nhau.

## Luật đã chốt với người dùng

### Đất tốt

- Mỗi ô nâng một lần, giá **8.000 xu**, giữ vĩnh viễn.
- Cây trên ô đất tốt chín nhanh hơn **30%**: `readyAt = now + floor(GrowSeconds × 0.7)`.
- Cây đang trồng giữ nguyên giờ chín; áp dụng từ lần gieo sau, gồm cả gieo một ô (`Plant`) và trồng tất cả (`PlantAll`).
- Tiền nâng cấp tính vào giá trị nông trại ở Khu danh vọng (`Worth`).

### Zombie

- **Thời điểm:** đúng đầu mỗi giờ theo giờ server (phút 00), chung cho mọi nông trại. Mỗi đợt có **5 phút** để đập.
- **Mức mất mục tiêu:** mỗi cây có `p = 1 − 0,5^(1/12) ≈ 5,61%` nguy cơ bị ăn mỗi giờ nếu không canh, tức mất 50% sau 12 giờ.
- **Số lượng:** `max(1, ceil(p × số cây đủ điều kiện))`, không vượt quá số cây đó; nhà không có cây thì không có zombie.
- **Xác suất ăn:** mỗi zombie không bị đập có xác suất `q = p × số cây / số zombie` ăn cây, ngược lại nó bỏ đi và cây còn nguyên.
  Vì mục tiêu chọn đều trong các cây, nguy cơ của mỗi cây đúng bằng `p`.

| Số cây đang có | Số zombie | Xác suất ăn nếu không đập |
|---|---|---|
| 6 | 1 | 34% |
| 20 | 2 | 56% |
| 48 | 3 | 90% |

- **Mục tiêu:** chọn ngẫu nhiên trong các ô có cây được gieo trước thời điểm đợt bắt đầu, gồm cả cây đã chín. Mỗi ô tối đa 1 zombie.
- **Đập:** 1 nhát chết, mỗi nhát trừ **2 máu**. Không có thưởng xu hay XP; phần thưởng là giữ được cây.
- **Không đập kịp:** hết 5 phút thì zombie ăn cây theo xác suất `q` (mất cây và mất hạt) hoặc bỏ đi, kể cả khi chủ nông trại offline.

Không canh thì cây 12 giờ còn 50% cơ hội sống tới lúc chín; cây ngắn mất ít hơn cho mỗi vụ (cây 1 giờ khoảng 5,6%). Cây đã chín
để lâu không thu hoạch vẫn tiếp tục chịu nguy cơ này.

### Luật phụ

- Ô có zombie **không thu hoạch được** (cả `Harvest` và `HarvestAll` bỏ qua ô đó) và **không bị bạn bè trộm được**.
  Nếu không, thu hoạch sẽ là cách né zombie.
- **Được đập khi máu dưới 40.** Thao tác nông trại khác vẫn cần từ 40 máu. Luật vào khu trên bản đồ giữ nguyên: người dưới
  40 máu đang ở ngoài phải chữa máu mới vào được.
- **Đang nằm viện thì không đập được.** Nhát đập làm hết máu thì vào viện theo `wallet.Tx.Damage`, như các nguồn mất máu khác.
- Chỉ chủ nông trại đập được, bạn bè không đập giúp.
- Đất tốt không chống được zombie.
- Lần đầu server thấy một nông trại (kể cả ngay sau khi deploy tính năng), không tính đợt nào trong quá khứ; đợt đầu tiên
  là đầu giờ kế tiếp.

## Kiến trúc: tính lười

Không có tiến trình nền. Mọi đường đọc hoặc ghi nông trại gọi `settleZombies(ctx, tx, uid, now)` **trước** `load`, trong
cùng transaction, theo đúng cách `settleChases` đang làm ở `State` và `do`.

Lý do chọn: khớp cách tính lười của con vật, không bỏ sót đợt khi server tắt, không có đợt ghi hàng loạt vào SQLite mỗi giờ.
Cách dùng tiến trình nền bị loại vì có các nhược điểm ngược lại.

### Dữ liệu

```sql
ALTER TABLE farm_plots ADD COLUMN good INTEGER NOT NULL DEFAULT 0; -- 1 = đất tốt

CREATE TABLE IF NOT EXISTS farm_zombies (
	user_id INTEGER NOT NULL REFERENCES users(id),
	idx     INTEGER NOT NULL,          -- ô đất
	wave    INTEGER NOT NULL,          -- giờ của đợt: unix / 3600
	bites   INTEGER NOT NULL,          -- 1 = hết hạn sẽ ăn cây, 0 = bỏ đi; quyết định lúc zombie xuất hiện, không gửi cho giao diện
	PRIMARY KEY (user_id, idx)
);
CREATE TABLE IF NOT EXISTS farm_zombie_progress (
	user_id   INTEGER PRIMARY KEY REFERENCES users(id),
	last_wave INTEGER NOT NULL,        -- đợt mới nhất đã tính
	eaten     INTEGER NOT NULL DEFAULT 0 -- số cây bị ăn chưa báo cho chủ
);
```

Cột `good` thêm bằng cơ chế nâng cấp bảng sẵn có (`migrate` ở `games/farm/feed.go`), kiểm tra cột trước khi thêm vì SQLite
không có `ADD COLUMN IF NOT EXISTS`.

### `settleZombies`

Với `cur = now / 3600`:

1. Chưa có dòng `farm_zombie_progress`: tạo với `last_wave = cur`, dừng.
2. Với zombie đang có mà `now >= wave × 3600 + 300`: xoá zombie; nếu `bites = 1` thì xoá cây (`clearPlot`) và tăng `eaten`.
3. Với mỗi đợt `w` từ `last_wave + 1` tới `cur`, theo thứ tự thời gian:
   - Đọc các ô có cây với `planted_at < w × 3600` và chưa có zombie.
   - Chọn mục tiêu và quyết định `bites` bằng bộ sinh ngẫu nhiên có seed cố định từ `(uid, w)`, để tính lại không ra kết quả khác.
   - Thêm zombie; đợt đã quá 5 phút thì xử lý hết hạn ngay như bước 2.
4. Ghi `last_wave = cur`.

Vòng lặp ở bước 3 giới hạn tối đa **720 đợt** (30 ngày) gần nhất để người vắng rất lâu không làm chậm request. Nhà có 0 ô có
cây thì đợt đó không có gì để chọn.

`settleZombies` chỉ ghi khi có thay đổi, nhưng luôn chạy trong transaction của `State` / `do` hiện có.

Vì cây được chọn theo `planted_at < w × 3600` và zombie chặn thu hoạch, cây có zombie không thể bị thay giữa chừng trừ khi
bị ăn, nên không cần lưu `planted_at` trong `farm_zombies`.

### Các đường đọc cần xử lý

| Đường đọc | Xử lý |
|---|---|
| `Farm.State`, `Farm.do` | gọi `settleZombies` cho chủ nông trại |
| Trộm và xem nông trại bạn (`steal.go`) | gọi `settleZombies` cho **chủ** nông trại trước khi đọc; từ chối trộm ô có zombie; cuộc chó đuổi đang chạy trên ô bị zombie giữ nguyên, ô đó chốt theo luật trộm hiện có trước khi tính cây bị ăn |
| `HarvestAll` | bỏ qua ô có zombie |
| `Worth` | không gọi; chỉ đọc nên lệch tối đa một đợt, chấp nhận được |

### API

Trả về cùng dạng `{ farm, user }` như các API nông trại khác để giao diện cập nhật máu và xu.

- `POST /api/farm/plot/upgrade` `{ plot }`: nâng ô lên đất tốt. Lỗi: ô không hợp lệ, đã là đất tốt, không đủ xu.
- `POST /api/farm/zombie/hit` `{ plot }`: đập zombie ở ô đó, trừ 2 máu. Route này **không** dùng kiểm tra `CheckZone` chung; thay
  vào đó từ chối nếu đang nằm viện. Lỗi: ô không có zombie (kể cả vừa bị ăn), đang nằm viện.
- `GET /api/farm/zombie/status`: `{ count, until }` cho bản đồ thành phố, `count` là số zombie đang có, `until` là lúc hết hạn đập.

`State` thêm:

- `Plot.good` (bool), `Plot.zombie` (bool).
- `zombieUntil` (unix, 0 nếu không có zombie).
- `zombieEaten` (số cây bị ăn kể từ lần báo trước). Chỉ `Farm.State` (GET) đọc rồi đặt lại về 0; `do` trả số hiện có mà không xoá.
- `catalog` thêm `goodSoilPrice`, `goodSoilSpeedup`.

## Giao diện

- Ô có zombie: ảnh zombie đứng trên ô, bấm để đập, có hiệu ứng trúng đòn và số `-2` máu. Có đồng hồ đếm ngược 5 phút trên nông trại.
- Ô đất tốt: ảnh đất riêng. Hộp thông tin ô có nút nâng cấp ghi giá và mức nhanh hơn.
- `zombieEaten > 0`: thông báo "Zombie đã ăn mất N cây" khi vào nông trại.
- Bản đồ thành phố (`public/hub.js`): khi mở trang và vào đầu mỗi giờ, gọi `GET /api/farm/zombie/status`; nếu `count > 0` thì hiện
  thông báo kèm nút về nông trại. Không có thông báo ở các trang game khác.
- Mọi chữ hiển thị bằng tiếng Việt.

### Hình ảnh (bắt buộc theo `docs/ART.md`)

- Zombie: PNG từ **Kenney Toon Characters** (CC0), cùng bộ với nhân vật hiện có. Cần kiểm tra lại gói có zombie khi tải; nếu không
  có thì tìm trong nguồn CC0 khác và hỏi người dùng trước khi dùng nguồn có ghi công.
- Đất tốt: PNG mới ghép bằng PIL từ `public/img/toon/farm/soil_plot.png`, đậm màu hơn để phân biệt, phát hành CC0.
- Không vẽ bằng SVG hoặc CSS gradient. Cập nhật `manifest.json`, `CREDITS.csv` / `LICENSE.txt` của thư mục.

## Tài liệu

Cập nhật mục Nông trại trong `README.md`: luật đất tốt, luật zombie, API mới. Sửa câu "cây dài (4–12 giờ) cho người treo máy" vì
không còn đúng hoàn toàn.

## Kiểm thử

Test Go dùng `Farm.now` để giả lập thời gian:

- Đất tốt: giá, không nâng hai lần, chín nhanh 30% với `Plant` và `PlantAll`, cây đang trồng không đổi giờ chín, `Worth` có cộng.
- Số zombie và xác suất: 6 / 20 / 48 cây ra 1 / 2 / 3 con với `q` đúng bảng; không vượt quá số cây; không có cây thì không có zombie.
- Mức mất: giả lập nhiều nhà 6, 20, 39, 48 cây không canh trong 12 đợt, tỉ lệ cây còn lại gần 50% ở mọi cỡ.
- Mục tiêu: chỉ cây gieo trước đợt; kết quả giống nhau khi tính lại với cùng `(uid, wave)`.
- Đập: trừ đúng 2 máu; được đập khi máu dưới 40; thao tác khác vẫn bị chặn dưới 40; từ chối khi nằm viện; hết máu vào viện.
- Hết hạn: `bites = 1` thì đúng 5 phút mất cây, trước đó chưa mất; `bites = 0` thì zombie bỏ đi, cây còn và thu hoạch lại được; `zombieEaten` được báo một lần rồi về 0.
- Offline: vắng nhiều giờ tính đủ từng đợt; vắng quá 720 đợt vẫn chạy nhanh; lần đầu thấy nông trại không tính ngược.
- Chặn né: không thu hoạch, không `HarvestAll`, không trộm được ô có zombie.

Chạy `go test -race ./...`, rồi chơi thử trên Chrome thật bằng cổng và database riêng ở thư mục tạm (không đụng cổng 8090 và
`data/gamehub.db`), tự xem ảnh chụp ở cỡ desktop và điện thoại.

## Ngoài phạm vi

- Bạn bè đập giúp, phần thưởng khi đập, zombie tấn công vật nuôi hay ao cá, nhiều cấp đất tốt.
- Thông báo đẩy ngoài trang web, thông báo ở các trang game khác.
- Deploy: làm riêng khi người dùng yêu cầu.

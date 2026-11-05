# Nông trại: zombie ăn vật nuôi và cá

## Mục tiêu

Mở rộng đợt zombie mỗi giờ sang chuồng, sân và ao. Thành công khi: mỗi con không được canh có nguy cơ khoảng 5,61% mỗi giờ
(mất 50% sau 12 giờ), không có cách né zombie ngoài đập hoặc bắn. Luật ở ruộng chỉ đổi cách chọn mục tiêu. Chó không bị tấn công.

Luật gốc của zombie ở `2026-11-04-farm-dat-tot-zombie-design.md` vẫn giữ nguyên.

## Luật đã chốt với người dùng

- **Khu:** ruộng, chuồng (`barn`), sân (`yard`), ao (`pond`) tính riêng. Khu có con thì có ít nhất 1 zombie.
- **Số zombie và xác suất ăn:** dùng `zombieCount(số con đủ điều kiện)` như cây.
- **Đủ điều kiện:** mọi con có `bought_at` trước lúc đợt bắt đầu, gồm con non và con đói. Mỗi con tối đa 1 zombie.
- **Chọn mục tiêu (áp dụng cho cả cây ở ruộng):** zombie lựa con hoặc cây gần trưởng thành nhất. Xếp theo tiến độ lớn giảm dần, lấy
  `count` mục tiêu đầu. Tiến độ tính tại lúc xét đợt, chặn ở 1,0, nên đã chín hoặc đã lớn đều ở mức cao nhất và để lâu không thu
  hoạch không an toàn. Bằng tiến độ thì chọn ngẫu nhiên theo seed. Cây: `(đầu đợt − planted_at) / (ready_at − planted_at)`.
  Vật nuôi: `grow_progress / thời gian lớn của loại đó`, dùng giá trị đã lưu. Tỉ lệ mất chung vẫn khoảng 50% sau 12 giờ, nhưng
  rủi ro dồn vào cây và con gần lớn, con non gần như an toàn khi nhà có nhiều con lớn hơn.
- **Đập:** 5 phút, 1 nhát, trừ 2 máu, được đập khi máu dưới 40, đang nằm viện thì không đập được, chỉ chủ đập được.
- **Hết hạn:** con `bites = 1` ăn mất con vật, con còn lại bỏ đi, kể cả khi chủ offline.
- **Súng đậu:** bắn cả zombie vật nuôi, mỗi con 1 viên. Thứ tự: đợt cũ trước; trong một đợt, ruộng trước theo `idx`, rồi vật nuôi theo `stock_id`.
- **Con đang bị tấn công:** từ chối bán, thu hoạch, lấy sản phẩm với thông báo "Phải đập zombie trước"; thao tác hàng loạt bỏ qua; vẫn cho ăn được.
- **Khi deploy:** không tính bù đợt quá khứ cho vật nuôi. Lần đầu thấy nông trại thì đợt đầu của vật nuôi là đầu giờ kế tiếp, kể cả người offline lâu đã có tiến độ zombie ở ruộng.

## Kiến trúc

Chọn bảng riêng, không đổi khoá của `farm_zombies`, để truy vấn ở ruộng và phần trộm giữ nguyên.

```sql
CREATE TABLE IF NOT EXISTS farm_stock_zombies (
	stock_id INTEGER PRIMARY KEY,      -- farm_stock.id
	user_id  INTEGER NOT NULL REFERENCES users(id),
	wave     INTEGER NOT NULL,
	bites    INTEGER NOT NULL          -- không gửi cho giao diện
);
CREATE INDEX IF NOT EXISTS farm_stock_zombies_user ON farm_stock_zombies(user_id);
CREATE TABLE IF NOT EXISTS farm_stock_zombie_progress (
	user_id   INTEGER PRIMARY KEY REFERENCES users(id),
	last_wave INTEGER NOT NULL,        -- đợt vật nuôi mới nhất đã tính
	eaten     INTEGER NOT NULL DEFAULT 0
);
```

- `settleZombies` xử lý thêm vật nuôi trong cùng vòng lặp đợt, chỉ với đợt lớn hơn `last_wave` của vật nuôi. Dòng tiến độ vật nuôi
  phải được tạo cả khi ruộng đi nhánh "lần đầu thấy" trả về sớm.
- Seed riêng cho mỗi khu: `rand.NewPCG(uid, wave*4 + zone)` với barn 1, yard 2, pond 3. Seed của ruộng giữ nguyên; xáo ngẫu nhiên trước rồi sắp xếp ổn định theo tiến độ, để tính lại vẫn ra cùng kết quả.
- Mọi đường xoá `farm_stock` xoá luôn dòng zombie của con đó, để không sót dòng và không làm đếm sai.

## API và JSON

- `Animal` có thêm `zombie: bool`, cả trong `barn`, `yard`, `pond`.
- `State.zombieUntil` tính cả zombie vật nuôi. Thêm `State.zombieEatenStock`, báo một lần như `zombieEaten`.
- `POST /api/farm/zombie/hit-stock {id}` trả `{farm, user}`, không qua `CheckZone`. 404 nếu không phải con của mình, 409 nếu không có zombie, hết hạn, hoặc đang nằm viện.
- `GET /api/farm/zombie/status`: `count` tính cả vật nuôi.
- Khách ghé thăm không nhận thêm dữ liệu gì mới.

## Giao diện

- Con có `zombie`: đứng yên, ảnh `zombie.png` đứng cạnh, bấm là đập, có hiệu ứng trúng đòn và `-2` máu như ở ruộng. Cá đứng yên, zombie đứng ngay chỗ cá.
- Bấm vào con đang bị tấn công cũng là đập, không mở hành động khác. Gợi ý `E` hiện "Đập zombie" và được ưu tiên.
- Súng đậu có đạn bay tới cả zombie vật nuôi.
- Thông báo: "Zombie đã ăn mất 3 cây, 2 con vật", thiếu vế nào thì bỏ vế đó.
- Băng đếm ngược và cảnh báo trên bản đồ không cần đổi logic vì dùng `zombieUntil` và `status`; số zombie ở băng tính cả vật nuôi.

## Kiểm thử

Test Go:

- Mỗi khu có con thì có ít nhất 1 zombie; giả lập 12 đợt không canh với 6, 20, 40 con: còn lại trong khoảng 0,44–0,56 (tính trung bình trên nhiều nông trại).
- Nhà có cây non và cây gần chín: zombie chọn cây gần chín trước; tương tự với con non và con gần lớn; cây đã chín không được miễn.
- Chó không có zombie; nhà chỉ có chó thì không có zombie.
- Đập: trừ 2 máu, con còn nguyên; đập lần hai báo không có zombie và không trừ thêm máu; đúng giây hết hạn thì không đập được; máu ≤ 2 thì nhát đó vẫn tính và người chơi vào viện.
- Bán, thu hoạch, lấy sản phẩm con đang bị tấn công bị từ chối; `CollectAllAnimals`, `ProduceAll`, `CollectAllFish` bỏ qua con đó; cho ăn vẫn được.
- Súng bắn đúng thứ tự, đạn trừ đúng, hết đạn thì dừng.
- Không tính bù: người có tiến độ ruộng cũ và không có tiến độ vật nuôi thì không mất con nào cho các đợt trước khi có tính năng.
- Con bị ăn: dung lượng chuồng trống lại, `Worth` giảm, `zombieEatenStock` báo đúng một lần.
- Không sót dòng `farm_stock_zombies` sau mọi đường xoá con vật.
- Cùng `(uid, đợt)` tính lại ra cùng kết quả.

Trình duyệt thật, cỡ 1440×900 và 390×844, cổng và database riêng: zombie ở chuồng, sân, ao hiện đúng, bấm chuột thật đập được, con đứng yên, thông báo đúng, không lỗi console.

## Ngoài phạm vi

- Hình zombie riêng cho ao, đổi mức mất theo loại con, bạn bè thấy zombie vật nuôi khi ghé thăm.

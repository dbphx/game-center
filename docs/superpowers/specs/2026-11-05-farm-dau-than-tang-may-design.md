# Nông trại: đậu thần và tầng mây

## Mục tiêu

Cho người chơi đã mua đủ 48 ô đất một mục tiêu tiêu xu dài hạn: trồng đậu thần ở ô thứ 49 để mở các tầng mây, rồi mua chậu
và trồng cây quý trên mây.

Thành công khi: người chưa đủ 48 ô không thấy gì khác, mỗi hạt đậu mở đúng một tầng kể cả khi chủ nông trại offline, và mọi
khoản mua trên mây đều tính vào giá trị nông trại ở Khu danh vọng.

## Luật đã chốt với người dùng

- Khi đủ 48 ô đất thì hiện **ô thứ 49**, chỉ dùng để trồng đậu thần.
- Trồng đậu thần **mở tầng mây**. Trên mây người chơi **mua chậu**, giá từ **100.000 đến 10.000.000 xu**.
- **Chậu càng đắt thì trồng được cây càng quý.** Mỗi loại chậu trồng đúng một loại cây mây.
- Màn mây là **màn riêng**, đổi tầng bằng tab và bấm chuột vào chậu, không đi bộ trên mây.
- Bảng số dưới đây, luật không có zombie và không bị trộm đã được duyệt ở bước thiết kế.

## Số liệu

| Tầng | Giá hạt đậu | Thời gian chờ | Giá chậu | Cây mây (id) | Tiền hạt mỗi vụ | Giá bán | Lãi mỗi vụ | XP |
|---|---|---|---|---|---|---|---|---|
| 1 | 200.000 | 24 giờ | 100.000 | Dâu mây (`cloud_berry`) | 5.000 | 20.000 | 15.000 | 200 |
| 2 | 1.000.000 | 48 giờ | 500.000 | Lê sương (`mist_pear`) | 20.000 | 70.000 | 50.000 | 300 |
| 3 | 3.000.000 | 72 giờ | 2.000.000 | Nho sao (`star_grape`) | 50.000 | 200.000 | 150.000 | 450 |
| 4 | 10.000.000 | 96 giờ | 10.000.000 | Đào tiên (`fairy_peach`) | 100.000 | 600.000 | 500.000 | 700 |

Mọi cây mây chín sau **12 giờ**. Mỗi tầng có **6 chỗ đặt chậu**, chỉ đặt được loại chậu của tầng đó.

Tổng đầu tư tối đa: 75.600.000 xu tiền chậu và 14.200.000 xu tiền hạt đậu. Đủ 24 chậu và 2 vụ mỗi ngày thu khoảng
8.580.000 xu/ngày, so với khoảng 1.040.000 xu/ngày của 48 ô sầu riêng. Các số này nằm trong hằng số của `catalog` để dễ chỉnh
nếu thấy xu tăng quá nhanh.

## Luật chi tiết

### Đậu thần

- Chỉ trồng được khi có đủ `MaxPlots` ô đất.
- Trả tiền hạt ngay lúc trồng, không qua kho giống. Một lúc chỉ có một hạt đang lớn.
- Tầng mở lần lượt, không bỏ qua. Hạt chín thì tầng kế tiếp mở vĩnh viễn và ô 49 trống lại, kể cả khi chủ offline.
- Mở đủ 4 tầng thì ô 49 chỉ còn cây đậu trưởng thành để leo lên, không trồng thêm.
- Không huỷ được hạt đang lớn, không có nút trả tiền để chín nhanh.
- Không có điều kiện level. Điều kiện duy nhất là đủ 48 ô.

### Chậu và cây mây

- Mua từng chậu cho tầng đã mở, chậu thứ `n` nằm ở chỗ `n−1`. Không bán lại được.
- Trồng: trả tiền hạt trực tiếp bằng xu, không dùng kho giống. Chậu đang có cây thì không trồng thêm.
- Thu hoạch: 1 nông sản vào **kho sản phẩm** kèm XP, chậu trống lại. Kho đầy thì báo lỗi như thu hoạch dưới đất và cây vẫn còn.
- Bán nông sản mây qua đường bán hiện có.
- Cây chín để lâu không hỏng, giống cây dưới đất.
- Đất tốt không áp dụng cho chậu và ô 49.
- Bản đầu không có gieo tất cả hoặc thu hoạch tất cả trên mây. Nút thu hoạch tất cả dưới đất không đụng tới mây.

### An toàn và bạn bè

- Zombie không tấn công ô 49 và cây mây. Súng đậu không liên quan.
- Bạn bè không trộm được, cũng không thấy ô 49 và tầng mây khi ghé thăm.
- Thao tác trên mây dùng cùng điều kiện máu và vùng như thao tác nông trại khác, tức qua `CheckZone`.

## Kiến trúc

Đậu và chậu **không nằm trong `farm_plots`**. `MaxPlots` giữ nguyên 48. Nhờ vậy zombie, trộm, thu hoạch tất cả, gieo tất cả,
giá đất và tóm tắt cho bạn bè không phải sửa và không có nguy cơ tính nhầm ô 49.

Cây mây nằm ở danh sách riêng `CloudCrops`, không thêm vào `Crops`, để không hiện trong kho giống và hộp gieo hạt dưới đất.
Những chỗ định giá nông sản trong `catalog.go:128` và `worth.go:165` phải tra thêm `CloudCrops`.

### Dữ liệu

```sql
CREATE TABLE IF NOT EXISTS farm_cloud (
	user_id       INTEGER PRIMARY KEY REFERENCES users(id),
	floors        INTEGER NOT NULL DEFAULT 0, -- số tầng đã mở (0–4)
	bean_planted  INTEGER NOT NULL DEFAULT 0, -- 0 = không có hạt đang lớn
	bean_ready_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS farm_pots (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	floor      INTEGER NOT NULL,              -- 1–4
	slot       INTEGER NOT NULL,              -- 0–5
	planted_at INTEGER NOT NULL DEFAULT 0,    -- 0 = chậu trống
	ready_at   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, floor, slot)
);
```

Chỉ thêm bảng mới, không sửa bảng cũ. Vẫn sao lưu database thật trước khi deploy.

### Tính lười

`settleBean(ctx, tx, uid, now)` chạy trong transaction, trong `State` và `do`: nếu có hạt và `bean_ready_at <= now` thì
`floors + 1` và xoá hạt. Không có tiến trình nền.

### API

Đều cần đăng nhập, qua `CheckZone`, trả `{farm, user}`; thu hoạch trả thêm `gained`.

| Route | Body | Lỗi chính |
|---|---|---|
| `POST /api/farm/bean/plant` | không có | 409 chưa đủ 48 ô, đang có hạt, đã đủ 4 tầng; 402 không đủ xu |
| `POST /api/farm/pot/buy` | `{floor}` | 400 tầng sai; 409 tầng chưa mở, đã đủ 6 chậu; 402 |
| `POST /api/farm/pot/plant` | `{floor, slot}` | 400 chỗ sai; 409 chưa có chậu, đang có cây; 402 |
| `POST /api/farm/pot/harvest` | `{floor, slot}` | 400 chỗ sai; 409 chưa có chậu, chậu trống, chưa chín, kho đầy |

`State` thêm:

```json
"cloud": {
  "available": true,
  "floors": 1,
  "beanPlantedAt": 0,
  "beanReadyAt": 0,
  "pots": [{"floor": 1, "slot": 0, "plantedAt": 0, "readyAt": 0}]
}
```

`available` là có đủ 48 ô. Catalog thêm `cloudFloors`, mỗi phần tử gồm `floor`, `beanPrice`, `beanSeconds`, `potPrice`, `slots`
và `crop` (`id`, `name`, `seedPrice`, `sellPrice`, `growSeconds`, `xp`).

`Worth` cộng tiền hạt đậu của các tầng đã mở và của hạt đang lớn, giá chậu, tiền hạt của cây đang trồng trong chậu. Nông sản
mây trong kho tính như nông sản thường.

### Giao diện

- **Ô 49:** hiện trong cảnh nông trại khi `cloud.available` và không phải đang ghé thăm. Đặt ở khoảng đất trống gần các khối
  ruộng, vị trí chốt khi thử trên màn hình để không che ô đất, súng đậu hay đường đi. Là vật cản và có gợi ý `E` như các đồ khác.
- **Bấm ô 49:** hộp thoại theo trạng thái: trồng hạt (ghi giá và thời gian), đang lớn (đếm ngược), hoặc nút lên mây.
- **Màn mây:** file mới `public/farm/cloud.js` và `cloud.css`, phủ toàn màn hình trên cảnh nông trại. Có nút xuống đất, tab tầng
  1–4 (tầng khoá hiện biểu tượng khoá), nền trời với `bg_sky_hills.png` và `cloud_1..3.png`, 6 chỗ đặt chậu.
- **Mỗi chỗ đặt chậu:** chưa có chậu (hiện giá, bấm để mua), chậu trống (bấm để trồng, hiện tiền hạt), đang lớn (đếm ngược),
  chín (bấm để thu hoạch).
- **Hỏi xác nhận** trước khi trồng hạt đậu và mua chậu, vì giá lớn.
- Hiện tên, giá bán và ảnh của nông sản mây ở kho sản phẩm, nghĩa là `find` và `art.item` trong `farm.js` phải biết `CloudCrops`.
- Mở trang mà có tầng mới mở từ lúc trước thì báo "Đậu thần đã vươn tới tầng N". Lưu số tầng đã báo trong `localStorage`, có
  try/catch.

### Hình ảnh

PNG vào `public/img/toon/farm/`, cập nhật `manifest.json` và `CREDITS.csv`. Tìm ảnh CC0 trước, không có thì ghép bằng PIL như
các cây khác. Không dùng SVG hay CSS để vẽ.

- `bean_1.png`, `bean_2.png`, `bean_3.png`: ba giai đoạn của cây đậu, `bean_3` là cây cao vươn lên.
- `pot_1.png` … `pot_4.png`: bốn chậu, nhìn rõ chậu sau sang hơn chậu trước.
- `crop_<id>_1.png`, `crop_<id>_2.png`, `crop_<id>_ripe.png`, `item_<id>.png` cho bốn cây mây.

## Kiểm thử

Test Go:

- Chưa đủ 48 ô thì trồng đậu bị từ chối; đủ 48 ô thì trừ đúng giá tầng 1.
- Trồng hạt thứ hai khi đang có hạt bị từ chối và không trừ xu.
- Trước giây chín chưa mở tầng; đúng `bean_ready_at` thì mở, qua `State` lẫn `do`, không cần thao tác của chủ.
- Sau tầng 4 không trồng được nữa.
- Mua chậu ở tầng chưa mở, tầng 0, tầng 5, hoặc chậu thứ 7 đều bị từ chối và không trừ xu.
- Trồng, chưa chín thì thu hoạch bị từ chối, chín thì có 1 nông sản và XP đúng, chậu trống lại.
- Kho đầy thì thu hoạch báo lỗi và cây còn nguyên.
- Bán nông sản mây đúng giá.
- Đợt zombie không chọn ô 49 hay chậu; `Steal`, `HarvestAll`, `PlantAll` không đụng tới chậu.
- Khách ghé thăm không nhận được dữ liệu `cloud` của chủ.
- `Worth` cộng đúng hạt đậu, chậu và cây đang trồng.
- Hai yêu cầu mua chậu song song chỉ tạo đúng số chậu, không vượt 6.

Trình duyệt thật, cỡ 1440×900 và 390×844, dùng binary, cổng và database riêng, tự xem ảnh, không lỗi console:

- 47 ô không thấy ô 49, 48 ô thấy.
- Trồng đậu, chỉnh giờ chín trong database tạm, tải lại thì mở tầng và có thông báo.
- Lên mây, mua chậu, trồng, thu hoạch, bán, xuống đất.

## Ngoài phạm vi

- Đi bộ trên mây, quái hoặc khổng lồ, sự kiện trên mây.
- Gieo tất cả và thu hoạch tất cả trên mây.
- Bán lại chậu, nâng cấp chậu, bạn bè thăm tầng mây.
- Thêm tầng sau tầng 4.

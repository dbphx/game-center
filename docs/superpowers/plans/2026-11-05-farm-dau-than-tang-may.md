# Nông trại: đậu thần và tầng mây — kế hoạch triển khai

> **Cách chạy:** điều phối theo skill `dcity-orchestrate` (người dùng chọn trong `CLAUDE.md`). Task 1 và 2 chạy song song,
> Task 3 chạy sau cả hai vì cần API thật và ảnh thật để thử trên trình duyệt.

**Goal:** đủ 48 ô đất thì mở ô 49 trồng đậu thần, mỗi hạt mở một tầng mây; trên mây mua chậu và trồng cây quý.

**Architecture:** đậu và chậu nằm ở hai bảng riêng, không đụng `farm_plots` và `MaxPlots`, nên zombie, trộm và thao tác hàng loạt
không phải sửa. Hạt chín được chốt lười bằng `settleBean` trong `State` và `do`. Màn mây là file JS riêng phủ lên cảnh nông trại.

**Tech Stack:** Go + SQLite (`games/farm`), JS thuần (`public/farm`), PNG.

**Spec:** `docs/superpowers/specs/2026-11-05-farm-dau-than-tang-may-design.md`

## Global Constraints

- Không vẽ minh hoạ bằng SVG/CSS (`docs/ART.md`). Comment và chữ hiển thị tiếng Việt.
- Agent không commit, không đụng cổng 8090 hay `data/gamehub.db`; thử bằng binary, cổng và database riêng trong scratchpad.
- Trên Mac chạy Go bằng `env -u GOROOT go ...`.
- Bảng giá lấy nguyên từ mục Số liệu của spec. 4 tầng, 6 chậu mỗi tầng, cây mây chín sau 43.200 giây.

## Review Focus

- Bấm mua chậu hai lần liên tiếp hoặc song song: chỉ trừ tiền đúng số chậu tạo được, không vượt 6. Test ở Task 1.
- Hạt chín lúc đang đủ 3 tầng rồi trồng lại ngay trong cùng yêu cầu: tầng 4 mở rồi mới báo đã đủ tầng, không trồng hạt thứ năm. Test ở Task 1.
- Trồng cây mây khi không đủ xu: báo 402, chậu vẫn trống, không trừ gì. Test ở Task 1.
- Sau khi bán hết nông sản mây, kho sản phẩm vẫn hiện đúng tên và giá ở lần có hàng sau; món mây không lọt vào hộp gieo hạt dưới đất. Test Go ở Task 1, kiểm trên trình duyệt ở Task 3.
- Đang mở màn mây mà máu tụt dưới 40 hoặc bị đưa vào viện: thao tác báo lỗi rõ, màn mây không kẹt và vẫn xuống đất được. Kiểm ở Task 3.

---

### Task 1: Backend

**Files:** tạo `games/farm/cloud.go`, `games/farm/cloud_test.go`; sửa `games/farm/farm.go`, `catalog.go`, `worth.go`, `api.go`, `README.md`.

**Produces:**
- `var CloudFloors []CloudFloor` với `CloudFloor{Floor int; BeanPrice int64; BeanSeconds int64; PotPrice int64; Slots int; Crop Crop}`, JSON `floor`, `beanPrice`, `beanSeconds`, `potPrice`, `slots`, `crop`. `Crop` dùng lại struct có sẵn.
- `func findCloudCrop(id string) (Crop, bool)`
- `func (f *Farm) settleBean(ctx, tx *wallet.Tx, uid, now int64) error`
- `func (f *Farm) PlantBean(ctx, uid int64) (State, error)`
- `func (f *Farm) BuyPot(ctx, uid int64, floor int) (State, error)`
- `func (f *Farm) PlantPot(ctx, uid int64, floor, slot int) (State, error)`
- `func (f *Farm) HarvestPot(ctx, uid int64, floor, slot int) (State, Gained, error)`
- JSON và route đúng mục API của spec; catalog thêm `cloudFloors`; `State.Cloud` có JSON `cloud`.

- [ ] **Step 1:** viết `cloud_test.go` theo mục Kiểm thử của spec và 4 dòng Review Focus thuộc Task 1. Giá trị cố định: hạt tầng 1 trừ 200.000; chậu tầng 1 trừ 100.000; `cloud_berry` trừ 5.000 khi trồng, cho 1 món và 200 XP khi thu; `product("fairy_peach")` trả giá 600.000; `Worth` của nhà có 1 tầng, 1 chậu tầng 1 và 1 cây đang trồng tăng đúng 200.000 + 100.000 + 5.000 so với cùng nhà chưa có gì trên mây. Test zombie dùng nông trại không đặt `noZombies`.
- [ ] **Step 2:** chạy `env -u GOROOT go test ./games/farm/ -run 'Bean|Pot|Cloud'`, xác nhận fail vì chưa có hàm.
- [ ] **Step 3:** thêm hai bảng theo spec vào schema và bảng `CloudFloors` vào `cloud.go`.
- [ ] **Step 4:** viết `settleBean`, gọi ngay sau `settleZombies` trong `State` và `do`. Thêm `loadCloud` đọc ở `load`, `available = len(Plots) >= MaxPlots`. Không sửa `visit`: struct `Visit` riêng nên không chứa `cloud`.
- [ ] **Step 5:** viết bốn thao tác qua `f.do` và `tx.Charge`, kiểm số thứ tự chậu bằng đếm trong cùng transaction để không vượt `Slots`.
- [ ] **Step 6:** `product` và `itemValue` tra thêm `findCloudCrop`. Không thêm cây mây vào `Crops`. `Worth` cộng theo spec: tổng `BeanPrice` của tầng `1..floors`, hạt đang lớn tính `BeanPrice` của tầng `floors+1`, `PotPrice` mỗi chậu, `SeedPrice` mỗi chậu có cây.
- [ ] **Step 7:** thêm route bằng `handle` (qua `CheckZone`), body đọc thêm trường `floor` và `slot`.
- [ ] **Step 8:** chạy `gofmt -l games`, `env -u GOROOT go vet ./games/farm/`, `env -u GOROOT go test -race ./games/farm/ ./internal/...`, kỳ vọng pass.
- [ ] **Step 9:** cập nhật mục Nông trại trong `README.md`.

### Task 2: Hình ảnh

**Files:** tạo trong `public/img/toon/farm/`: `bean_1.png`, `bean_2.png`, `bean_3.png`, `pot_1.png`…`pot_4.png`, và `crop_<id>_1.png`, `crop_<id>_2.png`, `crop_<id>_ripe.png`, `item_<id>.png` cho `cloud_berry`, `mist_pear`, `star_grape`, `fairy_peach`; sửa `manifest.json`, `CREDITS.csv`.

**Produces:** ảnh nền trong suốt, neo giữa cạnh dưới. Cây mây cùng kích thước với `crop_durian_*` tương ứng, `item_*` cùng kích thước với `item_durian.png`. Báo lại kích thước `bean_*` và `pot_*` cho Task 3.

- [ ] **Step 1:** tìm ảnh CC0 cho chậu và thân leo vào thư mục mới trong scratchpad, kiểm giấy phép. Không có thì ghép bằng PIL từ các ảnh sẵn có.
- [ ] **Step 2:** tạo đủ ảnh. Bốn chậu nhìn rõ chậu sau sang hơn chậu trước. Bốn cây mây có màu lạ, phân biệt được với cây dưới đất ở cỡ nhỏ.
- [ ] **Step 3:** cập nhật manifest và ghi công, ghép một ảnh tổng hợp trong scratchpad và tự xem.

### Task 3: Giao diện

**Files:** tạo `public/farm/cloud.js`, `public/farm/cloud.css`; sửa `public/farm/farm.js`, `public/farm/index.html`.

**Consumes:** API và JSON của Task 1, ảnh của Task 2.

**Produces:** `window.FarmCloud = { open(farm, cat), close(), update(farm) }` trong `cloud.js`; `farm.js` gọi hàm này và truyền hàm gọi API, hàm hỏi xác nhận và toast có sẵn qua `FarmCloud.init({api, ask, toast, onState})`.

- [ ] **Step 1:** `find` trong `farm.js` tra thêm `cat.cloudFloors[].crop`, để kho sản phẩm hiện đúng tên, giá, ảnh. Hộp gieo hạt vẫn chỉ dùng `cat.crops`.
- [ ] **Step 2:** ô 49 trong cảnh: hiện khi `farm.cloud.available` và `!VISIT`, ảnh theo trạng thái (đất trống, `bean_1`/`bean_2` theo tiến độ, `bean_3` khi `floors ≥ 1` và không có hạt đang lớn), vật cản và gợi ý `E`. Chọn vị trí trống trên bản đồ, kiểm cả 1440×900 và 390×844.
- [ ] **Step 3:** hộp thoại ô 49 theo spec, hỏi xác nhận trước khi trồng.
- [ ] **Step 4:** màn mây trong `cloud.js`: tab tầng, 6 chỗ đặt chậu với 4 trạng thái, hỏi xác nhận khi mua chậu, nút xuống đất luôn bấm được, lỗi API hiện toast. Đồng hồ đếm ngược chạy một bộ đếm duy nhất, dừng khi đóng màn.
- [ ] **Step 5:** thông báo "Đậu thần đã vươn tới tầng N" khi `floors` lớn hơn số đã báo trong `localStorage`, có try/catch. Lần đầu không có giá trị lưu thì chỉ báo nếu `floors > 0`.
- [ ] **Step 6:** thử trên Chrome thật theo mục Kiểm thử của spec và dòng Review Focus về máu, cỡ 1440×900 và 390×844, tự xem ảnh, không lỗi console. Dùng database tạm: thêm 42 ô và xu, chỉnh `bean_ready_at`, `ready_at` để không phải chờ.

### Ghép

- [ ] `gofmt -l .`, `env -u GOROOT go vet ./...`, `env -u GOROOT go test -race ./...`; thử trọn luồng trên Chrome bằng bấm chuột thật; xem ảnh; commit các file đã xong kèm spec và plan; báo người dùng. Deploy và push khi được yêu cầu, sao lưu database thật trước khi deploy.

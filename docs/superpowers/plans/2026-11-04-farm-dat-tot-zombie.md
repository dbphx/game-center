# Nông trại: đất tốt và zombie — kế hoạch triển khai

> **Cách chạy:** điều phối theo skill `dcity-orchestrate` (người dùng chọn trong `CLAUDE.md`). Task 1 và 2 chạy song song,
> Task 3 chạy sau Task 1 vì cần API thật để thử trên trình duyệt.

**Goal:** thêm nâng cấp đất tốt và đợt zombie mỗi giờ vào nông trại, đúng mức mất 50% sau 12 giờ nếu không canh.

**Architecture:** tính lười như con vật: `settleZombies` chạy trong transaction trước mỗi lần đọc nông trại, ngẫu nhiên có seed
`(uid, wave)`. Không có tiến trình nền.

**Tech Stack:** Go + SQLite (`games/farm`, `internal/wallet`), JS thuần (`public/farm`, `public/hub.js`), PNG.

**Spec:** `docs/superpowers/specs/2026-11-04-farm-dat-tot-zombie-design.md`

## Global Constraints

- Không vẽ minh hoạ bằng SVG/CSS (`docs/ART.md`). Comment và chữ hiển thị tiếng Việt.
- Không commit, không đụng cổng 8090 hay `data/gamehub.db`; thử bằng binary, cổng và database riêng trong scratchpad.
- Trên Mac chạy Go bằng `env -u GOROOT go ...`.
- Giá 8.000 xu, nhanh hơn 30%, đợt đầu giờ, 5 phút, 1 nhát, 2 máu, `p = 1 − 0,5^(1/12)`.

## Review Focus

- Chỉ còn cây chín có zombie mà bấm thu hoạch tất cả: báo rõ phải đập zombie, không báo "chưa có ô nào chín". Test ở Task 1.
- Máu ≤ 2 mà đập: nhát đó vẫn tính, zombie chết, người chơi vào viện; nhát sau bị từ chối. Test ở Task 1.
- Bấm đập hai lần liên tiếp cùng một ô: lần hai báo không có zombie và không trừ thêm máu. Test ở Task 1.
- Đập đúng giây hết hạn (`now == wave×3600 + 300`): zombie đã hết hạn, không trừ máu. Test ở Task 1.
- Bạn bè xem nông trại khi chủ offline qua đầu giờ: thấy đúng cây còn lại, không trộm được ô có zombie. Test ở Task 1.

---

### Task 1: Backend

**Files:** tạo `games/farm/zombie.go`, `games/farm/zombie_test.go`; sửa `games/farm/farm.go`, `bulk.go`, `feed.go`, `steal.go`,
`catalog.go`, `worth.go`, `api.go`, `README.md`.

**Produces:**
- `func zombieCount(crops int) (count int, biteChance float64)`
- `func growSeconds(c Crop, good bool) int64` (`floor(GrowSeconds × 7 / 10)` khi `good`)
- `func (f *Farm) settleZombies(ctx, tx *wallet.Tx, uid, now int64) error`
- `func (f *Farm) UpgradePlot(ctx, uid int64, plot int) (State, error)`
- `func (f *Farm) HitZombie(ctx, uid int64, plot int) (State, error)`
- `func (f *Farm) ZombieStatus(ctx, uid int64) (count int, until int64, err error)`
- JSON: `Plot.good`, `Plot.zombie`; `State.zombieUntil`, `State.zombieEaten`; catalog `goodSoilPrice: 8000`, `goodSoilSpeedup: 0.3`.
- Route: `POST /api/farm/plot/upgrade {plot}`, `POST /api/farm/zombie/hit {plot}` (không qua `CheckZone`),
  `GET /api/farm/zombie/status` trả `{count, until}`. Hai POST trả `{farm, user}`.

- [ ] **Step 1:** viết test trong `zombie_test.go` theo mục Kiểm thử của spec và 5 dòng Review Focus. Giá trị cố định:
  `zombieCount(6) = (1, ≈0.337)`, `zombieCount(20) = (2, ≈0.561)`, `zombieCount(48) = (3, ≈0.898)`, `zombieCount(0) = (0, 0)`.
  Giả lập 12 đợt không canh với nhà 6, 20, 39, 48 cây: tỉ lệ còn lại trong khoảng 0,47–0,53.
- [ ] **Step 2:** chạy `env -u GOROOT go test ./games/farm/`, xác nhận fail vì chưa có hàm.
- [ ] **Step 3:** thêm bảng và cột theo spec. `migrate` kiểm tra `pragma_table_info('farm_plots')` trước khi thêm `good`.
- [ ] **Step 4:** viết `settleZombies` theo 4 bước của spec, tối đa 720 đợt. Chọn mục tiêu bằng `rand.New(rand.NewPCG(uint64(uid), uint64(w)))`,
  xáo các ô đủ điều kiện theo thứ tự `idx`, lấy `count` ô đầu, mỗi ô `bites = rng.Float64() < biteChance`.
- [ ] **Step 5:** gọi `settleZombies` sau `settleChases` trong `State` và `do`, trong `visit` / `Visit` và `Steal` cho **chủ** nông trại.
  `clearPlot` xoá luôn dòng `farm_zombies` của ô. `State` (GET) đọc `eaten` rồi đặt về 0; `do` không xoá.
- [ ] **Step 6:** dùng `growSeconds` trong `Plant`, `PlantAll`. `Harvest` từ chối ô có zombie, `HarvestAll` bỏ qua; `Steal` và `Summaries` loại ô có zombie.
- [ ] **Step 7:** `UpgradePlot`, `HitZombie` (`tx.Damage` trả 0 nghĩa là đang nằm viện thì trả `conflict`), `ZombieStatus`, route, catalog, `Worth` cộng 8.000 xu mỗi ô đất tốt.
- [ ] **Step 8:** chạy `gofmt -l games`, `env -u GOROOT go vet ./games/farm/`, `env -u GOROOT go test -race ./games/farm/ ./internal/...`, kỳ vọng pass.
- [ ] **Step 9:** cập nhật mục Nông trại trong `README.md`.

### Task 2: Hình ảnh

**Files:** tạo `public/img/toon/farm/zombie.png`, `soil_plot_good*.png`; sửa `public/img/toon/farm/manifest.json`, `CREDITS.csv` / `LICENSE.txt`, `docs/ART.md` nếu cần.

**Produces:** `zombie.png` nền trong, neo chân ở giữa cạnh dưới; `soil_plot_good.png` cùng kích thước với `soil_plot.png`, và một bản `_good` cho mỗi biến thể đất mà `public/farm/farm.js` đang dùng. Báo lại tên file.

- [ ] **Step 1:** tải Kenney Toon Characters vào thư mục mới trong scratchpad, kiểm giấy phép CC0, xác nhận có zombie. Không có thì dừng và báo.
- [ ] **Step 2:** cắt ảnh zombie đứng, thu về kích thước hợp với ô đất; ghép ảnh đất tốt bằng PIL.
- [ ] **Step 3:** cập nhật manifest và ghi công, tự xem ảnh.

### Task 3: Giao diện

**Files:** sửa `public/farm/farm.js`, `farm.css`, `index.html`, `public/hub.js`.

**Consumes:** API và JSON của Task 1, ảnh của Task 2.

- [ ] **Step 1:** ô có `zombie`: ảnh zombie trên ô, bấm gọi `/api/farm/zombie/hit`, hiệu ứng trúng đòn và `-2` máu, cập nhật máu từ `user`; đồng hồ đếm ngược tới `zombieUntil`. Bấm vào ô có zombie luôn là đập, không thu hoạch.
- [ ] **Step 2:** ô `good` dùng ảnh đất tốt; hộp thông tin ô có nút nâng cấp ghi giá và "nhanh hơn 30%".
- [ ] **Step 3:** `zombieEaten > 0` hiện thông báo "Zombie đã ăn mất N cây". Tự tải lại trạng thái khi vào đầu giờ và khi hết hạn.
- [ ] **Step 4:** `public/hub.js`: khi mở trang và đầu mỗi giờ gọi `GET /api/farm/zombie/status`; `count > 0` thì hiện thông báo kèm nút về nông trại.
- [ ] **Step 5:** thử trên Chrome thật với cỡ 1440×900 và 390×844, tự xem ảnh, không lỗi console. Cần zombie thì dùng binary trong scratchpad, đặt `last_wave` lùi trong database tạm.

### Ghép

- [ ] `gofmt -l .`, `env -u GOROOT go vet ./...`, `env -u GOROOT go test -race ./...`; thử trọn luồng trên Chrome; xem ảnh; commit các file đã xong; báo người dùng. Deploy và push khi được yêu cầu.

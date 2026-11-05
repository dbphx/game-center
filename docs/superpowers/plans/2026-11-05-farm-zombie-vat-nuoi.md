# Nông trại: zombie ăn vật nuôi và cá — kế hoạch triển khai

> **Cách chạy:** điều phối theo skill `dcity-orchestrate`. Task 1 xong mới chạy Task 2, vì Task 2 cần API thật để thử. Người dùng đã nói
> "tiến hành + deploy", nên ghép xong thì commit và deploy luôn, sao lưu database thật trước, không dùng `--force`.

**Goal:** đợt zombie mỗi giờ tấn công cả chuồng, sân, ao với mức mất 50% sau 12 giờ nếu không canh.

**Architecture:** bảng riêng `farm_stock_zombies`, chạy trong vòng lặp đợt của `settleZombies`; luật ở ruộng không đổi.

**Tech Stack:** Go + SQLite (`games/farm`), JS thuần (`public/farm`).

**Spec:** `docs/superpowers/specs/2026-11-05-farm-zombie-vat-nuoi-design.md`

## Global Constraints

- Không vẽ minh hoạ bằng SVG/CSS; dùng lại `zombie.png`. Comment và chữ hiển thị tiếng Việt.
- Agent không commit, không deploy, không đụng cổng 8090 hay `data/gamehub.db`, không đọc database thật; dùng `env -u GOROOT go ...`.

## Review Focus

- Người offline lâu, có tiến độ ruộng cũ: lần đầu mở sau deploy không mất con nào. Test ở Task 1.
- Con bị bán đúng lúc có zombie qua nút hàng loạt: không thoát được zombie. Test ở Task 1.
- Con vật bị xoá bằng bất kỳ đường nào: không sót dòng zombie, không làm đồng hồ đếm ngược chạy mãi. Test ở Task 1.
- Hai thao tác đập cùng lúc một con: chỉ trừ máu một lần. Test ở Task 1.
- Con có zombie đang bị chó, thức ăn hoặc logic đi lại kéo đi: vẫn đứng yên, bấm trúng được, cả trên điện thoại. Kiểm ở Task 2.

---

### Task 1: Backend

**Files:** sửa `games/farm/zombie.go`, `zombie_test.go`, `farm.go`, `bulk.go`, `storage.go`, `yield.go`, `api.go`, `README.md`.

**Produces:** `func (f *Farm) HitStockZombie(ctx, uid, id int64) (State, error)`; route, bảng và JSON đúng mục API và Kiến trúc của spec.

- [ ] **Step 1:** viết test theo mục Kiểm thử của spec và 4 dòng Review Focus của Task 1; xác nhận fail.
- [ ] **Step 2:** thêm schema, chọn mục tiêu theo khu, hết hạn, súng bắn cả hai bảng đúng thứ tự, tiến độ và `eaten` riêng.
- [ ] **Step 3:** chặn bán, thu hoạch, lấy sản phẩm; bulk bỏ qua; mọi đường xoá `farm_stock` xoá dòng zombie.
- [ ] **Step 4:** `HitStockZombie`, route không qua `CheckZone`, `zombieUntil`, `zombieEatenStock`, `status`, `Animal.zombie`.
- [ ] **Step 5:** `gofmt -l games`, `env -u GOROOT go vet ./games/farm/`, `env -u GOROOT go test -race ./games/farm/ ./internal/...`, kỳ vọng pass. Cập nhật mục Nông trại trong `README.md`.

### Task 2: Giao diện

**Files:** sửa `public/farm/farm.js`, `public/farm/farm.css`, `public/farm/cloud.js` nếu cần.

**Consumes:** JSON và route của Task 1.

- [ ] **Step 1:** con có `zombie` đứng yên, hiện `zombie.png`, bấm là gọi `/api/farm/zombie/hit-stock`, cùng hiệu ứng như ở ruộng. Thao tác trên con đó và gợi ý `E` ưu tiên đập.
- [ ] **Step 2:** đạn súng đậu bay tới cả zombie vật nuôi; thông báo gộp cây và con vật theo spec; dòng báo zombie trên màn mây tính cả vật nuôi.
- [ ] **Step 3:** thử trên Chrome thật theo spec và dòng Review Focus của Task 2, tự xem ảnh, không lỗi console. Tạo zombie bằng database tạm, đặt lùi `last_wave`.

### Ghép

- [ ] `gofmt -l .`, `env -u GOROOT go vet ./...`, `env -u GOROOT go test -race ./...`; xem ảnh; commit; sao lưu database thật; `env -u GOROOT sh deploy/deploy.sh`; kiểm trang thật; báo người dùng.

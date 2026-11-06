# Tiến Lên: dừng ván, máy khôn hơn, bỏ Diễn biến — kế hoạch triển khai

> **Cách chạy:** điều phối theo `dcity-orchestrate`. Task 1 và Task 3 chạy song song. Task 2 chạy sau Task 1 vì cùng sửa
> `engine.go`. Không deploy khi chưa được người dùng yêu cầu; deploy thì không dùng `--force` khi đang có bàn mở.

**Goal:** ván dừng khi có người về nhất, ai còn heo/hàng đều thối, máy đánh khôn hẳn, bỏ khung Diễn biến.

**Architecture:** đổi `Play` và `finish` trong `engine.go`; máy mới thay `ChooseMove` với `Context` giàu thông tin công khai hơn.

**Tech Stack:** Go (`games/tienlen`, `games/tienlen/online`), JS thuần (`public/tienlen`).

**Spec:** `docs/superpowers/specs/2026-11-06-tienlen-dung-van-may-khon-design.md`

## Global Constraints

- Agent không commit, không deploy, không đụng cổng 8090 hay `data/gamehub.db`; dùng `env -u GOROOT go ...`.
- Comment và chữ hiển thị tiếng Việt. Máy không được đọc bài của người khác.

## Review Focus

- Người nhất hết bài bằng nước chặt heo: tiền chặt và tiền đếm lá cùng được tính, ván vẫn dừng ngay. Test ở Task 1.
- Bàn 2 người: người thua vừa là bét vừa có thể bị cóng, không trả thối hai lần. Test ở Task 1.
- Người vắng mặt, máy đánh thay đúng lúc người khác về nhất: phòng chuyển sang kết quả, không có lượt treo. Test ở Task 1.
- Máy có `MustInclude` (3♠ ván đầu) và chỉ có cách đánh làm vỡ bộ: vẫn đánh hợp lệ. Test ở Task 2.
- Bàn 2–3 người, lá không chia: máy không coi bộ là chắc thắng khi lá chặn được có thể nằm ở phần không chia. Test ở Task 2.

---

### Task 1: Luật dừng ván

**Files:** sửa `games/tienlen/engine.go` (chỉ `Play`, `finish` và hàm phụ mới), `money.go` nếu cần, `game_test.go`, `money_test.go`, `games/tienlen/online/room.go`, `room_test.go`, `README.md` mục Tiến Lên.

- [ ] **Step 1:** viết test theo mục Kiểm thử phần luật và 3 dòng Review Focus của Task 1; xác nhận fail.
- [ ] **Step 2:** `Play` gọi `finish` ngay khi `len(Finished) == 1`; `finish` xếp hạng theo spec, thối từng người thua.
- [ ] **Step 3:** sửa phòng online nếu còn giả định ván đánh tiếp; giữ XP theo `Finished`.
- [ ] **Step 4:** `gofmt`, `vet`, `go test -race ./games/tienlen/...` pass; cập nhật `README.md`.

### Task 2: Máy đánh

**Files:** sửa `games/tienlen/ai.go`, `engine.go` (chỉ `Context` và `Game.Context`); tạo `games/tienlen/ai_test.go` (máy cũ làm đối chứng nằm ở đây).

**Interfaces:** giữ `func ChooseMove(hand []Card, ctx Context) []Card`; `Context` thêm `Played`, `Counts`, `Seat`, `Passed`, `LastPlayer` theo spec.

- [ ] **Step 1:** chép máy hiện tại sang `ai_test.go` thành `legacyChooseMove`; viết test tình huống, test hợp lệ qua nhiều ván, test thời gian, test tỉ lệ thắng (chạy đủ 2.000 ván khi không có `-short`).
- [ ] **Step 2:** cài máy mới theo mục Hành vi của spec.
- [ ] **Step 3:** đo, chỉnh tới khi đạt mục tiêu hoặc báo rõ con số đạt được; `go test -race ./games/tienlen/...` pass.

### Task 3: Giao diện

**Files:** sửa `public/tienlen/index.html`, `client.js`, `tienlen.css`, `rules.js` nếu có chữ luật.

- [ ] **Step 1:** bỏ khung Diễn biến và code vẽ nó; cột phải vẫn gọn trên 1440×900 và 390×844.
- [ ] **Step 2:** màn kết quả theo thứ tự `finished`, thối từng người; chữ luật trong trang theo spec.
- [ ] **Step 3:** sau khi Task 1 xong, thử trên Chrome thật bàn có 3 máy theo spec, tự xem ảnh, không lỗi console.

### Ghép

- [ ] `gofmt -l .`, `env -u GOROOT go vet ./...`, `env -u GOROOT go test -race ./...`; xem ảnh; commit; báo người dùng tỉ lệ thắng thật. Deploy khi được yêu cầu.

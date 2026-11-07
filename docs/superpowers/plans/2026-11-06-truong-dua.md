# Trường đua 3D — kế hoạch triển khai

> **Cách chạy:** `dcity-orchestrate`. Task 1 (server) và Task 2 (giao diện 3D + hình) chạy song song theo hợp đồng dưới đây.
> Task 3 (ghép, bản đồ thành phố, thử 2 người thật) sau cùng. Không deploy khi chưa được yêu cầu.

**Goal:** khu Trường đua: phòng đua 2–6 xe 3D, cược xu, xe + đồ nâng cấp nhìn thấy được.

**Spec:** `docs/superpowers/specs/2026-11-06-truong-dua-design.md`

## Global Constraints

- Agent không commit, push, deploy; không đụng cổng 8090 hay `data/gamehub.db`; `env -u GOROOT go ...`.
- Không dùng hình, tên, logo, xe ZingSpeed. Mô hình 3D CC0 (Kenney) và three.js (MIT) được đặt trong repo
  (`public/vendor/three/`, `public/race/models/`), không tải từ CDN lúc chạy. Ghi nguồn và giấy phép.
- Comment và chữ hiển thị tiếng Việt.

## Hợp đồng

### Dữ liệu đường đua — `public/race/tracks/<id>.json` (server đọc qua `go:embed` từ cùng file)

```json
{"id":"neon","name":"Đường Neon","laps":3,"minLapMs":25000,
 "spawn":[{"x":0,"z":0,"rot":0}],             // 6 vị trí xuất phát
 "checkpoints":[{"x":0,"z":0,"r":12}],          // theo thứ tự, cái cuối là vạch đích
 "path":[{"x":0,"z":0}],                        // đường mẫu cho máy đua, khép kín
 "pieces":[{"model":"roadStraight","x":0,"z":0,"rot":0}]}  // khối dựng đường (giao diện dùng)
```

Hai đường: `neon` (đêm, sân vận động) và `city` (đêm, phố). Toạ độ mét, trục y lên.

### Chỉ số xe — server là nguồn đúng, gửi trong `GET /api/race/garage`

`maxSpeed` (m/s), `accel` (m/s²), `grip` (0–1). Xe khởi đầu cấp 0: 30 / 12 / 0,70. Xe đắt nhất cấp 5 tối đa ≈ +20% mỗi chỉ số.
Nitro: ×1,3 tốc độ tối đa trong 2 s; drift nạp nitro.

### API garage (`/api/race`, cần đăng nhập)

- `GET /api/race/garage` → `{cars:[{id,name,price,owned,levels:{engine,turbo,tires},stats:{maxSpeed,accel,grip},
  cosmetics:{spoiler,neon,paint,decal}}], current, items:[{id,kind,name,price,owned}], upgradePrice:{...}}`
- `POST /api/race/car/buy {car}`, `POST /api/race/car/select {car}`, `POST /api/race/upgrade {car, part}` (part ∈ engine/turbo/tires),
  `POST /api/race/item/buy {item}`, `POST /api/race/equip {car, kind, item}` (kind ∈ spoiler/neon/paint/decal). Trả `{garage, user}`.

### WebSocket `/ws/race?token=...` (theo mẫu Cá Ngựa)

Client → server: `create {bet, track}`, `join {code}`, `addBot`, `kick {seat}`, `setBet`, `setTrack`, `start`, `leave`,
`state {t, x, z, rot, speed, lap, cp, nitro, drift}` (≈15/giây khi đua), `chat`.

Server → client: `room` (phòng, ghế, xe và đồ đang lắp của mỗi ghế, phase lobby/countdown/racing/over, `startAt` ms),
`cars {t, list:[{seat, x, z, rot, speed, lap, cp, nitro, drift}]}` (≈15/giây), `finish {seat, place, timeMs}`,
`result {places:[{seat, timeMs}], money, tax, xp}`, `error`.

## Tasks

### Task 1: Server `games/race`

Files: `games/race/**` (cars.go, garage.go, room.go, hub.go, ws.go, track.go, *_test.go), mount trong `main.go`, `ZoneMinHP["race"] = 30`
trong `internal/wallet/hp.go`, README mục Trường đua. TDD. Test: giá, nâng cấp tới cấp 5, mua đồ, lắp đồ; phòng, cược, thiếu xu;
kiểm gói (vượt tốc, bỏ checkpoint, dịch chuyển, vòng quá nhanh) và loại khi vi phạm lặp lại; thứ tự về đích, hết 90 s, rời phòng/mất
mạng 15 s; trả xu qua `wallet.SettleTaxed`, XP; máy đua chạy theo `path` và về đích.

### Task 2: Giao diện 3D + hình `public/race/**`, `public/vendor/three/**`

Sảnh phòng (tạo/vào, mức cược, đường, thêm máy, bắt đầu), garage (xem xe xoay 3D, mua, nâng 3 món, mua và lắp đồ trang trí, sơn),
cuộc đua (camera sau xe, điều khiển phím/cảm ứng, drift, nitro, đếm ngược, bảng vòng/hạng, minimap), kết quả. Đồ hoạ: đêm, bloom
neon, rào mũi tên, tia lửa và vệt tốc độ khi nitro; điện thoại mặc định chất lượng thấp. Mô hình xe từ Kenney Car Kit / Racing Kit,
đồ nâng cấp gắn thêm mesh (ống xả, bình turbo, mâm, cánh gió, đèn gầm). Khi server chưa có, chạy được chế độ "lái thử một mình"
không cần WebSocket để chỉnh cảm giác lái.

### Task 3: Ghép

Toà nhà Trường đua trên bản đồ (`public/city.js`, ảnh PNG), thử trọn luồng hai tài khoản thật + máy, đo FPS điện thoại 4×,
`go test -race ./...`, commit.

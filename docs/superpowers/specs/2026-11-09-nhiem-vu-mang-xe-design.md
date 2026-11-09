# Nhiệm vụ nông trại → mảnh ghép xe đua (NPC trong vườn)

Ngày: 2026-11-09 · Trạng thái: chờ người dùng duyệt

## Người dùng yêu cầu

- Thêm **nhiệm vụ hệ thống**; phần thưởng là **mảnh ghép** — chỉ cho **Trường đua** (ngưng Đấu trường).
- Nhiệm vụ nằm **trong khu nông trại**: làm việc nông trại hoặc các game liên quan; **nhận / đổi nhiệm vụ** và nhận thưởng tại
  **NPC Ông già Noel** đứng trang trí trong vườn.
- Kiểu nhiệm vụ: **hằng ngày + hằng tuần**. Mảnh ra **ngẫu nhiên**. Thời gian ghép **theo giá xe** (rẻ ≈ 1 tuần … đắt nhất 6–8
  tuần, độc quyền ≈ 4–5 tuần). **Kết hợp**: mảnh xe thường ghép ra xe đó + **2 xe độc quyền** (1 xe, 1 moto; chỉ số ngang
  Siêu Tân Tinh, không vượt trần +20%) và **3–4 đồ trang trí độc quyền**, chỉ có từ mảnh.

## Giả định của Claude (sửa nếu sai)

1. NPC luôn có trong vườn: mùa Noel (1/12–6/1, năm nay bật ngay) mặc **Ông già Noel**, ngoài mùa là **Bác nông dân** — nhiệm vụ
   vẫn chạy quanh năm.
2. "Đổi nhiệm vụ" = đổi một nhiệm vụ ngày chưa xong lấy nhiệm vụ khác: 1 lần miễn phí / ngày, sau đó 2.000 xu / lần (tối đa 5).
3. Nhiệm vụ lấy từ: nông trại (vườn + tầng mây), hồ câu cá, Trường đua, trộm / thăm vườn bạn. Không lấy từ casino, Đấu trường.
4. Mảnh thừa (xe đã có, hoặc đã đủ mảnh) **đổi tại NPC**: 3 mảnh thừa cùng bậc → 1 mảnh ngẫu nhiên bậc cao hơn một bậc
   (bậc cao nhất: 3 → 1 mảnh độc quyền ngẫu nhiên). Không đổi ra xu (giữ cân bằng vừa chỉnh).
5. Giờ reset: ngày 0:00, tuần thứ Hai 0:00 giờ Việt Nam.

## Nhiệm vụ

- **Ngày**: 5 nhiệm vụ rút ngẫu nhiên từ bảng mẫu (không trùng loại), mỗi cái thưởng **1 mảnh** + 20 XP. Làm đủ 5 → rương ngày
  thêm **1 mảnh**.
- **Tuần**: 3 nhiệm vụ lớn, mỗi cái **3 mảnh** + 100 XP. Đủ 3 → rương tuần **1 mảnh độc quyền** (xe hoặc đồ trang trí độc quyền).
- Tiến độ tự cộng khi người chơi làm việc tương ứng; xong thì phải tới NPC bấm **Nhận** (chấm "!" vàng trên đầu NPC khi có thưởng
  chờ / có nhiệm vụ mới).
- Bảng mẫu (số lượng theo cấp người chơi, cấp cao cần nhiều hơn):

| Sự kiện | Ngày (ví dụ cấp 1 → 30) | Tuần |
|---|---|---|
| `farm_harvest` thu hoạch ô cây | 10 → 30 | 150 |
| `farm_plant` gieo hạt | 10 → 30 | — |
| `farm_product` lấy trứng / sữa / len | 5 → 15 | 60 |
| `farm_animal` bán / thu con vật, cá ao | 2 → 6 | 25 |
| `farm_zombie` súng đậu hạ zombie | 2 → 5 | 20 |
| `farm_cloud` thu hoạch tầng mây | 1 → 3 (khi đã mở tầng mây) | 10 |
| `farm_visit` thăm / trộm vườn bạn | 3 | 15 |
| `fish_catch` câu được cá | 5 → 15 | 60 |
| `race_finish` hoàn thành cuộc đua (có người hoặc máy) | 2 → 4 | 15 |
| `race_win` về nhất | 1 | 5 |

## Mảnh và ghép xe

- Mỗi xe bán bằng xu (trừ Xe Tập Lái) có loại mảnh riêng; chia **bậc** theo hạng:

| Bậc | Xe | Mảnh cần | Tỉ lệ rơi bậc |
|---|---|---|---|
| Thường | Sao Chổi, Gió Lốc, Tốc Hành, Tia Chớp, Chim Ưng | 6 | 60% |
| Hiếm | Rắn Lục, Bão Tố, Thiên Đỉnh, Bóng Ma, Neon Rider | 8 | 28% |
| Sử thi | Lốc Xoáy, Đỉnh Cao, Hoả Tiễn, Siêu Tân Tinh | 8 | 10% |
| Độc quyền | 2 xe + 4 đồ trang trí | xe 5 · đồ 3 | 2% (+ rương tuần) |

  Trong bậc chia đều. Khoảng ≈ 44 mảnh / tuần nếu làm đủ → mỗi xe Thường ≈ 1 tuần, Hiếm ≈ 3 tuần, Sử thi ≈ 7 tuần, một món độc
  quyền ≈ 4–5 tuần. Giá trị xe nhận được ≈ 1 triệu xu / tuần (ngang thu nhập nông trại cùng thời gian) — chỉ là xe đua.
- Đủ mảnh → **Ghép** trong garage Trường đua hoặc tại NPC: tiêu mảnh, xe vào garage như mua (cấp 0, sơn mặc định), **không cần đủ
  cấp nhân vật** để ghép (cấp chỉ chặn mua bằng xu). Đã có xe thì không ghép được (mảnh đó tính là thừa).
- Xe độc quyền: ID `x_aurora` (xe, "Cực Quang") và `x_frost` (moto, "Băng Phong"), hạng bằng Siêu Tân Tinh (chỉ số như Siêu Tân
  Tinh; moto theo quy tắc moto), `price: 0`, cờ `exclusive` (không mua được). Đồ trang trí độc quyền: sơn ánh kim đổi màu, decal
  bông tuyết, neon chuyển sắc, cánh gió phát sáng. Mô hình / hình do agent xe làm theo phong cách vừa làm lại.

## Kỹ thuật

- Gói mới `games/quests/`: bảng `quest_daily`, `quest_weekly` (user, kỳ, slot, kind, target, progress, claimed), `race_shards`
  (user, item, qty), `quest_rerolls`. Hàm `quests.Record(ctx, w, user, kind, n)` — gọi từ farm / fishing / race / friends ở chỗ
  sự kiện đã ghi sổ thành công; lỗi ở đây không làm hỏng thao tác gốc. Nhiệm vụ kỳ mới sinh lười khi người chơi mở / ghi sự kiện.
- Ghép xe gọi hàm mới của gói `race` (`GrantCar`, `GrantItem`) trong cùng giao dịch.
- API (cần đăng nhập): `GET /api/quests` → `{daily[], weekly[], chest{daily, weekly}, shards[{item, name, tier, qty, need,
  owned}], rerollCost, resetAt{day, week}}`; `POST /api/quests/claim {id}` → mảnh nhận được; `POST /api/quests/chest {kind}`;
  `POST /api/quests/reroll {id}`; `POST /api/quests/assemble {item}`; `POST /api/quests/exchange {tier}`. Lỗi: 400 sai tham số,
  402 thiếu xu, 409 chưa xong / đã nhận / thiếu mảnh / đã có.
- Giao diện: nhấn NPC (`#farm-npc`, sự kiện `farm:npc` — agent Noel làm) mở bảng nhiệm vụ (tab Ngày / Tuần / Mảnh xe), hoạt ảnh
  mở mảnh nhận được (ảnh xe). Garage hiện mảnh / nút Ghép trên thẻ xe chưa có.
- Test: sinh nhiệm vụ theo kỳ, reset ngày / tuần (giờ VN), cộng tiến độ từ từng game, nhận một lần, rương, đổi nhiệm vụ trừ xu,
  tỉ lệ rơi (thống kê), ghép + không ghép trùng, đổi mảnh thừa, chạy đồng thời (`-race`).

## Ngoài phạm vi

Đấu trường, casino; bán mảnh lấy xu; nhiệm vụ cốt truyện / thành tích trọn đời.

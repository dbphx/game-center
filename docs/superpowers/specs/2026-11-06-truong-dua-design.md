# Trường đua 3D

## Mục tiêu

Khu **Trường đua** trên bản đồ: người chơi tạo phòng, đua kart 3D với nhau theo phong cách vui nhộn kiểu ZingSpeed (drift, nitro),
mua xe và nâng cấp bằng xu.

Thành công khi: đua 2–6 xe mượt trên máy tính và điện thoại tầm trung; về đích và trả xu do server quyết định; người mới với xe
khởi đầu vẫn thắng được nếu lái giỏi; các trang khác không nặng thêm.

## Luật đã chốt với người dùng

- **Đồ hoạ 3D sau xe**, three.js, mô hình 3D CC0 của Kenney (Racing Kit, Car Kit). Đây là **ngoại lệ được người dùng đồng ý** của
  quy tắc chỉ dùng ảnh PNG; chỉ áp dụng cho Trường đua. Không dùng hình, tên, xe của ZingSpeed.
- **Cược xu, nhất ăn:** mức cược 100 / 500 / 1.000 / 5.000 / 10.000 xu mỗi người; về nhất nhận toàn bộ cược, trừ thuế 5% tiền
  thắng ròng như các game khác (`wallet.SettleTaxed`). XP theo thứ hạng như Tiến Lên.
- **Xe và nâng cấp:** 1 xe khởi đầu miễn phí, 5 xe mua bằng xu (20.000; 80.000; 250.000; 800.000; 2.000.000). 3 chỉ số: Tốc độ,
  Tăng tốc, Bám đường; mỗi chỉ số 5 cấp, giá cấp n = giá xe × 10% × n (xe khởi đầu tính như 10.000). Đổi màu sơn miễn phí.
  Xe đắt nhất nâng tối đa hơn xe khởi đầu tối đa khoảng 20% ở mỗi chỉ số.
- **Đồ nâng cấp nhìn thấy trên xe** (người dùng gửi ảnh tham chiếu ZingSpeed Mobile: xe đua mui trần dáng thấp, góc cạnh, có người
  lái, đèn neon, đường đua đêm trong sân vận động, rào mũi tên vàng đen, tia lửa): mỗi chỉ số là một món đồ, cấp càng cao hình càng
  khác — Động cơ / ống xả (Tốc độ: ống xả to dần, lửa xanh khi nitro), Turbo (Tăng tốc: bình turbo lộ trên xe), Lốp / mâm
  (Bám đường: mâm và lốp đổi kiểu). Đồ trang trí không có chỉ số: cánh gió (3 kiểu), đèn neon gầm (6 màu), sơn, decal số
  — bán bằng xu (5.000–50.000). Bảng `race_cosmetics(user_id, item, PRIMARY KEY(user_id, item))` và lựa chọn đang lắp lưu
  theo xe. Không dùng hình, tên, logo, xe của ZingSpeed; chỉ học phong cách: sơn bóng, dải neon phát sáng (bloom), đường đua ban
  đêm, rào mũi tên, tia lửa và vệt tốc độ khi nitro. Người lái là nhân vật low-poly đơn giản đội mũ bảo hiểm.

## Luật đua

- Chủ phòng chọn đường đua (2 đường lúc đầu) và mức cược; 2–6 xe; thiếu người thì thêm máy. Ai không đủ xu thì không bắt đầu được.
- Cần ít nhất 30 HP để vào (như Casino); thêm `race` vào `ZoneMinHP`.
- 3 vòng, đếm ngược 3‑2‑1. Về đích theo thứ tự server ghi nhận. Hết 90 giây sau người nhất mà chưa về thì xếp theo vòng và điểm kiểm tra.
- Rời phòng, mất mạng quá 15 giây: tính về bét, vẫn mất cược.
- Máy (bot) không có ví: người thua máy thì cược đó mất (như các game khác thua máy), người thắng có cược của máy do "nhà" trả.

## Lái

- Máy tính: mũi tên / WASD lái, Shift drift, Space nitro. Điện thoại: tự chạy, nút trái/phải, drift, nitro trên màn hình.
- Drift nạp thanh nitro; nitro tăng tốc trong 2 giây. Đâm tường giảm tốc; không có vật phẩm tấn công trong bản đầu.

## Kiến trúc

- **Backend `games/race`:** hub phòng qua WebSocket `/ws/race` (theo mẫu Cá Ngựa / Caro), API `/api/race` cho garage
  (xe đã mua, cấp, màu; mua xe, nâng cấp), bảng `race_cars(user_id, car, speed, accel, grip, color, PRIMARY KEY(user_id, car))`,
  `race_garage(user_id PK, current_car)`.
- **Mạng:** mỗi máy tự mô phỏng xe mình, gửi `{t, pos, rot, speed, lap, checkpoint}` khoảng 15 lần/giây; server chuyển cho người khác
  và kiểm: tốc độ không vượt trần của xe (cộng nitro, dung sai 10%), điểm kiểm tra đúng thứ tự, khoảng cách giữa hai gói không vượt
  `trần tốc độ × thời gian × 1,2`, thời gian vòng không nhỏ hơn mức tối thiểu của đường. Vi phạm thì bỏ gói và đánh dấu; vi phạm
  lặp lại thì loại khỏi cuộc đua (tính bét).
- **Máy đua** chạy ở server theo đường mẫu của từng đường, tốc độ ngẫu nhiên nhẹ để không quá giỏi, gửi vị trí như người.
- **Giao diện `public/race/`:** sảnh phòng, garage (xem xe 3D xoay, mua, nâng, sơn), cuộc đua, màn kết quả. three.js và mô hình
  chỉ tải trong `/race/`; mặc định đồ hoạ thấp trên điện thoại (ít bóng, độ phân giải 0,75).
- **Đường đua** dựng từ các khối Racing Kit; dữ liệu đường (khối, điểm kiểm tra, đường mẫu cho máy, thời gian vòng tối thiểu) nằm
  trong file JSON dùng chung cho server và giao diện.
- **Bản đồ thành phố:** thêm toà nhà Trường đua (ảnh PNG CC0) và lối vào.

## Kiểm thử

Test Go: tạo phòng, cược, thiếu xu; mua xe, nâng cấp, giá, không vượt cấp 5; kiểm gói vị trí (vượt tốc, bỏ điểm kiểm tra, dịch
chuyển); thứ tự về đích, trả xu và thuế, XP; rời phòng giữa chừng; máy đua về đích.

Trình duyệt thật 1440×900 và 390×844 (CPU 4×): đua một phòng với máy từ đầu tới kết quả, hai người thật trong cùng phòng, garage
mua và nâng, FPS khi đua, không lỗi console.

## Ngoài phạm vi (bản đầu)

- Vật phẩm tấn công, đua đội, xếp hạng mùa, phát lại, nhiều hơn 2 đường.

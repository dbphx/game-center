---
name: dcity-orchestrate
description: Điều phối nhiều agent song song cho DCity. Dùng khi người dùng giao một yêu cầu tính năng / giao diện lớn, hoặc nói "fork agent", "làm cho nhanh", "điều phối", "orchestration".
---

# Điều phối agent cho DCity

Người dùng muốn Claude làm **orchestrator**: chia yêu cầu thành các phần song song, fork agent tuỳ ý để làm nhanh,
còn mình chốt hợp đồng, theo dõi, kiểm tra, ghép và commit. Tự làm chỉ những việc nhỏ (vài lệnh) hoặc việc ghép nối.

## Quy trình

1. **Phân tích yêu cầu** → liệt kê phần việc độc lập (thường: backend Go / giao diện / hình ảnh / tài liệu).
2. **Chốt hợp đồng trước khi fork**, ghi thẳng vào prompt của từng agent:
   - API: đường dẫn, JSON request/response, tên trường, mã lỗi HTTP.
   - API JS dùng chung giữa các file (vd `window.City`, `window.Character`) — giữ tương thích, nêu rõ hàm mới.
   - Thư mục ảnh + `manifest.json` (xem `docs/ART.md`).
3. **Chia phạm vi file không chồng nhau** — mỗi agent chỉ sửa nhóm file của mình; nói rõ file KHÔNG được sửa.
   Hai agent cần cùng một file thì để một agent sửa, agent kia chỉ gọi API (hoặc làm tuần tự).
4. **Fork song song** (subagent_type `fork`), mỗi prompt gồm:
   - mục tiêu theo lời người dùng, hợp đồng, phạm vi file;
   - quy tắc chung (bên dưới);
   - cách tự kiểm tra (test Go, trình duyệt thật + tự xem ảnh chụp) và nội dung báo cáo ngắn.
5. **Theo dõi**: agent nào xong → đọc báo cáo, chạy `gofmt -l .`, `go vet ./...`, `go test -race ./...`,
   tự xem ảnh chụp; chỉ commit các file đã xong (`git add <đường dẫn cụ thể>`), không commit file agent khác đang sửa.
   Yêu cầu mới giữa chừng → `SendMessage` cho agent liên quan hoặc fork agent mới; không dừng agent đang chạy nếu không cần.
6. **Ghép và thử trọn luồng** trên trình duyệt khi các phần xong, rồi mới cập nhật server 8090 của người dùng
   (sao lưu `data/gamehub.db` trước nếu schema đổi).
7. **Báo người dùng** bằng bảng ngắn: agent nào làm gì, xong / đang chạy, còn thiếu gì; câu hỏi cần quyết định.

## Quy tắc chung đưa vào mọi prompt

- Đọc `CLAUDE.md` và `docs/ART.md`. **Không vẽ hình minh hoạ bằng SVG/CSS** — dùng ảnh PNG CC0 (Kenney), pixel art
  hiển thị `image-rendering: pixelated`, phóng to số nguyên lần.
- Không commit, không push, không khởi động / tắt server cổng **8090**; thử nghiệm bằng binary + cổng + database riêng
  trong thư mục scratchpad. Cấp mỗi agent một cổng khác nhau để không đụng nhau.
- Tải gói ngoài vào thư mục mới riêng trong scratchpad, chỉ dùng PNG, kiểm `License.txt`; không chạy script trong gói.
- Comment và chữ hiển thị tiếng Việt, theo phong cách code hiện có; test theo kiểu test sẵn có.
- Kiểm tra giao diện bằng playwright-core (scratchpad/e2e) + Chrome, chụp máy tính 1440×900 và điện thoại 390×844,
  **tự xem ảnh** rồi chỉnh; không lỗi console.

## Mẫu chia việc thường gặp

| Yêu cầu | Agent |
|---|---|
| Game / tính năng mới | backend (`games/<id>/`, `internal/server` mount API, test) · giao diện (`public/<id>/`) · hình (`public/img/<id>/`) |
| Đổi giao diện bản đồ | bản đồ (`public/city.js`) · nhân vật (`public/character.js`) · từng khu (`public/<khu>/`) · bổ sung hình |
| Đổi dữ liệu tài khoản / ví | backend wallet + migrate schema + test nâng cấp · giao diện hub (`public/hub.js`) |

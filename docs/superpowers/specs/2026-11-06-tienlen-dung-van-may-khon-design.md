# Tiến Lên: dừng ván khi có người về nhất, máy đánh khôn hơn, bỏ khung Diễn biến

## Mục tiêu

- Ván ngắn hơn: có người hết bài là ván dừng, không đánh tiếp để tìm người về bét.
- Máy đánh khôn hẳn, chỉ dùng thông tin công khai và bài trên tay.
- Bàn gọn hơn: bỏ khung Diễn biến.

Thành công khi: không còn ván nào đánh tiếp sau người nhất, tiền và XP tính đúng, máy mới thắng máy cũ rõ rệt, giao diện không còn khung Diễn biến.

## Luật đã chốt với người dùng

- **Dừng ván:** người đầu tiên hết bài thắng; ván kết thúc ngay trong lượt đó (`Over = true`).
- **Đếm lá:** giữ như hiện có: mỗi người thua trả `CardsLeft × Multiplier`; bị cóng tính 26 lá; tới bằng 3♠ nhân 2; tới trắng giữ nguyên.
- **Thối:** **mọi người thua** còn heo hoặc hàng trên tay (`Leftover`) đều trả người nhất, kể cả người bị cóng. Mỗi người tính riêng, không trùng.
- **Tiền chặt:** trong ván giữ nguyên.
- **Xếp hạng (`Finished`):** người nhất, rồi người còn ít lá hơn; bằng lá thì người ngồi gần sau người nhất theo chiều đánh xếp trước; người bị cóng ở cuối (theo cùng quy tắc ghế). XP dùng thứ hạng này với công thức hiện có.
- **Ván sau:** người nhất đánh trước, như cũ.
- **Bỏ khung Diễn biến** (`#log` và tiêu đề) trên mọi cỡ màn hình. Server vẫn gửi `history`.

## Máy đánh

Thay `ChooseMove` hiện có, giữ chữ ký `func ChooseMove(hand []Card, ctx Context) []Card` để `online/room.go` không phải đổi.

### Thông tin được dùng

`Context` thêm, do `Game.Context(p)` điền, chỉ từ thông tin công khai:

- `Played []Card`: mọi lá đã đánh ra trong ván.
- `Counts []int`: số lá đang cầm của từng ghế (0 với ghế đã xong hoặc không chơi).
- `Seat int`, `Passed []bool`, `LastPlayer int` (-1 khi đánh tự do).

Lá chưa thấy = cả bộ 52 lá trừ bài của mình và `Played`. Với bàn dưới 4 người, phần bài không chia cũng nằm trong lá chưa thấy, nên máy không biết chúng và phải coi như đối thủ có thể có.

### Hành vi

1. **Chia tay bài:** tìm cách chia tay thành các bộ hợp lệ, ít bộ nhất, ưu tiên giữ heo và hàng. Tìm theo nhánh có giới hạn, đủ nhanh với 13 lá.
2. **Bộ "chắc thắng":** bộ mà không tổ hợp nào từ lá chưa thấy chặn được, tính cả chặt bằng hàng.
3. **Về đích:** nếu sau khi đánh các bộ chắc thắng chỉ còn tối đa một bộ, đánh bộ chắc thắng trước rồi bộ cuối. Đánh hết bài được thì luôn đánh.
4. **Đánh tự do:** đánh bộ yếu nhất trong cách chia, ưu tiên bộ nhiều lá; giữ bộ chắc thắng để lấy lại quyền đánh.
5. **Chặn:** chỉ chặn bằng bộ nằm sẵn trong cách chia, trừ khi đối thủ sắp hết bài. Không đập vỡ đôi, sảnh, hàng để chặn lấy lệ.
6. **Đối thủ còn 1–2 lá:** khi đánh tự do, tránh đánh đúng loại họ có thể chặn hết (rác khi còn 1 lá, đôi khi còn 2 lá); khi chặn thì chặn bằng lá cao nhất có thể, dùng heo và hàng nếu cần.
7. **Heo và hàng:** ra heo khi lấy lại quyền đánh giúp về đích hoặc khi đối thủ sắp hết; giữ hàng để chặt heo.
8. **Lượt bắt buộc** (`MustInclude`) và luật hiện có vẫn được tôn trọng; mọi nước trả về phải qua `Validate`.

### Hiệu năng và đo

- Mỗi lần gọi `ChooseMove` dưới 50 ms trên máy dev, kể cả tay 13 lá nhiều đôi.
- Đo bằng test giả lập 2.000 ván, 4 ghế, 2 máy mới đấu 2 máy cũ, ghế xoay vòng; máy cũ được giữ trong file test làm đối chứng. Mục tiêu: máy mới thắng trung bình từ 32% trở lên mỗi ghế. Báo con số thật.

## Giao diện

- `public/tienlen`: bỏ khung Diễn biến và code vẽ nó, phần còn lại của cột bên phải giãn ra hợp lý.
- Màn kết quả: thứ hạng theo `finished`, hiện số lá và tiền thối của từng người; câu "còn N lá" như hiện có.
- Bảng luật trong trang, nếu có, và `README.md` mục Tiến Lên ghi luật mới.

## Kiểm thử

Test Go:

- Người đầu tiên hết bài: `Over` ngay, không còn lượt, `Finished` đúng thứ tự lá rồi ghế, người cóng ở cuối.
- Tiền: đếm lá, ×2 khi 3♠, cóng 26, thối cho từng người thua có heo/hàng, tổng tiền mỗi ván bằng 0.
- Bàn 2, 3, 4 người; tới trắng không đổi.
- Phòng online: ván kết thúc ngay, ví và XP đúng, ván sau người nhất đánh trước.
- Máy: luôn trả nước hợp lệ hoặc bỏ lượt hợp lệ qua hàng nghìn ván giả lập, không kẹt; tình huống chọn sẵn: về đích bằng bộ chắc thắng, chặn đối thủ còn 1 lá, không đánh rác khi đối thủ còn 1 lá mà có lựa chọn khác; thời gian mỗi nước; tỉ lệ thắng.

Trình duyệt thật, cỡ 1440×900 và 390×844, bàn với 3 máy: không còn khung Diễn biến, ván dừng khi có người về nhất, màn kết quả đúng, không lỗi console.

## Ngoài phạm vi

- Chọn mức độ khó của máy, máy mô phỏng bài ẩn.
- Đổi bảng tiền chặt, thối, mức cược.

# Nông trại: trộm vật nuôi và cá, chó ăn cộng dồn

## Mục tiêu

Khi ghé nhà bạn, trộm được vật nuôi và cá đã lớn, cùng luật với trộm cây. Chó được cho ăn cộng dồn để canh nhà lâu hơn.
Thành công khi: xu không tự sinh thêm qua việc trộm, chó canh vật nuôi như canh cây, khách không thấy thông tin nuôi của chủ.

## Trộm vật nuôi và cá

- **Trộm được:** con trong `barn`, `yard`, `pond` đã lớn và đang không có zombie. Con non không trộm được. Chó không bị trộm.
- **Kẻ trộm nhận:** đúng món chủ nhận khi thu con đó, dùng chung logic với `CollectStock`, 1 món vào kho sản phẩm. Kho đầy thì không trộm được. Không cộng XP.
- **Chủ mất:** con đó bị xoá, kèm dòng zombie nếu có; nhật ký trộm ghi món.
- **Giới hạn:** chung `StealsPerDay` với trộm cây. Mỗi người chỉ thử mỗi con một lần, bị bắt cũng mất lượt thử con đó.
- **Chó:** chó đói thì lấy ngay. Chó no thì đuổi: dùng chung `Chase`, `FinishSteal`, `bite`, `settleChases`. Trong lúc đuổi, con đó khoá với người khác.
- **Chủ thu hoặc bán con đó trong lúc đuổi:** kẻ trộm thoát nhưng kết quả là `gone`, không có đồ, không bị cắn. Cùng một con không thể bị lấy hai lần.
- Chỉ bạn bè trộm được, kiểm tra ở package `friends` như trộm cây.

### Dữ liệu

`farm_thefts` thêm cột `stock_id INTEGER NOT NULL DEFAULT 0` qua kiểm tra `pragma_table_info`. Lần trộm con có `idx = -1`, `planted_at = 0`,
`stock_id > 0`. `plotMarks` phải bỏ qua các dòng này. Thêm `stockMarks` trả con người xem đã thử và con đang khoá.

### API

- `POST /api/friends/{id}/steal` nhận `{plot}` hoặc `{stock}`; có cả hai hoặc không có gì thì 400. Kết quả giữ nguyên dạng hiện có; `Chase` thêm `stock` (0 nếu là ô đất).
- `Visit` thêm `stock: [{id, place, kind, grown, tried, locked, zombie}]`, đúng các trường này, không gửi thông tin no đói, sản phẩm hay tiến độ.
- Hàm mới `func (f *Farm) StealStock(ctx, thief, owner, id int64) (StealResult, error)`; `Friends.Steal` nhận thêm tham số hoặc có hàm `StealStock` riêng. Lỗi: con không tồn tại hoặc của nhà khác 404; chưa lớn, có zombie, đã thử, đang khoá, hết lượt, kho đầy 409.

### Giao diện

- `friends.js` và `applyVisit` đưa `stock` vào cảnh ghé thăm: thấy chuồng, sân, ao và các con của bạn, không có huy hiệu nuôi và thẻ thông tin của chủ.
- Con trộm được có nhãn "Trộm", bấm hoặc `E` để trộm, dùng chung `startChase`, `stealDone`. Con đã thử, đang khoá, có zombie, chưa lớn thì toast lý do như ô đất. Người chưa là bạn thì "Chỉ xem".
- Nhật ký trộm hiện đúng tên món.

## Chó ăn cộng dồn

- Mỗi phần thức ăn: `fed_until = max(now, fed_until) + DogFullFor` (4 giờ).
- Bỏ `dogRefeedGap`. Giới hạn `DogMaxFull = 7 × 24 × 3600` giây tính từ bây giờ: vượt thì 409 "Chó đã no tối đa", không trừ thức ăn và XP.
- `Dog` thêm `maxFull`. Nút cho ăn bấm được khi chưa tới mức tối đa; đồng hồ hiện tổng thời gian còn lại; thanh no tính theo `maxFull` và không vượt khung.

## Kiểm thử

Test Go:

- Trộm con đã lớn lúc chó đói: kẻ trộm có đúng món, chủ mất con, dung lượng trống lại, nhật ký đúng, lượt trừ.
- Con non, con có zombie, con của nhà khác, chó (không có trong stock), đã thử, hết lượt, kho đầy: từ chối, không đổi gì.
- Chó no: `chase`, con khoá với người khác; thoát thì lấy được; bị chạm hoặc quá hạn thì mất 50 máu và con còn; chủ bán trong lúc đuổi thì `gone`.
- Hai token cùng một con không lấy được hai lần; `{plot}` và `{stock}` cùng lúc 400; không phải bạn bè 403 như trộm cây.
- `Visit.stock` không chứa thông tin nuôi. Dòng trộm con không làm sai `tried` của ô đất.
- Worth hai bên đổi đúng: tổng giá trị không tăng so với lúc chủ tự thu.
- Chó: cho ăn khi đói được 4 giờ; còn 3 giờ cho ăn thành 7 giờ; tới mức tối đa bị từ chối, không trừ thức ăn. Cập nhật `TestDogFeedFourHours`.
- Database cũ không có cột `stock_id` được nâng cấp lúc khởi động.

Trình duyệt thật, cỡ 1440×900 và 390×844, cổng và database riêng, hai tài khoản bạn bè: thấy con ở nhà bạn, trộm khi chó đói, trộm khi chó no và thoát, bị bắt, nhật ký của chủ, cho chó ăn nhiều lần liên tiếp; không lỗi console.

## Ngoài phạm vi

- Bảng tóm tắt danh sách bạn bè vẫn chỉ đếm ô chín. Trộm sản phẩm như trứng, sữa. Chó bảo vệ riêng từng khu.

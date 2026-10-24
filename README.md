# Tiến Lên Miền Nam

Game bài Tiến Lên Miền Nam 4 người: bạn chơi với 3 máy, chạy trực tiếp trên trình duyệt, không cần cài đặt gì.

## Chạy

Mở `index.html` bằng trình duyệt (double-click là được).

- Bấm vào lá bài để chọn, **Đánh** (Enter) để đánh, **Bỏ lượt** (Space) để bỏ.
- **Gợi ý** chọn sẵn nước đi do bot đề xuất.

## Luật đã cài

- Thứ tự: 3 < 4 < … < K < A < 2; chất ♠ < ♣ < ♦ < ♥.
- Bộ bài: rác, đôi, sám cô, sảnh (≥3 lá, không có 2), đôi thông (≥3 đôi liên tiếp), tứ quý.
- Chặn: cùng loại, cùng số lá, lá cao nhất lớn hơn.
- Chặt: 3 đôi thông chặt heo; tứ quý chặt heo, đôi heo, 3 đôi thông; 4 đôi thông chặt heo, đôi heo, 3 đôi thông, tứ quý.
- Bỏ lượt là mất quyền đánh tới hết vòng. Người về trước mà không ai chặn thì người kế tiếp được đánh.
- Ván đầu: người có 3♠ đánh trước và phải đánh kèm 3♠. Các ván sau: người về nhất đánh trước.
- Tới trắng: tứ quý heo, sảnh rồng, 6 đôi, 5 đôi thông.
- Điểm: Nhất +3, Nhì +1, Ba −1, Bét −3; tới trắng +6, mỗi người còn lại −2.

Chưa có: thối heo/thối bom, cóng, chơi online nhiều người.

## Cấu trúc

- `js/rules.js` — nhận diện bộ bài, so bài, chặt, tới trắng.
- `js/engine.js` — trạng thái ván, lượt, vòng, xếp hạng.
- `js/ai.js` — bot.
- `js/game.js`, `index.html`, `style.css` — giao diện.

## Test

```
npm test
```

Gồm test luật và giả lập 500 ván bot đánh với nhau.

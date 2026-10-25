package plaza

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

// fakeConn ghi lại các tin gửi tới một phiên (dạng JSON đã giải mã cho dễ so).
type fakeConn struct {
	mu   sync.Mutex
	msgs []map[string]any
}

func (f *fakeConn) Send(msg any) {
	b, _ := json.Marshal(msg)
	var m map[string]any
	json.Unmarshal(b, &m)
	f.mu.Lock()
	f.msgs = append(f.msgs, m)
	f.mu.Unlock()
}

// of trả các tin có kiểu t.
func (f *fakeConn) of(t string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, m := range f.msgs {
		if m["t"] == t {
			out = append(out, m)
		}
	}
	return out
}

func (f *fakeConn) last(t string) map[string]any {
	l := f.of(t)
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	h     *Hub
	clock time.Time
}

func newEnv(t *testing.T) *env {
	w, err := wallet.Open(filepath.Join(t.TempDir(), "plaza.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	e := &env{t: t, w: w, h: NewHub(w), clock: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}
	e.h.now = func() time.Time { return e.clock }
	return e
}

// join tạo tài khoản và mở một phiên ở thành phố, đã gửi vị trí đầu tiên.
func (e *env) join(name string) (*Player, *fakeConn, string) {
	e.t.Helper()
	_, token, err := e.w.Register(ctx, name+"_u", "matkhau123", wallet.Profile{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return e.session(token)
}

func (e *env) session(token string) (*Player, *fakeConn, string) {
	e.t.Helper()
	c := &fakeConn{}
	p, err := e.h.Hello(ctx, token, c)
	if err != nil {
		e.t.Fatal(err)
	}
	e.h.Handle(ctx, p, Message{T: "move", X: 800, Y: 700, Dir: "down"})
	return p, c, token
}

func (e *env) tick(d time.Duration) { e.clock = e.clock.Add(d) }

func TestHelloJoinMoveLeave(t *testing.T) {
	e := newEnv(t)
	if _, err := e.h.Hello(ctx, "sai", &fakeConn{}); err == nil {
		t.Fatal("token sai phải bị từ chối")
	}
	a, ca, _ := e.join("an")
	b, cb, _ := e.join("binh")
	if w := cb.last("welcome"); w["you"] != b.ID || len(w["players"].([]any)) != 0 {
		t.Fatalf("welcome chưa có ai (chưa vào Quảng trường): %v", w)
	}
	if en := cb.last("enter"); en == nil || len(en["players"].([]any)) != 1 {
		t.Fatalf("vào Quảng trường thì binh phải thấy an: %v", en)
	}
	if j := ca.last("join"); j == nil || j["player"].(map[string]any)["name"] != "binh" {
		t.Fatalf("an phải nhận tin binh vào: %v", ca.of("join"))
	}
	// Vị trí gộp: nhiều gói trong một nhịp chỉ gửi gói cuối, không gửi lại cho chính người đi.
	for i := range 5 {
		e.h.Handle(ctx, a, Message{T: "move", X: 900 + float64(i), Y: 700, Dir: "right", Walking: true})
	}
	e.h.Handle(ctx, a, Message{T: "move", X: math.NaN(), Y: 1})
	e.h.Handle(ctx, a, Message{T: "move", X: 1e9, Y: 1})
	e.h.Flush()
	mv := cb.of("moves")
	if len(mv) != 1 {
		t.Fatalf("phải gộp thành 1 gói, được %d", len(mv))
	}
	got := mv[0]["list"].([]any)[0].(map[string]any)
	if got["x"] != 904.0 || got["dir"] != "right" || got["walking"] != true {
		t.Fatalf("vị trí cuối: %v", got)
	}
	if len(ca.of("moves")) != 0 {
		t.Fatal("người đi không nhận lại vị trí của chính mình")
	}
	e.h.Flush()
	if len(cb.of("moves")) != 1 {
		t.Fatal("không đổi gì thì không gửi thêm")
	}
	if e.h.Online() != 2 {
		t.Fatalf("online %d", e.h.Online())
	}
	e.h.Leave(a)
	if l := cb.last("leave"); l == nil || l["id"] != a.ID {
		t.Fatal("binh phải nhận tin an rời")
	}
	if e.h.Online() != 1 {
		t.Fatalf("online sau khi rời %d", e.h.Online())
	}
}

func TestChatSpamFilterHistory(t *testing.T) {
	e := newEnv(t)
	a, ca, _ := e.join("an")
	_, cb, _ := e.join("binh")
	long := strings.Repeat("á", 200)
	if err := e.h.Handle(ctx, a, Message{T: "chat", Text: "  xin   chào " + long}); err != nil {
		t.Fatal(err)
	}
	c := cb.last("chat")
	if c == nil || len([]rune(c["text"].(string))) != MaxChatRunes || !strings.HasPrefix(c["text"].(string), "xin chào á") {
		t.Fatalf("tin phải gọn khoảng trắng và cắt %d ký tự: %v", MaxChatRunes, c)
	}
	if ca.last("chat") == nil {
		t.Fatal("người nói cũng nhận tin (để hiện bong bóng)")
	}
	if err := e.h.Handle(ctx, a, Message{T: "chat", Text: "nữa"}); err == nil {
		t.Fatal("nói liền trong 1,5 giây phải bị chặn")
	}
	for i := range 4 {
		e.tick(1600 * time.Millisecond)
		if err := e.h.Handle(ctx, a, Message{T: "chat", Text: "tin " + string(rune('a'+i))}); err != nil {
			t.Fatalf("tin %d: %v", i, err)
		}
	}
	e.tick(1600 * time.Millisecond)
	if err := e.h.Handle(ctx, a, Message{T: "chat", Text: "tin thứ 6"}); err == nil {
		t.Fatal("quá 5 tin trong 10 giây phải bị chặn")
	}
	e.tick(10 * time.Second)
	e.h.Handle(ctx, a, Message{T: "chat", Text: "đm thằng này, ĐỊT, lon bia ngon"})
	if got := cb.last("chat")["text"]; got != "*** thằng này, ***, lon bia ngon" {
		t.Fatalf("lọc từ tục: %q", got)
	}
	// Người mới vào nhận lịch sử chat.
	_, cc, _ := e.join("chi")
	if h := cc.last("enter")["chat"].([]any); len(h) != 6 {
		t.Fatalf("lịch sử chat %d tin", len(h))
	}
	for range ChatHistory + 5 {
		e.tick(2 * time.Second)
		e.h.Handle(ctx, a, Message{T: "chat", Text: "x"})
	}
	if len(e.h.chat) != ChatHistory {
		t.Fatalf("lịch sử phải giữ %d tin, có %d", ChatHistory, len(e.h.chat))
	}
}

func TestEmote(t *testing.T) {
	e := newEnv(t)
	a, _, _ := e.join("an")
	_, cb, _ := e.join("binh")
	e.h.Handle(ctx, a, Message{T: "emote", Kind: "wave"})
	e.h.Handle(ctx, a, Message{T: "emote", Kind: "heart"}) // dồn trong 1 giây: bỏ
	e.h.Handle(ctx, a, Message{T: "emote", Kind: "boom"})  // lạ: bỏ
	e.tick(1100 * time.Millisecond)
	e.h.Handle(ctx, a, Message{T: "emote", Kind: "dance"})
	got := cb.of("emote")
	if len(got) != 2 || got[0]["kind"] != "wave" || got[1]["kind"] != "dance" || got[0]["id"] != a.ID {
		t.Fatalf("biểu cảm: %v", got)
	}
}

func TestGift(t *testing.T) {
	e := newEnv(t)
	a, ca, tokA := e.join("an")
	b, cb, _ := e.join("binh")
	a2, ca2, _ := e.session(tokA) // an mở thêm một tab
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: a2.ID, Amount: 50}); err == nil {
		t.Fatal("không tặng cho tab khác của chính mình")
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 5}); err == nil {
		t.Fatal("dưới mức tối thiểu phải lỗi")
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: "khong-co", Amount: 50}); err == nil {
		t.Fatal("người không có mặt phải lỗi")
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 50}); err != nil {
		t.Fatal(err)
	}
	ua, _ := e.w.User(ctx, a.uid)
	ub, _ := e.w.User(ctx, b.uid)
	if ua.Balance != wallet.StartBalance-50 || ub.Balance != wallet.StartBalance+50 {
		t.Fatalf("số dư: %d %d", ua.Balance, ub.Balance)
	}
	if ca.last("balance")["balance"] != float64(ua.Balance) || ca2.last("balance")["balance"] != float64(ua.Balance) ||
		cb.last("balance")["balance"] != float64(ub.Balance) {
		t.Fatal("mọi tab của 2 người phải nhận số dư mới")
	}
	if g := cb.last("gift"); g == nil || g["amount"] != 50.0 || g["from"] != a.ID || g["to"] != b.ID {
		t.Fatalf("hiệu ứng tặng: %v", g)
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 50}); err == nil {
		t.Fatal("tặng liên tục trong 2 giây phải chờ")
	}
	// Hạn mức 5000/ngày: nạp thêm xu rồi tặng tới khi chạm hạn mức.
	tx, _ := e.w.Begin(ctx)
	tx.Credit(a.uid, 10000, "test", "nạp")
	tx.Commit()
	for range 4 {
		e.tick(3 * time.Second)
		if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 1000}); err != nil {
			t.Fatal(err)
		}
	}
	e.tick(3 * time.Second)
	err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 1000})
	if err == nil || !strings.Contains(err.Error(), "950") {
		t.Fatalf("vượt hạn mức phải báo còn 950: %v", err)
	}
	if u, _ := e.w.User(ctx, b.uid); u.Balance != wallet.StartBalance+4050 {
		t.Fatalf("người nhận sau hạn mức: %d", u.Balance)
	}
	// Không đủ xu.
	_, _, tokC := e.join("chi")
	c, _, _ := e.session(tokC)
	if err := e.h.Handle(ctx, c, Message{T: "gift", To: b.ID, Amount: 1000}); err != nil {
		t.Fatalf("chi có đúng 1000 xu: %v", err)
	}
	e.tick(3 * time.Second)
	if err := e.h.Handle(ctx, c, Message{T: "gift", To: b.ID, Amount: 10}); err == nil || !strings.Contains(err.Error(), "không đủ xu") {
		t.Fatalf("hết xu: %v", err)
	}
}

func TestLookRefreshAndInvite(t *testing.T) {
	e := newEnv(t)
	a, _, _ := e.join("an")
	b, cb, _ := e.join("binh")
	tx, _ := e.w.Begin(ctx)
	tx.ExecContext(ctx, `UPDATE users SET hospital_until = ?, hp = 0 WHERE id = ?`, time.Now().Add(time.Hour).Unix(), a.uid)
	tx.Commit()
	e.h.Handle(ctx, a, Message{T: "look"})
	if l := cb.last("look"); l == nil || l["hospital"] != true || l["id"] != a.ID {
		t.Fatalf("đổi trạng thái phải báo người khác: %v", l)
	}
	if err := e.h.Handle(ctx, a, Message{T: "invite", To: b.ID, Game: "caro", Link: "javascript:alert(1)"}); err == nil {
		t.Fatal("link lạ phải bị từ chối")
	}
	if err := e.h.Handle(ctx, a, Message{T: "invite", To: b.ID, Game: "baucua", Link: "/baucua/?room=ABC123"}); err != nil {
		t.Fatal(err)
	}
	if inv := cb.last("invite"); inv == nil || inv["link"] != "/baucua/?room=ABC123" || inv["name"] != "an" {
		t.Fatalf("lời mời: %v", inv)
	}
}

// Chỉ người đứng trong Quảng trường mới thấy nhau, chat, biểu cảm, tặng xu với nhau.
func TestOnlyPlazaSeesEachOther(t *testing.T) {
	if !InPlaza(816, 780) || InPlaza(224, 994) || InPlaza(816, 600) || InPlaza(1400, 790) {
		t.Fatal("vùng Quảng trường sai (cửa Quảng trường phải ở trong, Bệnh viện / đường / Nông trại ở ngoài)")
	}
	e := newEnv(t)
	a, ca, _ := e.join("an") // ở Quảng trường (800, 700)
	_, tokB, err := e.w.Register(ctx, "binh_u", "matkhau123", wallet.Profile{Name: "binh"})
	if err != nil {
		t.Fatal(err)
	}
	cb := &fakeConn{}
	b, _ := e.h.Hello(ctx, tokB, cb)
	e.h.Handle(ctx, b, Message{T: "move", X: 224, Y: 994, Dir: "down"}) // trước cửa Bệnh viện
	if len(ca.of("join")) != 0 || len(cb.of("enter")) != 0 {
		t.Fatal("người ngoài Quảng trường không được hiện")
	}
	e.h.Handle(ctx, a, Message{T: "move", X: 820, Y: 760, Dir: "left", Walking: true})
	e.h.Handle(ctx, b, Message{T: "move", X: 230, Y: 990, Dir: "left", Walking: true})
	e.h.Flush()
	if len(ca.of("moves")) != 0 || len(cb.of("moves")) != 0 {
		t.Fatal("không gửi vị trí giữa trong và ngoài Quảng trường")
	}
	e.h.Handle(ctx, a, Message{T: "chat", Text: "có ai không"})
	if len(cb.of("chat")) != 0 {
		t.Fatal("người ngoài không nhận chat")
	}
	if err := e.h.Handle(ctx, b, Message{T: "chat", Text: "alo"}); err == nil {
		t.Fatal("ngoài Quảng trường không chat được")
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 50}); err == nil {
		t.Fatal("không tặng xu cho người ngoài Quảng trường")
	}
	if e.h.Online() != 1 {
		t.Fatalf("online (ở Quảng trường) %d", e.h.Online())
	}

	// binh đi vào: hai bên thấy nhau, binh nhận lịch sử chat.
	e.h.Handle(ctx, b, Message{T: "move", X: 700, Y: 800, Dir: "right"})
	if j := ca.last("join"); j == nil || j["player"].(map[string]any)["id"] != b.ID {
		t.Fatalf("an phải thấy binh vào: %v", ca.of("join"))
	}
	en := cb.last("enter")
	if en == nil || len(en["players"].([]any)) != 1 || len(en["chat"].([]any)) != 1 {
		t.Fatalf("binh vào phải nhận an + lịch sử chat: %v", en)
	}
	e.h.Handle(ctx, b, Message{T: "move", X: 710, Y: 800, Dir: "right", Walking: true})
	e.h.Flush()
	if len(ca.of("moves")) != 1 {
		t.Fatal("cùng ở Quảng trường thì thấy nhau đi")
	}
	if err := e.h.Handle(ctx, a, Message{T: "gift", To: b.ID, Amount: 50}); err != nil {
		t.Fatalf("cùng ở Quảng trường thì tặng được: %v", err)
	}

	// binh đi ra: an nhận leave, binh nhận exit; ra rồi không nhận chat nữa.
	e.h.Handle(ctx, b, Message{T: "move", X: 1400, Y: 790, Dir: "right"})
	if l := ca.last("leave"); l == nil || l["id"] != b.ID {
		t.Fatal("binh rời Quảng trường thì an phải nhận leave")
	}
	if cb.last("exit") == nil {
		t.Fatal("binh phải nhận exit")
	}
	n := len(cb.of("chat"))
	e.tick(2 * time.Second)
	e.h.Handle(ctx, a, Message{T: "chat", Text: "đi rồi à"})
	if len(cb.of("chat")) != n {
		t.Fatal("rời Quảng trường thì không nhận chat")
	}
}

// Nhiều phiên cùng đi, chat, tặng xu song song (chạy với -race).
func TestConcurrent(t *testing.T) {
	e := newEnv(t)
	var ps []*Player
	for _, n := range []string{"p1", "p2", "p3", "p4"} {
		p, _, _ := e.join(n)
		ps = append(ps, p)
	}
	e.h.now = time.Now
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range 50 {
				e.h.Handle(ctx, p, Message{T: "move", X: 700 + float64(k), Y: 750 + float64(i), Dir: "left", Walking: true})
				if k%10 == 0 {
					e.h.Handle(ctx, p, Message{T: "chat", Text: "chào"})
					e.h.Handle(ctx, p, Message{T: "gift", To: ps[(i+1)%len(ps)].ID, Amount: 10})
				}
				e.h.Flush()
			}
		}()
	}
	wg.Wait()
	var total int64
	for _, p := range ps {
		u, _ := e.w.User(ctx, p.uid)
		total += u.Balance
	}
	if total != 4*wallet.StartBalance {
		t.Fatalf("tổng xu phải giữ nguyên, được %d", total)
	}
}

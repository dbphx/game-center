package online

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeConn struct {
	mu   sync.Mutex
	msgs []any
}

func (f *fakeConn) Send(msg any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
}

func (f *fakeConn) lastState() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.msgs) - 1; i >= 0; i-- {
		if s, ok := f.msgs[i].(State); ok {
			return s
		}
	}
	return State{}
}

func (f *fakeConn) got(t string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.msgs {
		if mm, ok := m.(map[string]string); ok && mm["t"] == t {
			return true
		}
	}
	return false
}

// fakeBank là ví trong bộ nhớ, cùng quy tắc với wallet thật: số dư không âm.
type fakeBank struct {
	mu       sync.Mutex
	balances map[int64]int64
	settled  int
}

func newBank(balances map[int64]int64) *fakeBank { return &fakeBank{balances: balances} }

func (b *fakeBank) Balance(_ context.Context, id int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bal, ok := b.balances[id]
	if !ok {
		return 0, errors.New("không có tài khoản")
	}
	return bal, nil
}

func (b *fakeBank) Settle(_ context.Context, _, _ string, deltas map[int64]int64) (map[int64]int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.settled++
	out := map[int64]int64{}
	for id, d := range deltas {
		b.balances[id] = max(0, b.balances[id]+d)
		out[id] = b.balances[id]
	}
	return out, nil
}

func (b *fakeBank) get(id int64) int64 {
	bal, _ := b.Balance(context.Background(), id)
	return bal
}

var (
	an   = User{ID: 1, Name: "An"}
	binh = User{ID: 2, Name: "Bình"}
	chi  = User{ID: 3, Name: "Chi"}
)

func richBank() *fakeBank { return newBank(map[int64]int64{1: 1000, 2: 1000, 3: 1000}) }

var fastOptions = Options{BotDelay: time.Millisecond, TurnTimeout: 5 * time.Millisecond, Grace: 20 * time.Millisecond}
var slowOptions = Options{BotDelay: time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("hết thời gian chờ: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func mustCreate(t *testing.T, h *Hub, u User) (*Session, *fakeConn) {
	t.Helper()
	c := &fakeConn{}
	s, err := h.Create(u, c)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func mustJoin(t *testing.T, h *Hub, code string, u User) (*Session, *fakeConn) {
	t.Helper()
	c := &fakeConn{}
	s, err := h.Join(code, u, c)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

// Một người với 3 máy: người không đánh thì hết giờ máy đánh thay, ván phải tự chạy tới hết
// và kết quả của người thật được ghi vào ví.
func TestSoloWithBotsSettlesIntoWallet(t *testing.T) {
	bank := richBank()
	h := NewHub(bank, fastOptions)
	s, c := mustCreate(t, h, an)
	for range 3 {
		if err := s.Room.Handle(s.Player, Message{T: "addBot"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Room.Handle(s.Player, Message{T: "addBot"}); err == nil {
		t.Fatal("phòng 4 người không được thêm máy nữa")
	}
	if err := s.Room.Handle(s.Player, Message{T: "start"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "hết ván", func() bool { return c.lastState().Phase == PhaseOver })

	st := c.lastState()
	if want := max(0, 1000+int64(st.Money[0])); bank.get(an.ID) != want || st.Players[0].Coins != want {
		t.Fatalf("ví sau ván: %d, hiển thị %d, muốn %d", bank.get(an.ID), st.Players[0].Coins, want)
	}
	for _, p := range st.Players[1:] {
		if p.Reveal == nil && p.Cards > 0 {
			t.Error("hết ván phải lộ bài còn lại")
		}
	}
}

func TestTwoHumansSeeOnlyOwnHand(t *testing.T) {
	h := NewHub(richBank(), slowOptions)
	host, hc := mustCreate(t, h, an)
	guest, gc := mustJoin(t, h, " "+host.Room.Code+" ", binh)
	if err := guest.Room.Handle(guest.Player, Message{T: "start"}); err == nil {
		t.Fatal("khách không được bắt đầu")
	}
	// Hiếm khi có người tới trắng làm ván kết thúc ngay; chia lại tới khi có ván đang chơi.
	for hc.lastState().Phase != PhasePlaying {
		if err := host.Room.Handle(host.Player, Message{T: "start"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Join(host.Room.Code, chi, &fakeConn{}); err == nil {
		t.Fatal("không được vào khi đang chơi")
	}

	hs, gs := hc.lastState(), gc.lastState()
	if len(hs.Hand) != 13 || len(gs.Hand) != 13 || hs.Hand[0] == gs.Hand[0] {
		t.Fatal("mỗi người phải thấy đúng 13 lá của mình")
	}
	for _, p := range hs.Players {
		if p.Reveal != nil {
			t.Fatal("không được lộ bài người khác giữa ván")
		}
	}
	notTurn, notTurnState := host, hs
	if hs.Players[hs.Turn].ID == host.Player.ID {
		notTurn, notTurnState = guest, gs
	}
	if err := notTurn.Room.Handle(notTurn.Player, Message{T: "play", Cards: notTurnState.Hand[:1]}); err == nil {
		t.Fatal("đánh khi chưa tới lượt phải bị từ chối")
	}
}

// Mất kết nối rồi vào lại bằng mã phòng (kể cả giữa ván) thì được trả đúng ghế cũ.
func TestRejoinGetsSameSeat(t *testing.T) {
	h := NewHub(richBank(), slowOptions)
	host, _ := mustCreate(t, h, an)
	guest, gc := mustJoin(t, h, host.Room.Code, binh)
	host.Room.Handle(host.Player, Message{T: "addBot"})
	host.Room.Handle(host.Player, Message{T: "start"})

	guest.Room.Disconnect(guest.Player, gc)
	if guest.Player.Online {
		t.Fatal("phải đánh dấu offline")
	}
	again, gc2 := mustJoin(t, h, host.Room.Code, binh)
	if again.Player != guest.Player || !again.Player.Online {
		t.Fatal("vào lại phải được ghế cũ")
	}
	if n := len(gc2.lastState().Players); n != 3 {
		t.Fatalf("không được thêm ghế mới, có %d người", n)
	}
	// Mở tab thứ hai: tab cũ bị thay thế.
	mustJoin(t, h, host.Room.Code, binh)
	if !gc2.got("replaced") {
		t.Fatal("tab cũ phải nhận tin replaced")
	}
}

func TestOfflinePlayerRemovedAfterGraceInLobby(t *testing.T) {
	h := NewHub(richBank(), fastOptions)
	host, hc := mustCreate(t, h, an)
	guest, gc := mustJoin(t, h, host.Room.Code, binh)
	guest.Room.Disconnect(guest.Player, gc)
	waitFor(t, "xoá người offline", func() bool { return len(hc.lastState().Players) == 1 })
}

func TestHostLeavesPassesHostAndLastLeaveClosesRoom(t *testing.T) {
	h := NewHub(richBank(), fastOptions)
	host, hc := mustCreate(t, h, an)
	guest, gc := mustJoin(t, h, host.Room.Code, binh)

	host.Room.Handle(host.Player, Message{T: "leave"})
	if !hc.got("left") {
		t.Fatal("người rời phải nhận tin left")
	}
	st := gc.lastState()
	if len(st.Players) != 1 || !st.Players[0].Host {
		t.Fatal("khách phải thành chủ phòng")
	}
	guest.Room.Handle(guest.Player, Message{T: "leave"})
	if h.RoomCount() != 0 {
		t.Fatal("phòng trống phải bị xoá")
	}
}

// Người cuối cùng rời giữa ván: máy đánh nốt ngay để chốt xu rồi mới đóng phòng.
func TestLastHumanLeavingMidGameStillSettles(t *testing.T) {
	bank := richBank()
	h := NewHub(bank, slowOptions)
	s, _ := mustCreate(t, h, an)
	s.Room.Handle(s.Player, Message{T: "addBot"})
	s.Room.Handle(s.Player, Message{T: "start"})
	s.Room.Handle(s.Player, Message{T: "leave"})
	if h.RoomCount() != 0 {
		t.Fatal("phòng phải đóng")
	}
	if bank.settled != 1 {
		t.Fatalf("ván phải được chốt xu đúng 1 lần, được %d", bank.settled)
	}
}

func TestLeaveMidGameBotTakesOver(t *testing.T) {
	h := NewHub(richBank(), slowOptions)
	host, hc := mustCreate(t, h, an)
	guest, _ := mustJoin(t, h, host.Room.Code, binh)
	host.Room.Handle(host.Player, Message{T: "addBot"})
	host.Room.Handle(host.Player, Message{T: "start"})

	guest.Room.Handle(guest.Player, Message{T: "leave"})
	// Chủ phòng đánh theo bot cho nhanh; khách đã rời được máy đánh thay.
	r := host.Room
	waitFor(t, "hết ván", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.phase == PhaseOver {
			return true
		}
		if r.players[r.game.Turn] == host.Player {
			r.autoMove()
		}
		return false
	})
	host.Room.Handle(host.Player, Message{T: "start"})
	if n := len(hc.lastState().Players); n != 2 {
		t.Fatalf("người đã rời phải bị xoá trước ván mới, còn %d người", n)
	}
}

func TestBetRules(t *testing.T) {
	bank := newBank(map[int64]int64{1: 1000, 2: 50})
	h := NewHub(bank, slowOptions)
	host, _ := mustCreate(t, h, an)
	guest, gc := mustJoin(t, h, host.Room.Code, binh)

	if err := guest.Room.Handle(guest.Player, Message{T: "setBet", Bet: 100}); err == nil {
		t.Fatal("khách không được đổi mức cược")
	}
	if err := host.Room.Handle(host.Player, Message{T: "setBet", Bet: 7}); err == nil {
		t.Fatal("mức cược lạ phải bị từ chối")
	}
	if err := host.Room.Handle(host.Player, Message{T: "setBet", Bet: 100}); err != nil {
		t.Fatal(err)
	}
	if gc.lastState().Bet != 100 {
		t.Fatal("mọi người phải thấy mức cược mới")
	}
	if err := host.Room.Handle(host.Player, Message{T: "start"}); err == nil {
		t.Fatal("người không đủ xu thì không được bắt đầu")
	}
	// Bình nạp thêm xu ở nơi khác (ví dụ nhận thưởng ở hub): bắt đầu lại phải đọc số dư mới.
	bank.mu.Lock()
	bank.balances[2] = 500
	bank.mu.Unlock()
	if err := host.Room.Handle(host.Player, Message{T: "start"}); err != nil {
		t.Fatal(err)
	}
}

package room

import (
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

var fastOptions = Options{BotDelay: time.Millisecond, TurnTimeout: 5 * time.Millisecond, Grace: 20 * time.Millisecond}

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

func mustCreate(t *testing.T, h *Hub, name string) (*Session, *fakeConn) {
	t.Helper()
	c := &fakeConn{}
	s, err := h.Create(name, c)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

// Một người với 3 máy: người không đánh thì hết giờ máy đánh thay, ván phải tự chạy tới hết.
func TestSoloWithBotsFinishes(t *testing.T) {
	h := NewHub(fastOptions)
	s, c := mustCreate(t, h, "An")
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
	sum := 0
	for _, p := range st.Players {
		sum += p.Coins - StartCoins
		if p.ID != st.You && p.Reveal == nil && p.Cards > 0 {
			t.Error("hết ván phải lộ bài còn lại")
		}
	}
	if sum != 0 {
		t.Errorf("tổng xu thay đổi %d, phải bằng 0", sum)
	}
}

func TestTwoHumansSeeOnlyOwnHand(t *testing.T) {
	h := NewHub(Options{BotDelay: time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour})
	host, hc := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, err := h.Join(" "+host.Room.Code+" ", "Bình", gc)
	if err != nil {
		t.Fatal(err)
	}
	if err := guest.Room.Handle(guest.Player, Message{T: "start"}); err == nil {
		t.Fatal("khách không được bắt đầu")
	}
	// Hiếm khi có người tới trắng làm ván kết thúc ngay; chia lại tới khi có ván đang chơi.
	for hc.lastState().Phase != PhasePlaying {
		if err := host.Room.Handle(host.Player, Message{T: "start"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Join(host.Room.Code, "Chi", &fakeConn{}); err == nil {
		t.Fatal("không được vào khi đang chơi")
	}

	hs, gs := hc.lastState(), gc.lastState()
	if hs.Phase != PhasePlaying || len(hs.Players) != 2 {
		t.Fatalf("trạng thái sai: %+v", hs)
	}
	if len(hs.Hand) != 13 || len(gs.Hand) != 13 || hs.Hand[0] == gs.Hand[0] {
		t.Fatal("mỗi người phải thấy đúng 13 lá của mình")
	}
	for _, p := range hs.Players {
		if p.Reveal != nil {
			t.Fatal("không được lộ bài người khác giữa ván")
		}
	}

	// Người không tới lượt đánh thì bị từ chối.
	notTurn, notTurnState := host, hs
	if hs.Players[hs.Turn].ID == host.Player.ID {
		notTurn, notTurnState = guest, gs
	}
	if err := notTurn.Room.Handle(notTurn.Player, Message{T: "play", Cards: notTurnState.Hand[:1]}); err == nil {
		t.Fatal("đánh khi chưa tới lượt phải bị từ chối")
	}
}

func TestDisconnectAndResume(t *testing.T) {
	h := NewHub(Options{BotDelay: time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour})
	host, _ := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, _ := h.Join(host.Room.Code, "Bình", gc)

	guest.Room.Disconnect(guest.Player, gc)
	if guest.Player.Online {
		t.Fatal("phải đánh dấu offline")
	}
	gc2 := &fakeConn{}
	s, err := h.Resume(guest.Token, gc2)
	if err != nil || s.Player != guest.Player || !s.Player.Online {
		t.Fatalf("vào lại thất bại: %v", err)
	}
	if gc2.lastState().Code != host.Room.Code {
		t.Fatal("phải nhận lại trạng thái phòng")
	}
}

func TestOfflinePlayerRemovedAfterGraceInLobby(t *testing.T) {
	h := NewHub(fastOptions)
	host, hc := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, _ := h.Join(host.Room.Code, "Bình", gc)
	guest.Room.Disconnect(guest.Player, gc)
	waitFor(t, "xoá người offline", func() bool { return len(hc.lastState().Players) == 1 })
	if _, err := h.Resume(guest.Token, &fakeConn{}); err == nil {
		t.Fatal("token cũ không được dùng lại sau khi bị xoá")
	}
}

func TestHostLeavesPassesHostAndLastLeaveClosesRoom(t *testing.T) {
	h := NewHub(fastOptions)
	host, hc := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, _ := h.Join(host.Room.Code, "Bình", gc)

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

func TestLeaveMidGameBotTakesOver(t *testing.T) {
	h := NewHub(Options{BotDelay: time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour})
	host, hc := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, _ := h.Join(host.Room.Code, "Bình", gc)
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

func TestBetAndCoins(t *testing.T) {
	h := NewHub(fastOptions)
	host, hc := mustCreate(t, h, "An")
	gc := &fakeConn{}
	guest, _ := h.Join(host.Room.Code, "Bình", gc)

	if err := guest.Room.Handle(guest.Player, Message{T: "setBet", Bet: 100}); err == nil {
		t.Fatal("khách không được đổi mức cược")
	}
	if err := host.Room.Handle(host.Player, Message{T: "setBet", Bet: 7}); err == nil {
		t.Fatal("mức cược lạ phải bị từ chối")
	}
	if err := host.Room.Handle(host.Player, Message{T: "setBet", Bet: 100}); err != nil {
		t.Fatal(err)
	}
	if err := guest.Room.Handle(guest.Player, Message{T: "refill"}); err == nil {
		t.Fatal("còn đủ xu thì không được nhận thêm")
	}

	// Chơi hết một ván (người không đánh thì hết giờ máy đánh thay); xu đổi theo tiền ván × mức cược.
	host.Room.Handle(host.Player, Message{T: "start"})
	waitFor(t, "hết ván", func() bool { return hc.lastState().Phase == PhaseOver })
	st := hc.lastState()
	if st.GameBet != 100 {
		t.Fatalf("mức cược ván phải là 100, được %d", st.GameBet)
	}
	for i, p := range st.Players {
		if p.Coins != StartCoins+st.Money[i] || st.Money[i]%100 != 0 {
			t.Fatalf("%s: xu %d, tiền ván %d", p.Name, p.Coins, st.Money[i])
		}
	}

	// Hết xu thì không vào ván được cho tới khi nhận lại.
	host.Room.mu.Lock()
	guest.Player.Coins = 50
	host.Room.mu.Unlock()
	if err := host.Room.Handle(host.Player, Message{T: "start"}); err == nil {
		t.Fatal("người không đủ xu thì không được bắt đầu")
	}
	if err := guest.Room.Handle(guest.Player, Message{T: "refill"}); err != nil {
		t.Fatal(err)
	}
	if v := gc.lastState().Players[1]; v.Coins != StartCoins || v.Refills != 1 {
		t.Fatalf("nhận lại xu sai: %+v", v)
	}
}

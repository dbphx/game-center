package caro

import (
	"context"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

// ---------- Luật và máy ----------

// put đặt quân theo danh sách "x,y" cho một bên.
func put(b *Board, m byte, pts ...[2]int) {
	for _, p := range pts {
		b[p[1]*Size+p[0]] = m
	}
}

func row(y, from, n int) [][2]int {
	var out [][2]int
	for i := range n {
		out = append(out, [2]int{from + i, y})
	}
	return out
}

func TestWinFree(t *testing.T) {
	b := NewBoard()
	put(&b, X, row(7, 3, 5)...)
	if line := b.WinLine(5, 7, RuleFree); len(line) != 5 {
		t.Fatalf("5 quân ngang phải thắng, được %v", line)
	}
	b2 := NewBoard()
	put(&b2, O, [2]int{2, 2}, [2]int{3, 3}, [2]int{4, 4}, [2]int{5, 5}, [2]int{6, 6})
	if b2.WinLine(4, 4, RuleBlock2) == nil {
		t.Fatal("5 quân chéo không bị chặn phải thắng")
	}
	b3 := NewBoard()
	put(&b3, X, row(7, 3, 4)...)
	if b3.WinLine(4, 7, RuleFree) != nil {
		t.Fatal("4 quân chưa thắng")
	}
}

func TestBlock2Rule(t *testing.T) {
	b := NewBoard()
	put(&b, X, row(7, 3, 5)...)
	put(&b, O, [2]int{2, 7}, [2]int{8, 7})
	if b.WinLine(5, 7, RuleBlock2) != nil {
		t.Fatal("luật chặn 2 đầu: đúng 5 bị chặn cả hai đầu không thắng")
	}
	if b.WinLine(5, 7, RuleFree) == nil {
		t.Fatal("luật tự do: bị chặn vẫn thắng")
	}
	// Chỉ chặn một đầu thì vẫn thắng.
	b[8+7*Size] = Empty
	if b.WinLine(5, 7, RuleBlock2) == nil {
		t.Fatal("chặn một đầu vẫn thắng")
	}
	// Mép bàn không tính là chặn.
	e := NewBoard()
	put(&e, X, row(0, 0, 5)...)
	put(&e, O, [2]int{5, 0})
	if e.WinLine(2, 0, RuleBlock2) == nil {
		t.Fatal("sát mép bàn + một đầu bị chặn vẫn thắng")
	}
	// 6 quân bị chặn hai đầu vẫn thắng.
	s := NewBoard()
	put(&s, X, row(7, 3, 6)...)
	put(&s, O, [2]int{2, 7}, [2]int{9, 7})
	if line := s.WinLine(5, 7, RuleBlock2); len(line) != 6 {
		t.Fatalf("6 quân vẫn thắng, được %v", line)
	}
}

func TestBotWinsAndBlocks(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, lv := range []string{BotNormal, BotHard} {
		// Máy (O) có 4 quân mở: phải đánh nước thắng.
		b := NewBoard()
		put(&b, O, row(5, 4, 4)...)
		put(&b, X, [2]int{1, 1}, [2]int{2, 2}, [2]int{1, 3})
		x, y, _ := BestMove(b, O, RuleBlock2, lv, rng)
		if !(y == 5 && (x == 3 || x == 8)) {
			t.Fatalf("%s: có 4 thì phải thắng, đánh (%d,%d)", lv, x, y)
		}
		// Đối thủ (X) có 4 mở một đầu: máy phải chặn.
		b = NewBoard()
		put(&b, X, row(9, 5, 4)...)
		put(&b, O, [2]int{4, 9}, [2]int{0, 0})
		x, y, _ = BestMove(b, O, RuleBlock2, lv, rng)
		if x != 9 || y != 9 {
			t.Fatalf("%s: phải chặn ở (9,9), đánh (%d,%d)", lv, x, y)
		}
		// Đối thủ có 3 mở hai đầu: máy phải chặn ở một đầu.
		b = NewBoard()
		put(&b, X, row(9, 5, 3)...)
		put(&b, O, [2]int{0, 0})
		x, y, _ = BestMove(b, O, RuleBlock2, lv, rng)
		if !(y == 9 && (x == 4 || x == 8)) {
			t.Fatalf("%s: phải chặn 3 mở, đánh (%d,%d)", lv, x, y)
		}
	}
	// Bàn trống thì đánh giữa.
	if x, y, _ := BestMove(NewBoard(), X, RuleFree, BotEasy, rng); x != 7 || y != 7 {
		t.Fatalf("bàn trống phải đánh giữa, được (%d,%d)", x, y)
	}
}

// ---------- Bàn online ----------

type fakeConn struct {
	mu   sync.Mutex
	msgs []any
}

func (f *fakeConn) Send(msg any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
}

func (f *fakeConn) last() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.msgs) - 1; i >= 0; i-- {
		if s, ok := f.msgs[i].(State); ok {
			return s
		}
	}
	return State{}
}

type fakeBank struct {
	mu       sync.Mutex
	balances map[int64]int64
	xp       map[int64]int64
}

func newBank() *fakeBank {
	return &fakeBank{balances: map[int64]int64{1: 1000, 2: 1000, 3: 1000}, xp: map[int64]int64{}}
}

func (b *fakeBank) Balance(_ context.Context, id int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.balances[id], nil
}

func (b *fakeBank) Settle(_ context.Context, _, _ string, d map[int64]int64) (map[int64]int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[int64]int64{}
	for id, v := range d {
		b.balances[id] = max(0, b.balances[id]+v)
		out[id] = b.balances[id]
	}
	return out, nil
}

func (b *fakeBank) AwardXP(_ context.Context, xp map[int64]int64) (map[int64]int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[int64]int{}
	for id, v := range xp {
		b.xp[id] += v
		out[id] = 1
	}
	return out, nil
}

func (b *fakeBank) get(id int64) (int64, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.balances[id], b.xp[id]
}

var (
	an   = User{ID: 1, Name: "An", HP: 100}
	binh = User{ID: 2, Name: "Bình", HP: 100}
	chi  = User{ID: 3, Name: "Chi", HP: 100}
	fast = Options{BotDelay: 5 * time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour}
)

func ptr(v int) *int { return &v }

// table tạo bàn 2 người (An chủ bàn) với mức cược bet.
func table(t *testing.T, opts Options, bet int) (*Hub, *fakeBank, *Session, *fakeConn, *Session, *fakeConn) {
	t.Helper()
	bank := newBank()
	h := NewHub(bank, opts)
	ca, cb := &fakeConn{}, &fakeConn{}
	sa, err := h.Create(an, ca, ptr(bet), RuleBlock2)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := h.Join(sa.Room.Code, binh, cb)
	if err != nil {
		t.Fatal(err)
	}
	return h, bank, sa, ca, sb, cb
}

func move(t *testing.T, s *Session, x, y int) {
	t.Helper()
	if err := s.Room.Handle(s.Player, Message{T: "move", X: x, Y: y}); err != nil {
		t.Fatalf("đánh (%d,%d): %v", x, y, err)
	}
}

func TestOnlineWinPaysBet(t *testing.T) {
	_, bank, a, ca, b, _ := table(t, fast, 100)
	if err := b.Room.Handle(b.Player, Message{T: "start"}); err == nil {
		t.Fatal("chỉ chủ bàn được bắt đầu")
	}
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err != nil {
		t.Fatal(err)
	}
	st := ca.last()
	if st.Phase != PhasePlaying || st.Turn != "x" || st.Players[0].Mark != "x" {
		t.Fatalf("ván mới: %+v", st)
	}
	if err := b.Room.Handle(b.Player, Message{T: "move", X: 0, Y: 0}); err == nil {
		t.Fatal("chưa tới lượt O")
	}
	for i := range 4 {
		move(t, a, 3+i, 7)
		move(t, b, 3+i, 9)
	}
	if err := b.Room.Handle(b.Player, Message{T: "move", X: 3, Y: 7}); err == nil {
		t.Fatal("ô đã có quân / chưa tới lượt")
	}
	move(t, a, 7, 7)
	st = ca.last()
	if st.Phase != PhaseOver || st.Result == nil || st.Result.Winner != "x" || st.Result.Reason != "five" || len(st.WinLine) != 5 {
		t.Fatalf("X thắng: %+v", st.Result)
	}
	if bal, xp := bank.get(1); bal != 1100 || xp != XPWin {
		t.Fatalf("An thắng: ví %d XP %d", bal, xp)
	}
	if bal, xp := bank.get(2); bal != 900 || xp != XPLose {
		t.Fatalf("Bình thua: ví %d XP %d", bal, xp)
	}
	// Chơi lại: cả hai bấm, người thua (Bình) cầm X đi trước.
	if err := a.Room.Handle(a.Player, Message{T: "rematch"}); err != nil {
		t.Fatal(err)
	}
	if ca.last().Phase != PhaseOver {
		t.Fatal("phải chờ cả hai bấm chơi lại")
	}
	if err := b.Room.Handle(b.Player, Message{T: "rematch"}); err != nil {
		t.Fatal(err)
	}
	if st = ca.last(); st.Phase != PhasePlaying || st.Players[1].Mark != "x" || st.Moves != 0 {
		t.Fatalf("ván sau người thua đi trước: %+v", st.Players)
	}
}

func TestResignTimeoutAndDraw(t *testing.T) {
	_, bank, a, ca, b, _ := table(t, fast, 0)
	a.Room.Handle(a.Player, Message{T: "start"})
	if err := b.Room.Handle(b.Player, Message{T: "resign"}); err != nil {
		t.Fatal(err)
	}
	if r := ca.last().Result; r == nil || r.Winner != "x" || r.Reason != "resign" {
		t.Fatalf("đầu hàng: %+v", r)
	}
	if bal, xp := bank.get(1); bal != 1000 || xp != XPWin {
		t.Fatalf("bàn 0 xu: ví không đổi nhưng có XP, được %d / %d", bal, xp)
	}

	// Hết giờ: người tới lượt thua.
	opts := fast
	opts.TurnTimeout = 30 * time.Millisecond
	_, _, a2, ca2, _, _ := table(t, opts, 10)
	a2.Room.Handle(a2.Player, Message{T: "start"})
	deadline := time.Now().Add(2 * time.Second)
	for ca2.last().Phase != PhaseOver && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r := ca2.last().Result; r == nil || r.Winner != "o" || r.Reason != "timeout" {
		t.Fatalf("X hết giờ phải thua: %+v", r)
	}

	// Hoà: lấp đầy bàn không có 5 (dùng thẳng place để dựng thế).
	_, bank3, a3, ca3, _, _ := table(t, fast, 50)
	a3.Room.Handle(a3.Player, Message{T: "start"})
	r := a3.Room
	r.mu.Lock()
	for y := range Size {
		for x := range Size {
			// Mẫu sọc lệch theo cặp cột: không có 5 liên tiếp theo hướng nào.
			if ((x/2)+y)%2 == 0 {
				r.board[y*Size+x] = X
			} else {
				r.board[y*Size+x] = O
			}
		}
	}
	r.board[Size*Size-1] = Empty
	r.turn = X
	if r.board.WinLine(14, 14, RuleBlock2) != nil {
		r.mu.Unlock()
		t.Fatal("mẫu hoà không được có 5")
	}
	r.mu.Unlock()
	move(t, a3, 14, 14)
	st := ca3.last()
	if st.Result == nil {
		t.Fatal("hết ô phải kết thúc")
	}
	if st.Result.Reason != "draw" || st.Result.Winner != "" {
		t.Fatalf("hết ô không ai 5 là hoà: %+v", st.Result)
	}
	if bal, xp := bank3.get(1); bal != 1000 || xp != XPDraw {
		t.Fatalf("hoà: ví %d XP %d", bal, xp)
	}
}

func TestPlayVsBot(t *testing.T) {
	bank := newBank()
	h := NewHub(bank, fast)
	c := &fakeConn{}
	s, _ := h.Create(an, c, ptr(200), RuleFree)
	if err := s.Room.Handle(s.Player, Message{T: "addBot", Level: BotHard}); err != nil {
		t.Fatal(err)
	}
	if err := s.Room.Handle(s.Player, Message{T: "addBot"}); err == nil {
		t.Fatal("bàn đủ 2 thì không thêm máy")
	}
	if err := s.Room.Handle(s.Player, Message{T: "start"}); err != nil {
		t.Fatal(err)
	}
	// Người đánh bừa theo hàng, máy phải thắng hoặc ván kết thúc trong giới hạn.
	deadline := time.Now().Add(5 * time.Second)
	for x := 0; c.last().Phase == PhasePlaying && time.Now().Before(deadline); {
		st := c.last()
		if st.Turn == "x" {
			for st.Board[x] != '.' {
				x++
			}
			s.Room.Handle(s.Player, Message{T: "move", X: x % Size, Y: x / Size})
		}
		time.Sleep(2 * time.Millisecond)
	}
	st := c.last()
	if st.Phase != PhaseOver || st.Result.Winner != "o" {
		t.Fatalf("máy khó phải thắng người đánh bừa: %+v", st.Result)
	}
	if bal, _ := bank.get(1); bal != 800 {
		t.Fatalf("thua máy mất 200, ví %d", bal)
	}
}

func TestLeaveMidGameLoses(t *testing.T) {
	h, bank, a, ca, b, _ := table(t, fast, 100)
	a.Room.Handle(a.Player, Message{T: "start"})
	move(t, a, 7, 7)
	if err := b.Room.Handle(b.Player, Message{T: "leave"}); err != nil {
		t.Fatal(err)
	}
	st := ca.last()
	if st.Result == nil || st.Result.Reason != "left" || st.Result.Winner != "x" || len(st.Players) != 1 {
		t.Fatalf("rời giữa ván phải thua: %+v %d người", st.Result, len(st.Players))
	}
	if bal, _ := bank.get(2); bal != 900 {
		t.Fatalf("người rời mất cược, ví %d", bal)
	}
	if rooms := h.Rooms(); len(rooms) != 1 || len(rooms[0].Players) != 1 {
		t.Fatalf("sảnh: %+v", rooms)
	}
	a.Room.Handle(a.Player, Message{T: "leave"})
	if h.RoomCount() != 0 {
		t.Fatal("bàn trống phải đóng")
	}
}

func TestSpectatorAndBetChecks(t *testing.T) {
	h, bank, a, ca, _, _ := table(t, fast, 500)
	cc := &fakeConn{}
	sc, err := h.Join(a.Room.Code, chi, cc)
	if err != nil {
		t.Fatal(err)
	}
	if st := cc.last(); st.You != "" || st.Spectators != 1 {
		t.Fatalf("người thứ 3 phải là người xem: you=%q xem=%d", st.You, st.Spectators)
	}
	if err := sc.Room.Handle(sc.Player, Message{T: "move"}); err == nil {
		t.Fatal("người xem không được đánh")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setBet", Bet: ptr(7)}); err == nil {
		t.Fatal("mức cược lạ bị từ chối")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setRule", Rule: RuleFree}); err != nil || ca.last().Rule != RuleFree {
		t.Fatalf("đổi luật: %v", err)
	}
	bank.mu.Lock()
	bank.balances[2] = 100
	bank.mu.Unlock()
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err == nil {
		t.Fatal("Bình không đủ 500 xu thì không bắt đầu được")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setBet", Bet: ptr(0)}); err != nil {
		t.Fatal(err)
	}
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err != nil {
		t.Fatalf("bàn 0 xu: %v", err)
	}
	if st := cc.last(); st.Phase != PhasePlaying {
		t.Fatal("người xem cũng nhận trạng thái ván")
	}
	sc.Room.Handle(sc.Player, Message{T: "leave"})
	if ca.last().Spectators != 0 {
		t.Fatal("người xem rời")
	}
}

func TestHPGate(t *testing.T) {
	opts := fast
	opts.MinHP = 30
	h := NewHub(newBank(), opts)
	if _, err := h.Create(User{ID: 1, Name: "Yếu", HP: 10}, &fakeConn{}, nil, ""); err == nil {
		t.Fatal("thiếu máu không mở bàn được")
	}
	s, err := h.Create(an, &fakeConn{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Room.bet != DefaultBet || s.Room.rule != RuleBlock2 {
		t.Fatalf("mặc định: cược %d luật %s", s.Room.bet, s.Room.rule)
	}
}

// Mất kết nối giữa ván quá thời gian chờ thì xử thua "left"; vào lại kịp thì giữ ghế.
func TestDisconnectGrace(t *testing.T) {
	opts := fast
	opts.Grace = 40 * time.Millisecond
	h, _, a, ca, b, cb := table(t, opts, 0)
	a.Room.Handle(a.Player, Message{T: "start"})
	b.Room.Disconnect(b.Player, cb)
	if _, err := h.Join(a.Room.Code, binh, cb); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if ca.last().Phase != PhasePlaying {
		t.Fatal("vào lại kịp thì ván vẫn chơi")
	}
	b.Room.Disconnect(b.Player, cb)
	deadline := time.Now().Add(2 * time.Second)
	for ca.last().Phase == PhasePlaying && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if r := ca.last().Result; r == nil || r.Reason != "left" || r.Winner != "x" {
		t.Fatalf("quá hạn chờ phải thua: %+v", r)
	}
}

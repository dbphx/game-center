package cangua

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"
)

// ---------- Luật ----------

// moveOf trả nước mặc định (đi đủ tổng) của một ngựa, không có thì lấy nước còn lại.
func moveOf(b Board, seat, horse int, d Dice) (Move, bool) {
	var found *Move
	for _, m := range b.Moves(seat, d) {
		if m.Horse != horse {
			continue
		}
		if !m.Short {
			return m, true
		}
		found = &m
	}
	if found != nil {
		return *found, true
	}
	return Move{}, false
}

func TestLeaveHomeOnlyOn11Or16Or66(t *testing.T) {
	b := NewBoard(2)
	for a := 1; a <= 6; a++ {
		for c := 1; c <= 6; c++ {
			want := (a == 1 || a == 6) && (c == 1 || c == 6)
			if got := len(b.Moves(0, Dice{a, c})) == Horses; got != want {
				t.Fatalf("đổ %d + %d: xuất quân = %v, muốn %v", a, c, got, want)
			}
		}
	}
	// Ra ô xuất phát rồi dừng, không đi thêm.
	m, ok := moveOf(b, 1, 0, Dice{6, 1})
	if !ok || m.To != Side {
		t.Fatalf("màu 1 xuất quân ở ô %d, muốn %d", m.To, Side)
	}
	for _, d := range []Dice{{6, 3}, {1, 4}, {2, 2}, {5, 5}} {
		if _, ok := moveOf(b, 0, 0, d); ok {
			t.Fatalf("đổ %v không được ra quân", d)
		}
	}
	// Ô xuất phát có ngựa mình thì không xuất quân được.
	b[0][2] = 0
	if _, ok := moveOf(b, 0, 3, Dice{6, 6}); ok {
		t.Fatal("ô xuất phát đang có ngựa mình")
	}
}

func TestSumMoveBlockedAndKick(t *testing.T) {
	b := NewBoard(2)
	b[0][0] = 5
	if m, ok := moveOf(b, 0, 0, Dice{5, 4}); !ok || m.To != 14 {
		t.Fatalf("5 + 4 phải đi 9 ô: %+v %v", m, ok)
	}
	b[1][0] = 8 // đối thủ chắn ở ô 8
	if _, ok := moveOf(b, 0, 0, Dice{2, 2}); ok {
		t.Fatal("không được vượt qua ngựa ở ô 8")
	}
	m, ok := moveOf(b, 0, 0, Dice{1, 2})
	if !ok || m.KickSeat != 1 || m.KickHorse != 0 {
		t.Fatalf("đến đúng ô đối thủ phải đá: %+v %v", m, ok)
	}
	b.Apply(m)
	if b[1][0] != Home || b[0][0] != 8 {
		t.Fatalf("sau khi đá: %v", b)
	}
	b[0][1] = 10
	if _, ok := moveOf(b, 0, 0, Dice{1, 1}); !ok {
		// 1 + 1 tới ô 10 có ngựa mình, nhưng vẫn được chọn đi 1 ô.
		t.Fatal("đổ 1 + 1 vẫn đi được 1 ô")
	}
	if _, ok := moveOf(b, 0, 0, Dice{2, 3}); ok {
		t.Fatal("không vượt ngựa mình")
	}
}

func TestSurplusStopsAtDoor(t *testing.T) {
	door := Track - 1
	b := NewBoard(2)
	b[0][0] = door - 2
	if m, ok := moveOf(b, 0, 0, Dice{6, 5}); !ok || m.To != door {
		t.Fatalf("tổng dư phải dừng ở cửa: %+v %v", m, ok)
	}
	// Có ngựa chắn trước cửa thì vẫn không vượt được; đối thủ đứng ở cửa thì bị đá.
	b[1][0] = door - 1
	if _, ok := moveOf(b, 0, 0, Dice{6, 5}); ok {
		t.Fatal("không được vượt ngựa trên đường tới cửa")
	}
	b[1][0] = door
	if m, ok := moveOf(b, 0, 0, Dice{6, 5}); !ok || m.KickSeat != 1 {
		t.Fatalf("đối thủ ở cửa phải bị đá: %+v %v", m, ok)
	}
	// Màu 1: cửa là ô 13.
	b2 := NewBoard(2)
	b2[1][0] = Side - 2
	if m, ok := moveOf(b2, 1, 0, Dice{6, 6}); !ok || m.To != Side-1 {
		t.Fatalf("màu 1 dừng ở cửa ô 13: %+v %v", m, ok)
	}
}

func TestDoubleOneChoosesOneOrTwoCells(t *testing.T) {
	b := NewBoard(2)
	b[0][0] = 5
	moves := b.Moves(0, Dice{1, 1})
	var long, short *Move
	for i := range moves {
		if moves[i].Horse != 0 {
			continue
		}
		if moves[i].Short {
			short = &moves[i]
		} else {
			long = &moves[i]
		}
	}
	if long == nil || short == nil || long.To != 7 || short.To != 6 {
		t.Fatalf("đổ 1 + 1 phải chọn được 1 hoặc 2 ô: %+v", moves)
	}
	// Ngựa ngay phía trước: chỉ còn nước 1 ô (đá) — đây là cách gỡ kẹt.
	b[1][0] = 6
	m, ok := moveOf(b, 0, 0, Dice{1, 1})
	if !ok || !m.Short || m.To != 6 || m.KickSeat != 1 {
		t.Fatalf("chỉ còn nước 1 ô: %+v %v", m, ok)
	}
	// Cách cửa 1 ô: hai cách đều tới cửa thì chỉ tính một nước.
	b2 := NewBoard(2)
	b2[0][0] = Track - 2
	if got := len(b2.Moves(0, Dice{1, 1})); got != Horses {
		t.Fatalf("muốn 3 ngựa ra quân + 1 nước tới cửa, có %d nước", got)
	}
	// Các số khác không có nước ngắn.
	b3 := NewBoard(2)
	b3[0][0] = 5
	for _, m := range b3.Moves(0, Dice{2, 2}) {
		if m.Short {
			t.Fatalf("chỉ đổ 1 + 1 mới có nước ngắn: %+v", m)
		}
	}
}

func TestFinishColumn(t *testing.T) {
	door := Track - 1
	b := NewBoard(2)
	b[0][0] = door
	// Từ cửa lên bậc bằng viên lớn hơn.
	m, ok := moveOf(b, 0, 0, Dice{2, 3})
	if !ok || m.To != FinishBase+3 {
		t.Fatalf("từ cửa đổ 2 + 3 lên bậc 3: %+v", m)
	}
	b.Apply(m)
	// Trong đích: có viên nào bằng bậc kế tiếp thì lên; tổng không tính.
	if _, ok := moveOf(b, 0, 0, Dice{2, 2}); ok {
		t.Fatal("tổng 4 không tính trong chuồng đích")
	}
	if _, ok := moveOf(b, 0, 0, Dice{5, 6}); ok {
		t.Fatal("trong đích chỉ lên đúng bậc kế tiếp (4)")
	}
	if m, ok := moveOf(b, 0, 0, Dice{1, 4}); !ok || m.To != FinishBase+4 {
		t.Fatalf("bậc 3 có viên 4 lên bậc 4: %+v", m)
	}
	// Viên lớn bị ngựa mình chặn thì dùng viên nhỏ.
	b[0][1] = door
	if m, ok := moveOf(b, 0, 1, Dice{5, 2}); !ok || m.To != FinishBase+2 {
		t.Fatalf("viên 5 bị chặn ở bậc 3, phải dùng viên 2: %+v %v", m, ok)
	}
	// Cả hai viên đều bị chặn thì không đi được.
	if _, ok := moveOf(b, 0, 1, Dice{5, 4}); ok {
		t.Fatal("không vượt ngựa mình ở bậc 3")
	}
	// Từ cửa đổ 1 + 1 lên bậc 1, không có nước ngắn thứ hai.
	b2 := NewBoard(2)
	b2[1][0] = Side - 1
	if m, ok := moveOf(b2, 1, 0, Dice{1, 1}); !ok || m.To != FinishBase+1 || m.Short {
		t.Fatalf("màu 1 ở cửa ô 13 lên bậc 1: %+v", m)
	}
	b3 := NewBoard(2)
	b3[0] = [Horses]int{FinishBase + 1, FinishBase + 2, FinishBase + 5, FinishBase + 6}
	if !b3.Won(0) {
		t.Fatal("đủ 4 ngựa trong đích là thắng")
	}
}

func TestThreatenedRangeIsSum(t *testing.T) {
	b := NewBoard(2)
	b[1][0] = 20
	if !b.threatened(0, 32) || !b.threatened(0, 22) {
		t.Fatal("trong tầm 2–12 ô là nguy hiểm")
	}
	if b.threatened(0, 33) {
		t.Fatal("cách 13 ô thì an toàn")
	}
}

func TestBotPrefersKickAndNeverIllegal(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	b := NewBoard(2)
	b[0][0], b[0][1] = 3, 20
	b[1][0] = 6
	for _, lv := range []string{BotNormal, BotHard} {
		m, ok := BestMove(b, 0, Dice{1, 2}, lv, rng)
		if !ok || m.Horse != 0 || m.KickSeat != 1 {
			t.Fatalf("%s phải đá ngựa: %+v", lv, m)
		}
	}
	// Ngẫu nhiên nhiều bàn: nước của máy luôn hợp lệ.
	for i := 0; i < 2000; i++ {
		b := NewBoard(4)
		for s := range 4 {
			for h := range Horses {
				switch rng.IntN(3) {
				case 0:
					b[s][h] = Home
				case 1:
					b[s][h] = rng.IntN(Track)
				}
			}
		}
		d := Dice{rng.IntN(6) + 1, rng.IntN(6) + 1}
		for _, lv := range BotLevels {
			if m, ok := BestMove(b, 1, d, lv, rng); ok {
				if !slices.Contains(b.Moves(1, d), m) {
					t.Fatalf("%s đi nước sai %+v", lv, m)
				}
			}
		}
	}
}

// Máy tự đánh nhiều ván: ván nào cũng phải kết thúc (không có thế kẹt vĩnh viễn).
func TestBotGamesNeverDeadlock(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for g := 0; g < 300; g++ {
		seats := 2 + g%3
		b := NewBoard(seats)
		turn, doubles, rolls := 0, 0, 0
		for {
			rolls++
			if rolls > 20000 {
				t.Fatalf("ván %d (%d ghế) không kết thúc: %v", g, seats, b)
			}
			d := Dice{rng.IntN(6) + 1, rng.IntN(6) + 1}
			again := false
			if d.Double() {
				doubles++
				again = doubles < maxDoubles
			}
			if !d.Double() || again {
				if m, ok := BestMove(b, turn, d, BotLevels[g%len(BotLevels)], rng); ok {
					b.Apply(m)
					if b.Won(turn) {
						break
					}
				}
			}
			if !again {
				turn, doubles = (turn+1)%seats, 0
			}
		}
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
	slow = Options{BotDelay: time.Hour, TurnTimeout: time.Hour, Grace: time.Hour}
	fast = Options{BotDelay: time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour}
)

func ptr(v int) *int { return &v }

// dice trả xúc xắc theo danh sách cho trước, hết danh sách thì lặp số cuối.
func dice(r *Room, seq ...int) {
	i := 0
	r.dicer = func() int {
		d := seq[min(i, len(seq)-1)]
		i++
		return d
	}
}

// waitFor chờ (tối đa 5 s) tới khi cond đúng — các bước chờ hoạt ảnh chạy bằng hẹn giờ.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("quá hạn chờ")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func do(t *testing.T, s *Session, m Message) {
	t.Helper()
	if err := s.Room.Handle(s.Player, m); err != nil {
		t.Fatalf("%s: %v", m.T, err)
	}
}

func table(t *testing.T, opts Options, bet int) (*fakeBank, *Session, *fakeConn, *Session, *fakeConn) {
	t.Helper()
	bank := newBank()
	h := NewHub(bank, opts)
	ca, cb := &fakeConn{}, &fakeConn{}
	sa, err := h.Create(an, ca, ptr(bet))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := h.Join(sa.Room.Code, binh, cb)
	if err != nil {
		t.Fatal(err)
	}
	return bank, sa, ca, sb, cb
}

func TestTurnFlowDoublesAndSkip(t *testing.T) {
	_, a, ca, b, _ := table(t, slow, 10)
	r := a.Room
	do(t, a, Message{T: "start"})
	st := ca.last()
	if st.Turn != a.Player.ID || st.Players[0].Color != "red" || st.Players[1].Color != "blue" {
		t.Fatalf("chủ bàn (đỏ) đi trước: %+v", st)
	}
	if err := b.Room.Handle(b.Player, Message{T: "roll"}); err == nil {
		t.Fatal("chưa tới lượt Bình")
	}
	// Ra 2 + 3, chưa có ngựa nào ngoài: không đi được → sang lượt Bình.
	dice(r, 2, 3)
	do(t, a, Message{T: "roll"})
	if st = ca.last(); st.Turn != a.Player.ID || *st.Dice != (Dice{2, 3}) || len(st.Movable) != 0 {
		t.Fatalf("không đi được: giữ số trên màn hình trước khi chuyển lượt: %+v", st)
	}
	if err := a.Room.Handle(a.Player, Message{T: "move", Horse: 0}); err == nil {
		t.Fatal("không có nước thì không đi được")
	}
	waitFor(t, func() bool { return ca.last().Turn == b.Player.ID })
	if st = ca.last(); *st.Dice != (Dice{2, 3}) {
		t.Fatalf("không đi được phải sang lượt: %+v", st)
	}
	// Bình đổ đôi 6: xuất quân rồi được đổ tiếp.
	dice(r, 6)
	do(t, b, Message{T: "roll"})
	if err := b.Room.Handle(b.Player, Message{T: "move", Horse: 0}); err != nil {
		t.Fatal(err)
	}
	if st = ca.last(); st.Turn != b.Player.ID || st.Rolled || st.Horses["blue"][0] != Side {
		t.Fatalf("đổ đôi được đổ tiếp: %+v", st)
	}
	if err := b.Room.Handle(b.Player, Message{T: "move", Horse: 1}); err == nil {
		t.Fatal("chưa đổ thì không đi được")
	}
	// Đôi lần 2 rồi đôi lần 3: mất lượt.
	do(t, b, Message{T: "roll"})
	do(t, b, Message{T: "move", Horse: 0}) // ô xuất phát đang có ngựa 0 nên chỉ đi ngựa 0 được
	do(t, b, Message{T: "roll"})
	if err := b.Room.Handle(b.Player, Message{T: "move", Horse: 0}); err == nil {
		t.Fatal("đôi lần thứ 3 không được đi")
	}
	waitFor(t, func() bool { return ca.last().Turn == a.Player.ID })
	if st = ca.last(); st.Turn != a.Player.ID || st.Doubles != 0 {
		t.Fatalf("3 lần đôi liên tiếp mất lượt: %+v", st)
	}
	r.mu.Lock()
	moved := r.board[1]
	r.mu.Unlock()
	if moved[1] != Home || moved[0] != Side+12 {
		t.Fatalf("đôi lần thứ 3 không được đi: %v", moved)
	}
}

// Đổ đôi nhưng không có nước nào thì vẫn được đổ tiếp; số 6 lẻ không còn được đổ thêm.
func TestDoubleWithoutMoveRollsAgainAndSixDoesNot(t *testing.T) {
	_, a, ca, b, _ := table(t, slow, 10)
	r := a.Room
	do(t, a, Message{T: "start"})
	r.mu.Lock()
	// Ba ngựa trong chuồng đích, một ngựa ở cửa: đôi 5 không ngựa nào đi được.
	r.board[0] = [Horses]int{FinishBase + 1, FinishBase + 2, FinishBase + 3, Track - 1}
	r.mu.Unlock()
	dice(r, 5)
	do(t, a, Message{T: "roll"})
	if st := ca.last(); st.Turn != a.Player.ID || st.Rolled || st.Doubles != 1 {
		t.Fatalf("đôi không có nước vẫn được đổ tiếp: %+v", st)
	}
	// Số 6 lẻ: đi xong là sang lượt.
	r.mu.Lock()
	r.board[0] = [Horses]int{5, Home, Home, Home}
	r.mu.Unlock()
	dice(r, 6, 2)
	do(t, a, Message{T: "roll"})
	do(t, a, Message{T: "move", Horse: 0})
	if st := ca.last(); st.Turn != b.Player.ID {
		t.Fatalf("6 + 2 đi xong phải sang lượt: %+v", st)
	}
}

// Đổ 1 + 1: người chơi chọn đi 1 ô hoặc 2 ô; danh sách nước đi có đủ ô tới để xem trước.
func TestDoubleOneShortMoveInRoom(t *testing.T) {
	_, a, ca, _, _ := table(t, slow, 10)
	r := a.Room
	do(t, a, Message{T: "start"})
	r.mu.Lock()
	r.board[0] = [Horses]int{5, 20, Home, Home}
	r.mu.Unlock()
	dice(r, 1)
	do(t, a, Message{T: "roll"})
	st := ca.last()
	want := MoveView{Horse: 0, To: 6, Short: true}
	if !slices.Contains(st.Moves, want) || !slices.Contains(st.Moves, MoveView{Horse: 0, To: 7}) {
		t.Fatalf("state phải có cả nước 1 ô và 2 ô: %+v", st.Moves)
	}
	do(t, a, Message{T: "move", Horse: 0, Short: true})
	if st = ca.last(); st.Horses["red"][0] != 6 || st.Turn != a.Player.ID || st.Rolled {
		t.Fatalf("đi 1 ô rồi được đổ tiếp vì ra đôi: %+v", st)
	}
	// Mặc định (không chọn) đi đủ 2 ô.
	do(t, a, Message{T: "roll"})
	do(t, a, Message{T: "move", Horse: 0})
	if st = ca.last(); st.Horses["red"][0] != 8 {
		t.Fatalf("mặc định đi đủ tổng: %+v", st.Horses)
	}
	// Không phải 1 + 1 thì không chọn đi ngắn được.
	dice(r, 2, 3)
	do(t, a, Message{T: "roll"})
	if err := a.Room.Handle(a.Player, Message{T: "move", Horse: 0, Short: true}); err == nil {
		t.Fatal("chỉ đổ 1 + 1 mới được đi ngắn")
	}
}

func TestWinPaysEachLoser(t *testing.T) {
	bank, a, ca, b, _ := table(t, slow, 100)
	c, err := a.Room.hub.Join(a.Room.Code, chi, &fakeConn{})
	if err != nil {
		t.Fatal(err)
	}
	do(t, a, Message{T: "start"})
	r := a.Room
	r.mu.Lock()
	r.board[0] = [Horses]int{FinishBase + 4, FinishBase + 5, FinishBase + 6, Track - 1}
	r.mu.Unlock()
	dice(r, 3, 1)
	do(t, a, Message{T: "roll"})
	if st := ca.last(); !slices.Contains(st.Movable, 3) {
		t.Fatalf("ngựa 3 phải đi được: %v", st.Movable)
	}
	do(t, a, Message{T: "move", Horse: 3})
	st := ca.last()
	if st.Phase != PhaseOver || st.Winner == nil || *st.Winner != a.Player.ID {
		t.Fatalf("An phải thắng: %+v", st)
	}
	balA, xpA := bank.get(1)
	balB, xpB := bank.get(2)
	balC, _ := bank.get(3)
	if balA != 1200 || balB != 900 || balC != 900 || xpA != XPWin || xpB != XPOther {
		t.Fatalf("tiền / XP: %d %d %d · %d %d", balA, balB, balC, xpA, xpB)
	}
	// Chơi lại: cả 3 bấm thì ván mới, người thắng đi trước.
	for _, s := range []*Session{a, b, c} {
		do(t, s, Message{T: "rematch"})
	}
	if st = ca.last(); st.Phase != PhasePlaying || st.Turn != a.Player.ID {
		t.Fatalf("ván mới: %+v", st)
	}
}

func TestBotGameFinishesAndTimeoutAutoplay(t *testing.T) {
	bank := newBank()
	h := NewHub(bank, Options{BotDelay: time.Millisecond, TurnTimeout: 5 * time.Millisecond, Grace: time.Hour})
	ca := &fakeConn{}
	a, err := h.Create(an, ca, ptr(10))
	if err != nil {
		t.Fatal(err)
	}
	do(t, a, Message{T: "addBot", Level: BotHard})
	do(t, a, Message{T: "addBot", Level: BotEasy})
	do(t, a, Message{T: "start"})
	// Người không bấm gì: hết giờ máy đổ / đi thay; ván phải tự chạy tới hết.
	deadline := time.Now().Add(20 * time.Second)
	for ca.last().Phase != PhaseOver {
		if time.Now().After(deadline) {
			t.Fatal("ván không kết thúc")
		}
		time.Sleep(5 * time.Millisecond)
	}
	st := ca.last()
	bal, xp := bank.get(1)
	if *st.Winner == a.Player.ID {
		if bal != 1020 || xp != XPWin {
			t.Fatalf("thắng 2 máy: %d xu %d XP", bal, xp)
		}
	} else if bal != 990 || xp != XPOther {
		t.Fatalf("thua: %d xu %d XP", bal, xp)
	}
}

func TestLeaveMidGameAutoplaysAndSpectator(t *testing.T) {
	bank, a, ca, b, _ := table(t, fast, 50)
	// Người thứ 3 vào khi đang chơi thì xem.
	do(t, a, Message{T: "start"})
	cs := &fakeConn{}
	s, err := a.Room.hub.Join(a.Room.Code, chi, cs)
	if err != nil {
		t.Fatal(err)
	}
	if st := cs.last(); st.You != "" || st.Spectators != 1 {
		t.Fatalf("người vào sau là người xem: %+v", st)
	}
	if err := s.Room.Handle(s.Player, Message{T: "roll"}); err == nil {
		t.Fatal("người xem không đổ được")
	}
	do(t, s, Message{T: "chat", Text: "cố lên"})
	// Bình rời giữa ván: máy đánh thay, An vẫn chơi.
	do(t, b, Message{T: "leave"})
	if st := ca.last(); !st.Players[1].Auto || st.Phase != PhasePlaying {
		t.Fatalf("Bình rời thì máy đánh thay: %+v", st.Players)
	}
	// An cũng rời và người xem đi nốt: không còn ai → máy chơi nốt ván và chốt tiền.
	do(t, s, Message{T: "leave"})
	do(t, a, Message{T: "leave"})
	balA, _ := bank.get(1)
	balB, _ := bank.get(2)
	if balA+balB != 2000 || balA == 1000 {
		t.Fatalf("ván phải được chốt: %d %d", balA, balB)
	}
	if a.Room.hub.RoomCount() != 0 {
		t.Fatal("bàn trống phải đóng")
	}
}

func TestBetChecks(t *testing.T) {
	_, a, _, b, _ := table(t, slow, 10)
	if err := b.Room.Handle(b.Player, Message{T: "setBet", Bet: ptr(100)}); err == nil {
		t.Fatal("chỉ chủ bàn đổi cược")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setBet", Bet: ptr(7)}); err == nil {
		t.Fatal("mức cược lạ")
	}
	do(t, a, Message{T: "setBet", Bet: ptr(1000)})
	a.Room.hub.bank.(*fakeBank).balances[2] = 500
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err == nil {
		t.Fatal("Bình không đủ xu")
	}
	h := NewHub(newBank(), slow)
	solo, _ := h.Create(an, &fakeConn{}, ptr(0))
	if err := solo.Room.Handle(solo.Player, Message{T: "start"}); err == nil {
		t.Fatal("cần ít nhất 2 người")
	}
	if rooms := h.Rooms(); len(rooms) != 1 || rooms[0].Players[0].Color != "red" || rooms[0].Max != MaxPlayers {
		t.Fatalf("sảnh: %+v", rooms)
	}
}

// Mất kết nối quá Grace giữa ván: máy đánh thay; vào lại thì lấy lại quyền.
func TestDisconnectAutoplayAndResume(t *testing.T) {
	bank := newBank()
	h := NewHub(bank, Options{BotDelay: time.Hour, TurnTimeout: time.Hour, Grace: 10 * time.Millisecond})
	ca, cb := &fakeConn{}, &fakeConn{}
	a, _ := h.Create(an, ca, ptr(0))
	b, _ := h.Join(a.Room.Code, binh, cb)
	do(t, a, Message{T: "start"})
	b.Room.Disconnect(b.Player, cb)
	deadline := time.Now().Add(5 * time.Second)
	for !ca.last().Players[1].Auto {
		if time.Now().After(deadline) {
			t.Fatal("quá hạn chờ mà máy chưa đánh thay")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cb2 := &fakeConn{}
	if _, err := h.Join(a.Room.Code, binh, cb2); err != nil {
		t.Fatal(err)
	}
	if st := cb2.last(); st.Players[1].Auto || !st.Players[1].Online || st.You != b.Player.ID {
		t.Fatalf("vào lại phải lấy lại ghế: %+v", st.Players[1])
	}
}

// Nhịp hoạt ảnh: máy chờ BotDelay mới đổ, chờ RollShow + BotMoveDelay mới đi, lượt sau chờ ngựa chạy xong;
// đồng hồ của người cộng thêm phần hoạt ảnh còn lại.
func TestAnimationPacing(t *testing.T) {
	opts := Options{BotDelay: 40 * time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour,
		RollShow: 60 * time.Millisecond, BotMoveDelay: 10 * time.Millisecond, StepAnim: 20 * time.Millisecond,
		MaxMoveAnim: 70 * time.Millisecond}
	h := NewHub(newBank(), opts)
	ca := &fakeConn{}
	a, _ := h.Create(an, ca, ptr(0))
	do(t, a, Message{T: "addBot", Level: BotNormal})
	r := a.Room
	r.mu.Lock()
	r.lastWinner = "" // An (chủ bàn) đi trước
	r.mu.Unlock()
	do(t, a, Message{T: "start"})
	dice(r, 6, 6, 2, 4, 2, 3) // An: đôi 6 (ra quân), 2 + 4 (đi 6 ô); máy: 2 + 3 (không đi được)
	do(t, a, Message{T: "roll"})
	if st := ca.last(); st.TurnMs < int((time.Hour + 50*time.Millisecond).Milliseconds()) {
		t.Fatalf("đồng hồ phải cộng thời gian số hiện: %d ms", st.TurnMs)
	}
	do(t, a, Message{T: "move", Horse: 0})
	do(t, a, Message{T: "roll"})
	do(t, a, Message{T: "move", Horse: 0}) // 6 ô → máy chờ 70 ms hoạt ảnh + 40 ms rồi mới đổ
	moved := time.Now()
	waitFor(t, func() bool { st := ca.last(); return st.Turn != a.Player.ID && st.Rolled })
	if d := time.Since(moved); d < 90*time.Millisecond {
		t.Fatalf("máy đổ quá sớm sau nước đi: %v", d)
	}
	// Máy ra 2 + 3 không có ngựa ngoài: giữ số RollShow rồi mới trả lượt cho An.
	rolledAt := time.Now()
	waitFor(t, func() bool { return ca.last().Turn == a.Player.ID })
	if d := time.Since(rolledAt); d < 45*time.Millisecond {
		t.Fatalf("chuyển lượt trước khi số hiện xong: %v", d)
	}
}

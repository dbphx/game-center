package arena

import (
	"context"
	"math"
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

// lastOf là tin cuối cùng có t = kind (dạng map).
func (f *fakeConn) lastOf(kind string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.msgs) - 1; i >= 0; i-- {
		if m, ok := f.msgs[i].(map[string]any); ok && m["t"] == kind {
			return m
		}
	}
	return nil
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

type fakeGear map[int64]Gear

func (f fakeGear) Gear(_ context.Context, id int64) (Gear, error) { return f[id], nil }

var (
	an   = User{ID: 1, Name: "An", HP: 100}
	binh = User{ID: 2, Name: "Bình", HP: 100}
	chi  = User{ID: 3, Name: "Chi", HP: 100}
	fast = Options{BotDelay: 5 * time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour}
)

func ptr(v int) *int { return &v }

func do(t *testing.T, s *Session, m Message) {
	t.Helper()
	if err := s.Room.Handle(s.Player, m); err != nil {
		t.Fatalf("%s: %v", m.T, err)
	}
}

// table tạo bàn (An chủ bàn) với mức cược bet và những người vào thêm.
func table(t *testing.T, opts Options, bet int, gear GearSource, others ...User) (*fakeBank, []*Session, []*fakeConn) {
	t.Helper()
	bank := newBank()
	h := NewHub(bank, opts, gear)
	ca := &fakeConn{}
	sa, err := h.Create(an, ca, ptr(bet), "meadow")
	if err != nil {
		t.Fatal(err)
	}
	ss, cs := []*Session{sa}, []*fakeConn{ca}
	for _, u := range others {
		c := &fakeConn{}
		s, err := h.Join(sa.Room.Code, u, c)
		if err != nil {
			t.Fatal(err)
		}
		ss, cs = append(ss, s), append(cs, c)
	}
	return bank, ss, cs
}

// flatten đặt sân phẳng và chỗ đứng cố định cho các ghế (để phát bắn tính trước được).
func flatten(r *Room, xs ...float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.ground {
		r.ground[i] = 700
	}
	for i, p := range r.players {
		p.f.X, p.f.Y = xs[i], 700
	}
	r.wind = 0
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 400 {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("chờ quá lâu: %s", what)
}

func turnOf(ss []*Session, id string) *Session {
	for _, s := range ss {
		if s.Player.ID == id {
			return s
		}
	}
	return nil
}

func TestLobbySettingsAndStart(t *testing.T) {
	_, ss, cs := table(t, fast, 100, fakeGear{1: {Gun: 2, Hat: 4}}, binh)
	a, b := ss[0], ss[1]
	st := cs[0].last()
	if st.Phase != PhaseLobby || st.Map.ID != "meadow" || st.MapChoice != "meadow" || len(st.Ground) != Width || len(st.Maps) != 5 ||
		len(st.Weapons) != 4 || st.Players[0].Power != 1000+240+320 || st.Players[0].MaxHP != 260 {
		t.Fatalf("phòng chờ: %+v", st.Players)
	}
	if err := b.Room.Handle(b.Player, Message{T: "settings", Map: "desert"}); err == nil {
		t.Fatal("chỉ chủ bàn đổi map")
	}
	if err := a.Room.Handle(a.Player, Message{T: "settings", Map: "mars"}); err == nil {
		t.Fatal("map lạ")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setBet", Bet: ptr(7)}); err == nil {
		t.Fatal("mức cược lạ")
	}
	do(t, a, Message{T: "settings", Map: "beach", Bet: ptr(50)})
	if st = cs[1].last(); st.Map.ID != "beach" || st.Bet != 50 {
		t.Fatalf("cài đặt: %+v %d", st.Map, st.Bet)
	}
	do(t, a, Message{T: "settings", Map: MapRandom})
	if st = cs[1].last(); st.MapChoice != MapRandom {
		t.Fatal("map ngẫu nhiên")
	}
	if err := b.Room.Handle(b.Player, Message{T: "start"}); err == nil {
		t.Fatal("chỉ chủ bàn được bắt đầu")
	}
	do(t, a, Message{T: "start"})
	st = cs[0].last()
	if st.Phase != PhasePlaying || st.Turn == "" || st.Round != 1 || st.MoveLeft != MoveFund || st.TurnMs <= 0 || st.GameBet != 50 {
		t.Fatalf("trận mới: %+v", st)
	}
	for _, p := range st.Players {
		if !p.Alive || p.HP != p.MaxHP || p.Items[WeaponTriple] != 2 || p.Items[WeaponHeavy] != 1 || p.Items[WeaponTele] != 1 ||
			p.Y != float64(st.Ground[int(p.X)]) {
			t.Fatalf("người chơi đầu trận: %+v", p)
		}
	}
	if err := a.Room.Handle(a.Player, Message{T: "addBot"}); err == nil {
		t.Fatal("không thêm máy giữa trận")
	}
}

func TestFireMoveAimTurns(t *testing.T) {
	bank, ss, cs := table(t, fast, 100, nil, binh)
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	flatten(r, 200, 900)
	st := cs[0].last()
	cur := turnOf(ss, st.Turn)
	other := ss[0]
	if cur == other {
		other = ss[1]
	}
	if err := other.Room.Handle(other.Player, Message{T: "fire", Angle: 45, Power: 50}); err == nil {
		t.Fatal("chưa tới lượt")
	}
	// Đi: tối đa 120 đơn vị mỗi lượt.
	start := cur.Player.f.X
	do(t, cur, Message{T: "move", X: start + 500})
	if st = cs[0].last(); st.MoveLeft != 0 {
		t.Fatalf("hết quỹ di chuyển: %d", st.MoveLeft)
	}
	for _, p := range st.Players {
		if p.ID == cur.Player.ID && p.X != start+MoveFund {
			t.Fatalf("đi 120: %v → %v", start, p.X)
		}
	}
	// Ngắm: người khác nhận tin aim.
	do(t, cur, Message{T: "aim", Angle: 120, Power: 33})
	oc := cs[0]
	if other == ss[1] {
		oc = cs[1]
	}
	if m := oc.lastOf("aim"); m == nil || m["angle"] != 120.0 || m["power"] != 33.0 {
		t.Fatalf("tin ngắm: %v", m)
	}
	if err := cur.Room.Handle(cur.Player, Message{T: "fire", Weapon: "laser"}); err == nil {
		t.Fatal("vũ khí lạ")
	}
	// Bắn thẳng vào đối thủ bằng bom tạ.
	r.mu.Lock()
	me, foe := cur.Player.f, other.Player.f
	angle := 45.0
	if foe.X < me.X {
		angle = 135
	}
	var power float64
	for p := 20.0; p <= 100; p += 0.5 {
		f := Simulate(r.ground, me.X, me.Y, angle, p, 0, me.ID, bodiesOf(r.fighters()))
		if f.Body == foe.ID {
			power = p
			break
		}
	}
	r.mu.Unlock()
	if power == 0 {
		t.Fatal("không tìm được lực bắn trúng")
	}
	do(t, cur, Message{T: "fire", Angle: angle, Power: power, Weapon: WeaponHeavy})
	shot := oc.lastOf("shot")
	if shot == nil || shot["by"] != cur.Player.ID || shot["weapon"] != WeaponHeavy || len(shot["paths"].([][][2]int)) != 1 {
		t.Fatalf("tin shot: %v", shot)
	}
	hits := shot["hits"].([]Hit)
	if len(hits) == 0 || hits[0].ID != other.Player.ID || hits[0].Dmg != 55 {
		t.Fatalf("trúng bom tạ 55: %+v", hits)
	}
	if err := cur.Room.Handle(cur.Player, Message{T: "fire", Weapon: WeaponHeavy}); err == nil {
		t.Fatal("bắn khi đạn đang bay / hết bom tạ")
	}
	// Đạn bay xong: sang lượt người kia, gió mới, quỹ đi mới.
	waitFor(t, "sang lượt", func() bool { return cs[0].last().Turn == other.Player.ID })
	st = cs[0].last()
	if st.MoveLeft != MoveFund || st.Phase != PhasePlaying {
		t.Fatalf("lượt mới: %+v", st)
	}
	for _, p := range st.Players {
		if p.ID == cur.Player.ID && p.Items[WeaponHeavy] != 0 {
			t.Fatal("bom tạ đã dùng")
		}
		if p.ID == other.Player.ID && p.HP != p.MaxHP-55 {
			t.Fatalf("máu sau khi trúng: %d", p.HP)
		}
	}
	// Đánh tới khi hạ được đối thủ: dùng hàm ngắm của máy khó cho cả hai.
	for range 200 {
		st = cs[0].last()
		if st.Phase == PhaseOver {
			break
		}
		s := turnOf(ss, st.Turn)
		if s == nil {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		r.mu.Lock()
		aim := BotAim(r.ground, r.fighters(), s.Player.f, map[string]int{}, BotHard, r.wind, r.rng)
		r.mu.Unlock()
		s.Room.Handle(s.Player, Message{T: "fire", Angle: aim.Angle, Power: aim.Power})
		waitFor(t, "hết lượt", func() bool { st := cs[0].last(); return st.Turn != s.Player.ID || st.Phase == PhaseOver })
	}
	st = cs[0].last()
	if st.Phase != PhaseOver || st.Result == nil || st.Result.Reason != "last" || st.Result.Winner == "" {
		t.Fatalf("hết trận: %+v", st.Result)
	}
	win := 1
	if st.Result.Winner == ss[1].Player.ID {
		win = 2
	}
	if bal, xp := bank.get(int64(win)); bal != 1100 || xp != XPWin {
		t.Fatalf("người thắng: ví %d XP %d", bal, xp)
	}
	if bal, xp := bank.get(int64(3 - win)); bal != 900 || xp != XPLose {
		t.Fatalf("người thua: ví %d XP %d", bal, xp)
	}
	// Chơi lại: cả hai bấm.
	do(t, ss[0], Message{T: "rematch"})
	if cs[0].last().Phase != PhaseOver {
		t.Fatal("chờ cả hai bấm chơi lại")
	}
	do(t, ss[1], Message{T: "ready"})
	if st = cs[0].last(); st.Phase != PhasePlaying || st.Players[0].HP != st.Players[0].MaxHP {
		t.Fatalf("trận mới: %+v", st.Phase)
	}
}

func TestPlayVsBots(t *testing.T) {
	bank, ss, cs := table(t, fast, 100, nil)
	a := ss[0]
	do(t, a, Message{T: "addBot", Level: BotHard})
	do(t, a, Message{T: "add_bot", Level: BotEasy})
	st := cs[0].last()
	if len(st.Players) != 3 || st.Players[1].Power != 1000+6*300 || st.Players[1].MaxHP != 290 || st.Players[2].Name != "Máy dễ 2" {
		t.Fatalf("máy: %+v", st.Players)
	}
	do(t, a, Message{T: "start"})
	// Người chơi đứng yên bắn trượt lên trời: máy phải tự đánh tới hết trận.
	for range 2000 {
		st = cs[0].last()
		if st.Phase == PhaseOver {
			break
		}
		if st.Turn == a.Player.ID {
			a.Room.Handle(a.Player, Message{T: "fire", Angle: 10, Power: 100})
		}
		time.Sleep(3 * time.Millisecond)
	}
	if st.Phase != PhaseOver || st.Result == nil {
		t.Fatalf("trận với máy phải kết thúc: %+v", st.Phase)
	}
	bal, xp := bank.get(1)
	switch {
	case st.Result.Winner == a.Player.ID && bal != 1200:
		t.Fatalf("thắng 2 máy: %d", bal)
	case st.Result.Winner != a.Player.ID && st.Result.Winner != "" && (bal != 900 || xp != XPLose):
		t.Fatalf("thua máy: %d %d", bal, xp)
	}
}

func TestTimeoutAndRoundsDraw(t *testing.T) {
	opts := fast
	opts.TurnTimeout = 20 * time.Millisecond
	opts.MaxRounds = 2
	bank, ss, cs := table(t, opts, 100, nil, binh)
	do(t, ss[0], Message{T: "start"})
	first := cs[0].last().Turn
	waitFor(t, "hết giờ mất lượt", func() bool { st := cs[0].last(); return st.Turn != first && st.Turn != "" })
	waitFor(t, "hết số vòng", func() bool { return cs[0].last().Phase == PhaseOver })
	st := cs[0].last()
	if st.Result.Reason != "draw" || st.Result.Winner != "" || st.Round != 2 {
		t.Fatalf("hết vòng, máu bằng nhau thì hoà: %+v round %d", st.Result, st.Round)
	}
	if bal, xp := bank.get(1); bal != 1000 || xp != XPDraw {
		t.Fatalf("hoà: ví %d XP %d", bal, xp)
	}
	// Hết vòng, ai nhiều máu (theo %) hơn thắng.
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	r.mu.Lock()
	r.players[0].f.HP = 50
	r.mu.Unlock()
	waitFor(t, "hết số vòng", func() bool { return cs[0].last().Phase == PhaseOver })
	if st = cs[0].last(); st.Result.Reason != "rounds" || st.Result.Winner != ss[1].Player.ID {
		t.Fatalf("thắng theo máu: %+v", st.Result)
	}
}

func TestLeaveMidGameForfeits(t *testing.T) {
	bank, ss, cs := table(t, fast, 100, nil, binh, chi)
	do(t, ss[0], Message{T: "start"})
	do(t, ss[2], Message{T: "leave"})
	if bal, _ := bank.get(3); bal != 900 {
		t.Fatalf("bỏ trận mất cược ngay: %d", bal)
	}
	st := cs[0].last()
	if st.Phase != PhasePlaying || len(st.Players) != 2 || st.Turn == ss[2].Player.ID || st.Turn == "" {
		t.Fatalf("còn 2 người đánh tiếp: %+v", st)
	}
	do(t, ss[0], Message{T: "leave"})
	st = cs[1].last()
	if st.Phase != PhaseOver || st.Result.Winner != ss[1].Player.ID || st.Result.Reason != "left" {
		t.Fatalf("còn một người thì thắng: %+v", st.Result)
	}
	if bal, _ := bank.get(2); bal != 1200 {
		t.Fatalf("người thắng ăn cược 2 người bỏ trận: %d", bal)
	}
	if bal, _ := bank.get(1); bal != 900 {
		t.Fatalf("An bỏ trận: %d", bal)
	}
}

func TestSpectatorHPGateAndGrace(t *testing.T) {
	opts := fast
	opts.MinHP = 30
	opts.Grace = 20 * time.Millisecond
	bank := newBank()
	h := NewHub(bank, opts, nil)
	if _, err := h.Create(User{ID: 1, Name: "Yếu", HP: 10}, &fakeConn{}, nil, ""); err == nil {
		t.Fatal("thiếu máu không mở được bàn")
	}
	ca, cb := &fakeConn{}, &fakeConn{}
	sa, err := h.Create(an, ca, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if st := ca.last(); st.MapChoice != MapRandom || st.Bet != DefaultBet {
		t.Fatalf("mặc định: %+v %d", st.MapChoice, st.Bet)
	}
	sb, _ := h.Join(sa.Room.Code, binh, cb)
	do(t, sa, Message{T: "start"})
	// Vào khi đang đánh thì xem.
	cc := &fakeConn{}
	sc, err := h.Join(sa.Room.Code, chi, cc)
	if err != nil || cc.last().You != "" || cc.last().Spectators != 1 {
		t.Fatalf("người xem: %v", err)
	}
	if err := sc.Room.Handle(sc.Player, Message{T: "fire"}); err == nil {
		t.Fatal("người xem không được bắn")
	}
	if rooms := h.Rooms(); len(rooms) != 1 || rooms[0].Max != MaxPlayers || rooms[0].Map == "" || len(rooms[0].Players) != 2 {
		t.Fatalf("danh sách bàn: %+v", rooms)
	}
	// Bình mất kết nối quá Grace: thua.
	sb.Room.Disconnect(sb.Player, cb)
	waitFor(t, "hết Grace", func() bool { return ca.last().Phase == PhaseOver })
	if st := ca.last(); st.Result.Winner != sa.Player.ID {
		t.Fatalf("người mất kết nối thua: %+v", st.Result)
	}
	if s := h.Stats(); s.Rooms != 1 {
		t.Fatalf("stats: %+v", s)
	}
}

func TestTeleMovesShooter(t *testing.T) {
	_, ss, cs := table(t, fast, 0, nil, binh)
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	flatten(r, 300, 1300)
	cur := turnOf(ss, cs[0].last().Turn)
	from := cur.Player.f.X
	angle := 60.0
	if from > Width/2 {
		angle = 120
	}
	do(t, cur, Message{T: "fire", Angle: angle, Power: 40, Weapon: WeaponTele})
	shot := cs[0].lastOf("shot")
	tp, _ := shot["tele"].(*Point)
	if tp == nil || math.Abs(float64(tp.X)-from) < 50 {
		t.Fatalf("dịch chuyển: %v", shot["tele"])
	}
	for _, p := range cs[0].last().Players {
		if p.ID == cur.Player.ID && (p.X != float64(tp.X) || p.Items[WeaponTele] != 0) {
			t.Fatalf("người bắn ở chỗ mới: %+v", p)
		}
	}
}

package race

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- Đồ giả ----------

type fakeConn struct {
	mu   sync.Mutex
	msgs []any
}

func (f *fakeConn) Send(msg any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
}

// count đếm tin có trường t bằng kind.
func (f *fakeConn) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		b, _ := json.Marshal(m)
		var v struct{ T string }
		json.Unmarshal(b, &v)
		if v.T == kind {
			n++
		}
	}
	return n
}

type fakeBank struct {
	mu       sync.Mutex
	balances map[int64]int64
	xp       map[int64]int64
	notes    []string
}

func newBank() *fakeBank {
	return &fakeBank{balances: map[int64]int64{1: 10_000, 2: 10_000, 3: 10_000}, xp: map[int64]int64{}}
}

func (b *fakeBank) Balance(_ context.Context, id int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.balances[id], nil
}

func (b *fakeBank) Settle(_ context.Context, _, note string, d map[int64]int64) (map[int64]int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notes = append(b.notes, note)
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

// fakeLoadouts: mọi người dùng xe khởi đầu cấp 0 (trần 30 m/s, nitro 39 m/s).
type fakeLoadouts struct{}

func (fakeLoadouts) Loadout(context.Context, int64) (CarView, error) {
	return carView(Cars[0], ownedCar{cosmetics: Cosmetics{Paint: DefaultPaint}}, true), nil
}

var (
	an   = User{ID: 1, Name: "An", HP: 100}
	binh = User{ID: 2, Name: "Bình", HP: 100}
	chi  = User{ID: 3, Name: "Chi", HP: 100}
)

func ptr(v int) *int       { return &v }
func ptr64(v int64) *int64 { return &v }

// clock là đồng hồ giả của phòng.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	t    *testing.T
	bank *fakeBank
	hub  *Hub
	clk  *clock
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ts, err := LoadTracks(os.DirFS("testdata/tracks"))
	if err != nil || ts["square"] == nil {
		t.Fatalf("đọc đường thử: %v", err)
	}
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	opts := Options{Countdown: 3 * time.Second, FinishWindow: 90 * time.Second, MaxRace: 10 * time.Minute,
		Grace: time.Hour, Now: clk.now}
	bank := newBank()
	return &env{t: t, bank: bank, hub: NewHub(bank, fakeLoadouts{}, ts, opts), clk: clk}
}

func (e *env) create(u User, bet int) (*Session, *fakeConn) {
	e.t.Helper()
	c := &fakeConn{}
	s, err := e.hub.Create(u, c, ptr(bet), "square", "")
	if err != nil {
		e.t.Fatal(err)
	}
	return s, c
}

func (e *env) join(code string, u User) (*Session, *fakeConn) {
	e.t.Helper()
	c := &fakeConn{}
	s, err := e.hub.Join(code, u, c)
	if err != nil {
		e.t.Fatal(err)
	}
	return s, c
}

func (e *env) do(s *Session, m Message) {
	e.t.Helper()
	if err := s.Room.Handle(s.Player, m); err != nil {
		e.t.Fatalf("%s: %v", m.T, err)
	}
}

// step chạy một nhịp phòng (máy đua, phát vị trí, kiểm hết giờ) ở giờ hiện tại.
func (e *env) step(r *Room) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.racing() {
		r.step()
	}
}

// drive cho người chơi chạy dọc đường mẫu với tốc độ v (m/s) trong d, mỗi gói cách nhau 66 ms.
// s là quãng đường hiện tại trên đường mẫu, trả quãng đường mới.
func (e *env) drive(s *Session, sDist, v float64, d time.Duration) float64 {
	e.t.Helper()
	tr := s.Room.track
	for el := time.Duration(0); el < d; el += 66 * time.Millisecond {
		e.clk.add(66 * time.Millisecond)
		sDist += v * 0.066
		p, rot := tr.at(sDist)
		s.Room.Handle(s.Player, Message{T: "state", X: p.X, Z: p.Z, Rot: rot, Speed: v})
		e.step(s.Room)
	}
	return sDist
}

func (e *env) phase(r *Room) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phase
}

func (e *env) result(r *Room) *Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.result
}

// startRace cho hai người vào phòng, bắt đầu và chờ hết đếm ngược.
func (e *env) startRace(bet int) (*Session, *fakeConn, *Session, *fakeConn) {
	e.t.Helper()
	a, ca := e.create(an, bet)
	b, cb := e.join(a.Room.Code, binh)
	e.do(a, Message{T: "start"})
	if e.phase(a.Room) != PhaseCountdown {
		e.t.Fatalf("sau start phải đếm ngược, đang %s", e.phase(a.Room))
	}
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	if e.phase(a.Room) != PhaseRacing {
		e.t.Fatalf("hết đếm ngược phải đua, đang %s", e.phase(a.Room))
	}
	return a, ca, b, cb
}

// ---------- Đường đua ----------

func TestTrackGeometry(t *testing.T) {
	ts, err := LoadTracks(os.DirFS("testdata/tracks"))
	if err != nil {
		t.Fatal(err)
	}
	tr := ts["square"]
	if tr.length != 400 || tr.Laps != 3 || tr.MinLapMs != 12000 {
		t.Fatalf("đường vuông: dài %v, %d vòng, %d ms", tr.length, tr.Laps, tr.MinLapMs)
	}
	if p, _ := tr.at(150); math.Abs(p.X-100) > 1e-9 || math.Abs(p.Z-50) > 1e-9 {
		t.Fatalf("điểm ở 150 m: %+v", p)
	}
	if p, _ := tr.at(410); math.Abs(p.X-10) > 1e-9 || p.Z != 0 {
		t.Fatalf("quãng đường quá một vòng phải quay lại đầu: %+v", p)
	}
	if s := tr.nearestS(Point{60, 2}); math.Abs(s-60) > 1e-9 {
		t.Fatalf("điểm gần nhất: %v", s)
	}
	if sp := tr.spawnOf(7); sp.X >= tr.Spawn[5].X {
		t.Fatalf("ghế ngoài danh sách phải xếp lùi sau: %+v", sp)
	}
	if _, err := LoadTracks(os.DirFS("testdata/none")); err != nil {
		t.Fatalf("thư mục không có đường không phải lỗi: %v", err)
	}
}

// ---------- Phòng ----------

func TestRoomBetsAndStartChecks(t *testing.T) {
	e := newEnv(t)
	a, _ := e.create(an, 777) // mức lạ → mặc định
	if a.Room.bet != DefaultBet || a.Room.track.ID != "square" {
		t.Fatalf("phòng mới: cược %d, đường %v", a.Room.bet, a.Room.track)
	}
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err == nil {
		t.Fatal("một người không được bắt đầu")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setBet", Bet: ptr(123)}); err == nil {
		t.Fatal("mức cược lạ phải bị từ chối")
	}
	if err := a.Room.Handle(a.Player, Message{T: "setTrack", Track: "khong-co"}); err == nil {
		t.Fatal("đường lạ phải bị từ chối")
	}
	b, _ := e.join(a.Room.Code, binh)
	if err := b.Room.Handle(b.Player, Message{T: "start"}); err == nil {
		t.Fatal("chỉ chủ phòng được bắt đầu")
	}
	e.do(a, Message{T: "setBet", Bet: ptr(10_000)})
	e.bank.balances[2] = 9_999
	if err := a.Room.Handle(a.Player, Message{T: "start"}); err == nil || e.phase(a.Room) != PhaseLobby {
		t.Fatal("người thiếu xu thì không bắt đầu được")
	}
	e.do(a, Message{T: "setBet", Bet: ptr(500)})
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "start"})
	if e.phase(a.Room) != PhaseCountdown {
		t.Fatal("đủ xu phải bắt đầu đếm ngược")
	}
	if err := a.Room.Handle(a.Player, Message{T: "addBot"}); err == nil {
		t.Fatal("đang đua không thêm máy được")
	}
	// Gói trước giờ xuất phát bị bỏ, không tính vi phạm.
	a.Room.Handle(a.Player, Message{T: "state", X: 50, Z: 0, Speed: 10})
	if a.Player.violations != 0 || a.Player.prog.pos.X == 50 {
		t.Fatal("gói trước xuất phát phải bị bỏ qua")
	}
}

func TestRoomCapacityAndMinHP(t *testing.T) {
	e := newEnv(t)
	e.hub.opts.MinHP = 30
	if _, err := e.hub.Create(User{ID: 9, Name: "Yếu", HP: 20}, &fakeConn{}, nil, "", ""); err == nil {
		t.Fatal("dưới 30 HP không vào được")
	}
	a, _ := e.create(an, 100)
	for range MaxPlayers - 1 {
		e.do(a, Message{T: "addBot"})
	}
	if err := a.Room.Handle(a.Player, Message{T: "addBot"}); err == nil {
		t.Fatal("tối đa 6 xe")
	}
	s, _ := e.join(a.Room.Code, binh)
	if s.Room.seatOf(s.Player) >= 0 {
		t.Fatal("đủ 6 xe thì người mới chỉ được xem")
	}
}

func TestHonestRaceWinnerTakesBetsWithTax(t *testing.T) {
	e := newEnv(t)
	a, ca, b, _ := e.startRace(1_000)
	sa, sb := 0.0, 0.0
	// An chạy 28 m/s, Bình 24 m/s: một vòng 400 m ≈ 14,3 s / 16,7 s (≥ 12 s tối thiểu).
	for e.phase(a.Room) == PhaseRacing {
		e.clk.add(66 * time.Millisecond)
		sa += 28 * 0.066
		sb += 24 * 0.066
		pa, ra := a.Room.track.at(sa)
		pb, rb := b.Room.track.at(sb)
		a.Room.Handle(a.Player, Message{T: "state", X: pa.X, Z: pa.Z, Rot: ra, Speed: 28, Nitro: true})
		b.Room.Handle(b.Player, Message{T: "state", X: pb.X, Z: pb.Z, Rot: rb, Speed: 24})
		e.step(a.Room)
		if sa > 5000 {
			t.Fatal("cuộc đua không kết thúc")
		}
	}
	if a.Player.violations != 0 || b.Player.violations != 0 {
		t.Fatalf("chạy đúng luật mà bị tính vi phạm: %d %d", a.Player.violations, b.Player.violations)
	}
	res := e.result(a.Room)
	if res == nil || len(res.Places) != 2 || res.Places[0].Seat != 0 || res.Places[1].Seat != 1 {
		t.Fatalf("thứ hạng: %+v", res)
	}
	if res.Places[0].TimeMs < 3*12000 || res.Places[1].TimeMs <= res.Places[0].TimeMs {
		t.Fatalf("thời gian về đích: %+v", res.Places)
	}
	// Nhất ăn 1.000 xu cược của Bình, nộp thuế 5% = 50.
	if res.Money[0] != 950 || res.Tax[0] != 50 || res.Money[1] != -1_000 {
		t.Fatalf("tiền: %+v thuế %+v", res.Money, res.Tax)
	}
	if bal, xp := e.bank.get(1); bal != 10_950 || xp != 30 {
		t.Fatalf("ví An: %d xu, %d XP", bal, xp)
	}
	if bal, xp := e.bank.get(2); bal != 9_000 || xp != 10 {
		t.Fatalf("ví Bình: %d xu, %d XP", bal, xp)
	}
	if ca.count("finish") != 2 || ca.count("result") != 1 || ca.count("cars") == 0 {
		t.Fatalf("tin gửi An: finish %d, result %d, cars %d", ca.count("finish"), ca.count("result"), ca.count("cars"))
	}
	// Đua xong, chủ phòng bắt đầu lại được.
	e.do(a, Message{T: "start"})
	if e.phase(a.Room) != PhaseCountdown || e.result(a.Room) != nil {
		t.Fatal("bắt đầu cuộc đua mới")
	}
}

func TestSpeedAndTeleportPacketsAreDroppedThenDisqualified(t *testing.T) {
	e := newEnv(t)
	a, ca, b, _ := e.startRace(100)
	e.drive(a, 0, 20, time.Second)
	if a.Player.violations != 0 {
		t.Fatal("chạy bình thường không vi phạm")
	}
	before := a.Player.prog.pos
	e.clk.add(66 * time.Millisecond)
	a.Room.Handle(a.Player, Message{T: "state", X: before.X + 1, Z: 0, Speed: 60}) // vượt trần 39 m/s × 1,1
	e.clk.add(66 * time.Millisecond)
	a.Room.Handle(a.Player, Message{T: "state", X: before.X + 60, Z: 0, Speed: 20}) // dịch chuyển 60 m
	if a.Player.violations != 2 || a.Player.prog.pos != before {
		t.Fatalf("gói sai phải bị bỏ: vi phạm %d, vị trí %+v", a.Player.violations, a.Player.prog.pos)
	}
	for range maxViolations {
		e.clk.add(66 * time.Millisecond)
		a.Room.Handle(a.Player, Message{T: "state", X: 0, Z: 90, Speed: 20})
	}
	if !a.Player.dnf || ca.count("error") == 0 {
		t.Fatal("vi phạm lặp lại phải bị loại")
	}
	// Bình chạy nốt và thắng; An bị loại xếp bét, mất cược.
	e.drive(b, 0, 28, 50*time.Second)
	res := e.result(a.Room)
	if res == nil || res.Places[0].Seat != 1 || !res.Places[1].DNF || res.Money[0] != -100 {
		t.Fatalf("kết quả khi có người bị loại: %+v", res)
	}
}

func TestSkippingCheckpointsDoesNotCountLap(t *testing.T) {
	e := newEnv(t)
	a, _, _, _ := e.startRace(100)
	// Đi lùi từ vạch xuất phát về phía (0, 100) rồi quay lại vạch đích: bỏ qua các điểm kiểm tra.
	x, z := -4.0, 0.0
	for range 20 {
		e.clk.add(time.Second)
		z += 20
		if z > 100 {
			z = 100
		}
		a.Room.Handle(a.Player, Message{T: "state", X: x, Z: z, Speed: 20})
	}
	for range 10 {
		e.clk.add(time.Second)
		z = math.Max(0, z-20)
		a.Room.Handle(a.Player, Message{T: "state", X: x, Z: z, Speed: 20})
	}
	if a.Player.prog.lap != 0 || a.Player.prog.cp != 0 {
		t.Fatalf("bỏ qua điểm kiểm tra mà vẫn tính: vòng %d, điểm %d", a.Player.prog.lap, a.Player.prog.cp)
	}
}

func TestLapFasterThanMinimumIsRejected(t *testing.T) {
	e := newEnv(t)
	a, _, _, _ := e.startRace(100)
	// 38 m/s (dưới trần nitro 39 × 1,1) → một vòng 400 m ≈ 10,5 s, nhanh hơn 12 s tối thiểu của đường.
	e.drive(a, 0, 38, 11*time.Second)
	if a.Player.prog.lap != 0 || a.Player.violations == 0 {
		t.Fatalf("vòng quá nhanh không được tính: vòng %d, vi phạm %d", a.Player.prog.lap, a.Player.violations)
	}
}

func TestBotsRaceAlongPathAndFinish(t *testing.T) {
	e := newEnv(t)
	a, _ := e.create(an, 500)
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	// An đứng yên ở vạch xuất phát; máy chạy hết 3 vòng.
	for i := 0; i < 5000 && e.phase(a.Room) != PhaseOver; i++ {
		e.clk.add(66 * time.Millisecond)
		e.step(a.Room)
		if e.result(a.Room) == nil && !a.Room.firstAt.IsZero() {
			break
		}
	}
	if a.Room.firstAt.IsZero() {
		t.Fatal("máy phải về đích")
	}
	bot := a.Room.players[1]
	if !bot.prog.finished && !a.Room.players[2].prog.finished {
		t.Fatal("ít nhất một máy về đích")
	}
	// Người thật chưa về: chờ hết 90 s sau người nhất thì chốt, An bét, mất cược.
	e.clk.add(91 * time.Second)
	e.step(a.Room)
	res := e.result(a.Room)
	if res == nil || res.Places[2].Seat != 0 || res.Money[0] != -500 {
		t.Fatalf("hết giờ chốt kết quả: %+v", res)
	}
	if bal, xp := e.bank.get(1); bal != 9_500 || xp != 10 {
		t.Fatalf("ví An: %d xu, %d XP", bal, xp)
	}
}

func TestLeaveMidRaceIsLastAndPays(t *testing.T) {
	e := newEnv(t)
	a, _, b, cb := e.startRace(1_000)
	e.do(b, Message{T: "leave"})
	if cb.count("left") != 1 || !b.Player.dnf {
		t.Fatal("rời giữa cuộc đua phải tính bỏ cuộc")
	}
	e.drive(a, 0, 28, 50*time.Second)
	res := e.result(a.Room)
	if res == nil || res.Places[0].Seat != 0 || !res.Places[1].DNF || res.Money[1] != -1_000 {
		t.Fatalf("kết quả khi có người rời: %+v", res)
	}
	if bal, _ := e.bank.get(2); bal != 9_000 {
		t.Fatalf("người rời vẫn mất cược: %d", bal)
	}
	if len(a.Room.players) != 1 {
		t.Fatal("người đã rời bị bỏ ghế sau khi chốt")
	}
}

// Người thật rời giữa cuộc đua, máy đi tiếp và về đích trong nhịp step: chốt kết quả bỏ ghế người đã rời ngay
// giữa vòng lặp máy — từng làm r.players có phần tử nil (slices.DeleteFunc xoá đè tại chỗ) và server sập.
func TestBotFinishAfterHumansLeftDoesNotPanic(t *testing.T) {
	// Tốc độ máy ngẫu nhiên: lặp nhiều ván để có ván máy ở ghế đầu về trước (trường hợp từng sập).
	for round := 0; round < 40; round++ {
		botFinishAfterHumansLeft(t)
	}
}

func botFinishAfterHumansLeft(t *testing.T) {
	e := newEnv(t)
	a, _ := e.create(an, 0)
	b, _ := e.join(a.Room.Code, binh)
	for i := 0; i < 3; i++ {
		e.do(a, Message{T: "addBot"})
	}
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	e.do(a, Message{T: "leave"})
	e.do(b, Message{T: "leave"})
	for i := 0; i < 20000 && e.phase(a.Room) != PhaseOver; i++ {
		e.clk.add(66 * time.Millisecond)
		e.step(a.Room)
	}
	if e.phase(a.Room) != PhaseOver {
		t.Fatalf("máy về đích phải chốt cuộc đua, đang %s", e.phase(a.Room))
	}
	for _, p := range a.Room.players {
		if p == nil || !p.Bot {
			t.Fatalf("sau khi chốt chỉ còn máy: %+v", a.Room.players)
		}
	}
}

func TestDisconnectGraceCountsAsDNF(t *testing.T) {
	e := newEnv(t)
	e.hub.opts.Grace = 20 * time.Millisecond
	a, _ := e.create(an, 100)
	b, cb := e.join(a.Room.Code, binh)
	a.Room.opts.Grace = 20 * time.Millisecond
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	b.Room.Disconnect(b.Player, cb)
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.Room.mu.Lock()
		dnf := b.Player.dnf
		a.Room.mu.Unlock()
		if dnf {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mất kết nối quá hạn phải tính bỏ cuộc")
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.drive(a, 0, 28, 50*time.Second)
	if res := e.result(a.Room); res == nil || res.Places[0].Seat != 0 || !res.Places[1].DNF {
		t.Fatalf("kết quả: %+v", res)
	}
}

func TestReconnectKeepsSeatAndRoomsSummary(t *testing.T) {
	e := newEnv(t)
	a, ca := e.create(an, 100)
	e.do(a, Message{T: "addBot"})
	a.Room.Disconnect(a.Player, ca)
	s, _ := e.join(a.Room.Code, an)
	if s.Player != a.Player || !s.Player.Online {
		t.Fatal("vào lại phải giữ ghế cũ")
	}
	rooms := e.hub.Rooms()
	if len(rooms) != 1 || rooms[0].Track != "square" || len(rooms[0].Players) != 2 || !rooms[0].Players[1].Bot {
		t.Fatalf("danh sách phòng: %+v", rooms)
	}
	if st := e.hub.Stats(); st.Rooms != 1 || st.Players != 1 {
		t.Fatalf("thống kê: %+v", st)
	}
	st := a.Room.stateFor(a.Player)
	if st.Seat != 0 || len(st.Seats) != 2 || st.Seats[0].Car.ID != "starter" || len(st.Tracks) != 1 || st.Laps != 3 {
		t.Fatalf("tin room: %+v", st)
	}
	e.do(a, Message{T: "leave"})
	if e.hub.RoomCount() != 0 {
		t.Fatal("chủ phòng rời khi chỉ còn máy thì đóng phòng")
	}
}

func TestFlexBoolAcceptsNumbers(t *testing.T) {
	var m Message
	if err := json.Unmarshal([]byte(`{"t":"state","nitro":0.7,"drift":true}`), &m); err != nil {
		t.Fatal(err)
	}
	if !m.Nitro || !m.Drift {
		t.Fatalf("nitro / drift: %+v", m)
	}
	if err := json.Unmarshal([]byte(`{"t":"state","nitro":0,"drift":"x"}`), &m); err != nil || m.Nitro || m.Drift {
		t.Fatalf("giá trị lạ phải là false: %+v %v", m, err)
	}
}

// Mốc thời gian vị trí gửi cho máy khác lấy theo lúc máy người chơi lấy mẫu (Ms) cộng độ lệch nhỏ nhất đã thấy,
// không theo lúc từng gói tới server: gói tới chập chờn thì mốc vẫn đều; Ms vô lý thì dùng lúc nhận; luôn tăng dần.
func TestSampleAtUsesClientTimeline(t *testing.T) {
	start := int64(1_000_000)
	p := &Player{}
	// Đồng hồ máy chậm hơn server 40 ms; gói lấy mẫu mỗi 66 ms, tới server trễ 30 / 150 / 10 / 90 ms (cộng 40).
	delays := []int64{30, 150, 10, 90}
	var got []int64
	for i, d := range delays {
		ms := int64(66 * (i + 1))
		got = append(got, p.sampleAt(start, ptr64(ms), start+ms+40+d))
	}
	// Gói 3 tới nhanh hơn gói 1 (lệch 50 < 70): độ lệch chỉ hạ dần 2 ms mỗi gói (68) để mốc không nhảy; mốc vẫn cách
	// đều 66 ms như nhịp lấy mẫu dù gói tới chập chờn.
	if got[1] != start+132+70 || got[2] != start+198+68 || got[3]-got[2] != 66 {
		t.Fatalf("mốc theo lúc lấy mẫu: %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("mốc phải tăng dần: %v", got)
		}
	}
	q := &Player{}
	if at := q.sampleAt(start, nil, start+5_200); at != start+5_200 {
		t.Fatalf("không có Ms thì dùng lúc nhận: %d", at)
	}
	if at := q.sampleAt(start, ptr64(9_000), start+5_300); at != start+5_300 {
		t.Fatalf("Ms sớm quá xa thì dùng lúc nhận: %d", at)
	}
	if at := q.sampleAt(start, ptr64(1_000), start+5_400); at != start+5_400 {
		t.Fatalf("Ms trễ quá xa thì dùng lúc nhận: %d", at)
	}
}

func TestCarsMessageCarriesClientSampleTime(t *testing.T) {
	e := newEnv(t)
	a, _, b, cb := e.startRace(0)
	startMs := a.Room.startAt.UnixMilli()
	tr := a.Room.track
	// Hai gói lấy mẫu lúc +66 / +132 ms, tới server lúc +100 / +300 ms: gói đầu tới nhanh (lệch 34 ms) nên gói sau
	// mang mốc 132 + 34, không phải lúc nhận 300.
	send := func(recv time.Duration, ms int64, d float64) {
		e.clk.add(recv)
		p, rot := tr.at(d)
		a.Room.Handle(a.Player, Message{T: "state", X: p.X, Z: p.Z, Rot: rot, Speed: 20, Ms: ptr64(ms)})
	}
	send(100*time.Millisecond, 66, 1)
	send(200*time.Millisecond, 132, 2)
	e.step(a.Room)
	_ = b
	at := int64(-1)
	cb.mu.Lock()
	for _, m := range cb.msgs {
		if mm, ok := m.(map[string]any); ok && mm["t"] == "cars" {
			for _, c := range mm["list"].([]CarState) {
				if c.Seat == 0 {
					at = c.At
				}
			}
		}
	}
	cb.mu.Unlock()
	if at != startMs+166 {
		t.Fatalf("mốc xe An phải là lúc lấy mẫu + lệch nhỏ nhất %d, được %d", startMs+166, at)
	}
}

// Điều khiển cuối (steer / throttle) đi theo vị trí trong tin "cars" để máy khác ngoại suy bằng vật lý xe: người chơi
// theo gói gửi (kẹp −1..1), máy có ga 1 và lái suy từ tốc độ quay; gói không có điều khiển (máy khách cũ) thì bỏ trống.
func TestCarsMessageCarriesControls(t *testing.T) {
	e := newEnv(t)
	a, _ := e.create(an, 0)
	b, cb := e.join(a.Room.Code, binh)
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	tr := a.Room.track
	f := func(v float64) *float64 { return &v }
	e.clk.add(66 * time.Millisecond)
	p, rot := tr.at(1)
	a.Room.Handle(a.Player, Message{T: "state", X: p.X, Z: p.Z, Rot: rot, Speed: 15, Ms: ptr64(66), Steer: f(2), Throttle: f(0.5)})
	p, rot = tr.at(1)
	b.Room.Handle(b.Player, Message{T: "state", X: p.X, Z: p.Z, Rot: rot, Speed: 15, Ms: ptr64(66)})
	for range 30 {
		e.clk.add(66 * time.Millisecond)
		e.step(a.Room)
	}
	var last []CarState
	cb.mu.Lock()
	for _, m := range cb.msgs {
		if mm, ok := m.(map[string]any); ok && mm["t"] == "cars" {
			last = mm["list"].([]CarState)
		}
	}
	cb.mu.Unlock()
	if len(last) != 3 {
		t.Fatalf("tin cars: %+v", last)
	}
	if s, th := last[0].Steer, last[0].Throttle; s == nil || th == nil || *s != 1 || *th != 0.5 {
		t.Fatalf("điều khiển của An (kẹp lái 2 → 1): %v %v", s, th)
	}
	if last[1].Steer != nil || last[1].Throttle != nil {
		t.Fatal("gói không có điều khiển thì để trống")
	}
	if bot := last[2]; bot.Throttle == nil || *bot.Throttle != 1 || bot.Steer == nil || math.Abs(*bot.Steer) > 1 {
		t.Fatalf("máy: ga 1, lái trong −1..1: %v %v", bot.Throttle, bot.Steer)
	}
	if b, _ := json.Marshal(last[1]); strings.Contains(string(b), "steer") {
		t.Fatalf("không có điều khiển thì không gửi trường steer: %s", b)
	}
}

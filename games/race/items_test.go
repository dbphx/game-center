package race

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------- Chế độ đạo cụ ----------

// startItems mở phòng chế độ đạo cụ cho các người u, bắt đầu và chờ hết đếm ngược.
func (e *env) startItems(us ...User) ([]*Session, []*fakeConn) {
	e.t.Helper()
	c0 := &fakeConn{}
	a, err := e.hub.Create(us[0], c0, ptr(100), "square", ModeItems)
	if err != nil {
		e.t.Fatal(err)
	}
	ss, cs := []*Session{a}, []*fakeConn{c0}
	for _, u := range us[1:] {
		s, c := e.join(a.Room.Code, u)
		ss, cs = append(ss, s), append(cs, c)
	}
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	if e.phase(a.Room) != PhaseRacing {
		e.t.Fatalf("hết đếm ngược phải đua, đang %s", e.phase(a.Room))
	}
	return ss, cs
}

// place đặt xe của s ở điểm p (vị trí server biết).
func place(s *Session, p Point) {
	s.Room.mu.Lock()
	s.Player.prog.pos = p
	s.Room.mu.Unlock()
}

// atS là điểm ở quãng đường d trên đường mẫu.
func atS(s *Session, d float64) Point {
	p, _ := s.Room.track.at(d)
	return p
}

func give(s *Session, items ...string) {
	s.Room.mu.Lock()
	s.Player.items = append([]string(nil), items...)
	s.Room.mu.Unlock()
}

// run chạy phòng d theo nhịp 50 ms (vị trí người chơi đứng yên).
func (e *env) run(r *Room, d time.Duration) {
	for el := time.Duration(0); el < d; el += 50 * time.Millisecond {
		e.clk.add(50 * time.Millisecond)
		e.step(r)
	}
}

// hits trả các tin "hit" đã nhận.
func (f *fakeConn) hits() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, m := range f.msgs {
		b, _ := json.Marshal(m)
		var v map[string]any
		json.Unmarshal(b, &v)
		if v["t"] == "hit" {
			out = append(out, v)
		}
	}
	return out
}

func spinning(s *Session, nowMs int64) bool {
	s.Room.mu.Lock()
	defer s.Room.mu.Unlock()
	return nowMs < s.Player.spinUntil
}

func TestPickupRules(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh)
	a := ss[0]
	r := a.Room
	place(a, Point{60, 0}) // hộp 0 ở (50, 0): xa 10 m
	if err := r.Handle(a.Player, Message{T: "pickup", Box: 0}); err == nil {
		t.Fatal("xa quá 6 m không được nhặt")
	}
	place(a, Point{52, 1})
	e.do(a, Message{T: "pickup", Box: 0})
	if len(a.Player.items) != 1 || cs[0].count("items") != 1 || cs[1].count("box") != 1 {
		t.Fatalf("nhặt hộp: món %v, tin items %d, tin box %d", a.Player.items, cs[0].count("items"), cs[1].count("box"))
	}
	if err := r.Handle(a.Player, Message{T: "pickup", Box: 0}); err == nil {
		t.Fatal("hộp vừa bị nhặt phải tắt")
	}
	if err := r.Handle(a.Player, Message{T: "pickup", Box: 9}); err == nil {
		t.Fatal("hộp không tồn tại")
	}
	e.run(r, 3100*time.Millisecond) // hộp bật lại sau 3 s
	if !r.boxes[0].active || cs[1].count("box") != 2 {
		t.Fatalf("hộp phải bật lại: %+v, tin box %d", r.boxes[0], cs[1].count("box"))
	}
	give(a, ItemOil, ItemShield)
	if err := r.Handle(a.Player, Message{T: "pickup", Box: 0}); err == nil {
		t.Fatal("đã giữ 2 món thì không nhặt thêm")
	}
}

func TestSpeedModeHasNoItems(t *testing.T) {
	e := newEnv(t)
	a, _, _, _ := e.startRace(100)
	r := a.Room
	st := r.stateFor(a.Player)
	if st.Mode != ModeSpeed || len(st.Boxes) != 0 || st.NitroCharge != NitroCharge {
		t.Fatalf("phòng tốc độ: %+v", st)
	}
	place(a, Point{50, 0})
	if err := r.Handle(a.Player, Message{T: "pickup", Box: 0}); err == nil {
		t.Fatal("chế độ tốc độ không có hộp")
	}
	give(a, ItemMissile)
	if err := r.Handle(a.Player, Message{T: "use"}); err == nil {
		t.Fatal("chế độ tốc độ không dùng đạo cụ")
	}
}

func TestSetMode(t *testing.T) {
	e := newEnv(t)
	a, _ := e.create(an, 100)
	b, _ := e.join(a.Room.Code, binh)
	if err := a.Room.Handle(a.Player, Message{T: "setMode", Mode: "lạ"}); err == nil {
		t.Fatal("chế độ lạ phải lỗi")
	}
	if err := b.Room.Handle(b.Player, Message{T: "setMode", Mode: ModeItems}); err == nil {
		t.Fatal("chỉ chủ phòng đổi chế độ")
	}
	e.do(a, Message{T: "setMode", Mode: ModeItems})
	e.do(a, Message{T: "start"})
	if st := a.Room.stateFor(b.Player); st.Mode != ModeItems || len(st.Boxes) != 3 {
		t.Fatalf("phòng đạo cụ: chế độ %s, %d hộp", st.Mode, len(st.Boxes))
	}
}

func TestRollItemByRank(t *testing.T) {
	count := func(rank, total int) map[string]int {
		out := map[string]int{}
		for i := range 1000 {
			out[rollItem(rank, total, (float64(i)+0.5)/1000)]++
		}
		return out
	}
	lead := count(1, 4)
	if lead[ItemHoming] != 0 || lead[ItemOil]+lead[ItemShield] < 750 {
		t.Fatalf("dẫn đầu chủ yếu dầu / khiên, không có tên lửa đuổi: %v", lead)
	}
	last := count(4, 4)
	if last[ItemOil] != 0 || last[ItemHoming]+last[ItemBoost] < 650 {
		t.Fatalf("xếp cuối dễ ra tên lửa đuổi / tăng tốc: %v", last)
	}
	mid := count(2, 4)
	if len(mid) != 5 {
		t.Fatalf("ở giữa ra đủ 5 món: %v", mid)
	}
}

func TestMissileHitsCarAhead(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh)
	a, b := ss[0], ss[1]
	place(a, atS(a, 10))
	place(b, atS(b, 45))
	give(a, ItemMissile)
	e.do(a, Message{T: "use"})
	if cs[1].count("fx") != 1 || len(a.Player.items) != 0 {
		t.Fatalf("bắn tên lửa: fx %d, món còn %v", cs[1].count("fx"), a.Player.items)
	}
	e.run(a.Room, time.Second)
	h := cs[0].hits()
	if len(h) != 1 || h[0]["seat"] != 1.0 || h[0]["by"] != 0.0 || h[0]["kind"] != ItemMissile {
		t.Fatalf("tên lửa phải trúng xe phía trước: %v", h)
	}
	if !spinning(b, e.clk.now().UnixMilli()-500) || spinning(a, e.clk.now().UnixMilli()) {
		t.Fatal("xe bị trúng phải xoay, người bắn thì không")
	}
}

func TestMissileMissesCarBehind(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh)
	a, b := ss[0], ss[1]
	place(a, atS(a, 45))
	place(b, atS(b, 10))
	give(a, ItemMissile)
	e.do(a, Message{T: "use"})
	e.run(a.Room, 3500*time.Millisecond)
	if len(cs[0].hits()) != 0 {
		t.Fatalf("tên lửa bay về trước, không trúng xe phía sau: %v", cs[0].hits())
	}
	if len(a.Room.fx) != 0 {
		t.Fatal("hết 3 s tên lửa phải biến mất")
	}
}

func TestHomingChasesCarDirectlyAhead(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh, chi)
	a, b, c := ss[0], ss[1], ss[2]
	place(a, Point{90, 0})  // dẫn đầu, nằm trên đường mẫu
	place(b, Point{60, 10}) // hạng 2, lệch khỏi đường mẫu 10 m: tên lửa thường không trúng
	place(c, Point{20, 0})  // xếp cuối
	give(c, ItemHoming)
	e.do(c, Message{T: "use"})
	e.run(c.Room, 2*time.Second)
	h := cs[0].hits()
	if len(h) != 1 || h[0]["seat"] != 1.0 || h[0]["by"] != 2.0 || h[0]["kind"] != ItemHoming {
		t.Fatalf("tên lửa đuổi phải trúng đúng xe ngay trên (ghế 1): %v", h)
	}
}

func TestOilHitsOthersNotOwnerRightAway(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh)
	a, b := ss[0], ss[1]
	place(a, atS(a, 50))
	place(b, atS(b, 10))
	give(a, ItemOil)
	e.do(a, Message{T: "use"})
	oil := a.Room.fx[0].pos
	if d := oil.X - 46; d < -0.01 || d > 0.01 {
		t.Fatalf("dầu phải ở sau xe 4 m: %+v", oil)
	}
	place(a, oil) // người thả lùi vào dầu ngay: an toàn
	e.run(a.Room, 200*time.Millisecond)
	if len(cs[0].hits()) != 0 {
		t.Fatal("người thả không dính dầu của mình ngay")
	}
	place(a, atS(a, 80))
	place(b, Point{oil.X + 1, oil.Z})
	e.run(a.Room, 100*time.Millisecond)
	h := cs[0].hits()
	if len(h) != 1 || h[0]["seat"] != 1.0 || h[0]["kind"] != ItemOil || len(a.Room.fx) != 0 {
		t.Fatalf("xe chạy vào dầu phải trượt và dầu mất: %v", h)
	}
}

func TestShieldBlocksOneHit(t *testing.T) {
	e := newEnv(t)
	ss, cs := e.startItems(an, binh)
	a, b := ss[0], ss[1]
	place(a, atS(a, 10))
	place(b, atS(b, 45))
	give(b, ItemShield)
	e.do(b, Message{T: "use"})
	give(a, ItemMissile)
	e.do(a, Message{T: "use"})
	e.run(a.Room, time.Second)
	h := cs[1].hits()
	if len(h) != 1 || h[0]["blocked"] != true {
		t.Fatalf("khiên phải chặn: %v", h)
	}
	if spinning(b, e.clk.now().UnixMilli()) || b.Player.shieldUntil != 0 {
		t.Fatal("chặn xong thì không xoay và mất khiên")
	}
}

func TestBoostLoosensSpeedCheck(t *testing.T) {
	e := newEnv(t)
	ss, _ := e.startItems(an, binh)
	a := ss[0]
	r := a.Room
	place(a, atS(a, 10))
	a.Player.prog.atMs = e.clk.now().UnixMilli()
	give(a, ItemBoost)
	e.do(a, Message{T: "use"})
	v := 30 * NitroBoost * boostMul // trần nitro × tăng tốc, vượt trần nitro thường
	e.clk.add(100 * time.Millisecond)
	p := atS(a, 10+v*0.1)
	e.do(a, Message{T: "state", X: p.X, Z: p.Z, Speed: v})
	if a.Player.violations != 0 {
		t.Fatalf("đang tăng tốc thì tốc độ %.1f hợp lệ, bị %d vi phạm", v, a.Player.violations)
	}
	e.clk.add(3 * time.Second) // hết tăng tốc
	r.mu.Lock()
	a.Player.prog.atMs = e.clk.now().UnixMilli() - 100
	r.mu.Unlock()
	q := atS(a, 10+v*0.2)
	e.do(a, Message{T: "state", X: q.X, Z: q.Z, Speed: v})
	if a.Player.violations != 1 {
		t.Fatalf("hết tăng tốc thì tốc độ đó là vi phạm: %d", a.Player.violations)
	}
}

func TestBotsPickAndUseItems(t *testing.T) {
	e := newEnv(t)
	c0 := &fakeConn{}
	a, err := e.hub.Create(an, c0, ptr(100), "square", ModeItems)
	if err != nil {
		t.Fatal(err)
	}
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "addBot"})
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.run(a.Room, 20*time.Second)
	used := 0
	for _, p := range a.Room.players {
		if p.Bot {
			used += p.used
		}
	}
	if c0.count("box") == 0 || used == 0 {
		t.Fatalf("máy phải nhặt hộp và dùng món: tin box %d, đã dùng %d", c0.count("box"), used)
	}
}

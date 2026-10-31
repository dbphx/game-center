package arena

import (
	"context"
	"sync"
	"testing"
)

// lootGear là kho trang bị giả nhận ngọc rơi.
type lootGear struct {
	mu   sync.Mutex
	got  map[int64][]GemKey
	coin map[int64]int64
}

func (l *lootGear) Gear(context.Context, int64) (Gear, error) { return Gear{}, nil }

func (l *lootGear) GrantLoot(_ context.Context, uid int64, gems []GemKey, coins int64, _ string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got[uid] = append(l.got[uid], gems...)
	l.coin[uid] += coins
	return nil
}

func bossTable(t *testing.T, boss string, lg *lootGear) ([]*Session, []*fakeConn) {
	t.Helper()
	h := NewHub(newBank(), fast, lg)
	ca := &fakeConn{}
	sa, err := h.Create(an, ca, ptr(100), "meadow", ModeBoss, boss)
	if err != nil {
		t.Fatal(err)
	}
	cb := &fakeConn{}
	sb, err := h.Join(sa.Room.Code, binh, cb)
	if err != nil {
		t.Fatal(err)
	}
	return []*Session{sa, sb}, []*fakeConn{ca, cb}
}

func TestBossWinLoot(t *testing.T) {
	lg := &lootGear{got: map[int64][]GemKey{}, coin: map[int64]int64{}}
	ss, cs := bossTable(t, "rong", lg)
	a := ss[0]
	st := cs[0].last()
	if st.Mode != ModeBoss || st.Boss == nil || st.Boss.ID != "rong" || st.Bet != 0 || len(st.Bosses) != 3 {
		t.Fatalf("phòng boss: %+v %+v", st.Mode, st.Boss)
	}
	do(t, a, Message{T: "addBot", Level: BotNormal})
	do(t, a, Message{T: "start"})
	st = cs[0].last()
	r := a.Room
	if st.Phase != PhasePlaying || st.Boss.MaxHP != 950*3 || st.Boss.HP != st.Boss.MaxHP || st.GameBet != 0 || st.Turn != st.Players[0].ID {
		t.Fatalf("bắt đầu: %+v %+v", st.Boss, st.Turn)
	}
	for _, p := range st.Players {
		if p.X >= st.Boss.X {
			t.Fatalf("người chơi phải đứng bên trái boss: %v ≥ %v", p.X, st.Boss.X)
		}
	}
	// Máy đồng đội ngắm boss, không ngắm người.
	r.mu.Lock()
	bot := r.players[2].f
	aim := BotAim(r.ground, r.fighters(), bot, BotHard, 0, r.rng)
	r.mu.Unlock()
	if aim.Angle > 90 && bot.X < r.boss.X {
		t.Fatalf("máy đồng đội phải bắn về phía boss: %+v", aim)
	}
	// Hai người lần lượt bắn trượt lên trời → máy → boss bắn → vòng 2 về ghế đầu.
	do(t, ss[0], Message{T: "fire", Angle: 90, Power: 100})
	waitFor(t, "lượt Bình", func() bool { return cs[0].last().Turn == ss[1].Player.ID })
	do(t, ss[1], Message{T: "fire", Angle: 90, Power: 100})
	waitFor(t, "boss bắn", func() bool { m := cs[0].lastOf("shot"); return m != nil && m["by"] == BossID })
	waitFor(t, "vòng 2", func() bool {
		st := cs[0].last()
		return st.Phase == PhaseOver || (st.Round == 2 && st.Turn == ss[0].Player.ID)
	})
	// Hạ boss.
	r.mu.Lock()
	r.boss.HP, r.boss.Alive = 0, false
	r.checkEnd("last")
	r.mu.Unlock()
	st = cs[0].last()
	if st.Phase != PhaseOver || st.Result == nil || !st.Result.Victory || st.Result.Reason != "boss" {
		t.Fatalf("thắng boss: %+v", st.Result)
	}
	loot := cs[1].lastOf("loot")
	if loot == nil {
		t.Fatal("thiếu tin loot")
	}
	for _, uid := range []int64{1, 2} {
		gems := lg.got[uid]
		if len(gems) < 3 || len(gems) > 4 || lg.coin[uid] != 200 {
			t.Fatalf("rơi ngọc rồng: %+v xu %d", gems, lg.coin[uid])
		}
		for _, k := range gems {
			if !k.valid() || k.Tier < 2 {
				t.Fatalf("rồng rơi ngọc bậc 2–3: %+v", k)
			}
		}
	}
	if len(st.Result.Loot) != 2 || len(lg.got) != 2 {
		t.Fatalf("máy không nhận ngọc: %+v", st.Result.Loot)
	}
}

func TestBossLoseNoLoot(t *testing.T) {
	lg := &lootGear{got: map[int64][]GemKey{}, coin: map[int64]int64{}}
	ss, cs := bossTable(t, "lạ", lg)
	if st := cs[0].last(); st.Boss.ID != "chuot" {
		t.Fatalf("boss lạ → Vua Chuột: %+v", st.Boss)
	}
	if err := ss[0].Room.Handle(ss[0].Player, Message{T: "settings", Boss: "rong2"}); err == nil {
		t.Fatal("boss không hợp lệ")
	}
	do(t, ss[0], Message{T: "settings", Boss: "robot", Bet: ptr(100)})
	if st := cs[0].last(); st.Boss.ID != "robot" || st.Bet != 0 {
		t.Fatalf("đổi boss, không cược: %+v %d", st.Boss, st.Bet)
	}
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	r.mu.Lock()
	for _, p := range r.players {
		p.f.HP, p.f.Alive = 0, false
	}
	r.checkEnd("last")
	r.mu.Unlock()
	st := cs[0].last()
	if st.Phase != PhaseOver || st.Result.Victory || st.Result.Reason != "wiped" || len(lg.got) != 0 || cs[0].lastOf("loot") != nil {
		t.Fatalf("thua boss: %+v %+v", st.Result, lg.got)
	}
}

func TestBossRollLoot(t *testing.T) {
	ss, _ := bossTable(t, "chuot", &lootGear{got: map[int64][]GemKey{}, coin: map[int64]int64{}})
	rng := ss[0].Room.rng
	for _, b := range Bosses {
		for range 200 {
			gems := b.rollLoot(rng)
			if len(gems) < b.Drops[0] || len(gems) > b.Drops[1] {
				t.Fatalf("%s số viên %d", b.ID, len(gems))
			}
			for _, k := range gems {
				if k.Tier < b.Tiers[0] || k.Tier > b.Tiers[1] || !k.valid() {
					t.Fatalf("%s bậc %+v", b.ID, k)
				}
			}
		}
	}
}

package arena

// Chế độ boss (đánh chung): 1–4 người (có thể thêm máy đồng đội) cùng đánh một boss do server điều khiển.
// Boss đứng yên ở mép phải, thân to, mỗi vòng bắn một phát mạnh; không cược. Boss chết thì mỗi người chơi thật
// nhận ngọc rơi ngẫu nhiên + ít xu; mọi người chết (hoặc hết số vòng) thì thua.

import (
	"context"
	"fmt"
	"log"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"time"
)

const (
	ModeVersus = "versus"
	ModeBoss   = "boss"
	BossID     = "boss" // id fighter của boss trong trận (tin shot.by / hits.id)
	teamHumans = 1
)

// Boss là một boss chọn được.
type Boss struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Level   string  `json:"level"` // easy | normal | hard
	HPPer   int     `json:"hpPer"` // máu mỗi người ngồi ghế
	Dmg     float64 `json:"dmg"`   // sát thương phát bắn ở tâm nổ
	R       float64 `json:"r"`     // bán kính nổ
	Size    float64 `json:"size"`  // hệ số thân
	Noise   float64 `json:"-"`     // sai số lực
	Coins   int64   `json:"coins"` // xu thưởng mỗi người khi thắng
	Drops   [2]int  `json:"-"`     // số viên ngọc rơi (min, max)
	Tiers   [2]int  `json:"-"`     // bậc ngọc (thấp, cao)
	HighPct int     `json:"-"`     // % ra bậc cao
}

var Bosses = []Boss{
	{ID: "chuot", Name: "Vua Chuột", Level: BotEasy, HPPer: 350, Dmg: 35, R: 45, Size: 1.6, Noise: 10, Coins: 50,
		Drops: [2]int{1, 2}, Tiers: [2]int{1, 1}},
	{ID: "robot", Name: "Robot Rác", Level: BotNormal, HPPer: 600, Dmg: 50, R: 55, Size: 1.9, Noise: 6, Coins: 100,
		Drops: [2]int{2, 3}, Tiers: [2]int{1, 2}, HighPct: 40},
	{ID: "rong", Name: "Rồng Lửa", Level: BotHard, HPPer: 950, Dmg: 70, R: 65, Size: 2.3, Noise: 3, Coins: 200,
		Drops: [2]int{3, 4}, Tiers: [2]int{2, 3}, HighPct: 25},
}

func findBoss(id string) (Boss, bool) {
	for _, b := range Bosses {
		if b.ID == id {
			return b, true
		}
	}
	return Boss{}, false
}

// BossView là boss gửi trong state.
type BossView struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Level string  `json:"level"`
	HP    int     `json:"hp"`
	MaxHP int     `json:"maxHp"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Size  float64 `json:"size"`
	Alive bool    `json:"alive"`
}

// LootGranter cộng ngọc rơi + xu thưởng (Armory).
type LootGranter interface {
	GrantLoot(ctx context.Context, uid int64, gems []GemKey, coins int64, note string) error
}

// rollLoot bốc ngọc rơi cho một người.
func (b Boss) rollLoot(rng *mrand.Rand) []GemKey {
	n := b.Drops[0] + rng.IntN(b.Drops[1]-b.Drops[0]+1)
	out := make([]GemKey, n)
	for i := range out {
		t := b.Tiers[0]
		if b.Tiers[1] > t && rng.IntN(100) < b.HighPct {
			t = b.Tiers[1]
		}
		out[i] = GemKey{GemKinds[rng.IntN(len(GemKinds))], t}
	}
	return out
}

// bossSpawn đặt người chơi ở 60% sân bên trái, boss ở mép phải.
func (r *Room) bossSpawn() {
	n := len(r.players)
	xs := spawnIn(r.ground, n, 0, Width*0.6, r.rng)
	for i, p := range r.players {
		x := xs[i]
		p.f = &Fighter{ID: p.ID, X: x, Y: float64(r.ground.At(x)), HP: p.Gear.MaxHP(), MaxHP: p.Gear.MaxHP(), Alive: true,
			Gear: p.Gear, Team: teamHumans}
		p.facing = 1
		p.lastAngle, p.lastPower = 55, 60
	}
	bx := Width - 150.0
	for r.ground.At(bx) >= AbyssY-20 && bx > Width/2 {
		bx -= 10
	}
	hp := r.bossDef.HPPer * n
	r.boss = &Fighter{ID: BossID, X: bx, Y: float64(r.ground.At(bx)), HP: hp, MaxHP: hp, Alive: true,
		Size: r.bossDef.Size, Gear: Gear{Arm: ArmCannon}}
}

// bossTurn: hết một vòng người chơi thì boss bắn một phát vào người gần nhất hoặc ngẫu nhiên.
func (r *Room) bossTurn() {
	r.turn = BossID
	r.moveLeft = 0
	r.wind = r.field.Wind(r.rng)
	r.stopTimer()
	gen := r.timerGen
	r.timer = time.AfterFunc(r.opts.BotDelay, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying || r.turn != BossID {
			return
		}
		r.bossFire()
	})
	r.broadcast()
}

func (r *Room) bossFire() {
	b := r.boss
	var target *Fighter
	alive := r.alive()
	if len(alive) == 0 {
		r.checkEnd("last")
		return
	}
	if r.rng.IntN(2) == 0 {
		target = alive[r.rng.IntN(len(alive))].f
	} else {
		for _, p := range alive {
			if target == nil || math.Abs(p.f.X-b.X) < math.Abs(target.X-b.X) {
				target = p.f
			}
		}
	}
	fs := r.fighters()
	bodies := targetsOf(fs, b)
	tx, ty := Body{X: target.X, Y: target.Y}.center()
	bestA, bestP, bestD := 135.0, 60.0, math.Inf(1)
	for a := 95.0; a <= 170; a += 2 {
		for p := 20.0; p <= 100; p += 2 {
			f := Simulate(r.ground, b.X, b.Y, a, p, r.wind, b.ID, bodies)
			if !f.Hit {
				continue
			}
			d := math.Hypot(f.X-tx, f.Y-ty)
			if f.Body == target.ID {
				d = 0
			}
			if f.Body == b.ID {
				d += 400
			}
			if d < bestD {
				bestA, bestP, bestD = a, p, d
			}
		}
	}
	bestP = math.Max(5, math.Min(100, bestP+(r.rng.Float64()*2-1)*r.bossDef.Noise))
	w := Weapon{ID: "boss_" + r.bossDef.ID, Name: r.bossDef.Name, Dmg: r.bossDef.Dmg, R: r.bossDef.R, Spread: []float64{0}, Uses: -1}
	res := Fire(r.ground, fs, b, w, bestA, bestP, r.wind, nil)
	r.sendAll(map[string]any{"t": "shot", "by": BossID, "weapon": w.ID, "arm": "", "angle": bestA, "power": bestP,
		"wind": r.wind, "paths": res.Paths, "blasts": orEmpty(res.Blasts), "hits": orEmpty(res.Hits),
		"deaths": orEmpty(res.Deaths), "tele": res.Tele})
	r.turn = ""
	if r.checkEnd("last") {
		return
	}
	r.broadcast()
	r.afterShot(res.Duration, func() {
		r.round++
		if r.round > r.opts.maxRounds() {
			r.round = r.opts.maxRounds()
			r.finishBoss(false, "rounds")
			return
		}
		r.passTurn(-1)
	})
}

// afterShot chờ đạn bay xong (+AfterShot) rồi chạy next dưới khoá bàn.
func (r *Room) afterShot(dur float64, next func()) {
	pause := time.Duration(dur*r.opts.ReplayScale*float64(time.Second)) + r.opts.AfterShot
	gen := r.timerGen
	r.timer = time.AfterFunc(pause, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying {
			return
		}
		next()
	})
}

// finishBoss chốt trận boss: thắng thì mỗi người thật nhận ngọc + xu; không có cược.
func (r *Room) finishBoss(win bool, reason string) {
	r.stopTimer()
	r.phase = PhaseOver
	r.turn = ""
	res := &Result{Reason: reason, Money: map[string]int64{}, XP: map[string]int64{}, Victory: win}
	if win {
		res.Winner = BossID // phe người chơi thắng; giao diện xem Victory
		res.Loot = map[string][]GemInfo{}
	}
	xp := map[int64]int64{}
	ctx := context.Background()
	granter, _ := r.hub.gear.(LootGranter)
	drops := map[string][]GemInfo{}
	for _, p := range r.players {
		if p.Bot {
			continue
		}
		gain := int64(XPLose)
		if win {
			gain = XPWin
			gems := r.bossDef.rollLoot(r.rng)
			infos := make([]GemInfo, len(gems))
			for i, k := range gems {
				infos[i] = gemInfo(k, 0)
			}
			drops[p.ID], res.Loot[p.ID] = infos, infos
			res.Money[p.ID] = r.bossDef.Coins
			if granter != nil {
				note := fmt.Sprintf("Đấu trường: hạ %s (bàn %s)", r.bossDef.Name, r.Code)
				if err := granter.GrantLoot(ctx, p.UserID, gems, r.bossDef.Coins, note); err != nil {
					log.Printf("arena: ngọc rơi bàn %s: %v", r.Code, err)
				} else if bal, err := r.hub.bank.Balance(ctx, p.UserID); err == nil {
					p.Coins = bal
				}
			}
		}
		res.XP[p.ID] = gain
		xp[p.UserID] = gain
	}
	r.result = res
	if len(xp) > 0 {
		levels, err := r.hub.bank.AwardXP(ctx, xp)
		if err != nil {
			log.Printf("arena: cộng XP bàn %s: %v", r.Code, err)
		}
		for _, p := range r.players {
			if lv, ok := levels[p.UserID]; ok && !p.Bot {
				p.Level = lv
			}
		}
	}
	if win {
		r.sendAll(map[string]any{"t": "loot", "drops": drops})
	}
	r.broadcast()
}

func (r *Room) bossView() *BossView {
	if r.mode != ModeBoss {
		return nil
	}
	d := r.bossDef
	v := &BossView{ID: d.ID, Name: d.Name, Level: d.Level, Size: d.Size, Alive: true}
	if b := r.boss; b != nil {
		v.HP, v.MaxHP, v.X, v.Y, v.Alive = b.HP, b.MaxHP, b.X, b.Y, b.Alive
	} else {
		v.HP = d.HPPer * max(1, len(r.players))
		v.MaxHP = v.HP
	}
	return v
}

// BossesHandler trả danh sách boss (GET /api/arena/bosses).
func BossesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, Bosses)
	})
}

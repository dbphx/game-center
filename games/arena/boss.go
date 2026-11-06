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
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Level    string  `json:"level"` // easy | normal | hard | veryhard | hell
	HPPer    int     `json:"hpPer"` // máu mỗi người ngồi ghế
	Dmg      float64 `json:"dmg"`   // sát thương phát bắn ở tâm nổ
	R        float64 `json:"r"`     // bán kính nổ
	Size     float64 `json:"size"`  // hệ số thân
	Noise    float64 `json:"-"`     // sai số lực
	Coins    int64   `json:"coins"` // xu thưởng mỗi người khi thắng
	Shots    int     `json:"shots"` // số phát mỗi lượt boss (nhắm các mục tiêu khác nhau nếu có)
	Skill    string  `json:"skill,omitempty"`
	LootText string  `json:"lootText"` // mô tả ngọc rơi
	Drops    [2]int  `json:"-"`        // số viên ngọc rơi (min, max)
	Tiers    [2]int  `json:"-"`        // bậc ngọc (thấp, cao)
	HighPct  int     `json:"-"`        // % ra bậc cao
}

const (
	BossVeryHard = "veryhard"
	BossHell     = "hell"
	// SkillRain: cứ RainEvery lượt boss thì "Mưa lửa" rơi trúng mọi người chơi còn sống.
	SkillRain = "rain"
	RainEvery = 3
	RainDmg   = 500
	RainR     = 40
)

var Bosses = []Boss{
	{ID: "chuot", Name: "Vua Chuột", Level: BotEasy, HPPer: 3500, Dmg: 350, R: 45, Size: 1.6, Noise: 10, Coins: 50, Shots: 1,
		LootText: "1–2 ngọc bậc I", Drops: [2]int{1, 2}, Tiers: [2]int{1, 1}},
	{ID: "robot", Name: "Robot Rác", Level: BotNormal, HPPer: 5000, Dmg: 500, R: 55, Size: 1.9, Noise: 6, Coins: 100, Shots: 1,
		LootText: "2–3 ngọc bậc I–II", Drops: [2]int{2, 3}, Tiers: [2]int{1, 2}, HighPct: 40},
	{ID: "rong", Name: "Rồng Lửa", Level: BotHard, HPPer: 6500, Dmg: 700, R: 65, Size: 2.3, Noise: 3, Coins: 200, Shots: 1,
		LootText: "3–4 ngọc bậc II–III", Drops: [2]int{3, 4}, Tiers: [2]int{2, 3}, HighPct: 25},
	{ID: "thuyquai", Name: "Thuỷ Quái", Level: BossVeryHard, HPPer: 8000, Dmg: 800, R: 75, Size: 2.5, Noise: 2.5, Coins: 400,
		Shots: 2, LootText: "3–5 ngọc bậc II–III (một nửa bậc III)", Drops: [2]int{3, 5}, Tiers: [2]int{2, 3}, HighPct: 50},
	{ID: "mavuong", Name: "Ma Vương", Level: BossHell, HPPer: 10000, Dmg: 1000, R: 85, Size: 2.8, Noise: 2, Coins: 800,
		Shots: 2, Skill: SkillRain, LootText: "4–6 ngọc, gần hết bậc III", Drops: [2]int{4, 6}, Tiers: [2]int{2, 3}, HighPct: 90},
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
		Size: r.bossDef.Size, Gear: Gear{Arm: ArmCannon}, Fixed: true}
	r.bossTurns = 0
}

// bossTurn: hết một vòng người chơi thì boss ra đòn: Shots phát (mỗi phát một mục tiêu khác nhau nếu có),
// lượt thứ RainEvery của boss có Skill "rain" thì thêm Mưa lửa. Các phát gửi lần lượt từng tin shot (by:"boss"),
// phát sau chờ phát trước bay xong như giữa hai lượt.
func (r *Room) bossTurn() {
	r.turn = BossID
	r.moveLeft = 0
	r.wind = r.field.Wind(r.rng)
	r.bossTurns++
	r.stopTimer()
	gen := r.timerGen
	r.timer = time.AfterFunc(r.opts.BotDelay, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying || r.turn != BossID {
			return
		}
		r.bossVolley(0, map[string]bool{})
	})
	r.broadcast()
}

// bossVolley bắn phát thứ i của lượt boss; aimed là những người đã bị nhắm trong lượt này.
func (r *Room) bossVolley(i int, aimed map[string]bool) {
	shots := max(1, r.bossDef.Shots)
	rain := r.bossDef.Skill == SkillRain && r.bossTurns%RainEvery == 0
	var dur float64
	if i < shots {
		dur = r.bossFire(aimed)
	} else {
		dur = r.bossRain()
	}
	r.turn = ""
	if r.checkEnd("last") {
		return
	}
	r.broadcast()
	more := i+1 < shots || (i+1 == shots && rain)
	r.afterShot(dur, func() {
		if more {
			r.turn = BossID
			r.bossVolley(i+1, aimed)
			return
		}
		r.round++
		if r.round > r.opts.maxRounds() {
			r.round = r.opts.maxRounds()
			r.finishBoss(false, "rounds")
			return
		}
		r.passTurn(-1)
	})
}

// bossFire bắn một phát vào người gần nhất hoặc ngẫu nhiên (ưu tiên người chưa bị nhắm trong lượt); trả thời gian bay.
func (r *Room) bossFire(aimed map[string]bool) float64 {
	b := r.boss
	var pool []*Player
	for _, p := range r.alive() {
		if !aimed[p.ID] {
			pool = append(pool, p)
		}
	}
	if len(pool) == 0 {
		pool = r.alive()
	}
	if len(pool) == 0 {
		return 0
	}
	var target *Fighter
	var tp *Player
	if r.rng.IntN(2) == 0 {
		tp = pool[r.rng.IntN(len(pool))]
	} else {
		for _, p := range pool {
			if tp == nil || math.Abs(p.f.X-b.X) < math.Abs(tp.f.X-b.X) {
				tp = p
			}
		}
	}
	target = tp.f
	aimed[tp.ID] = true
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
	w := r.bossWeapon()
	res := Fire(r.ground, fs, b, w, bestA, bestP, r.wind, nil)
	rage := r.applyRage(nil, w, res) // người trúng đòn boss được cộng nộ
	r.sendAll(map[string]any{"t": "shot", "by": BossID, "weapon": w.ID, "arm": "", "angle": bestA, "power": bestP,
		"wind": r.wind, "paths": res.Paths, "blasts": orEmpty(res.Blasts), "hits": orEmpty(res.Hits),
		"deaths": orEmpty(res.Deaths), "tele": res.Tele, "pushes": orEmpty(res.Pushes), "rage": rage})
	return res.Duration
}

func (r *Room) bossWeapon() Weapon {
	return Weapon{ID: "boss_" + r.bossDef.ID, Name: r.bossDef.Name, Dmg: r.bossDef.Dmg, R: r.bossDef.R, Spread: []float64{0}, Uses: -1}
}

// bossRain: Mưa lửa — mỗi người chơi còn sống bị một quả cầu lửa rơi thẳng từ trời xuống, RainDmg (qua giáp / Def).
// Tin shot có "skill":"rain", mỗi người một đường bay thẳng đứng; trả thời gian rơi.
func (r *Room) bossRain() float64 {
	alive := r.alive()
	fs := r.fighters()
	w := r.bossWeapon()
	res := RainFire(r.ground, fs, r.boss, alive)
	rage := r.applyRage(nil, w, res)
	r.sendAll(map[string]any{"t": "shot", "by": BossID, "weapon": w.ID, "arm": "", "skill": SkillRain,
		"skillName": "Mưa lửa", "angle": 270, "power": 0, "wind": r.wind, "paths": res.Paths, "blasts": orEmpty(res.Blasts),
		"hits": orEmpty(res.Hits), "deaths": orEmpty(res.Deaths), "tele": nil, "pushes": []Push{}, "rage": rage})
	return res.Duration
}

// RainFire thả cầu lửa thẳng xuống từng người trong targets; sửa địa hình và máu ngay tại chỗ.
func RainFire(g Ground, fs []*Fighter, boss *Fighter, targets []*Player) ShotResult {
	res := ShotResult{}
	const top, frames = -120.0, 30 // rơi trong 1 giây (30 điểm cách nhau 1/30 s)
	dmg := map[string]int{}
	for i, p := range targets {
		t := p.f
		cx, cy := Body{X: t.X, Y: t.Y, Size: t.Size}.center()
		path := make([][2]int, 0, frames+1)
		for k := 0; k <= frames; k++ {
			y := top + (cy-top)*float64(k)/frames
			path = append(path, [2]int{int(math.Round(cx)), int(math.Round(y))})
		}
		res.Paths = append(res.Paths, path)
		res.Blasts = append(res.Blasts, Blast{X: int(math.Round(cx)), Y: int(math.Round(cy)), R: RainR, At: frames, Path: i})
		Crater(g, cx, cy, RainR)
		dmg[t.ID] += Splash(RainDmg, RainR, 0, 1, t.Gear.ArmorPct())
	}
	res.Duration = 1
	for _, t := range fs {
		if !t.Alive || t == boss {
			continue
		}
		if n := dmg[t.ID]; n > 0 {
			t.HP = max(0, t.HP-n)
			res.Hits = append(res.Hits, Hit{ID: t.ID, Dmg: n, HP: t.HP})
		}
		t.Y = float64(g.At(t.X))
		if t.HP <= 0 || t.Y >= AbyssY {
			t.Alive, t.HP = false, 0
			res.Deaths = append(res.Deaths, t.ID)
		}
	}
	return res
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
	res := &Result{Reason: reason, Money: map[string]int64{}, Tax: map[string]int64{}, XP: map[string]int64{}, Victory: win}
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

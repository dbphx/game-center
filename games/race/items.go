package race

// Chế độ đạo cụ: hộp "?" trên đường, nhặt được tối đa 2 món, bắn / thả vào đối thủ.
//
// Server là nguồn đúng cho mọi thứ liên quan tới đạo cụ: hộp bật / tắt, món nhận được, đạn bay, dầu, trúng hay trượt.
// Client chỉ báo "pickup" khi chạy qua hộp và "use"; bị trúng thì client tự cho xe xoay và chậm lại.

import (
	"errors"
	"math"
	"slices"
)

// Chế độ đua.
const (
	ModeSpeed = "speed" // chỉ đua tốc độ, không có đạo cụ
	ModeItems = "items" // có hộp đạo cụ
)

// Đạo cụ.
const (
	ItemMissile = "missile" // bay dọc đường về trước
	ItemHoming  = "homing"  // đuổi xe đứng ngay trên mình một hạng
	ItemOil     = "oil"     // vũng dầu thả sau xe
	ItemShield  = "shield"  // chặn 1 đòn
	ItemBoost   = "boost"   // tăng tốc ngắn
)

const (
	maxHeld       = 2     // giữ tối đa 2 món
	boxRespawnMs  = 3000  // hộp tắt 3 s sau khi bị nhặt
	pickupRange   = 6.0   // m: xe phải ở gần hộp chừng này (theo vị trí server biết)
	hitRange      = 3.0   // m: đạn / dầu cách xe chừng này là trúng
	missileSpeed  = 70.0  // m/s
	missileLifeMs = 3000  //
	homingSpeed   = 55.0  // m/s
	homingLifeMs  = 6000  //
	oilBehind     = 4.0   // m sau xe
	oilLifeMs     = 20000 //
	oilSafeMs     = 1500  // người thả không dính dầu của mình trong chừng này
	shieldMs      = 5000
	boostMs       = 1500
	boostMul      = 1.25 // tăng tốc ×1,25 tốc độ tối đa
	spinMs        = 1200 // bị trúng: xoay 1,2 s
	spinSpeed     = 0.3  // tốc độ còn 30%
	fxStepMs      = 50   // mô phỏng đạn / dầu 20 lần mỗi giây
	hitGraceMs    = 1500 // sau khi trúng, nới kiểm dịch chuyển trong chừng này
	hitSlack      = 4.0  // m cộng thêm khi nới kiểm
)

// box là một hộp đạo cụ trên đường.
type box struct {
	pt       Point
	offUntil int64 // ms; 0 hoặc đã qua = đang bật
	active   bool  // trạng thái đã báo cho phòng lần cuối
}

// BoxView là một hộp trong tin "room".
type BoxView struct {
	ID     int     `json:"id"`
	X      float64 `json:"x"`
	Z      float64 `json:"z"`
	Active bool    `json:"active"`
}

// fx là một đạn hoặc vũng dầu đang có trên đường.
type fx struct {
	id      int
	kind    string
	owner   int // ghế người dùng
	target  int // homing: ghế bị đuổi, -1 nếu không có
	pos     Point
	rot     float64
	s       float64 // missile: quãng đường trên đường mẫu
	bornMs  int64
	untilMs int64
	dead    bool
}

// FxView là tin "fx" gửi cho phòng khi đạn / dầu được tạo, di chuyển hoặc biến mất.
type FxView struct {
	T     string  `json:"t"`
	ID    int     `json:"id"`
	Kind  string  `json:"kind"`
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Rot   float64 `json:"rot"`
	Owner int     `json:"owner"`
	Dead  bool    `json:"dead"`
}

func (f *fx) view() FxView {
	return FxView{T: "fx", ID: f.id, Kind: f.kind, X: f.pos.X, Z: f.pos.Z, Rot: f.rot, Owner: f.owner, Dead: f.dead}
}

func validMode(m string) bool { return m == ModeSpeed || m == ModeItems }

// resetItems chuẩn bị hộp và xoá đạo cụ khi bắt đầu cuộc đua.
func (r *Room) resetItems() {
	r.boxes = nil
	r.fx = nil
	r.fxAtMs = 0
	if r.mode == ModeItems && r.track != nil {
		for _, b := range r.track.ItemBoxes {
			r.boxes = append(r.boxes, &box{pt: b, active: true})
		}
	}
	for _, p := range r.players {
		p.items = nil
		p.shieldUntil, p.boostUntil, p.spinUntil, p.hitAtMs, p.botUseAt, p.used = 0, 0, 0, 0, 0, 0
	}
}

func (r *Room) boxViews() []BoxView {
	out := []BoxView{}
	for i, b := range r.boxes {
		out = append(out, BoxView{ID: i, X: b.pt.X, Z: b.pt.Z, Active: b.active})
	}
	return out
}

// racers là các xe đang đua (chưa về đích, chưa bỏ cuộc) xếp theo hạng hiện tại: dẫn đầu trước.
func (r *Room) racers() []*Player {
	out := []*Player{}
	for _, p := range r.players {
		if !p.prog.finished && !p.dnf {
			out = append(out, p)
		}
	}
	t := r.track
	slices.SortStableFunc(out, func(a, b *Player) int {
		sa, sb := a.prog.score(t), b.prog.score(t)
		switch {
		case sa > sb:
			return -1
		case sa < sb:
			return 1
		}
		return 0
	})
	return out
}

// rankOf là hạng của p trong số xe đang đua (1 = dẫn đầu) và số xe đang đua; xe đã về đích đứng trên hết.
func (r *Room) rankOf(p *Player) (int, int) {
	list := r.racers()
	done := r.finishes
	return done + slices.Index(list, p) + 1, done + len(list)
}

// itemWeights là tỉ lệ ra từng món theo vị trí: hạng sau dễ ra tên lửa đuổi / tăng tốc, hạng nhất chủ yếu dầu / khiên.
func itemWeights(rank, total int) map[string]int {
	switch {
	case rank <= 1:
		return map[string]int{ItemOil: 45, ItemShield: 35, ItemMissile: 15, ItemBoost: 5}
	case rank >= total:
		return map[string]int{ItemHoming: 35, ItemBoost: 35, ItemMissile: 20, ItemShield: 10}
	}
	return map[string]int{ItemMissile: 35, ItemOil: 20, ItemBoost: 20, ItemShield: 15, ItemHoming: 10}
}

var itemOrder = []string{ItemMissile, ItemHoming, ItemOil, ItemShield, ItemBoost}

// rollItem chọn một món theo tỉ lệ của hạng; roll ∈ [0, 1).
func rollItem(rank, total int, roll float64) string {
	w := itemWeights(rank, total)
	sum := 0
	for _, k := range itemOrder {
		sum += w[k]
	}
	x := int(roll * float64(sum))
	for _, k := range itemOrder {
		if x < w[k] {
			return k
		}
		x -= w[k]
	}
	return ItemMissile
}

// sendItems báo riêng người chơi các món đang giữ.
func (r *Room) sendItems(p *Player) {
	if p.conn != nil {
		list := p.items
		if list == nil {
			list = []string{}
		}
		p.conn.Send(map[string]any{"t": "items", "list": list})
	}
}

func (r *Room) itemsReady(p *Player) error {
	switch {
	case r.mode != ModeItems:
		return errors.New("chế độ Tốc độ không có đạo cụ")
	case r.phase != PhaseRacing || r.nowMs() < r.startAt.UnixMilli():
		return errors.New("chưa xuất phát")
	case p.prog.finished || p.dnf:
		return errors.New("bạn đã dừng đua")
	}
	return nil
}

// pickup: người chơi báo chạy qua hộp id. Server kiểm hộp đang bật, xe ở gần, còn chỗ giữ.
func (r *Room) pickup(p *Player, id int) error {
	if err := r.itemsReady(p); err != nil {
		return err
	}
	if id < 0 || id >= len(r.boxes) {
		return errors.New("không có hộp này")
	}
	b := r.boxes[id]
	now := r.nowMs()
	switch {
	case now < b.offUntil:
		return errors.New("hộp chưa có lại")
	case math.Hypot(b.pt.X-p.prog.pos.X, b.pt.Z-p.prog.pos.Z) > pickupRange:
		return errors.New("xe ở xa hộp")
	case len(p.items) >= maxHeld:
		return errors.New("đã giữ đủ 2 món")
	}
	r.takeBox(p, id, now)
	return nil
}

// takeBox cho p nhận một món ngẫu nhiên từ hộp id rồi tắt hộp.
func (r *Room) takeBox(p *Player, id int, now int64) {
	b := r.boxes[id]
	rank, total := r.rankOf(p)
	p.items = append(p.items, rollItem(rank, total, r.rng.Float64()))
	b.offUntil, b.active = now+boxRespawnMs, false
	r.sendItems(p)
	r.sendAll(map[string]any{"t": "box", "id": id, "active": false})
	if p.Bot && p.botUseAt == 0 {
		p.botUseAt = now + 1000 + int64(r.rng.IntN(3000)) // máy dùng món sau 1–4 s
	}
}

// use dùng món đầu tiên đang giữ.
func (r *Room) use(p *Player) error {
	if err := r.itemsReady(p); err != nil {
		return err
	}
	if len(p.items) == 0 {
		return errors.New("không có đạo cụ")
	}
	kind := p.items[0]
	p.items = p.items[1:]
	r.fire(p, kind, r.nowMs())
	r.sendItems(p)
	return nil
}

// fire thực hiện món kind của p.
func (r *Room) fire(p *Player, kind string, now int64) {
	seat := r.seatOf(p)
	t := r.track
	p.used++
	switch kind {
	case ItemShield:
		p.shieldUntil = now + shieldMs
		return
	case ItemBoost:
		p.boostUntil = now + boostMs
		return
	}
	s := t.nearestS(p.prog.pos)
	r.fxSeq++
	f := &fx{id: r.fxSeq, kind: kind, owner: seat, target: -1, bornMs: now}
	switch kind {
	case ItemMissile, ItemHoming:
		f.s = s
		f.pos, f.rot = t.at(s)
		if kind == ItemMissile {
			f.untilMs = now + missileLifeMs
		} else {
			f.untilMs = now + homingLifeMs
			if tg := r.targetAbove(p); tg != nil {
				f.target = r.seatOf(tg)
			}
		}
	case ItemOil:
		f.pos, f.rot = t.at(s - oilBehind)
		f.untilMs = now + oilLifeMs
	default:
		return
	}
	if r.fxAtMs == 0 {
		r.fxAtMs = now
	}
	r.fx = append(r.fx, f)
	r.sendAll(f.view())
}

// targetAbove là xe đang đua đứng ngay trên p một hạng (nil nếu p dẫn đầu).
func (r *Room) targetAbove(p *Player) *Player {
	list := r.racers()
	if i := slices.Index(list, p); i > 0 {
		return list[i-1]
	}
	return nil
}

// stepFx mô phỏng đạn / dầu tới nowMs theo bước 50 ms, kiểm trúng, bật lại hộp đã hết giờ tắt.
func (r *Room) stepFx(nowMs int64) {
	for i, b := range r.boxes {
		if !b.active && nowMs >= b.offUntil {
			b.active = true
			r.sendAll(map[string]any{"t": "box", "id": i, "active": true})
		}
	}
	if len(r.fx) == 0 {
		r.fxAtMs = nowMs
		return
	}
	if r.fxAtMs == 0 || r.fxAtMs > nowMs {
		r.fxAtMs = nowMs
	}
	moved := map[*fx]bool{}
	for r.fxAtMs < nowMs {
		h := min(int64(fxStepMs), nowMs-r.fxAtMs)
		r.fxAtMs += h
		dt := float64(h) / 1000
		for _, f := range r.fx {
			if f.dead {
				continue
			}
			r.moveFx(f, dt)
			moved[f] = true
			if r.fxAtMs >= f.untilMs {
				f.dead = true
				continue
			}
			r.checkHit(f, r.fxAtMs)
		}
	}
	for _, f := range r.fx {
		if f.dead || (moved[f] && f.kind != ItemOil) {
			r.sendAll(f.view())
		}
	}
	r.fx = slices.DeleteFunc(r.fx, func(f *fx) bool { return f.dead })
}

// moveFx: tên lửa bay dọc đường mẫu; tên lửa đuổi bay thẳng về xe bị đuổi; dầu đứng yên.
func (r *Room) moveFx(f *fx, dt float64) {
	switch f.kind {
	case ItemMissile:
		f.s += missileSpeed * dt
		f.pos, f.rot = r.track.at(f.s)
	case ItemHoming:
		var tg *Player
		if f.target >= 0 && f.target < len(r.players) {
			tg = r.players[f.target]
		}
		if tg == nil || tg.prog.finished || tg.dnf {
			// Không còn ai để đuổi: bay dọc đường như tên lửa thường.
			f.s = r.track.nearestS(f.pos) + homingSpeed*dt
			f.pos, f.rot = r.track.at(f.s)
			return
		}
		dx, dz := tg.prog.pos.X-f.pos.X, tg.prog.pos.Z-f.pos.Z
		d := math.Hypot(dx, dz)
		step := homingSpeed * dt
		if d <= step {
			f.pos = tg.prog.pos
		} else {
			f.pos = Point{f.pos.X + dx/d*step, f.pos.Z + dz/d*step}
		}
		f.rot = math.Atan2(dz, dx)
	}
}

// checkHit: đạn / dầu cách một xe ≤ 3 m là trúng (không trúng người bắn; tên lửa đuổi chỉ trúng xe bị đuổi).
func (r *Room) checkHit(f *fx, nowMs int64) {
	for seat, p := range r.players {
		if p.prog.finished || p.dnf {
			continue
		}
		if seat == f.owner && (f.kind != ItemOil || nowMs-f.bornMs < oilSafeMs) {
			continue
		}
		if f.kind == ItemHoming && f.target >= 0 && seat != f.target {
			continue
		}
		if math.Hypot(p.prog.pos.X-f.pos.X, p.prog.pos.Z-f.pos.Z) > hitRange {
			continue
		}
		f.dead = true
		r.hit(p, seat, f.owner, f.kind, nowMs)
		return
	}
}

// hit: xe ở ghế seat bị món kind của ghế by trúng. Có khiên thì khiên chặn và mất khiên.
func (r *Room) hit(p *Player, seat, by int, kind string, nowMs int64) {
	if nowMs < p.shieldUntil {
		p.shieldUntil = 0
		r.sendAll(map[string]any{"t": "hit", "seat": seat, "by": by, "kind": kind, "blocked": true})
		return
	}
	p.spinUntil = nowMs + spinMs
	p.hitAtMs = nowMs
	if p.Bot {
		p.botV *= spinSpeed
	}
	r.sendAll(map[string]any{"t": "hit", "seat": seat, "by": by, "kind": kind})
}

// botItems: máy nhặt hộp khi chạy qua và dùng món sau vài giây.
func (r *Room) botItems(p *Player, nowMs int64) {
	if r.mode != ModeItems || p.prog.finished || p.dnf {
		return
	}
	if len(p.items) < maxHeld {
		for i, b := range r.boxes {
			if nowMs >= b.offUntil && math.Hypot(b.pt.X-p.prog.pos.X, b.pt.Z-p.prog.pos.Z) <= pickupRange {
				r.takeBox(p, i, nowMs)
				break
			}
		}
	}
	if len(p.items) > 0 && p.botUseAt > 0 && nowMs >= p.botUseAt {
		kind := p.items[0]
		p.items = p.items[1:]
		r.fire(p, kind, nowMs)
		p.botUseAt = 0
		if len(p.items) > 0 {
			p.botUseAt = nowMs + 1000 + int64(r.rng.IntN(3000))
		}
	}
}

// itemSpeedCap là hệ số trần tốc độ khi kiểm gói: nitro, cộng thêm tăng tốc nếu vừa dùng boost.
func (p *Player) itemSpeedCap(nowMs int64) float64 {
	c := NitroBoost
	if nowMs < p.boostUntil+500 {
		c *= boostMul
	}
	return c
}

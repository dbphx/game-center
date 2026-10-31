package arena

// Vật lý Đấu trường: địa hình dạng "bản đồ độ cao", đạn bay theo góc, lực, trọng lực và gió,
// nổ khoét hố và gây sát thương giảm dần theo khoảng cách. Toàn bộ là hàm thuần (không khoá, không giờ),
// server dùng để tính phát bắn thật và máy dùng để ngắm thử.

import "math"

const (
	Width  = 1600 // chiều ngang thế giới (đơn vị)
	Height = 900  // chiều cao; ground[x] = Height là vực (không còn đất)

	Gravity   = 400.0 // u/s²
	PowerMul  = 9.0   // vận tốc bắn = lực × 9
	WindMul   = 12.0  // gia tốc ngang = gió × 12
	StepDt    = 1.0 / 60
	MaxFlight = 8.0 // giây
	PathEvery = 2   // ghi một điểm quỹ đạo mỗi 2 bước (1/30 s)

	BodyRadius = 18.0 // bán kính thân người chơi, tâm ở (x, y−16)
	BodyLift   = 16.0
	Muzzle     = 30.0 // đạn ra ở đầu nòng, cách tâm thân 30
	SelfSafe   = 0.25 // giây đầu không tự trúng thân mình
	SplashPad  = 20.0 // sát thương lan tới r + 20

	MinX     = 20.0
	MaxX     = 1580.0
	AbyssY   = 890 // mặt đất từ đây trở xuống là vực: đứng lên là rơi chết
	MoveFund = 120 // quỹ di chuyển mỗi lượt (đơn vị ngang)
	MaxClimb = 6   // dốc lên tối đa mỗi đơn vị ngang
)

// Ground là độ cao mặt đất theo từng cột x (0..Width−1); y hướng xuống.
type Ground []int

func (g Ground) Clone() Ground { return append(Ground(nil), g...) }

// At là mặt đất tại toạ độ thực x (ngoài thế giới thì coi như vực).
func (g Ground) At(x float64) int {
	i := int(math.Round(x))
	if i < 0 || i >= len(g) {
		return Height
	}
	return g[i]
}

// Body là thân một người chơi trên sân (để đạn va vào).
type Body struct {
	ID   string
	X    float64
	Y    float64 // mặt đất dưới chân
	Size float64 // hệ số thân (0 = 1; boss thân to hơn)
}

func (b Body) size() float64 {
	if b.Size > 0 {
		return b.Size
	}
	return 1
}

func (b Body) center() (float64, float64) { return b.X, b.Y - BodyLift*b.size() }

// Flight là kết quả bay của một viên đạn.
type Flight struct {
	Path [][2]int // điểm cách nhau 1/30 s; điểm cuối là chỗ nổ (nếu trúng)
	Hit  bool     // nổ (chạm đất hoặc chạm người); false = bay ra ngoài / hết giờ
	X, Y float64  // chỗ nổ
	Body string   // id người bị đạn chạm trực tiếp ("" nếu chạm đất)
	Time float64  // thời gian bay (giây)
}

// Launch là điểm đầu nòng và vận tốc ban đầu cho người đứng ở (x, y) bắn góc angle (độ), lực power (0..100).
func Launch(x, y, angle, power float64) (px, py, vx, vy float64) {
	a := angle * math.Pi / 180
	cx, cy := x, y-BodyLift
	v := power * PowerMul
	return cx + math.Cos(a)*Muzzle, cy - math.Sin(a)*Muzzle, v * math.Cos(a), -v * math.Sin(a)
}

// Simulate cho đạn bay từ người bắn (shooter, đứng ở x, y) theo góc / lực / gió trên địa hình g.
// bodies là thân mọi người còn sống (kể cả người bắn: chỉ tự trúng sau SelfSafe giây).
func Simulate(g Ground, x, y, angle, power float64, wind int, shooter string, bodies []Body) Flight {
	px, py, vx, vy := Launch(x, y, angle, power)
	ax := float64(wind) * WindMul
	f := Flight{Path: [][2]int{{int(math.Round(px)), int(math.Round(py))}}}
	steps := int(MaxFlight / StepDt)
	for i := 1; i <= steps; i++ {
		vx += ax * StepDt
		vy += Gravity * StepDt
		px += vx * StepDt
		py += vy * StepDt
		t := float64(i) * StepDt
		if px < -300 || px > Width+300 || py > Height+100 {
			f.Time = t
			f.Path = append(f.Path, [2]int{int(math.Round(px)), int(math.Round(py))})
			return f
		}
		hit, body := false, ""
		for _, b := range bodies {
			if b.ID == shooter && t < SelfSafe {
				continue
			}
			cx, cy := b.center()
			if math.Hypot(px-cx, py-cy) < BodyRadius*b.size() {
				hit, body = true, b.ID
				break
			}
		}
		if !hit && px >= 0 && px < Width && py >= float64(g.At(px)) {
			hit = true
			py = math.Min(py, float64(g.At(px)))
		}
		if hit {
			f.Hit, f.X, f.Y, f.Body, f.Time = true, px, py, body, t
			f.Path = append(f.Path, [2]int{int(math.Round(px)), int(math.Round(py))})
			return f
		}
		if i%PathEvery == 0 {
			f.Path = append(f.Path, [2]int{int(math.Round(px)), int(math.Round(py))})
		}
	}
	f.Time = MaxFlight
	return f
}

// Crater khoét hố tròn tâm (cx, cy) bán kính r: mặt đất chỉ bị hạ xuống, không bao giờ nâng lên.
func Crater(g Ground, cx, cy, r float64) {
	if r <= 0 {
		return
	}
	for x := int(math.Ceil(cx - r)); x <= int(math.Floor(cx+r)); x++ {
		if x < 0 || x >= len(g) {
			continue
		}
		dx := float64(x) - cx
		h := math.Sqrt(math.Max(0, r*r-dx*dx))
		bottom := int(math.Round(math.Min(Height, cy+h)))
		if bottom > g[x] {
			g[x] = bottom
		}
	}
}

// Splash là sát thương một vụ nổ (dmg, bán kính r) gây ra cho người có tâm thân cách tâm nổ d,
// nhân hệ số súng người bắn (gunMul ≥ 1) và giáp người trúng (armorPct 0..1). 0 khi ngoài tầm.
func Splash(dmg, r, d, gunMul, armorPct float64) int {
	reach := r + SplashPad
	if dmg <= 0 || d >= reach {
		return 0
	}
	v := dmg * (1 - d/reach) * gunMul * (1 - armorPct)
	return int(math.Round(v))
}

// Walk đi từ x về phía target trên địa hình g với quỹ budget: từng bước 1 đơn vị, dừng khi dốc lên quá MaxClimb,
// sắp bước xuống vực, hoặc hết quỹ. Trả vị trí mới và số đơn vị đã đi.
func Walk(g Ground, x, target float64, budget int) (float64, int) {
	target = math.Max(MinX, math.Min(MaxX, target))
	cur := int(math.Round(x))
	dir := 1
	if target < x {
		dir = -1
	}
	used := 0
	for used < budget {
		next := cur + dir
		if (dir > 0 && float64(next) > target) || (dir < 0 && float64(next) < target) {
			break
		}
		if next < int(MinX) || next > int(MaxX) {
			break
		}
		if g[next] >= AbyssY || g[cur]-g[next] > MaxClimb {
			break
		}
		cur = next
		used++
	}
	if used == 0 {
		return x, 0
	}
	return float64(cur), used
}

package arena

import (
	"math"
	mrand "math/rand/v2"
)

// Map là một sân đấu: tên, biên độ gió và cách sinh địa hình.
type Map struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	wind  int     // gió ngẫu nhiên trong −wind..wind
	base  float64 // độ cao nền (y)
	amp   float64 // biên độ đồi lớn
	rough float64 // biên độ gồ ghề nhỏ
	hill  float64 // núi giữa (âm = cao lên), 0 = không có
	water bool    // vũng nước sâu giữa sân (vực)
}

const MapRandom = "random"

var Maps = []Map{
	{ID: "meadow", Name: "Đồng cỏ", wind: 5, base: 640, amp: 70, rough: 6},
	{ID: "desert", Name: "Sa mạc", wind: 10, base: 660, amp: 90, rough: 3},
	{ID: "snow", Name: "Núi tuyết", wind: 8, base: 600, amp: 150, rough: 10},
	{ID: "volcano", Name: "Núi lửa", wind: 6, base: 650, amp: 80, rough: 22, hill: -230},
	{ID: "beach", Name: "Bãi biển", wind: 7, base: 620, amp: 60, rough: 5, water: true},
}

func findMap(id string) (Map, bool) {
	for _, m := range Maps {
		if m.ID == id {
			return m, true
		}
	}
	return Map{}, false
}

// MapInfo là map gửi cho giao diện.
type MapInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func mapList() []MapInfo {
	out := make([]MapInfo, 0, len(Maps))
	for _, m := range Maps {
		out = append(out, MapInfo{m.ID, m.Name})
	}
	return out
}

// Wind là gió ngẫu nhiên của một lượt trên map này.
func (m Map) Wind(rng *mrand.Rand) int { return rng.IntN(2*m.wind+1) - m.wind }

// Generate sinh địa hình mới cho map: tổng vài sóng sin ngẫu nhiên + gồ ghề nhỏ (+ núi giữa / vũng nước).
func (m Map) Generate(rng *mrand.Rand) Ground {
	g := make(Ground, Width)
	type wave struct{ f, ph, a float64 }
	waves := []wave{
		{1 + rng.Float64(), rng.Float64() * 2 * math.Pi, m.amp},
		{2.5 + rng.Float64()*1.5, rng.Float64() * 2 * math.Pi, m.amp * 0.45},
		{6 + rng.Float64()*4, rng.Float64() * 2 * math.Pi, m.rough},
		{15 + rng.Float64()*10, rng.Float64() * 2 * math.Pi, m.rough * 0.5},
	}
	for x := range g {
		t := float64(x) / Width
		y := m.base
		for _, w := range waves {
			y += w.a * math.Sin(2*math.Pi*w.f*t+w.ph)
		}
		if m.hill != 0 { // núi giữa hình chuông
			d := (t - 0.5) / 0.14
			y += m.hill * math.Exp(-d*d)
		}
		if m.water { // bờ dốc xuống vũng nước sâu ở giữa
			d := math.Abs(t-0.5) * Width // khoảng cách tới giữa sân
			switch {
			case d < 70:
				y = Height
			case d < 130:
				y += (130 - d) / 60 * 260
			}
		}
		g[x] = int(math.Round(math.Max(240, math.Min(Height, y))))
	}
	if !m.water { // không để vực ngẫu nhiên trên sân thường
		for x := range g {
			g[x] = min(g[x], 860)
		}
	}
	return g
}

// spawnXs chia chỗ đứng đầu trận cho n người: rải đều, lệch ngẫu nhiên, tránh vực.
func spawnXs(g Ground, n int, rng *mrand.Rand) []float64 { return spawnIn(g, n, 0, Width, rng) }

// spawnIn chia n chỗ đứng đều trong khoảng [lo, hi] của sân.
func spawnIn(g Ground, n int, lo, hi float64, rng *mrand.Rand) []float64 {
	xs := make([]float64, n)
	for i := range n {
		x := lo + (hi-lo)*(float64(i)+0.5)/float64(n) + float64(rng.IntN(121)-60)
		x = math.Max(80, math.Min(Width-80, x))
		// Tránh vực: dịch ra xa giữa sân cho tới chỗ có đất.
		dir := 1.0
		if x < Width/2 {
			dir = -1
		}
		for g.At(x) >= AbyssY-20 && x > MinX && x < MaxX {
			x += dir * 10
		}
		xs[i] = math.Round(x)
	}
	rng.Shuffle(n, func(i, j int) { xs[i], xs[j] = xs[j], xs[i] })
	return xs
}

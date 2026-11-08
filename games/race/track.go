package race

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
)

// ---------- Đường đua: đọc từ public/race/tracks/<id>.json, dùng chung với giao diện ----------

type Point struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
}

type Spawn struct {
	X   float64 `json:"x"`
	Z   float64 `json:"z"`
	Rot float64 `json:"rot"`
}

type Checkpoint struct {
	X float64 `json:"x"`
	Z float64 `json:"z"`
	R float64 `json:"r"`
}

// Track là một đường đua. Pieces (khối dựng đường) chỉ giao diện dùng, server bỏ qua.
type Track struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Laps        int          `json:"laps"`
	MinLapMs    int64        `json:"minLapMs"`
	Spawn       []Spawn      `json:"spawn"`
	Checkpoints []Checkpoint `json:"checkpoints"` // theo thứ tự, cái cuối là vạch đích
	Path        []Point      `json:"path"`        // đường mẫu cho máy đua, khép kín
	ItemBoxes   []Point      `json:"itemBoxes"`   // hộp đạo cụ (chế độ items); thiếu = không có hộp

	cum    []float64 // độ dài cộng dồn tới từng điểm của Path
	length float64   // chu vi đường mẫu
}

// TrackInfo là tên đường để chọn ở sảnh.
type TrackInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Laps int    `json:"laps"`
}

// LoadTracks đọc mọi file *.json trong fsys; file hỏng thì báo lỗi kèm tên file.
func LoadTracks(fsys fs.FS) (map[string]*Track, error) {
	tracks := map[string]*Track{}
	if fsys == nil {
		return tracks, nil
	}
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var t Track
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("đường %s: %w", name, err)
		}
		if t.ID == "" {
			t.ID = name[:len(name)-len(path.Ext(name))]
		}
		if err := t.prepare(); err != nil {
			return nil, fmt.Errorf("đường %s: %w", name, err)
		}
		tracks[t.ID] = &t
	}
	return tracks, nil
}

func (t *Track) prepare() error {
	switch {
	case t.Laps < 1:
		return errors.New("cần ít nhất 1 vòng")
	case len(t.Checkpoints) < 2:
		return errors.New("cần ít nhất 2 điểm kiểm tra")
	case len(t.Path) < 3:
		return errors.New("đường mẫu cần ít nhất 3 điểm")
	case len(t.Spawn) == 0:
		return errors.New("thiếu vị trí xuất phát")
	}
	for i := range t.Checkpoints {
		if t.Checkpoints[i].R <= 0 {
			t.Checkpoints[i].R = 12
		}
	}
	t.cum = make([]float64, len(t.Path)+1)
	for i := range t.Path {
		a, b := t.Path[i], t.Path[(i+1)%len(t.Path)]
		t.cum[i+1] = t.cum[i] + math.Hypot(b.X-a.X, b.Z-a.Z)
	}
	t.length = t.cum[len(t.Path)]
	if t.length <= 0 {
		return errors.New("đường mẫu có độ dài 0")
	}
	return nil
}

// spawnOf là vị trí xuất phát của ghế seat; ít vị trí hơn số ghế thì xếp lùi dần sau vị trí cuối.
func (t *Track) spawnOf(seat int) Spawn {
	if seat < len(t.Spawn) {
		return t.Spawn[seat]
	}
	s := t.Spawn[len(t.Spawn)-1]
	back := float64(seat-len(t.Spawn)+1) * 5
	s.X -= math.Cos(s.Rot) * back
	s.Z -= math.Sin(s.Rot) * back
	return s
}

// at trả điểm và hướng (radian, atan2(dz, dx)) trên đường mẫu ở quãng đường s (tính vòng).
func (t *Track) at(s float64) (Point, float64) {
	s = math.Mod(s, t.length)
	if s < 0 {
		s += t.length
	}
	i := sort.SearchFloat64s(t.cum, s)
	if i > 0 && (i >= len(t.cum) || t.cum[i] > s) {
		i--
	}
	i = min(i, len(t.Path)-1)
	a, b := t.Path[i], t.Path[(i+1)%len(t.Path)]
	seg := t.cum[i+1] - t.cum[i]
	f := 0.0
	if seg > 0 {
		f = (s - t.cum[i]) / seg
	}
	return Point{a.X + (b.X-a.X)*f, a.Z + (b.Z-a.Z)*f}, math.Atan2(b.Z-a.Z, b.X-a.X)
}

// segHits: đoạn a→b có đi qua vòng tròn c không (khoảng cách nhỏ nhất từ tâm tới đoạn ≤ bán kính).
func segHits(a, b Point, c Checkpoint) bool {
	dx, dz := b.X-a.X, b.Z-a.Z
	l2 := dx*dx + dz*dz
	f := 0.0
	if l2 > 0 {
		f = math.Max(0, math.Min(1, ((c.X-a.X)*dx+(c.Z-a.Z)*dz)/l2))
	}
	px, pz := a.X+dx*f, a.Z+dz*f
	return math.Hypot(c.X-px, c.Z-pz) <= c.R
}

// progress là tiến độ một xe do server tự tính: vòng đã xong, điểm kiểm tra kế tiếp, vị trí cuối đã nhận.
type progress struct {
	lap        int   // số vòng đã xong
	cp         int   // chỉ số điểm kiểm tra kế tiếp
	lapStartMs int64 // lúc bắt đầu vòng hiện tại (ms, đồng hồ server)
	pos        Point
	atMs       int64 // lúc nhận vị trí cuối (ms)
	finished   bool
	finishMs   int64 // thời gian về đích tính từ lúc xuất phát
}

// advanceResult là kết quả cập nhật vị trí.
type advanceResult int

const (
	advMoved    advanceResult = iota // đi bình thường
	advLap                           // vừa xong một vòng
	advFinished                      // vừa về đích
	advFastLap                       // qua vạch đích nhanh hơn MinLapMs: không tính
)

// move cập nhật tiến độ khi xe đi từ vị trí cũ tới p lúc nowMs (startMs là lúc xuất phát).
func (pr *progress) move(t *Track, p Point, nowMs, startMs int64) advanceResult {
	from := pr.pos
	pr.pos, pr.atMs = p, nowMs
	if pr.finished {
		return advMoved
	}
	res := advMoved
	// Một gói chỉ đi được vài mét nên mỗi lần xét tối đa 2 điểm kiểm tra liên tiếp.
	for range 2 {
		cp := t.Checkpoints[pr.cp]
		if !segHits(from, p, cp) {
			break
		}
		if pr.cp < len(t.Checkpoints)-1 {
			pr.cp++
			continue
		}
		// Vạch đích.
		if nowMs-pr.lapStartMs < t.MinLapMs {
			return advFastLap
		}
		pr.lap++
		pr.cp = 0
		pr.lapStartMs = nowMs
		res = advLap
		if pr.lap >= t.Laps {
			pr.finished = true
			pr.finishMs = nowMs - startMs
			return advFinished
		}
		break
	}
	return res
}

// score dùng để xếp người chưa về đích: nhiều vòng hơn, qua nhiều điểm hơn, gần điểm kế tiếp hơn thì xếp trước.
func (pr *progress) score(t *Track) float64 {
	cp := t.Checkpoints[pr.cp]
	d := math.Hypot(cp.X-pr.pos.X, cp.Z-pr.pos.Z)
	return float64(pr.lap*len(t.Checkpoints)+pr.cp)*1e6 - d
}

package race

import (
	"encoding/json"
	"log"
	"math"
)

// Số đo từ trận thật: cuối trận (hoặc lúc rời) máy người chơi gửi một bản tóm tắt nhỏ {t:"telemetry", tel:{…}} — FPS,
// khung dài, khựng / giật, độ trễ hiển thị, RTT, gói nhận / trùng / hụt, loại thiết bị. Server chỉ ghi một dòng log
// có cấu trúc (không lưu DB, không tên / IP / user agent), mỗi người một bản mỗi cuộc đua.

const maxTelemetryBytes = 2048

// Telemetry là các trường được nhận; trường lạ bị bỏ, số bị kẹp về khoảng hợp lý.
type Telemetry struct {
	V        int     `json:"v"`
	Dev      string  `json:"dev"` // "m" điện thoại / "d" máy tính
	Hz       float64 `json:"hz"`  // tần số màn hình ước lượng
	DPR      float64 `json:"dpr"`
	Low      bool    `json:"low"`
	Car      string  `json:"car"`
	N        int     `json:"n"`   // số xe trong trận
	Dur      float64 `json:"dur"` // giây đã đo
	FPS50    float64 `json:"fps50"`
	FPS5     float64 `json:"fps5"`
	F50      int     `json:"f50"`  // khung > 50 ms
	F100     int     `json:"f100"` // khung > 100 ms
	MeJerk   float64 `json:"meJerk"`
	MeStall  float64 `json:"meStall"`
	RemJerk  float64 `json:"remJerk"`
	RemStall float64 `json:"remStall"`
	Lag50    float64 `json:"lag50"`
	Lag95    float64 `json:"lag95"`
	RTT50    float64 `json:"rtt50"`
	RTT95    float64 `json:"rtt95"`
	Jit      float64 `json:"jit"`
	Rx       int     `json:"rx"`  // tin "cars" nhận
	Dup      int     `json:"dup"` // mẫu trùng bị bỏ
	Gap      int     `json:"gap"` // khoảng trống giữa hai tin "cars" > 2,5 nhịp
	Big      int     `json:"big"` // lần hiệu chỉnh lớn (xe người khác nhảy > 2 m)
}

// telemetryLog là nơi ghi dòng log (test thay để kiểm).
var telemetryLog = log.Printf

func clampF(v, hi float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	return math.Min(v, hi)
}

func clampI(v, hi int) int { return max(0, min(v, hi)) }

// sanitize kẹp số, chỉ giữ loại thiết bị / xe hợp lệ.
func (t *Telemetry) sanitize() {
	if t.Dev != "m" && t.Dev != "d" {
		t.Dev = "?"
	}
	if _, ok := findCar(t.Car); !ok {
		t.Car = ""
	}
	t.V = clampI(t.V, 100)
	t.N = clampI(t.N, MaxPlayers)
	for _, f := range []*float64{&t.FPS50, &t.FPS5, &t.Hz} {
		*f = clampF(*f, 1000)
	}
	t.DPR = clampF(t.DPR, 10)
	t.Dur = clampF(t.Dur, 3600)
	for _, f := range []*float64{&t.MeJerk, &t.MeStall, &t.RemJerk, &t.RemStall} {
		*f = clampF(*f, 100)
	}
	for _, f := range []*float64{&t.Lag50, &t.Lag95, &t.RTT50, &t.RTT95, &t.Jit} {
		*f = clampF(*f, 60_000)
	}
	for _, n := range []*int{&t.F50, &t.F100, &t.Rx, &t.Dup, &t.Gap, &t.Big} {
		*n = clampI(*n, 10_000_000)
	}
}

// telemetry ghi bản tóm tắt của p cho cuộc đua hiện tại (đang đua hoặc vừa xong). Quá cỡ, sai định dạng, ngoài cuộc đua
// hay đã gửi rồi thì bỏ im lặng — số đo không bao giờ làm hỏng trận.
func (r *Room) telemetry(p *Player, raw json.RawMessage) {
	if p.Bot || len(raw) == 0 || len(raw) > maxTelemetryBytes || r.startAt.IsZero() ||
		(!r.racing() && r.phase != PhaseOver) {
		return
	}
	race := r.startAt.UnixMilli()
	if p.telRace == race {
		return
	}
	var t Telemetry
	if err := json.Unmarshal(raw, &t); err != nil {
		return
	}
	t.sanitize()
	p.telRace = race
	rec := struct {
		Room  string `json:"room"`
		Race  int64  `json:"race"`
		Seat  int    `json:"seat"`
		UID   int64  `json:"uid"`
		Mode  string `json:"mode"`
		Track string `json:"track,omitempty"`
		Telemetry
	}{Room: r.Code, Race: race, Seat: r.seatOf(p), UID: p.UserID, Mode: r.mode, Telemetry: t}
	if r.track != nil {
		rec.Track = r.track.ID
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	telemetryLog("race-telemetry %s", b)
}

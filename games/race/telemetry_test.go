package race

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// captureTelemetry thay nơi ghi log số đo, trả các dòng đã ghi.
func captureTelemetry(t *testing.T) *[]string {
	t.Helper()
	var lines []string
	old := telemetryLog
	telemetryLog = func(format string, a ...any) { lines = append(lines, fmt.Sprintf(format, a...)) }
	t.Cleanup(func() { telemetryLog = old })
	return &lines
}

func tel(s string) Message { return Message{T: "telemetry", Tel: json.RawMessage(s)} }

func TestTelemetryLoggedOncePerRaceAndSanitized(t *testing.T) {
	lines := captureTelemetry(t)
	e := newEnv(t)
	a, _ := e.create(an, 0)
	e.do(a, tel(`{"fps50":60}`)) // chưa đua: bỏ
	if len(*lines) != 0 {
		t.Fatalf("chưa đua thì không ghi: %v", *lines)
	}
	b, _ := e.join(a.Room.Code, binh)
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	e.do(a, tel(`{"v":1,"dev":"m","hz":120,"car":"starter","fps50":59.5,"meJerk":250,"rtt50":-4,"f50":3,"name":"An","ua":"Mozilla/5.0"}`))
	e.do(a, tel(`{"v":1,"dev":"d","fps50":10}`)) // lần hai cùng trận: bỏ
	e.do(b, tel(`{"dev":"pc","car":"xe_la","fps50":30}`))
	if len(*lines) != 2 {
		t.Fatalf("mỗi người một bản mỗi trận, được %d: %v", len(*lines), *lines)
	}
	l := (*lines)[0]
	if !strings.HasPrefix(l, "race-telemetry {") {
		t.Fatalf("dòng log: %s", l)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "race-telemetry ")), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["room"] != a.Room.Code || rec["seat"] != 0.0 || rec["uid"] != 1.0 || rec["dev"] != "m" || rec["car"] != "starter" ||
		rec["fps50"] != 59.5 || rec["meJerk"] != 100.0 || rec["rtt50"] != 0.0 || rec["f50"] != 3.0 {
		t.Fatalf("bản ghi / kẹp số: %v", rec)
	}
	if _, ok := rec["name"]; ok || strings.Contains(l, "Mozilla") || strings.Contains(l, "An\"") {
		t.Fatalf("không được ghi trường lạ / tên / user agent: %s", l)
	}
	if !strings.Contains((*lines)[1], `"dev":"?"`) || !strings.Contains((*lines)[1], `"car":""`) {
		t.Fatalf("loại thiết bị / xe lạ phải bị bỏ: %s", (*lines)[1])
	}
}

func TestTelemetryRejectsOversizeAndBadJSON(t *testing.T) {
	lines := captureTelemetry(t)
	e := newEnv(t)
	a, _, b, _ := e.startRace(0)
	e.do(a, tel(`{"fps50":60,"pad":"`+strings.Repeat("x", maxTelemetryBytes)+`"}`))
	e.do(b, tel(`{"fps50":`))
	if len(*lines) != 0 {
		t.Fatalf("quá cỡ / sai JSON thì bỏ: %v", *lines)
	}
	// Bị bỏ không tính là đã gửi: bản hợp lệ sau đó vẫn được ghi.
	e.do(a, tel(`{"fps50":60}`))
	if len(*lines) != 1 {
		t.Fatalf("bản hợp lệ sau bản hỏng: %v", *lines)
	}
}

func TestTelemetryAfterFinishAndNextRace(t *testing.T) {
	lines := captureTelemetry(t)
	e := newEnv(t)
	a, _, b, _ := e.startRace(0)
	e.do(b, Message{T: "leave"})
	e.drive(a, 0, 28, 50*time.Second)
	if e.phase(a.Room) != PhaseOver {
		t.Fatalf("cuộc đua phải xong, đang %s", e.phase(a.Room))
	}
	e.do(a, tel(`{"fps50":60}`)) // vừa xong: vẫn nhận
	c, _ := e.join(a.Room.Code, chi)
	e.do(a, Message{T: "start"})
	e.clk.add(3 * time.Second)
	e.step(a.Room)
	e.do(a, tel(`{"fps50":58}`)) // trận mới: nhận lại
	_ = c
	if len(*lines) != 2 {
		t.Fatalf("sau khi xong và trận mới: %v", *lines)
	}
}

func TestPingAnswersPong(t *testing.T) {
	e := newEnv(t)
	a, ca, _, _ := e.startRace(0)
	e.do(a, Message{T: "ping", Ms: ptr64(12345)})
	ca.mu.Lock()
	defer ca.mu.Unlock()
	last := ca.msgs[len(ca.msgs)-1].(map[string]any)
	if last["t"] != "pong" || *(last["ms"].(*int64)) != 12345 {
		t.Fatalf("pong: %v", last)
	}
}

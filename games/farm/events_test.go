package farm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// eventLog ghi lại sự kiện nhiệm vụ nông trại gửi ra.
type eventLog struct {
	mu   sync.Mutex
	got  map[string]int
	inTx map[string]bool
}

func listen(f *Farm) *eventLog {
	l := &eventLog{got: map[string]int{}, inTx: map[string]bool{}}
	f.OnEvent = func(_ context.Context, tx *wallet.Tx, _ int64, kind string, n int) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.got[kind] += n
		l.inTx[kind] = tx != nil
	}
	return l
}

// Súng đậu hạ zombie → farm_zombie trong giao dịch nông trại (huỷ cùng giao dịch).
func TestGunKillsEmitZombieEvent(t *testing.T) {
	e := armed(t, 5)
	l := listen(e.f)
	e.ok(e.f.Plant(ctx, e.uid, 0, gunCrop))
	e.ok(e.f.Plant(ctx, e.uid, 1, gunCrop))
	e.spawn(0, true)
	e.spawn(1, true)
	e.clock = e.clock.Add(GunDelay * time.Second)
	e.ok(e.f.State(ctx, e.uid))
	if l.got["farm_zombie"] != 2 || !l.inTx["farm_zombie"] {
		t.Fatalf("súng hạ 2 zombie: %v %v", l.got, l.inTx)
	}
}

// Thao tác qua API gửi sự kiện theo số lượng thật (Gained.Count); thao tác lỗi thì không gửi.
func TestAPIEmitsQuestEvents(t *testing.T) {
	e := newEnv(t, 0, 0)
	u, token, err := e.w.Register(ctx, "nhiemvu", "matkhau123", wallet.Profile{Name: "NV"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.setBalance(u.ID, 1_000_000)
	l := listen(e.f)
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	post := func(path, body string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("/api/farm/plant-all", `{"crop":"water_spinach"}`); code != http.StatusOK {
		t.Fatalf("gieo tất cả: %d", code)
	}
	s := e.ok(e.f.State(ctx, u.ID))
	if l.got["farm_plant"] != len(s.Plots) {
		t.Fatalf("gieo %d ô, sự kiện %d", len(s.Plots), l.got["farm_plant"])
	}
	if code := post("/api/farm/harvest-all", `{}`); code == http.StatusOK {
		t.Fatal("chưa chín mà thu được")
	}
	if l.got["farm_harvest"] != 0 {
		t.Fatal("thu hoạch lỗi không được tính")
	}
	e.clock = e.clock.Add(24 * time.Hour)
	if code := post("/api/farm/harvest-all", `{}`); code != http.StatusOK {
		t.Fatalf("thu hoạch tất cả: %d", code)
	}
	if l.got["farm_harvest"] != len(s.Plots) || l.inTx["farm_harvest"] {
		t.Fatalf("thu hoạch: %v", l.got)
	}
}

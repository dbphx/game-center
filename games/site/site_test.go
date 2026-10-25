package site

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	s     *Site
	clock time.Time
	uid   int64
}

// newEnv tạo ví + công trường với đồng hồ giả (giữa trưa giờ VN) và một người chơi có XP cho trước.
func newEnv(t *testing.T, xp int64) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "site.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	s, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, w: w, s: s, clock: time.Date(2026, 10, 25, 12, 0, 0, 0, vnZone)}
	s.now = func() time.Time { return e.clock }
	u, _, err := w.Register(ctx, "tho", "matkhau123", wallet.Profile{Name: "Thợ"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.exec(`UPDATE users SET balance = 0, xp = ? WHERE id = ?`, xp, e.uid)
	return e
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) user() wallet.User {
	e.t.Helper()
	u, err := e.w.User(ctx, e.uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) start(job string) Shift {
	e.t.Helper()
	st, err := e.s.Start(ctx, e.uid, job)
	if err != nil {
		e.t.Fatal(err)
	}
	if st.Active == nil {
		e.t.Fatal("nhận ca xong phải có ca đang mở")
	}
	return *st.Active
}

func status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return errStatus(err)
}

func TestShiftPayAndAntiCheat(t *testing.T) {
	e := newEnv(t, 0)
	sh := e.start("bricks")
	if u := e.user(); u.HP != 92 {
		t.Fatalf("nhận ca Khuân gạch phải trừ 8 HP, còn %d", u.HP)
	}
	if sh.Targets != 15 || sh.Seconds != 30 || sh.EndsAt-sh.StartedAt != 30_000 {
		t.Fatalf("ca sai: %+v", sh)
	}
	if _, err := e.s.Start(ctx, e.uid, "bricks"); status(err) != http.StatusConflict {
		t.Fatalf("đang có ca không được nhận ca mới, được %v", err)
	}

	e.clock = e.clock.Add(20 * time.Second)
	if _, _, err := e.s.Finish(ctx, e.uid, sh.ID, 10); status(err) != http.StatusConflict {
		t.Fatalf("nộp sớm phải 409, được %v", err)
	}
	e.clock = e.clock.Add(9 * time.Second) // còn 1 giây: trong khung nộp sớm 2 giây
	if _, _, err := e.s.Finish(ctx, e.uid, sh.ID, 16); status(err) != http.StatusBadRequest {
		t.Fatalf("điểm vượt số mục tiêu phải 400, được %v", err)
	}
	st, res, err := e.s.Finish(ctx, e.uid, sh.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(120 + 180*10/15); res.Pay != want || res.XP != 4 || e.user().Balance != want || st.Today.Earned != want {
		t.Fatalf("tiền công: %+v, ví %d, hôm nay %d; muốn %d", res, e.user().Balance, st.Today.Earned, want)
	}
	if st.Active != nil {
		t.Fatal("nộp xong không còn ca mở")
	}
	if _, _, err := e.s.Finish(ctx, e.uid, sh.ID, 10); status(err) != http.StatusConflict {
		t.Fatalf("nộp lần 2 phải 409, được %v", err)
	}
	if e.user().XP != 4 {
		t.Fatalf("XP phải 4, được %d", e.user().XP)
	}

	// Nghỉ giữa ca: 20 giây sau giờ hết ca.
	if _, err := e.s.Start(ctx, e.uid, "bricks"); status(err) != http.StatusConflict {
		t.Fatalf("đang nghỉ giữa ca phải 409, được %v", err)
	}
	e.clock = e.clock.Add(25 * time.Second)
	sh = e.start("bricks")
	e.clock = e.clock.Add(61 * time.Second) // quá 30 giây sau giờ hết ca
	if _, _, err := e.s.Finish(ctx, e.uid, sh.ID, 15); status(err) != http.StatusConflict {
		t.Fatalf("nộp muộn phải 409, được %v", err)
	}
	if st, _ := e.s.State(ctx, e.uid); st.Active != nil {
		t.Fatal("ca quá hạn không còn là ca đang mở")
	}
	if _, _, err := e.s.Finish(ctx, e.uid, 9999, 1); status(err) != http.StatusNotFound {
		t.Fatalf("ca lạ phải 404, được %v", err)
	}
}

func TestJobLevelAndHP(t *testing.T) {
	e := newEnv(t, 0)
	if _, err := e.s.Start(ctx, e.uid, "crane"); status(err) != http.StatusForbidden {
		t.Fatalf("Lv1 lái cần cẩu phải 403, được %v", err)
	}
	if _, err := e.s.Start(ctx, e.uid, "dig"); status(err) != http.StatusBadRequest {
		t.Fatalf("việc lạ phải 400, được %v", err)
	}
	if u := e.user(); u.HP != 100 {
		t.Fatalf("bị từ chối thì không mất máu, còn %d", u.HP)
	}
	// Máu chỉ còn 8: không đủ cho ca 8 HP (không được làm tới ngất).
	e.exec(`UPDATE users SET hp = 8, hp_at = ? WHERE id = ?`, time.Now().Unix(), e.uid)
	if _, err := e.s.Start(ctx, e.uid, "bricks"); status(err) != http.StatusForbidden {
		t.Fatalf("thiếu máu phải 403, được %v", err)
	}
	e.exec(`UPDATE users SET hp = 0, hospital_until = ? WHERE id = ?`, time.Now().Unix()+300, e.uid)
	if _, err := e.s.Start(ctx, e.uid, "bricks"); !errors.Is(err, wallet.ErrInHospital) || status(err) != http.StatusLocked {
		t.Fatalf("nằm viện phải 423, được %v", err)
	}
}

func TestDailyCap(t *testing.T) {
	e := newEnv(t, 10_000)
	// Đã kiếm 5900 hôm nay: ca tiếp theo chỉ được thêm 100, sau đó hết suất.
	e.exec(`INSERT INTO site_shifts (user_id, job, started_ms, seconds, seed, day, finished, pay) VALUES (?, 'crane', 0, 40, 1, ?, 1, 5900)`,
		e.uid, e.s.day(e.clock))
	sh := e.start("crane")
	e.clock = e.clock.Add(40 * time.Second)
	st, res, err := e.s.Finish(ctx, e.uid, sh.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pay != 100 || st.Today.Earned != DailyCap {
		t.Fatalf("chạm giới hạn ngày: tiền %d, hôm nay %d", res.Pay, st.Today.Earned)
	}
	e.clock = e.clock.Add(time.Minute)
	if _, err := e.s.Start(ctx, e.uid, "bricks"); status(err) != http.StatusConflict || err.Error() != "Hôm nay đã làm đủ, mai quay lại nhé" {
		t.Fatalf("hết suất trong ngày phải 409, được %v", err)
	}
	// Sang ngày mới (giờ VN) thì làm lại được.
	e.clock = time.Date(2026, 10, 26, 0, 1, 0, 0, vnZone)
	if st, _ := e.s.State(ctx, e.uid); st.Today.Earned != 0 {
		t.Fatalf("ngày mới phải về 0, được %d", st.Today.Earned)
	}
	e.start("bricks")
}

// Nhiều lần nộp cùng một ca đồng thời chỉ được trả tiền một lần.
func TestConcurrentFinish(t *testing.T) {
	e := newEnv(t, 0)
	sh := e.start("bricks")
	e.clock = e.clock.Add(30 * time.Second)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := e.s.Finish(ctx, e.uid, sh.ID, 15); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 || e.user().Balance != 300 {
		t.Fatalf("nộp đồng thời: %d lần thành công, ví %d", ok, e.user().Balance)
	}
}

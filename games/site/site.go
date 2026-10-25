// Package site là Công trường của DCity (khu ngoại ô): người chơi làm thuê theo ca để kiếm xu.
// Mỗi ca tốn máu, giao diện chơi một mini-game trong thời gian của ca rồi báo điểm; tiền công tính theo điểm,
// có giới hạn thu nhập mỗi ngày và thời gian nghỉ giữa hai ca.
package site

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "site"

// Job là một việc làm thuê: tốn HP, kéo dài Seconds giây; tiền công từ BasePay (0 điểm) tới MaxPay (đủ điểm).
type Job struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Level   int    `json:"level"`
	HP      int    `json:"hp"`
	Seconds int    `json:"seconds"`
	BasePay int64  `json:"basePay"`
	MaxPay  int64  `json:"maxPay"`
	XP      int64  `json:"xp"`
	Targets int    `json:"targets"` // số mục tiêu tối đa của mini-game = Seconds / 2
}

var Jobs = func() []Job {
	jobs := []Job{
		{ID: "bricks", Name: "Khuân gạch", Level: 1, HP: 8, Seconds: 30, BasePay: 120, MaxPay: 300, XP: 4},
		{ID: "mortar", Name: "Trộn vữa", Level: 3, HP: 10, Seconds: 30, BasePay: 200, MaxPay: 500, XP: 6},
		{ID: "crane", Name: "Lái cần cẩu", Level: 6, HP: 12, Seconds: 40, BasePay: 400, MaxPay: 900, XP: 10},
	}
	for i := range jobs {
		jobs[i].Targets = jobs[i].Seconds / 2
	}
	return jobs
}()

const (
	DailyCap      = 6000 // xu kiếm được ở công trường tối đa mỗi ngày (theo giờ Việt Nam)
	Cooldown      = 20   // giây nghỉ sau khi hết giờ một ca mới được nhận ca mới
	EarlyGrace    = 2    // được nộp ca sớm tối đa 2 giây (trễ mạng / làm tròn đồng hồ)
	LateGrace     = 30   // quá 30 giây sau giờ hết ca thì ca bị huỷ, không nhận tiền
	finishedState = 1
)

// vnZone là múi giờ Việt Nam để tính "hôm nay".
var vnZone = time.FixedZone("ICT", 7*3600)

func findJob(id string) (Job, bool) {
	for _, j := range Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return Job{}, false
}

const schema = `
CREATE TABLE IF NOT EXISTS site_shifts (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	job        TEXT NOT NULL,
	started_ms INTEGER NOT NULL,
	seconds    INTEGER NOT NULL,
	seed       INTEGER NOT NULL,
	day        TEXT NOT NULL,             -- ngày (giờ VN) lúc nhận ca, để tính giới hạn thu nhập
	finished   INTEGER NOT NULL DEFAULT 0,
	score      INTEGER NOT NULL DEFAULT 0,
	pay        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS site_shifts_user ON site_shifts(user_id, id);
`

// Error là lỗi hiện cho người chơi, kèm mã HTTP.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func bad(msg string) error      { return &Error{http.StatusBadRequest, msg} }
func conflict(msg string) error { return &Error{http.StatusConflict, msg} }
func notFound(msg string) error { return &Error{http.StatusNotFound, msg} }

func errStatus(err error) int {
	var e *Error
	switch {
	case errors.As(err, &e):
		return e.Status
	case errors.Is(err, wallet.ErrInHospital):
		return http.StatusLocked
	case errors.Is(err, wallet.ErrLowHP), errors.Is(err, wallet.ErrLevelTooLow):
		return http.StatusForbidden
	case errors.Is(err, wallet.ErrNotFound):
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

// Site là dịch vụ công trường cho mọi người chơi.
type Site struct {
	w   *wallet.Wallet
	now func() time.Time
	mu  sync.Mutex // giữ rng
	rng *mrand.Rand
}

func New(ctx context.Context, w *wallet.Wallet) (*Site, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng công trường: %w", err)
	}
	return &Site{w: w, now: time.Now, rng: mrand.New(mrand.NewPCG(mrand.Uint64(), mrand.Uint64()))}, nil
}

func (s *Site) seed() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rng.Int64N(1 << 52)
}

// Shift là một ca làm đang mở.
type Shift struct {
	ID        int64  `json:"id"`
	Job       string `json:"job"`
	Seconds   int    `json:"seconds"`
	Targets   int    `json:"targets"`
	Seed      int64  `json:"seed"`
	StartedAt int64  `json:"startedAt"` // unix ms
	EndsAt    int64  `json:"endsAt"`    // unix ms: hết giờ chơi; nộp được tới EndsAt + 30 giây
}

type Today struct {
	Earned int64 `json:"earned"`
	Cap    int64 `json:"cap"`
}

// State là trạng thái công trường của một người chơi (GET /api/site).
type State struct {
	Now           int64  `json:"now"` // unix ms
	Jobs          []Job  `json:"jobs"`
	Today         Today  `json:"today"`
	Active        *Shift `json:"active"`
	CooldownUntil int64  `json:"cooldownUntil"` // unix ms; 0 = nhận ca được ngay
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Site) day(t time.Time) string { return t.In(vnZone).Format("2006-01-02") }

func (s *Site) load(ctx context.Context, q querier, uid int64) (State, error) {
	now := s.now()
	st := State{Now: now.UnixMilli(), Jobs: Jobs, Today: Today{Cap: DailyCap}}
	if err := q.QueryRowContext(ctx, `SELECT coalesce(SUM(pay), 0) FROM site_shifts WHERE user_id = ? AND day = ?`, uid, s.day(now)).
		Scan(&st.Today.Earned); err != nil {
		return State{}, err
	}
	var sh Shift
	var finished int
	err := q.QueryRowContext(ctx, `SELECT id, job, started_ms, seconds, seed, finished FROM site_shifts WHERE user_id = ? ORDER BY id DESC LIMIT 1`, uid).
		Scan(&sh.ID, &sh.Job, &sh.StartedAt, &sh.Seconds, &sh.Seed, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return State{}, err
	}
	sh.Targets = sh.Seconds / 2
	sh.EndsAt = sh.StartedAt + int64(sh.Seconds)*1000
	if cd := sh.EndsAt + Cooldown*1000; cd > st.Now {
		st.CooldownUntil = cd
	}
	if finished == 0 && st.Now <= sh.EndsAt+LateGrace*1000 {
		st.Active = &sh
	}
	return st, nil
}

// State trả trạng thái công trường của người chơi.
func (s *Site) State(ctx context.Context, uid int64) (State, error) {
	tx, err := s.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	return s.load(ctx, tx, uid)
}

// Start nhận một ca làm: kiểm level, trừ HP ngay, mở ca mới. Đang có ca, đang nghỉ giữa ca
// hoặc hôm nay đã kiếm đủ thì từ chối.
func (s *Site) Start(ctx context.Context, uid int64, jobID string) (State, error) {
	job, ok := findJob(jobID)
	if !ok {
		return State{}, bad("không có việc này")
	}
	tx, err := s.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	st, err := s.load(ctx, tx, uid)
	if err != nil {
		return State{}, err
	}
	switch {
	case st.Active != nil:
		return State{}, conflict("bạn đang có ca làm, hãy làm xong ca này")
	case st.Today.Earned >= DailyCap:
		return State{}, conflict("Hôm nay đã làm đủ, mai quay lại nhé")
	case st.CooldownUntil > 0:
		return State{}, conflict(fmt.Sprintf("nghỉ tay một chút, %d giây nữa mới nhận ca mới", (st.CooldownUntil-st.Now+999)/1000))
	}
	lv, err := tx.Level(uid)
	if err != nil {
		return State{}, err
	}
	if lv < job.Level {
		return State{}, &Error{http.StatusForbidden, fmt.Sprintf("cần level %d để %s", job.Level, job.Name)}
	}
	if _, err := tx.SpendHP(uid, job.HP); err != nil {
		if errors.Is(err, wallet.ErrLowHP) {
			return State{}, &Error{http.StatusForbidden, fmt.Sprintf("cần hơn %d HP để %s", job.HP, job.Name)}
		}
		return State{}, err
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO site_shifts (user_id, job, started_ms, seconds, seed, day) VALUES (?, ?, ?, ?, ?, ?)`,
		uid, job.ID, now.UnixMilli(), job.Seconds, s.seed(), s.day(now)); err != nil {
		return State{}, err
	}
	if st, err = s.load(ctx, tx, uid); err != nil {
		return State{}, err
	}
	return st, tx.Commit()
}

// Result là tiền công / XP của một ca vừa nộp.
type Result struct {
	Pay   int64 `json:"pay"`
	XP    int64 `json:"xp"`
	Score int   `json:"score"`
}

// Finish nộp ca: chỉ nhận trong khung [hết giờ − 2 giây, hết giờ + 30 giây], mỗi ca một lần.
// Tiền công = BasePay + (MaxPay − BasePay) × điểm / số mục tiêu, không vượt giới hạn ngày.
func (s *Site) Finish(ctx context.Context, uid, shiftID int64, score int) (State, Result, error) {
	tx, err := s.w.Begin(ctx)
	if err != nil {
		return State{}, Result{}, err
	}
	defer tx.Rollback()
	var jobID, day string
	var started int64
	var seconds, finished int
	err = tx.QueryRowContext(ctx, `SELECT job, started_ms, seconds, finished, day FROM site_shifts WHERE id = ? AND user_id = ?`, shiftID, uid).
		Scan(&jobID, &started, &seconds, &finished, &day)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, Result{}, notFound("không tìm thấy ca làm này")
	}
	if err != nil {
		return State{}, Result{}, err
	}
	job, _ := findJob(jobID)
	nowMs := s.now().UnixMilli()
	ends := started + int64(seconds)*1000
	switch {
	case finished != 0:
		return State{}, Result{}, conflict("ca này đã nộp rồi")
	case nowMs < ends-EarlyGrace*1000:
		return State{}, Result{}, conflict("chưa hết giờ làm, chưa nộp ca được")
	case nowMs > ends+LateGrace*1000:
		return State{}, Result{}, conflict("quá giờ nộp ca, ca này không được tính")
	case score < 0 || score > seconds/2:
		return State{}, Result{}, bad("điểm không hợp lệ")
	}
	targets := int64(seconds / 2)
	pay := job.BasePay + (job.MaxPay-job.BasePay)*int64(score)/targets
	var earned int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(pay), 0) FROM site_shifts WHERE user_id = ? AND day = ?`, uid, day).Scan(&earned); err != nil {
		return State{}, Result{}, err
	}
	pay = max(0, min(pay, DailyCap-earned))
	if _, err := tx.ExecContext(ctx, `UPDATE site_shifts SET finished = ?, score = ?, pay = ? WHERE id = ?`, finishedState, score, pay, shiftID); err != nil {
		return State{}, Result{}, err
	}
	if err := tx.Credit(uid, pay, GameID, fmt.Sprintf("Công trường: %s (%d/%d)", job.Name, score, targets)); err != nil {
		return State{}, Result{}, err
	}
	if err := tx.AddXP(uid, job.XP); err != nil {
		return State{}, Result{}, err
	}
	st, err := s.load(ctx, tx, uid)
	if err != nil {
		return State{}, Result{}, err
	}
	return st, Result{Pay: pay, XP: job.XP, Score: score}, tx.Commit()
}

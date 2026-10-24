package baucua

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

const round0 = 60_000_000 // một vòng bất kỳ, đồng hồ giả đặt ở đầu vòng này

type env struct {
	t     *testing.T
	path  string
	w     *wallet.Wallet
	g     *Game
	clock time.Time
	dice  [3]string
	rolls int
	uid   int64
}

// newEnv tạo ví + sòng với đồng hồ giả ở đầu vòng round0, xúc xắc cố định và một người chơi có sẵn xu.
func newEnv(t *testing.T, balance int64) *env {
	t.Helper()
	e := &env{t: t, path: filepath.Join(t.TempDir(), "baucua.db"), dice: [3]string{"cua", "ga", "cua"}}
	e.open()
	u, _, err := e.w.Register(ctx, "nguoichoi", "matkhau123", wallet.Profile{Name: "Người chơi"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.setBalance(e.uid, balance)
	return e
}

func (e *env) open() {
	e.t.Helper()
	w, err := wallet.Open(e.path)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { w.Close() })
	g, err := New(ctx, w)
	if err != nil {
		e.t.Fatal(err)
	}
	e.clock = time.UnixMilli(round0 * RoundTime.Milliseconds())
	g.now = func() time.Time { return e.clock }
	g.roll = func() [3]string { e.rolls++; return e.dice }
	e.w, e.g = w, g
}

func (e *env) setBalance(uid, balance int64) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ?, xp = 0 WHERE id = ?`, balance, uid); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) user(uid int64) wallet.User {
	e.t.Helper()
	u, err := e.w.User(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) bet(face string, amount int64) State {
	e.t.Helper()
	s, err := e.g.Bet(ctx, e.uid, face, amount)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func status(err error) int { return errStatus(err) }

func TestBetChargesAndPhases(t *testing.T) {
	e := newEnv(t, 1000)
	s := e.bet("cua", 100)
	s = e.bet("cua", 50)
	s = e.bet("nai", 10)
	if s.Phase != PhaseBetting || s.Round != round0 {
		t.Fatalf("phải ở pha cược vòng %d, được %s vòng %d", round0, s.Phase, s.Round)
	}
	if s.Mine.Total != 160 || s.Mine.Bets["cua"] != 150 || s.Totals["nai"] != 10 || *s.Balance != 840 {
		t.Fatalf("sau khi đặt: %+v, số dư %d", s.Mine, *s.Balance)
	}
	if len(s.Players) != 1 || s.Players[0].Total != 160 || s.Players[0].Name != "Người chơi" {
		t.Fatalf("danh sách người đặt sai: %+v", s.Players)
	}
	if s.Dice != nil {
		t.Fatal("chưa mở bát thì không lộ xúc xắc")
	}

	if _, err := e.g.Bet(ctx, e.uid, "rong", 10); status(err) != http.StatusBadRequest {
		t.Fatalf("mặt lạ phải 400, được %v", err)
	}
	if _, err := e.g.Bet(ctx, e.uid, "cua", 30); status(err) != http.StatusBadRequest {
		t.Fatalf("chip lạ phải 400, được %v", err)
	}
	if _, err := e.g.Bet(ctx, e.uid, "cua", 1000); status(err) != http.StatusPaymentRequired {
		t.Fatalf("không đủ xu phải 402, được %v", err)
	}

	e.clock = e.clock.Add(BettingTime)
	if _, err := e.g.Bet(ctx, e.uid, "cua", 10); status(err) != http.StatusConflict {
		t.Fatalf("hết giờ cược phải 409, được %v", err)
	}
	s, _ = e.g.State(ctx, e.uid)
	if s.Phase != PhaseRolling || s.Dice != nil {
		t.Fatalf("đang lắc: pha %s, xúc xắc %v", s.Phase, s.Dice)
	}
}

func TestMaxTotal(t *testing.T) {
	e := newEnv(t, 100000)
	for range 20 {
		e.bet("bau", 1000)
	}
	if _, err := e.g.Bet(ctx, e.uid, "tom", 10); status(err) != http.StatusConflict {
		t.Fatalf("vượt %d xu một vòng phải 409, được %v", MaxTotal, err)
	}
}

// Ra cua-ga-cua: cua 2 lần, gà 1 lần, còn lại 0.
func TestPayout(t *testing.T) {
	cases := []struct {
		dice [3]string
		face string
		want int64
	}{
		{[3]string{"bau", "tom", "ca"}, "cua", 0},
		{[3]string{"cua", "tom", "ca"}, "cua", 200},
		{[3]string{"cua", "tom", "cua"}, "cua", 300},
		{[3]string{"cua", "cua", "cua"}, "cua", 400},
	}
	for _, c := range cases {
		if got := payout(c.face, 100, c.dice); got != c.want {
			t.Errorf("%v cược 100 %s: muốn %d, được %d", c.dice, c.face, c.want, got)
		}
	}
}

func TestSettleOnceAndReveal(t *testing.T) {
	e := newEnv(t, 1000)
	e.bet("cua", 100) // ra 2 lần: nhận 300
	e.bet("ga", 50)   // ra 1 lần: nhận 100
	e.bet("bau", 10)  // không ra: mất
	// 840 sau khi đặt; nhận 400 → 1240.

	e.clock = e.clock.Add(BettingTime + RollingTime)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.g.State(ctx, e.uid); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s, err := e.g.State(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if e.rolls != 1 {
		t.Fatalf("mỗi vòng chỉ quay 1 lần, quay %d", e.rolls)
	}
	if s.Phase != PhaseResult || len(s.Dice) != 3 || s.Dice[0] != "cua" {
		t.Fatalf("mở bát phải lộ xúc xắc: %s %v", s.Phase, s.Dice)
	}
	if *s.Mine.Won != 400 || *s.Mine.Net != 240 || *s.Balance != 1240 {
		t.Fatalf("nhận %d, lời %d, số dư %d", *s.Mine.Won, *s.Mine.Net, *s.Balance)
	}
	u := e.user(e.uid)
	if u.Balance != 1240 || u.XP != XPPlay+XPWin {
		t.Fatalf("ví %d, XP %d", u.Balance, u.XP)
	}
	if len(s.History) != 1 || s.History[0][1] != "ga" {
		t.Fatalf("lịch sử phải có vòng vừa mở: %v", s.History)
	}

	// Vòng sau: lịch sử còn đó, cược mới về 0.
	e.clock = e.clock.Add(ResultTime)
	s, _ = e.g.State(ctx, e.uid)
	if s.Round != round0+1 || s.Phase != PhaseBetting || s.Mine.Total != 0 || len(s.History) != 1 {
		t.Fatalf("vòng mới: %+v", s)
	}
	if e.user(e.uid).Balance != 1240 {
		t.Fatal("không được trả thưởng lần hai")
	}
}

// Không ai gọi API trong lúc lắc / mở bát: vòng cũ vẫn được chốt ở request sau đó.
func TestLateSettle(t *testing.T) {
	e := newEnv(t, 1000)
	e.dice = [3]string{"bau", "bau", "bau"}
	e.bet("bau", 100)
	e.clock = e.clock.Add(5 * RoundTime)
	if _, err := e.g.State(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if u := e.user(e.uid); u.Balance != 1300 || u.XP != XPPlay+XPWin {
		t.Fatalf("chốt muộn: ví %d, XP %d", u.Balance, u.XP)
	}
}

func TestLoseGivesPlayXP(t *testing.T) {
	e := newEnv(t, 1000)
	e.bet("nai", 500)
	e.clock = e.clock.Add(BettingTime)
	s, _ := e.g.State(ctx, e.uid)
	e.clock = e.clock.Add(RollingTime)
	s, _ = e.g.State(ctx, e.uid)
	if *s.Mine.Won != 0 || *s.Mine.Net != -500 || *s.Balance != 500 {
		t.Fatalf("thua: %+v số dư %d", s.Mine, *s.Balance)
	}
	if e.user(e.uid).XP != XPPlay {
		t.Fatalf("thua vẫn được %d XP", XPPlay)
	}
}

func TestClearRefunds(t *testing.T) {
	e := newEnv(t, 1000)
	e.bet("tom", 500)
	e.bet("ca", 100)
	s, err := e.g.Clear(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if *s.Balance != 1000 || s.Mine.Total != 0 || len(s.Players) != 0 {
		t.Fatalf("huỷ cược: số dư %d, %+v", *s.Balance, s.Mine)
	}
	if _, err := e.g.Clear(ctx, e.uid); status(err) != http.StatusConflict {
		t.Fatalf("huỷ khi chưa đặt phải 409, được %v", err)
	}
	e.bet("tom", 10)
	e.clock = e.clock.Add(BettingTime)
	if _, err := e.g.Clear(ctx, e.uid); status(err) != http.StatusConflict {
		t.Fatalf("hết giờ cược không huỷ được, được %v", err)
	}
}

// Server tắt giữa vòng: lần mở sau hoàn cược còn treo, và không chốt lại vòng đó nữa.
func TestRefundOnRestart(t *testing.T) {
	e := newEnv(t, 1000)
	e.bet("ga", 1000)
	if b := e.user(e.uid).Balance; b != 0 {
		t.Fatalf("đã trừ cược, còn %d", b)
	}
	e.w.Close()
	e.open()
	if b := e.user(e.uid).Balance; b != 1000 {
		t.Fatalf("khởi động lại phải hoàn cược, số dư %d", b)
	}
	e.clock = e.clock.Add(BettingTime + RollingTime)
	s, _ := e.g.State(ctx, e.uid)
	if s.Mine.Total != 0 || s.Mine.Won != nil {
		t.Fatalf("cược đã hoàn không còn tính: %+v", s.Mine)
	}
	if b := e.user(e.uid).Balance; b != 1000 {
		t.Fatalf("không được trả thưởng cho cược đã hoàn, số dư %d", b)
	}
}

func TestHPGateInHandler(t *testing.T) {
	e := newEnv(t, 1000)
	if err := wallet.CheckZone(wallet.User{HP: 10}, "casino"); !errors.Is(err, wallet.ErrLowHP) || status(err) != http.StatusForbidden {
		t.Fatalf("thiếu máu phải 403, được %v", err)
	}
	if status(wallet.ErrInHospital) != http.StatusLocked {
		t.Fatal("nằm viện phải 423")
	}
	_ = e
}

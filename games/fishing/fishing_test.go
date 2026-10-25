package fishing

import (
	"context"
	mrand "math/rand/v2"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	p     *Pond
	clock time.Time
	uid   int64
	next  string // loài của lần thả câu tới
}

// newEnv tạo ví + hồ câu với đồng hồ giả, kết quả câu cố định (e.next, 30 cm, cắn câu sau 5 giây).
func newEnv(t *testing.T, balance int64) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "fishing.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	p, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, w: w, p: p, clock: time.Unix(1_800_000_000, 0), next: "chep"}
	p.now = func() time.Time { return e.clock }
	p.pick = func(int) (Fish, int, int64) {
		f, _ := findFish(e.next)
		cm := 0
		if f.MaxCm > 0 {
			cm = 30
		}
		return f, cm, 5000
	}
	u, _, err := w.Register(ctx, "cancau", "matkhau123", wallet.Profile{Name: "Cần thủ"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.exec(`UPDATE users SET balance = ?, xp = 0 WHERE id = ?`, balance, e.uid)
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

func (e *env) cast(bait string) *Cast {
	e.t.Helper()
	st, err := e.p.CastLine(ctx, e.uid, bait)
	if err != nil {
		e.t.Fatal(err)
	}
	if st.Cast == nil {
		e.t.Fatal("thả câu xong phải có lần thả đang mở")
	}
	return st.Cast
}

func status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return errStatus(err)
}

func qty(st State, bait string) int {
	for _, b := range st.Baits {
		if b.ID == bait {
			return b.Qty
		}
	}
	return -1
}

func TestBaitAndCatch(t *testing.T) {
	e := newEnv(t, 1000)
	if _, err := e.p.CastLine(ctx, e.uid, "worm"); status(err) != http.StatusConflict {
		t.Fatalf("hết mồi phải 409, được %v", err)
	}
	if _, err := e.p.BuyBait(ctx, e.uid, "worm", 51); status(err) != http.StatusBadRequest {
		t.Fatalf("mua quá 50 mồi phải 400, được %v", err)
	}
	if _, err := e.p.BuyBait(ctx, e.uid, "special", 7); status(err) != http.StatusPaymentRequired {
		t.Fatalf("không đủ xu phải 402, được %v", err)
	}
	st, err := e.p.BuyBait(ctx, e.uid, "worm", 5)
	if err != nil || qty(st, "worm") != 5 || e.user().Balance != 950 {
		t.Fatalf("mua 5 giun: kho %d, ví %d, lỗi %v", qty(st, "worm"), e.user().Balance, err)
	}

	c := e.cast("worm")
	if c.BiteAt != e.clock.UnixMilli()+5000 || c.WindowMs != WindowMs || c.Difficulty != 2 {
		t.Fatalf("lần thả câu sai: %+v", c)
	}
	if _, err := e.p.CastLine(ctx, e.uid, "worm"); status(err) != http.StatusConflict {
		t.Fatalf("đang thả câu không được thả tiếp, được %v", err)
	}
	e.clock = e.clock.Add(6 * time.Second)
	st, outcome, _, got, xp, err := e.p.Reel(ctx, e.uid, c.ID, true)
	if err != nil || outcome != OutcomeCaught || got == nil || got.FishID != "chep" || got.Cm != 30 || got.Price != 150 || xp != 4 {
		t.Fatalf("câu được chép 30 cm: %s %+v xp %d lỗi %v", outcome, got, xp, err)
	}
	if len(st.Basket) != 1 || st.Record["chep"] != 30 || st.Cast != nil || qty(st, "worm") != 4 || e.user().XP != 4 {
		t.Fatalf("sau khi câu: giỏ %d, kỷ lục %v, cast %v, giun %d", len(st.Basket), st.Record, st.Cast, qty(st, "worm"))
	}
	if _, _, _, _, _, err := e.p.Reel(ctx, e.uid, c.ID, true); status(err) != http.StatusNotFound {
		t.Fatalf("giật lại lần thả đã đóng phải 404, được %v", err)
	}
}

func TestReelTiming(t *testing.T) {
	e := newEnv(t, 1000)
	if _, err := e.p.BuyBait(ctx, e.uid, "worm", 10); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		after   time.Duration
		success bool
		want    string
	}{
		// Cá chưa cắn; quá giờ; kéo cần thua; đúng lúc cắn.
		{4 * time.Second, true, OutcomeEarly},
		{5*time.Second + WindowMs*time.Millisecond + ReelMs*time.Millisecond + time.Second, true, OutcomeLate},
		{6 * time.Second, false, OutcomeMiss},
		{5 * time.Second, true, OutcomeCaught},
	}
	for _, c := range cases {
		cast := e.cast("worm")
		e.clock = e.clock.Add(c.after)
		st, outcome, msg, got, _, err := e.p.Reel(ctx, e.uid, cast.ID, c.success)
		if err != nil || outcome != c.want || msg == "" {
			t.Fatalf("sau %v (success=%v): %s %q, lỗi %v", c.after, c.success, outcome, msg, err)
		}
		if (got != nil) != (c.want == OutcomeCaught) || st.Cast != nil {
			t.Fatalf("%s: cá %+v, cast %v", c.want, got, st.Cast)
		}
	}
	if st, _ := e.p.State(ctx, e.uid); qty(st, "worm") != 6 || len(st.Basket) != 1 {
		t.Fatalf("mỗi lần thả mất 1 mồi dù hụt: còn %d giun, giỏ %d", qty(st, "worm"), len(st.Basket))
	}

	// Rác: được XP nhưng không vào giỏ.
	e.next = "dep"
	cast := e.cast("worm")
	e.clock = e.clock.Add(5 * time.Second)
	st, outcome, _, got, xp, _ := e.p.Reel(ctx, e.uid, cast.ID, true)
	if outcome != OutcomeCaught || got.Rarity != Junk || xp != 1 || len(st.Basket) != 1 {
		t.Fatalf("vớt rác: %s %+v xp %d giỏ %d", outcome, got, xp, len(st.Basket))
	}

	// Lần thả bỏ dở quá hạn thì được thả lần mới.
	e.cast("worm")
	e.clock = e.clock.Add(time.Minute)
	if st, _ := e.p.State(ctx, e.uid); st.Cast != nil {
		t.Fatal("lần thả quá hạn không còn là đang mở")
	}
	e.cast("worm")
}

func TestBasketFullAndSell(t *testing.T) {
	e := newEnv(t, 1000)
	if _, err := e.p.BuyBait(ctx, e.uid, "worm", 2); err != nil {
		t.Fatal(err)
	}
	for i := range BasketMax {
		e.exec(`INSERT INTO fishing_basket (user_id, fish, cm, price, caught_at) VALUES (?, 'ro', 10, ?, 0)`, e.uid, 40+i)
	}
	if _, err := e.p.CastLine(ctx, e.uid, "worm"); status(err) != http.StatusConflict {
		t.Fatalf("giỏ đầy phải 409, được %v", err)
	}
	st, _ := e.p.State(ctx, e.uid)
	if qty(st, "worm") != 2 {
		t.Fatal("giỏ đầy thì không mất mồi")
	}
	st, coins, n, err := e.p.Sell(ctx, e.uid, []int64{st.Basket[0].ID, st.Basket[1].ID}, false)
	if err != nil || coins != 81 || n != 2 || len(st.Basket) != BasketMax-2 || e.user().Balance != 1061 {
		t.Fatalf("bán 2 con: %d xu, %d con, giỏ %d, ví %d, lỗi %v", coins, n, len(st.Basket), e.user().Balance, err)
	}
	st, coins, n, err = e.p.Sell(ctx, e.uid, nil, true)
	if err != nil || n != BasketMax-2 || len(st.Basket) != 0 {
		t.Fatalf("bán hết: %d con, giỏ %d, lỗi %v", n, len(st.Basket), err)
	}
	if want := int64(1061) + coins; e.user().Balance != want {
		t.Fatalf("ví sau bán hết %d, muốn %d", e.user().Balance, want)
	}
	if _, _, _, err := e.p.Sell(ctx, e.uid, nil, true); status(err) != http.StatusConflict {
		t.Fatalf("giỏ trống phải 409, được %v", err)
	}
}

// Mồi tốt dịch tỉ lệ sang cá hiếm: thống kê 20000 lần quay với rng cố định.
func TestRarityByLuck(t *testing.T) {
	p := &Pond{rng: mrand.New(mrand.NewPCG(1, 2))}
	const n = 20000
	share := func(luck int) map[string]float64 {
		cnt := map[string]int{}
		for range n {
			f, cm, bite := p.roll(luck)
			if f.MaxCm > 0 && (cm < f.MinCm || cm > f.MaxCm) {
				t.Fatalf("cỡ %d ngoài [%d, %d] của %s", cm, f.MinCm, f.MaxCm, f.ID)
			}
			if bite < BiteMinMs || bite > BiteMaxMs {
				t.Fatalf("thời điểm cắn câu %d ms ngoài khoảng", bite)
			}
			cnt[f.Rarity]++
		}
		out := map[string]float64{}
		for r, c := range cnt {
			out[r] = float64(c) / n
		}
		return out
	}
	worm, special := share(0), share(2)
	rarePlus := func(s map[string]float64) float64 { return s[Rare] + s[Epic] + s[Legendary] }
	if !(rarePlus(special) > rarePlus(worm)*2.5) || special[Common] >= worm[Common] {
		t.Fatalf("mồi đặc biệt phải ra cá hiếm nhiều hơn: giun %v, đặc biệt %v", worm, special)
	}
	if l := special[Legendary]; l < 0.002 || l > 0.009 {
		t.Fatalf("huyền thoại với mồi đặc biệt ~0.5%%, được %.3f%%", l*100)
	}
	if worm[Common] < 0.5 || worm[Common] > 0.6 {
		t.Fatalf("giun ra cá thường ~55%%, được %.1f%%", worm[Common]*100)
	}
}

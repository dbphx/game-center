package farm

import (
	"net/http"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// thiefEnv thêm người thứ hai (kẻ trộm) với số dư cho trước; chủ nhà là e.uid.
func thiefEnv(t *testing.T, balance int64) (*env, int64) {
	t.Helper()
	e := newEnv(t, 100_000, 0)
	u, _, err := e.w.Register(ctx, "keTrom", "matkhau123", wallet.Profile{Name: "Kẻ trộm"})
	if err != nil {
		t.Fatal(err)
	}
	e.setBalance(u.ID, balance)
	return e, u.ID
}

func (e *env) setBalance(uid, balance int64) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ? WHERE id = ?`, balance, uid); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) balanceOf(uid int64) int64 {
	e.t.Helper()
	u, err := e.w.User(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.Balance
}

// giveItem cất thẳng một món vào kho của uid.
func (e *env) giveItem(uid int64, store, item string, qty int) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := addItem(ctx, tx, uid, store, item, qty); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// ripeCarrots gieo cà rốt vào n ô đầu của chủ nhà rồi cho thời gian trôi tới lúc chín.
func (e *env) ripeCarrots(n int) {
	e.t.Helper()
	for i := range n {
		e.ok(e.f.Plant(ctx, e.uid, i, "carrot"))
	}
	c, _ := findCrop("carrot")
	e.clock = e.clock.Add(time.Duration(c.GrowSeconds) * time.Second)
}

func TestDogFeedFourHours(t *testing.T) {
	e := newEnv(t, 10_000, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Dog.Guarding || s.Dog.FedUntil != 0 || s.Dog.FullFor != 4*3600 {
		t.Fatalf("chó mới: %+v", s.Dog)
	}
	_, err := e.f.FeedDog(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // chưa có thức ăn cho chó
	e.giveItem(e.uid, storeSeeds, DogFeedID, 3)
	s = e.ok(e.f.FeedDog(ctx, e.uid))
	if !s.Dog.Guarding || s.Dog.FedUntil != e.clock.Unix()+4*3600 || s.Storage.Seeds.Items[DogFeedID] != 2 {
		t.Fatalf("sau khi cho ăn: %+v, còn %d phần", s.Dog, s.Storage.Seeds.Items[DogFeedID])
	}
	// Còn no hơn 1 giờ thì chưa cho ăn lại được.
	e.clock = e.clock.Add(2*time.Hour + 59*time.Minute)
	_, err = e.f.FeedDog(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	// Hết 4 giờ: đói, không canh nhà.
	e.clock = e.clock.Add(61 * time.Minute)
	if s = e.ok(e.f.State(ctx, e.uid)); s.Dog.Guarding {
		t.Fatalf("quá 4 giờ chó vẫn canh: %+v", s.Dog)
	}
	if s = e.ok(e.f.FeedDog(ctx, e.uid)); !s.Dog.Guarding {
		t.Fatal("cho ăn lại mà chó không canh")
	}
}

// Chó đói: trộm được ngay, mỗi ô mỗi vụ một lần; chủ thu hoạch ô bị trộm chỉ còn XP.
func TestStealHungryDog(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	_, err := e.f.Steal(ctx, thief, e.uid, 1)
	wantStatus(t, err, http.StatusConflict) // chưa chín
	_, err = e.f.Steal(ctx, thief, e.uid, 0)
	wantStatus(t, err, http.StatusConflict) // ô trống
	_, err = e.f.Steal(ctx, thief, e.uid, 99)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.Steal(ctx, e.uid, e.uid, 1)
	wantStatus(t, err, http.StatusBadRequest)

	c, _ := findCrop("carrot")
	e.clock = e.clock.Add(time.Duration(c.GrowSeconds) * time.Second)
	res, err := e.f.Steal(ctx, thief, e.uid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "stolen" || res.Item != "carrot" || res.Qty != 1 || res.HPLost != 0 || res.Chase != nil {
		t.Fatalf("kết quả: %+v", res)
	}
	if p := res.Visit.Plots[1]; !p.Ripe || !p.Stolen || !p.Tried || res.Visit.StealsLeft != StealsPerDay-1 {
		t.Fatalf("sau khi trộm: %+v, còn %d lượt", p, res.Visit.StealsLeft)
	}
	if ts := e.ok(e.f.State(ctx, thief)); ts.Storage.Products.Items["carrot"] != 1 {
		t.Fatalf("kho kẻ trộm: %v", ts.Storage.Products.Items)
	}
	_, err = e.f.Steal(ctx, e.newThief("nguoikhac"), e.uid, 1)
	if err == nil || err.Error() != "Ô này đã bị trộm rồi" {
		t.Fatalf("trộm lại ô đã bị trộm: %v", err)
	}

	s := e.ok(e.f.State(ctx, e.uid))
	if th := s.Thefts; !s.Plots[1].Stolen || len(th) != 1 || th[0].Thief.Name != "Kẻ trộm" || th[0].Caught || th[0].Qty != 1 {
		t.Fatalf("nhật ký chủ nhà: %+v, ô %+v", th, s.Plots[1])
	}
	xp := e.user().XP
	s, g, err := e.f.Harvest(ctx, e.uid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if g.XP != c.XP || len(g.Items) != 0 || s.Storage.Products.Items["carrot"] != 0 || e.user().XP != xp+c.XP {
		t.Fatalf("thu hoạch ô bị trộm: %+v, kho %v", g, s.Storage.Products.Items)
	}
	if s.Plots[1].Stolen {
		t.Fatal("ô đã dọn vẫn đánh dấu bị trộm")
	}
	// Vụ mới trên cùng ô lại trộm được.
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.clock = e.clock.Add(time.Duration(c.GrowSeconds) * time.Second)
	if _, err := e.f.Steal(ctx, thief, e.uid, 1); err != nil {
		t.Fatalf("vụ mới: %v", err)
	}
}

func TestHarvestAllSkipsStolenItems(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(3)
	if _, err := e.f.Steal(ctx, thief, e.uid, 0); err != nil {
		t.Fatal(err)
	}
	s, g, err := e.f.HarvestAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 3 || g.Items["carrot"] != 2 || s.Storage.Products.Items["carrot"] != 2 {
		t.Fatalf("thu hoạch tất cả: %+v, kho %v", g, s.Storage.Products.Items)
	}
}

func TestStealNeedsRoomInThiefStore(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(1)
	ts := e.ok(e.f.State(ctx, thief))
	e.giveItem(thief, storeProducts, "corn", ts.Storage.Products.Capacity)
	_, err := e.f.Steal(ctx, thief, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if v, _ := e.f.Visit(ctx, e.uid, thief); v.Plots[0].Stolen || v.Plots[0].Tried || v.StealsLeft != StealsPerDay {
		t.Fatalf("kho đầy mà vẫn tính trộm: %+v", v)
	}
}

func TestSummaries(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(3)
	if _, err := e.f.Steal(ctx, thief, e.uid, 2); err != nil {
		t.Fatal(err)
	}
	sum, err := e.f.Summaries(ctx, thief, []int64{e.uid, thief})
	if err != nil {
		t.Fatal(err)
	}
	if sum[e.uid].Ripe != 2 || sum[e.uid].DogGuarding || sum[thief].Ripe != 0 {
		t.Fatalf("tóm tắt: %+v", sum)
	}
}

// newThief đăng ký thêm một người chơi.
func (e *env) newThief(username string) int64 {
	e.t.Helper()
	u, _, err := e.w.Register(ctx, username, "matkhau123", wallet.Profile{Name: username})
	if err != nil {
		e.t.Fatal(err)
	}
	return u.ID
}

// guard cho chó nhà e.uid ăn (canh 4 giờ).
func (e *env) guard() {
	e.t.Helper()
	e.giveItem(e.uid, storeSeeds, DogFeedID, 1)
	e.ok(e.f.FeedDog(ctx, e.uid))
}

func (e *env) after(ms int64) { e.clock = e.clock.Add(time.Duration(ms) * time.Millisecond) }

// chase bắt đầu một cuộc đuổi ở ô plot, kiểm tra kết quả "chase".
func (e *env) chase(thief int64, plot int) *Chase {
	e.t.Helper()
	res, err := e.f.Steal(ctx, thief, e.uid, plot)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.Result != "chase" || res.Chase == nil || res.Qty != 1 {
		e.t.Fatalf("chó canh mà không đuổi: %+v", res)
	}
	return res.Chase
}

func (e *env) hp(uid int64) wallet.User {
	e.t.Helper()
	u, err := e.w.User(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func TestChaseEscape(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	other := e.newThief("nguoikhac")
	e.ripeCarrots(2)
	e.guard()

	ch := e.chase(thief, 0)
	start := e.clock.UnixMilli()
	if ch.Plot != 0 || ch.Item != "carrot" || ch.StartedAt != start || ch.DogSpeed != DogSpeed || ch.MinMs != ChaseMinMs ||
		ch.ExpiresAt != start+ChaseTTLMs || len(ch.ID) < 16 {
		t.Fatalf("cuộc đuổi: %+v", ch)
	}
	// Đang đuổi: chưa có đồ, ô chưa bị trộm; mình đã thử ô này, mất một lượt; người khác thấy ô bị khoá.
	v, _ := e.f.Visit(ctx, e.uid, thief)
	if p := v.Plots[0]; p.Stolen || !p.Tried || p.Locked || v.StealsLeft != StealsPerDay-1 {
		t.Fatalf("kẻ trộm thấy: %+v, còn %d lượt", p, v.StealsLeft)
	}
	v, _ = e.f.Visit(ctx, e.uid, other)
	if p := v.Plots[0]; !p.Locked || p.Tried {
		t.Fatalf("người khác thấy: %+v", p)
	}
	_, err := e.f.Steal(ctx, other, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if ts := e.ok(e.f.State(ctx, thief)); ts.Storage.Products.Items["carrot"] != 0 {
		t.Fatal("đang đuổi mà đã có đồ")
	}
	if s := e.ok(e.f.State(ctx, e.uid)); len(s.Thefts) != 1 || !s.Thefts[0].Chasing {
		t.Fatalf("nhật ký lúc đang đuổi: %+v", s.Thefts)
	}

	e.after(ChaseMinMs)
	res, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "stolen" || res.Qty != 1 || res.HPLost != 0 || !res.Visit.Plots[0].Stolen {
		t.Fatalf("thoát: %+v", res)
	}
	if ts := e.ok(e.f.State(ctx, thief)); ts.Storage.Products.Items["carrot"] != 1 {
		t.Fatalf("thoát mà không có đồ: %v", ts.Storage.Products.Items)
	}
	if e.hp(thief).HP != wallet.MaxHP {
		t.Fatal("thoát mà vẫn mất máu")
	}
	// Token chỉ dùng một lần.
	_, err = e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	wantStatus(t, err, http.StatusConflict)
	s := e.ok(e.f.State(ctx, e.uid))
	if th := s.Thefts[0]; th.Chasing || th.Caught || th.Qty != 1 || th.HPLost != 0 {
		t.Fatalf("nhật ký sau khi thoát: %+v", th)
	}
}

func TestChaseCaughtBites(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(3)
	e.guard()
	balance := e.balanceOf(thief)

	ch := e.chase(thief, 0)
	e.after(3000)
	res, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "caught" || res.Qty != 0 || res.HPLost != ChaseBiteHP || res.Visit.Plots[0].Stolen || !res.Visit.Plots[0].Tried {
		t.Fatalf("bị bắt: %+v", res)
	}
	if u := e.hp(thief); u.HP != wallet.MaxHP-ChaseBiteHP || u.Balance != balance {
		t.Fatalf("sau khi bị cắn: HP %d, xu %d", u.HP, u.Balance)
	}
	// Bị bắt cũng mất lượt thử ô đó.
	_, err = e.f.Steal(ctx, thief, e.uid, 0)
	if err == nil || err.Error() != "Bạn đã thử trộm ô này rồi" {
		t.Fatalf("thử lại ô đã bị bắt: %v", err)
	}
	// Báo thoát quá sớm (chưa đủ ChaseMinMs) tính là bị bắt: hết máu, nằm viện.
	ch = e.chase(thief, 1)
	e.after(ChaseMinMs - 1)
	res, err = e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if u := e.hp(thief); res.Result != "caught" || res.HPLost != ChaseBiteHP || u.HP != 0 || u.HospitalUntil == 0 {
		t.Fatalf("thoát quá sớm: %+v, HP %d, viện %d", res, u.HP, u.HospitalUntil)
	}
	s := e.ok(e.f.State(ctx, e.uid))
	if th := s.Thefts[0]; !th.Caught || th.HPLost != ChaseBiteHP || th.Chasing || th.Thief.ID != thief {
		t.Fatalf("nhật ký chủ nhà: %+v", s.Thefts)
	}
}

func TestChaseExpires(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	other := e.newThief("nguoikhac")
	e.ripeCarrots(2)
	e.guard()

	// Kẻ trộm bỏ chạy giữa chừng: quá hạn thì chủ mở nông trại là chốt bị cắn, ô mở khoá cho người khác.
	ch := e.chase(thief, 0)
	e.after(ChaseTTLMs)
	s := e.ok(e.f.State(ctx, e.uid))
	if th := s.Thefts[0]; !th.Caught || th.HPLost != ChaseBiteHP || th.Chasing {
		t.Fatalf("quá hạn: %+v", th)
	}
	if e.hp(thief).HP != wallet.MaxHP-ChaseBiteHP {
		t.Fatal("quá hạn mà không mất máu")
	}
	_, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	wantStatus(t, err, http.StatusConflict)
	if v, _ := e.f.Visit(ctx, e.uid, other); v.Plots[0].Locked {
		t.Fatal("quá hạn mà ô vẫn khoá")
	}

	// Báo thoát sau khi quá hạn (chưa ai chốt) cũng là bị bắt.
	ch = e.chase(other, 0)
	e.after(ChaseTTLMs + 1)
	res, err := e.f.FinishSteal(ctx, other, e.uid, ch.ID, true)
	if err != nil || res.Result != "caught" || res.HPLost != ChaseBiteHP {
		t.Fatalf("báo thoát muộn: %+v, %v", res, err)
	}
}

func TestChaseTokenBelongsToThief(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	other := e.newThief("nguoikhac")
	e.ripeCarrots(1)
	e.guard()
	ch := e.chase(thief, 0)
	e.after(ChaseMinMs)
	_, err := e.f.FinishSteal(ctx, other, e.uid, ch.ID, true)
	wantStatus(t, err, http.StatusNotFound)
	_, err = e.f.FinishSteal(ctx, thief, other, ch.ID, true)
	wantStatus(t, err, http.StatusNotFound)
	_, err = e.f.FinishSteal(ctx, thief, e.uid, "", true)
	wantStatus(t, err, http.StatusBadRequest)
	if res, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true); err != nil || res.Result != "stolen" {
		t.Fatalf("chủ token: %+v, %v", res, err)
	}
}

func TestChaseOwnerHarvestsMeanwhile(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(1)
	e.guard()
	ch := e.chase(thief, 0)
	if _, _, err := e.f.Harvest(ctx, e.uid, 0); err != nil {
		t.Fatal(err)
	}
	e.after(ChaseMinMs)
	res, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	if err != nil || res.Result != "gone" || res.Qty != 0 || res.HPLost != 0 {
		t.Fatalf("chủ đã thu hoạch: %+v, %v", res, err)
	}
	if ts := e.ok(e.f.State(ctx, thief)); ts.Storage.Products.Items["carrot"] != 0 {
		t.Fatal("ô đã thu hoạch mà vẫn lấy được")
	}
}

func TestStealLimitPerDay(t *testing.T) {
	e, thief := thiefEnv(t, 1_000_000)
	// 00:30 giờ Việt Nam để cả 5 lần nằm trong một ngày.
	e.clock = time.Date(2027, 1, 10, 0, 30, 0, 0, vnZone)
	e.ripeCarrots(FreePlots) // 6 ô: mỗi ô chỉ thử được một lần
	for i := range StealsPerDay {
		if _, err := e.f.Steal(ctx, thief, e.uid, i); err != nil {
			t.Fatalf("lần %d: %v", i+1, err)
		}
	}
	_, err := e.f.Steal(ctx, thief, e.uid, StealsPerDay)
	wantStatus(t, err, http.StatusConflict)
	v, err := e.f.Visit(ctx, e.uid, thief)
	if err != nil || v.StealsLeft != 0 {
		t.Fatalf("lượt còn: %d, %v", v.StealsLeft, err)
	}
	// Sang ngày mới (0h giờ Việt Nam) có lại lượt.
	e.clock = time.Date(2027, 1, 11, 0, 0, 1, 0, vnZone)
	if _, err := e.f.Steal(ctx, thief, e.uid, StealsPerDay); err != nil {
		t.Fatalf("ngày mới: %v", err)
	}
}

// Mỗi ô mỗi vụ: bị trộm thành công một lần (ai trộm cũng tính); mỗi người thử một lần, bị bắt rồi người khác vẫn thử được.
func TestOneTryPerPlotPerCrop(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	thief2, thief3 := e.newThief("ketrom2"), e.newThief("ketrom3")
	e.ripeCarrots(2)
	e.guard()

	ch := e.chase(thief, 0)
	if _, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, false); err != nil {
		t.Fatal(err)
	}
	if sum, _ := e.f.Summaries(ctx, thief, []int64{e.uid}); sum[e.uid].Ripe != 1 || !sum[e.uid].DogGuarding {
		t.Fatalf("ô chín còn thử được cho người 1: %+v", sum[e.uid])
	}
	v, _ := e.f.Visit(ctx, e.uid, thief2)
	if p := v.Plots[0]; p.Tried || p.Stolen || p.Locked {
		t.Fatalf("người thứ hai thấy ô 0: %+v", p)
	}
	ch = e.chase(thief2, 0)
	e.after(ChaseMinMs)
	if res, err := e.f.FinishSteal(ctx, thief2, e.uid, ch.ID, true); err != nil || res.Result != "stolen" {
		t.Fatalf("người thứ hai: %+v, %v", res, err)
	}
	_, err := e.f.Steal(ctx, thief3, e.uid, 0)
	if err == nil || err.Error() != "Ô này đã bị trộm rồi" {
		t.Fatalf("ô đã bị trộm: %v", err)
	}
	if sum, _ := e.f.Summaries(ctx, thief3, []int64{e.uid}); sum[e.uid].Ripe != 1 {
		t.Fatalf("người 3 còn %d ô chín", sum[e.uid].Ripe)
	}

	// Chủ thu hoạch rồi trồng lại: vụ mới, người cũ thử lại được.
	if _, _, err := e.f.HarvestAll(ctx, e.uid); err != nil {
		t.Fatal(err)
	}
	e.ripeCarrots(1)
	v, _ = e.f.Visit(ctx, e.uid, thief)
	if p := v.Plots[0]; p.Tried || p.Stolen {
		t.Fatalf("vụ mới vẫn đánh dấu: %+v", p)
	}
	e.chase(thief, 0)
}

func TestChaseFinishNeedsRoom(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(1)
	e.guard()
	ch := e.chase(thief, 0)
	ts := e.ok(e.f.State(ctx, thief))
	e.giveItem(thief, storeProducts, "corn", ts.Storage.Products.free())
	e.after(ChaseMinMs)
	_, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true)
	wantStatus(t, err, http.StatusConflict)
	// Còn hạn: bán bớt rồi báo lại được.
	e.giveItem(thief, storeProducts, "corn", -1)
	if res, err := e.f.FinishSteal(ctx, thief, e.uid, ch.ID, true); err != nil || res.Result != "stolen" {
		t.Fatalf("báo lại: %+v, %v", res, err)
	}
}

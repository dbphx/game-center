package farm

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
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
	// Hết 4 giờ: đói, không canh nhà.
	e.clock = e.clock.Add(4 * time.Hour)
	if s = e.ok(e.f.State(ctx, e.uid)); s.Dog.Guarding {
		t.Fatalf("quá 4 giờ chó vẫn canh: %+v", s.Dog)
	}
	if s = e.ok(e.f.FeedDog(ctx, e.uid)); !s.Dog.Guarding {
		t.Fatal("cho ăn lại mà chó không canh")
	}
}

// Thức ăn cho chó cộng dồn: mỗi phần thêm 4 giờ vào thời gian còn lại, tối đa 7 ngày tính từ bây giờ.
func TestDogFeedStacks(t *testing.T) {
	e := newEnv(t, 10_000, 0)
	e.giveItem(e.uid, storeSeeds, DogFeedID, 50)
	s := e.ok(e.f.FeedDog(ctx, e.uid))
	if s.Dog.MaxFull != 7*24*3600 {
		t.Fatalf("maxFull: %+v", s.Dog)
	}
	e.clock = e.clock.Add(time.Hour) // còn 3 giờ, cho ăn thành 7 giờ
	if s = e.ok(e.f.FeedDog(ctx, e.uid)); s.Dog.FedUntil != e.clock.Unix()+7*3600 {
		t.Fatalf("còn 3 giờ cho ăn thêm: còn %d giây", s.Dog.FedUntil-e.clock.Unix())
	}
	// Đói từ lâu thì tính từ bây giờ, không cộng phần đã qua.
	e.clock = e.clock.Add(24 * time.Hour)
	if s = e.ok(e.f.FeedDog(ctx, e.uid)); s.Dog.FedUntil != e.clock.Unix()+4*3600 {
		t.Fatalf("đói rồi cho ăn: còn %d giây", s.Dog.FedUntil-e.clock.Unix())
	}
	for range 41 { // đủ 42 phần = đúng 7 ngày
		s = e.ok(e.f.FeedDog(ctx, e.uid))
	}
	if s.Dog.FedUntil != e.clock.Unix()+DogMaxFull {
		t.Fatalf("42 phần: còn %d giây", s.Dog.FedUntil-e.clock.Unix())
	}
	before := s.Storage.Seeds.Items[DogFeedID]
	xp := e.xpOf(e.uid)
	_, err := e.f.FeedDog(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	s = e.ok(e.f.State(ctx, e.uid))
	if s.Storage.Seeds.Items[DogFeedID] != before || e.xpOf(e.uid) != xp || s.Dog.FedUntil != e.clock.Unix()+DogMaxFull {
		t.Fatalf("no tối đa mà vẫn trừ: %+v còn %d phần", s.Dog, s.Storage.Seeds.Items[DogFeedID])
	}
	// Vơi đi 4 giờ thì lại cho ăn được đúng 1 phần.
	e.clock = e.clock.Add(4 * time.Hour)
	e.ok(e.f.FeedDog(ctx, e.uid))
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

// ---------- Trộm vật nuôi và cá ----------

// grownStock thêm vào nhà chủ một con đã lớn.
func (e *env) grownStock(place, kind string) int64 {
	e.t.Helper()
	st, ok := findStock(place, kind)
	if !ok {
		e.t.Fatalf("không có %s ở %s", kind, place)
	}
	return e.addStock(place, kind, st.GrowSeconds)
}

func (e *env) feedOwnerDog() {
	e.t.Helper()
	e.exec(`INSERT INTO farm_dog (user_id, fed_until) VALUES (?, ?) ON CONFLICT (user_id) DO UPDATE SET fed_until = excluded.fed_until`,
		e.uid, e.clock.Unix()+4*3600)
}

func (e *env) hasStock(id int64) bool {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM farm_stock WHERE id = ?`, id) == 1
}

func (e *env) xpOf(uid int64) int64 {
	e.t.Helper()
	u, err := e.w.User(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.XP
}

// Chó đói: lấy ngay đúng món chủ nhận khi thu con đó, không có XP; chủ mất con và chỗ trong chuồng.
func TestStealStockHungryDog(t *testing.T) {
	cases := []struct{ place, kind, item string }{
		{placeBarn, "cow", "meat_cow"},
		{placeYard, "chicken", "meat_chicken"},
		{placePond, "koi", "koi"},
	}
	for _, c := range cases {
		e, thief := thiefEnv(t, 1000)
		id := e.grownStock(c.place, c.kind)
		used := penOf(e.ok(e.f.State(ctx, e.uid)), c.place).Used
		xp := e.xpOf(thief)
		res, err := e.f.StealStock(ctx, thief, e.uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if res.Result != "stolen" || res.Item != c.item || res.Qty != 1 || res.HPLost != 0 || res.Chase != nil {
			t.Fatalf("%s: %+v", c.kind, res)
		}
		if res.Visit.StealsLeft != StealsPerDay-1 || len(res.Visit.Stock) != 0 {
			t.Fatalf("%s: sau khi trộm %+v", c.kind, res.Visit)
		}
		if e.hasStock(id) {
			t.Fatalf("%s: chủ vẫn còn con", c.kind)
		}
		st := e.ok(e.f.State(ctx, e.uid))
		if penOf(st, c.place).Used >= used {
			t.Fatalf("%s: chuồng không trống lại: %d", c.kind, penOf(st, c.place).Used)
		}
		if len(st.Thefts) != 1 || st.Thefts[0].Item != c.item || st.Thefts[0].Qty != 1 || st.Thefts[0].Caught || st.Thefts[0].Thief.ID != thief {
			t.Fatalf("%s: nhật ký %+v", c.kind, st.Thefts)
		}
		if got := e.ok(e.f.State(ctx, thief)).Storage.Products.Items[c.item]; got != 1 {
			t.Fatalf("%s: kho kẻ trộm có %d", c.kind, got)
		}
		if e.xpOf(thief) != xp {
			t.Fatalf("%s: trộm không được cộng XP", c.kind)
		}
	}
}

// Các trường hợp bị từ chối: không đổi gì ở cả hai nhà, không mất lượt.
func TestStealStockRefused(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	refused := func(id int64, status int) {
		t.Helper()
		left := e.mustVisit(thief).StealsLeft
		had := e.hasStock(id)
		_, err := e.f.StealStock(ctx, thief, e.uid, id)
		wantStatus(t, err, status)
		if e.mustVisit(thief).StealsLeft != left || e.hasStock(id) != had {
			t.Fatalf("con %d: bị từ chối mà vẫn đổi", id)
		}
	}
	young := e.addStock(placeBarn, "pig", 0)
	refused(young, http.StatusConflict)
	refused(999_999, http.StatusNotFound)
	refused(0, http.StatusNotFound)

	// Con của nhà khác, kể cả của chính kẻ trộm.
	e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at, fed_until, grow_progress, progress_at)
		VALUES (?, 'barn', 'pig', 1, 0, 0, 999999, 1)`, thief)
	other := int64(e.count(`SELECT MAX(id) FROM farm_stock WHERE user_id = ?`, thief))
	refused(other, http.StatusNotFound)

	bitten := e.grownStock(placeBarn, "pig")
	e.exec(`INSERT INTO farm_stock_zombies (stock_id, user_id, wave, bites) VALUES (?, ?, ?, 0)`, bitten, e.uid, e.clock.Unix()/3600)
	refused(bitten, http.StatusConflict)

	id := e.grownStock(placeBarn, "pig")
	e.giveItem(thief, storeProducts, "carrot", e.ok(e.f.State(ctx, thief)).Storage.Products.Capacity)
	refused(id, http.StatusConflict) // kho đầy
	e.exec(`DELETE FROM farm_items WHERE user_id = ?`, thief)

	for range StealsPerDay { // hết lượt, chung với trộm cây
		e.exec(`INSERT INTO farm_thefts (owner_id, thief_id, idx, planted_at, item, qty, at) VALUES (?, ?, 0, 0, 'carrot', 1, ?)`,
			e.uid, thief, e.clock.Unix())
	}
	refused(id, http.StatusConflict)

	if _, err := e.f.StealStock(ctx, e.uid, e.uid, id); errStatus(err) != http.StatusBadRequest {
		t.Fatalf("trộm nhà mình: %v", err)
	}
}

func (e *env) mustVisit(viewer int64) Visit {
	e.t.Helper()
	v, err := e.f.Visit(ctx, e.uid, viewer)
	if err != nil {
		e.t.Fatal(err)
	}
	return v
}

// Chó no: thành cuộc đuổi, con khoá với người khác; thoát thì lấy được.
func TestStealStockChaseEscape(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	other := e.newThief("keKhac")
	id := e.grownStock(placeBarn, "pig")
	e.feedOwnerDog()
	res, err := e.f.StealStock(ctx, thief, e.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "chase" || res.Chase == nil || res.Chase.Stock != id || res.Chase.Plot != -1 || res.Chase.Item != "meat_pig" {
		t.Fatalf("cuộc đuổi: %+v %+v", res, res.Chase)
	}
	if !e.hasStock(id) || e.count(`SELECT count(*) FROM farm_items WHERE user_id = ? AND qty > 0`, thief) != 0 {
		t.Fatal("chưa thoát mà đã lấy được đồ")
	}
	if v := e.mustVisit(other); len(v.Stock) != 1 || !v.Stock[0].Locked || v.Stock[0].Tried {
		t.Fatalf("người khác xem: %+v", v.Stock)
	}
	if v := e.mustVisit(thief); !v.Stock[0].Tried || v.Stock[0].Locked {
		t.Fatalf("kẻ trộm xem: %+v", v.Stock)
	}
	_, err = e.f.StealStock(ctx, other, e.uid, id)
	wantStatus(t, err, http.StatusConflict) // đang khoá
	_, err = e.f.StealStock(ctx, thief, e.uid, id)
	wantStatus(t, err, http.StatusConflict) // đã thử
	e.clock = e.clock.Add(ChaseMinMs * time.Millisecond)
	done, err := e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if done.Result != "stolen" || done.Item != "meat_pig" || done.Qty != 1 || e.hasStock(id) {
		t.Fatalf("thoát: %+v", done)
	}
	if got := e.ok(e.f.State(ctx, thief)).Storage.Products.Items["meat_pig"]; got != 1 {
		t.Fatalf("kho kẻ trộm: %d", got)
	}
	if _, err = e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true); errStatus(err) != http.StatusConflict {
		t.Fatalf("dùng lại token: %v", err)
	}
	if got := e.ok(e.f.State(ctx, thief)).Storage.Products.Items["meat_pig"]; got != 1 {
		t.Fatalf("dùng lại token mà có thêm đồ: %d", got)
	}
}

// Bị chạm hoặc quá hạn: mất 50 máu, con còn, và kẻ trộm đã mất lượt thử con đó.
func TestStealStockCaught(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	id := e.grownStock(placeBarn, "pig")
	id2 := e.grownStock(placeBarn, "pig")
	e.feedOwnerDog()
	hp := func() int {
		u, err := e.w.User(ctx, thief)
		if err != nil {
			t.Fatal(err)
		}
		return u.HP
	}
	before := hp()
	res, err := e.f.StealStock(ctx, thief, e.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	done, err := e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if done.Result != "caught" || done.HPLost != ChaseBiteHP || hp() != before-ChaseBiteHP || !e.hasStock(id) {
		t.Fatalf("bị bắt: %+v hp %d", done, hp())
	}
	_, err = e.f.StealStock(ctx, thief, e.uid, id)
	wantStatus(t, err, http.StatusConflict) // bị bắt cũng mất lượt thử con đó
	if v := e.mustVisit(thief); !v.Stock[0].Tried {
		t.Fatalf("visit: %+v", v.Stock)
	}
	// Báo thoát quá sớm cũng tính là bị bắt.
	res, err = e.f.StealStock(ctx, thief, e.uid, id2)
	if err != nil {
		t.Fatal(err)
	}
	if done, err = e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true); err != nil || done.Result != "caught" || !e.hasStock(id2) {
		t.Fatalf("báo thoát sớm: %+v %v", done, err)
	}
}

func TestStealStockChaseExpires(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	id := e.grownStock(placeBarn, "pig")
	e.feedOwnerDog()
	if _, err := e.f.StealStock(ctx, thief, e.uid, id); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(ChaseTTLMs * time.Millisecond)
	st := e.ok(e.f.State(ctx, e.uid))
	if !e.hasStock(id) || len(st.Thefts) != 1 || !st.Thefts[0].Caught || st.Thefts[0].HPLost != ChaseBiteHP {
		t.Fatalf("quá hạn: %+v", st.Thefts)
	}
	if v := e.mustVisit(e.newThief("keKhac")); v.Stock[0].Locked {
		t.Fatalf("hết hạn vẫn khoá: %+v", v.Stock)
	}
}

// Chủ thu hoặc bán con trong lúc đuổi: thoát mà không có đồ, không bị cắn.
func TestStealStockOwnerActsDuringChase(t *testing.T) {
	for _, act := range []string{"sell", "collect"} {
		e, thief := thiefEnv(t, 1000)
		id := e.grownStock(placeBarn, "pig")
		e.feedOwnerDog()
		res, err := e.f.StealStock(ctx, thief, e.uid, id)
		if err != nil {
			t.Fatal(err)
		}
		if act == "sell" {
			if _, _, err = e.f.SellStock(ctx, e.uid, placeBarn, id); err != nil {
				t.Fatal(err)
			}
		} else if _, _, err = e.f.CollectStock(ctx, e.uid, placeBarn, id); err != nil {
			t.Fatal(err)
		}
		e.clock = e.clock.Add(ChaseMinMs * time.Millisecond)
		u, _ := e.w.User(ctx, thief)
		done, err := e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := e.w.User(ctx, thief)
		if done.Result != "gone" || done.Qty != 0 || done.HPLost != 0 || after.HP != u.HP {
			t.Fatalf("%s: %+v", act, done)
		}
		if n := e.ok(e.f.State(ctx, thief)).Storage.Products.Used; n != 0 {
			t.Fatalf("%s: kẻ trộm có %d món", act, n)
		}
	}
}

// Hai cuộc đuổi cùng một con (dựng thẳng trong database) cũng không lấy được hai lần.
func TestStealStockTwoTokensOneAnimal(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	other := e.newThief("keKhac")
	id := e.grownStock(placeBarn, "pig")
	e.feedOwnerDog()
	res, err := e.f.StealStock(ctx, thief, e.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	nowMs := e.clock.UnixMilli()
	e.exec(`INSERT INTO farm_thefts (owner_id, thief_id, idx, planted_at, stock_id, item, qty, at, pending, token, started_ms, expires_ms)
		VALUES (?, ?, -1, 0, ?, 'meat_pig', 0, ?, 1, 'token2', ?, ?)`, e.uid, other, id, e.clock.Unix(), nowMs, nowMs+ChaseTTLMs)
	e.clock = e.clock.Add(ChaseMinMs * time.Millisecond)
	first, err := e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true)
	if err != nil || first.Result != "stolen" {
		t.Fatalf("lần đầu: %+v %v", first, err)
	}
	second, err := e.f.FinishSteal(ctx, other, e.uid, "token2", true)
	if err != nil || second.Result != "gone" {
		t.Fatalf("lần hai: %+v %v", second, err)
	}
	if n := e.ok(e.f.State(ctx, other)).Storage.Products.Used; n != 0 {
		t.Fatalf("lấy được hai lần: %d", n)
	}
}

// Visit.stock chỉ có đúng các trường đã thống nhất, không lộ thông tin nuôi của chủ.
func TestVisitStockFields(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	id := e.grownStock(placePond, "tilapia")
	young := e.addStock(placeBarn, "pig", 0)
	v := e.mustVisit(thief)
	if len(v.Stock) != 2 {
		t.Fatalf("stock: %+v", v.Stock)
	}
	byID := map[int64]VisitStock{}
	for _, s := range v.Stock {
		byID[s.ID] = s
	}
	if s := byID[id]; s.Place != placePond || s.Kind != "tilapia" || !s.Grown || s.Tried || s.Locked || s.Zombie {
		t.Fatalf("cá: %+v", s)
	}
	if s := byID[young]; s.Place != placeBarn || s.Grown {
		t.Fatalf("heo non: %+v", s)
	}
	raw, err := json.Marshal(v.Stock[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	want := []string{"grown", "id", "kind", "locked", "place", "tried", "zombie"}
	got := make([]string, 0, len(fields))
	for k := range fields {
		got = append(got, k)
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("trường của stock: %v", got)
	}
	// Nhà không có con nào: mảng rỗng, không phải null.
	e2, thief2 := thiefEnv(t, 1000)
	raw, _ = json.Marshal(e2.mustVisit(thief2))
	if !strings.Contains(string(raw), `"stock":[]`) {
		t.Fatalf("visit rỗng: %s", raw)
	}
}

// Dòng trộm con không làm sai dấu của ô đất, và trộm cây không làm sai dấu của con.
func TestStockTheftKeepsPlotMarks(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(1)
	id := e.grownStock(placeBarn, "pig")
	id2 := e.grownStock(placeBarn, "pig")
	if _, err := e.f.StealStock(ctx, thief, e.uid, id); err != nil {
		t.Fatal(err)
	}
	v := e.mustVisit(thief)
	if v.Plots[0].Tried || v.Plots[0].Stolen || v.Plots[0].Locked {
		t.Fatalf("ô đất bị đánh dấu nhầm: %+v", v.Plots[0])
	}
	if _, err := e.f.Steal(ctx, thief, e.uid, 0); err != nil {
		t.Fatal(err)
	}
	v = e.mustVisit(thief)
	if !v.Plots[0].Tried || len(v.Stock) != 1 || v.Stock[0].ID != id2 || v.Stock[0].Tried {
		t.Fatalf("sau khi trộm cây: %+v %+v", v.Plots[0], v.Stock)
	}
	if v.StealsLeft != StealsPerDay-2 {
		t.Fatalf("lượt chung: %d", v.StealsLeft)
	}
	if s, err := e.f.Summaries(ctx, thief, []int64{e.uid}); err != nil || s[e.uid].Ripe != 0 {
		t.Fatalf("tóm tắt: %+v %v", s, err)
	}
}

// Xoá con bằng đường trộm thì không sót dòng zombie.
func TestStealStockLeavesNoZombieRow(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	id := e.grownStock(placeBarn, "pig")
	e.feedOwnerDog()
	res, err := e.f.StealStock(ctx, thief, e.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	// Zombie tới đúng lúc đang đuổi.
	e.exec(`INSERT INTO farm_stock_zombies (stock_id, user_id, wave, bites) VALUES (?, ?, ?, 0)`, id, e.uid, e.clock.Unix()/3600)
	e.clock = e.clock.Add(ChaseMinMs * time.Millisecond)
	if _, err := e.f.FinishSteal(ctx, thief, e.uid, res.Chase.ID, true); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM farm_stock_zombies WHERE stock_id = ?`, id); n != 0 {
		t.Fatalf("sót %d dòng zombie", n)
	}
}

// Trộm không tạo thêm giá trị so với chủ tự thu.
func TestStealStockWorthNotCreated(t *testing.T) {
	stolen, thief := thiefEnv(t, 1000)
	id := stolen.grownStock(placeBarn, "cow")
	if _, err := stolen.f.StealStock(ctx, thief, stolen.uid, id); err != nil {
		t.Fatal(err)
	}
	w1, err := stolen.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	self, thief2 := thiefEnv(t, 1000)
	id = self.grownStock(placeBarn, "cow")
	if _, _, err := self.f.CollectStock(ctx, self.uid, placeBarn, id); err != nil {
		t.Fatal(err)
	}
	w2, err := self.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := w1[stolen.uid]+w1[thief], w2[self.uid]+w2[thief2]; a != b {
		t.Fatalf("tổng giá trị khi trộm %d, khi tự thu %d", a, b)
	}
	if w1[thief] == 0 {
		t.Fatal("kẻ trộm phải có giá trị món trộm")
	}
}

// Database cũ không có cột stock_id được nâng cấp lúc khởi động, dòng cũ vẫn đúng.
func TestMigrateAddsStockIDColumn(t *testing.T) {
	e, thief := thiefEnv(t, 1000)
	e.ripeCarrots(1)
	e.exec(`ALTER TABLE farm_thefts DROP COLUMN stock_id`)
	e.exec(`INSERT INTO farm_thefts (owner_id, thief_id, idx, planted_at, item, qty, at) VALUES (?, ?, 0, ?, 'carrot', 1, ?)`,
		e.uid, thief, e.ok(e.f.State(ctx, e.uid)).Plots[0].PlantedAt, e.clock.Unix())
	for range 2 { // khởi động lại hai lần: lần sau không lỗi
		if _, err := New(ctx, e.w); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.count(`SELECT count(*) FROM farm_thefts WHERE stock_id = 0`); n != 1 {
		t.Fatalf("dòng cũ: %d", n)
	}
	if v := e.mustVisit(thief); !v.Plots[0].Tried {
		t.Fatalf("dấu ô đất cũ mất: %+v", v.Plots[0])
	}
}

// Khách ghé thăm thấy cây đậu thần ở ô 49 và khẩu súng đậu của chủ, không thấy số đạn hay chậu trên mây.
func TestVisitShowsBeanAndGun(t *testing.T) {
	e, thief := thiefEnv(t, 100_000)
	v, err := e.f.Visit(ctx, e.uid, thief)
	if err != nil {
		t.Fatal(err)
	}
	if v.Bean.Available || v.Bean.Floors != 0 || v.Gun {
		t.Fatalf("nhà chưa có gì: %+v gun=%v", v.Bean, v.Gun)
	}
	e.set(100_000_000, 0)
	e.openFloors(2)
	e.ok(e.f.PlantBean(ctx, e.uid))
	e.ok(e.f.BuyGun(ctx, e.uid))
	e.ok(e.f.BuyAmmo(ctx, e.uid, 7))
	if v, err = e.f.Visit(ctx, e.uid, thief); err != nil {
		t.Fatal(err)
	}
	if !v.Bean.Available || v.Bean.Floors != 2 || v.Bean.PlantedAt == 0 || v.Bean.ReadyAt <= v.Bean.PlantedAt || !v.Gun {
		t.Fatalf("phải thấy đậu tầng 2 đang lớn và súng: %+v gun=%v", v.Bean, v.Gun)
	}
	raw, _ := json.Marshal(v)
	for _, leak := range []string{"ammo", "pots", "cloud"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("lộ %q khi ghé thăm: %s", leak, raw)
		}
	}
}

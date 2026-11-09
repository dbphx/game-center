package quests

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/games/race"
	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	q     *Quests
	clock time.Time
	uid   int64
}

// newEnv: ví + nhiệm vụ + garage đua, đồng hồ giả (thứ Ba 10/11/2026 9:00 giờ VN), một người chơi.
func newEnv(t *testing.T, balance int64) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	if _, err := race.NewGarage(ctx, w); err != nil {
		t.Fatal(err)
	}
	q, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, w: w, q: q, clock: time.Date(2026, 11, 10, 9, 0, 0, 0, vietnam)}
	q.now = func() time.Time { return e.clock }
	u, _, err := w.Register(ctx, "nongdan", "matkhau123", wallet.Profile{Name: "Nông dân"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.exec(`UPDATE users SET balance = ? WHERE id = ?`, balance, e.uid)
	return e
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) view() State {
	e.t.Helper()
	s, err := e.q.View(ctx, e.uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// finish cộng đủ tiến độ cho nhiệm vụ id.
func (e *env) finish(s State, id string) {
	e.t.Helper()
	qu, ok := findQuest(s, id)
	if !ok {
		e.t.Fatalf("không có %s", id)
	}
	e.q.Record(ctx, nil, e.uid, qu.Kind, qu.Target)
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if got := errStatus(err); got != status {
		t.Fatalf("lỗi %v: mã %d, muốn %d", err, got, status)
	}
}

func shardOf(s State, item string) ShardView {
	for _, v := range s.Shards {
		if v.Item == item {
			return v
		}
	}
	return ShardView{}
}

func TestGenerateDailyWeekly(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	if len(s.Daily) != DailyCount || len(s.Weekly) != WeeklyCount {
		t.Fatalf("muốn 5 ngày + 3 tuần, được %d + %d", len(s.Daily), len(s.Weekly))
	}
	seen := map[string]bool{}
	for _, q := range s.Daily {
		if seen[q.Kind] {
			t.Fatalf("trùng loại %s", q.Kind)
		}
		seen[q.Kind] = true
		if q.Kind == FarmCloud {
			t.Fatal("chưa có chậu tầng mây thì không giao nhiệm vụ tầng mây")
		}
		tp, _ := findTemplate(q.Kind)
		if q.Target != tp.DailyMin || q.Reward != (Reward{1, 1, 20}) {
			t.Fatalf("nhiệm vụ cấp 1: %+v", q)
		}
	}
	for _, q := range s.Weekly {
		tp, _ := findTemplate(q.Kind)
		if tp.Weekly == 0 || q.Target != tp.Weekly || q.Reward != (Reward{3, 3, 100}) {
			t.Fatalf("nhiệm vụ tuần: %+v", q)
		}
	}
	// Xem lại không sinh mới.
	if s2 := e.view(); s2.Daily[0] != s.Daily[0] {
		t.Fatal("xem lại phải giữ nhiệm vụ cũ")
	}
	if s.NPC != "santa" || s.Pending != 0 || s.RerollCost != 0 || s.RerollsLeft != RerollsPerDay {
		t.Fatalf("trạng thái đầu: npc %s pending %d reroll %d/%d", s.NPC, s.Pending, s.RerollCost, s.RerollsLeft)
	}
	if len(s.Shards) != len(Shards) || len(Shards) != 14+6 {
		t.Fatalf("muốn 20 loại mảnh, được %d", len(s.Shards))
	}
}

func TestTargetsScaleWithLevel(t *testing.T) {
	tp, _ := findTemplate(FarmHarvest)
	if tp.dailyTarget(1) != 10 || tp.dailyTarget(30) != 30 || tp.dailyTarget(99) != 30 || tp.dailyTarget(15) != 20 {
		t.Fatalf("thu hoạch theo cấp: %d %d %d", tp.dailyTarget(1), tp.dailyTarget(15), tp.dailyTarget(30))
	}
}

func TestResetDayAndWeekVietnamTime(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	if want := time.Date(2026, 11, 11, 0, 0, 0, 0, vietnam).Unix(); s.ResetAt.Day != want {
		t.Fatalf("reset ngày %d, muốn %d", s.ResetAt.Day, want)
	}
	if want := time.Date(2026, 11, 16, 0, 0, 0, 0, vietnam).Unix(); s.ResetAt.Week != want {
		t.Fatalf("reset tuần %d, muốn %d", s.ResetAt.Week, want)
	}
	e.finish(s, "d0")
	e.finish(s, "w0")
	// 23:59 cùng ngày: vẫn kỳ cũ.
	e.clock = time.Date(2026, 11, 10, 23, 59, 0, 0, vietnam)
	if !e.view().Daily[0].Done {
		t.Fatal("chưa qua ngày thì giữ tiến độ")
	}
	// 0:00 hôm sau (17:00 UTC): ngày mới, tuần giữ.
	e.clock = time.Date(2026, 11, 10, 17, 0, 0, 0, time.UTC)
	s = e.view()
	for _, q := range s.Daily {
		if q.Progress != 0 {
			t.Fatal("ngày mới phải có nhiệm vụ mới")
		}
	}
	if !s.Weekly[0].Done {
		t.Fatal("trong tuần thì giữ nhiệm vụ tuần")
	}
	// Chủ nhật 23:00 vẫn tuần cũ; thứ Hai 0:00 tuần mới.
	e.clock = time.Date(2026, 11, 15, 23, 0, 0, 0, vietnam)
	if !e.view().Weekly[0].Done {
		t.Fatal("chủ nhật vẫn tuần cũ")
	}
	e.clock = time.Date(2026, 11, 16, 0, 0, 0, 0, vietnam)
	if e.view().Weekly[0].Done {
		t.Fatal("thứ Hai là tuần mới")
	}
}

func TestRecordProgressAndClaim(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	d0 := s.Daily[0]
	e.q.Record(ctx, nil, e.uid, d0.Kind, 1)
	e.q.Record(ctx, nil, e.uid, "khong_co", 5) // loại lạ: bỏ qua
	if got, _ := findQuest(e.view(), "d0"); got.Progress != 1 || got.Done {
		t.Fatalf("tiến độ sau 1: %+v", got)
	}
	_, _, _, err := e.q.Claim(ctx, e.uid, "d0")
	wantStatus(t, err, http.StatusConflict) // chưa xong
	e.q.Record(ctx, nil, e.uid, d0.Kind, 1000)
	s = e.view()
	got, _ := findQuest(s, "d0")
	if got.Progress != got.Target || !got.Done || s.Pending < 1 {
		t.Fatalf("tiến độ không vượt mục tiêu: %+v pending %d", got, s.Pending)
	}
	if got.Reward.Parts != DailyParts {
		t.Fatalf("thưởng nhiệm vụ ngày phải có %d phụ kiện: %+v", DailyParts, got.Reward)
	}
	s, gained, parts, err := e.q.Claim(ctx, e.uid, "d0")
	if err != nil {
		t.Fatal(err)
	}
	if len(gained) != 1 || gained[0].Qty != 1 || shardOf(s, gained[0].Item).Qty != 1 {
		t.Fatalf("nhận 1 mảnh: %+v", gained)
	}
	if partSum(parts) != DailyParts || totalParts(s) != DailyParts || parts[0].Name == "" {
		t.Fatalf("nhận phụ kiện: %+v, kho %v", parts, s.Parts)
	}
	u, _ := e.w.User(ctx, e.uid)
	if u.XP != DailyXP {
		t.Fatalf("XP %d, muốn %d", u.XP, DailyXP)
	}
	_, _, _, err = e.q.Claim(ctx, e.uid, "d0")
	wantStatus(t, err, http.StatusConflict) // nhận một lần
	_, _, _, err = e.q.Claim(ctx, e.uid, "x9")
	wantStatus(t, err, http.StatusBadRequest)
	// Đã nhận thì sự kiện sau không cộng nữa.
	e.q.Record(ctx, nil, e.uid, d0.Kind, 1)
}

func TestRecordInsideGameTxRollsBackWithIt(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	tx, _ := e.w.Begin(ctx)
	e.q.Record(ctx, tx, e.uid, s.Daily[0].Kind, 1)
	tx.Rollback()
	if got, _ := findQuest(e.view(), "d0"); got.Progress != 0 {
		t.Fatal("thao tác game huỷ thì tiến độ cũng huỷ")
	}
}

func TestRecordUniquePerDay(t *testing.T) {
	e := newEnv(t, 0)
	e.view()
	e.exec(`UPDATE quest_slots SET kind = ?, target = 3 WHERE user_id = ? AND slot = 0 AND period LIKE 'd:%'`, FarmVisit, e.uid)
	for range 3 {
		e.q.RecordUnique(ctx, nil, e.uid, FarmVisit, "ban-a")
	}
	e.q.RecordUnique(ctx, nil, e.uid, FarmVisit, "ban-b")
	if got, _ := findQuest(e.view(), "d0"); got.Progress != 2 {
		t.Fatalf("thăm 2 người khác nhau = 2, được %d", got.Progress)
	}
	e.clock = e.clock.Add(24 * time.Hour)
	e.view()
	e.exec(`UPDATE quest_slots SET kind = ?, target = 3 WHERE user_id = ? AND slot = 0 AND period = ?`, FarmVisit, e.uid, periodsAt(e.clock).day)
	e.q.RecordUnique(ctx, nil, e.uid, FarmVisit, "ban-a")
	if got, _ := findQuest(e.view(), "d0"); got.Progress != 1 {
		t.Fatal("ngày mới thăm lại người cũ vẫn tính")
	}
}

func TestChests(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	_, _, _, err := e.q.Chest(ctx, e.uid, "daily")
	wantStatus(t, err, http.StatusConflict)
	_, _, _, err = e.q.Chest(ctx, e.uid, "monthly")
	wantStatus(t, err, http.StatusBadRequest)
	for _, q := range s.Daily {
		e.finish(s, q.ID)
	}
	for _, q := range s.Weekly {
		e.finish(s, q.ID)
	}
	s = e.view()
	if !s.Chest.Daily.Ready || !s.Chest.Weekly.Ready || s.Pending != DailyCount+WeeklyCount+2 {
		t.Fatalf("rương sẵn sàng: %+v pending %d", s.Chest, s.Pending)
	}
	if s.Chest.Daily.Parts != DailyChestParts || s.Chest.Weekly.Parts != WeeklyChestParts {
		t.Fatalf("phụ kiện rương: %+v", s.Chest)
	}
	s, g, parts, err := e.q.Chest(ctx, e.uid, "daily")
	if err != nil || len(g) != 1 || !s.Chest.Daily.Claimed {
		t.Fatalf("mở rương ngày: %v %+v", err, g)
	}
	if partSum(parts) != DailyChestParts || totalParts(s) != DailyChestParts {
		t.Fatalf("rương ngày ra %d phụ kiện: %+v", DailyChestParts, parts)
	}
	_, _, _, err = e.q.Chest(ctx, e.uid, "daily")
	wantStatus(t, err, http.StatusConflict)
	_, g, parts, err = e.q.Chest(ctx, e.uid, "weekly")
	if err != nil || len(g) != 1 || g[0].Tier != TierExclusive {
		t.Fatalf("rương tuần ra mảnh độc quyền: %v %+v", err, g)
	}
	if partSum(parts) != WeeklyChestParts {
		t.Fatalf("rương tuần ra %d phụ kiện: %+v", WeeklyChestParts, parts)
	}
	_, _, parts, err = e.q.Claim(ctx, e.uid, "w0")
	if err != nil || partSum(parts) != WeeklyParts {
		t.Fatalf("nhiệm vụ tuần ra %d phụ kiện: %v %+v", WeeklyParts, err, parts)
	}
}

func TestReroll(t *testing.T) {
	e := newEnv(t, 5000)
	s := e.view()
	old := s.Daily[1]
	s, cost, err := e.q.Reroll(ctx, e.uid, "d1")
	if err != nil || cost != 0 {
		t.Fatalf("lần đầu miễn phí: %v %d", err, cost)
	}
	if s.Daily[1].Kind == old.Kind || s.Daily[1].Progress != 0 {
		t.Fatalf("phải đổi loại khác: %+v", s.Daily[1])
	}
	kinds := map[string]bool{}
	for _, q := range s.Daily {
		if kinds[q.Kind] {
			t.Fatal("đổi xong không được trùng loại")
		}
		kinds[q.Kind] = true
	}
	if s.RerollCost != RerollCost || s.RerollsLeft != RerollsPerDay-1 {
		t.Fatalf("sau lần đầu: giá %d còn %d", s.RerollCost, s.RerollsLeft)
	}
	if _, cost, err = e.q.Reroll(ctx, e.uid, "d1"); err != nil || cost != RerollCost {
		t.Fatalf("lần hai tốn xu: %v %d", err, cost)
	}
	if b, _ := e.w.Balance(ctx, e.uid); b != 3000 {
		t.Fatalf("số dư %d, muốn 3000", b)
	}
	if _, _, err = e.q.Reroll(ctx, e.uid, "d1"); err != nil {
		t.Fatal(err)
	}
	_, _, err = e.q.Reroll(ctx, e.uid, "d1")
	wantStatus(t, err, http.StatusPaymentRequired)
	_, _, err = e.q.Reroll(ctx, e.uid, "w0")
	wantStatus(t, err, http.StatusBadRequest)
	e.exec(`UPDATE users SET balance = 100000 WHERE id = ?`, e.uid)
	e.q.Reroll(ctx, e.uid, "d1")
	e.q.Reroll(ctx, e.uid, "d1")
	_, _, err = e.q.Reroll(ctx, e.uid, "d1")
	wantStatus(t, err, http.StatusConflict) // hết 5 lượt
	s = e.view()
	e.finish(s, "d2")
	_, _, err = e.q.Reroll(ctx, e.uid, "d2")
	wantStatus(t, err, http.StatusConflict)
}

func TestDropRates(t *testing.T) {
	e := newEnv(t, 0)
	count := map[string]int{}
	const n = 100_000
	for range n {
		count[e.q.rollShard("").Tier]++
	}
	for _, tier := range Tiers {
		got := float64(count[tier.ID]) / n * 1000
		if d := got - float64(tier.Rate); d > 8 || d < -8 {
			t.Fatalf("bậc %s: %.1f‰, muốn %d‰", tier.ID, got, tier.Rate)
		}
	}
	for _, tier := range Tiers {
		for _, sh := range shardsOf(tier.ID) {
			if sh.Tier != tier.ID {
				t.Fatal("rút theo bậc ra sai bậc")
			}
		}
	}
	// Bậc và số mảnh cần theo spec.
	for item, want := range map[string]int{"comet": 6, "raptor": 6, "viper": 8, "moto_neon": 8, "nova": 8, "x_aurora": 5, "x_frost": 5, "spoiler_x_glow": 3} {
		if sh, _ := findShard(item); sh.Need != want {
			t.Fatalf("%s cần %d mảnh, muốn %d", item, sh.Need, want)
		}
	}
	if _, ok := findShard("starter"); ok {
		t.Fatal("Xe Tập Lái không có mảnh")
	}
}

func TestAssemble(t *testing.T) {
	e := newEnv(t, 0)
	_, _, err := e.q.Assemble(ctx, e.uid, "comet")
	wantStatus(t, err, http.StatusConflict) // thiếu mảnh
	_, _, err = e.q.Assemble(ctx, e.uid, "starter")
	wantStatus(t, err, http.StatusBadRequest)
	e.exec(`INSERT INTO race_shards (user_id, item, qty) VALUES (?, 'nova', 9), (?, 'x_frost', 5), (?, 'paint_x_chroma', 3)`, e.uid, e.uid, e.uid)
	// Cấp 1 vẫn ghép được Siêu Tân Tinh (cấp chỉ chặn mua bằng xu).
	s, g, err := e.q.Assemble(ctx, e.uid, "nova")
	if err != nil || g != (Granted{"nova", "car"}) {
		t.Fatalf("ghép xe: %v %+v", err, g)
	}
	if v := shardOf(s, "nova"); !v.Owned || v.Qty != 1 {
		t.Fatalf("sau ghép: %+v", v)
	}
	_, _, err = e.q.Assemble(ctx, e.uid, "nova")
	wantStatus(t, err, http.StatusConflict) // đã có
	if _, _, err = e.q.Assemble(ctx, e.uid, "x_frost"); err != nil {
		t.Fatal(err)
	}
	if _, g, err = e.q.Assemble(ctx, e.uid, "paint_x_chroma"); err != nil || g.Kind != "item" {
		t.Fatalf("ghép đồ: %v %+v", err, g)
	}
	gar, _ := race.NewGarage(ctx, e.w)
	v, err := gar.View(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	owned := map[string]bool{}
	for _, c := range v.Cars {
		owned[c.ID] = c.Owned
	}
	for _, it := range v.Items {
		owned[it.ID] = it.Owned
	}
	if !owned["nova"] || !owned["x_frost"] || !owned["paint_x_chroma"] || owned["x_aurora"] || owned["decal_x_snow"] {
		t.Fatalf("garage sau khi ghép: %v", owned)
	}
	if v.Current != "starter" {
		t.Fatal("ghép xe không đổi xe đang chọn")
	}
	// Lắp được đồ độc quyền đã ghép.
	if _, err := gar.Equip(ctx, e.uid, "nova", "paint", "paint_x_chroma"); err != nil {
		t.Fatal(err)
	}
}

func TestExchange(t *testing.T) {
	e := newEnv(t, 0)
	_, _, err := e.q.Exchange(ctx, e.uid, TierExclusive)
	wantStatus(t, err, http.StatusBadRequest)
	_, _, err = e.q.Exchange(ctx, e.uid, "x")
	wantStatus(t, err, http.StatusBadRequest)
	// comet: chưa có xe, 7 mảnh (cần 6) → thừa 1; sprint: đã có xe, 2 mảnh → thừa 2.
	e.exec(`INSERT INTO race_shards (user_id, item, qty) VALUES (?, 'comet', 7), (?, 'sprint', 2), (?, 'bolt', 5)`, e.uid, e.uid, e.uid)
	e.exec(`INSERT INTO race_cars (user_id, car) VALUES (?, 'sprint')`, e.uid)
	s, g, err := e.q.Exchange(ctx, e.uid, TierCommon)
	if err != nil || len(g) != 1 || g[0].Tier != TierRare {
		t.Fatalf("đổi 3 thường → 1 hiếm: %v %+v", err, g)
	}
	if shardOf(s, "comet").Qty != 6 || shardOf(s, "sprint").Qty != 0 || shardOf(s, "bolt").Qty != 5 {
		t.Fatalf("chỉ lấy mảnh thừa: comet %d sprint %d bolt %d", shardOf(s, "comet").Qty, shardOf(s, "sprint").Qty, shardOf(s, "bolt").Qty)
	}
	_, _, err = e.q.Exchange(ctx, e.uid, TierCommon)
	wantStatus(t, err, http.StatusConflict) // hết mảnh thừa
	e.exec(`INSERT INTO race_shards (user_id, item, qty) VALUES (?, 'nova', 11)`, e.uid)
	if _, g, err = e.q.Exchange(ctx, e.uid, TierEpic); err != nil || g[0].Tier != TierExclusive {
		t.Fatalf("sử thi → độc quyền: %v %+v", err, g)
	}
}

func TestConcurrentRecordAndClaim(t *testing.T) {
	e := newEnv(t, 0)
	s := e.view()
	d0 := s.Daily[0]
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.q.Record(ctx, nil, e.uid, d0.Kind, 1)
		}()
	}
	wg.Wait()
	if got, _ := findQuest(e.view(), "d0"); got.Progress != min(20, got.Target) {
		t.Fatalf("tiến độ đồng thời %d", got.Progress)
	}
	ok := make(chan bool, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _, err := e.q.Claim(ctx, e.uid, "d0")
			ok <- err == nil
		}()
	}
	wg.Wait()
	close(ok)
	n := 0
	for v := range ok {
		if v {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("nhận đồng thời %d lần, muốn 1", n)
	}
	if got := totalParts(e.view()); got != DailyParts {
		t.Fatalf("nhận đồng thời: %d phụ kiện, muốn %d", got, DailyParts)
	}
}

func partSum(list []PartGain) int {
	n := 0
	for _, p := range list {
		n += p.Qty
	}
	return n
}

func totalParts(s State) int {
	n := 0
	for _, q := range s.Parts {
		n += q
	}
	return n
}

// Phụ kiện chia đều 3 loại; phụ kiện nhận được dùng nâng cấp xe được.
func TestPartsEvenAndUsable(t *testing.T) {
	e := newEnv(t, 1_000_000)
	i := 0
	e.q.intN = func(n int) int { i++; return i % n }
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.q.giveParts(ctx, tx, e.uid, 9)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("9 phụ kiện xoay vòng phải đủ 3 loại: %+v", got)
	}
	for _, p := range got {
		if p.Qty != 3 {
			t.Fatalf("mỗi loại 3: %+v", got)
		}
	}
	s := e.view()
	if len(s.PartInfo) != 3 || s.Parts[race.PartPiston] != 3 {
		t.Fatalf("kho phụ kiện: %v %+v", s.Parts, s.PartInfo)
	}
	g, err := race.NewGarage(ctx, e.w)
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.Upgrade(ctx, e.uid, "starter", "engine")
	if err != nil || v.Parts[race.PartPiston] != 2 {
		t.Fatalf("dùng phụ kiện nhiệm vụ nâng cấp: %v %v", err, v.Parts)
	}
}

func TestNoelActive(t *testing.T) {
	for _, c := range []struct {
		t    time.Time
		want bool
	}{
		{time.Date(2026, 11, 9, 12, 0, 0, 0, vietnam), true},
		{time.Date(2027, 11, 30, 12, 0, 0, 0, vietnam), false},
		{time.Date(2027, 12, 1, 0, 0, 0, 0, vietnam), true},
		{time.Date(2027, 1, 6, 23, 0, 0, 0, vietnam), true},
		{time.Date(2027, 1, 7, 0, 0, 0, 0, vietnam), false},
		{time.Date(2027, 1, 6, 17, 30, 0, 0, time.UTC), false}, // = 7/1 0:30 giờ VN
	} {
		if NoelActive(c.t) != c.want {
			t.Fatalf("%v: muốn %v", c.t, c.want)
		}
	}
}

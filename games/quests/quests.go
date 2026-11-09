// Package quests là nhiệm vụ hệ thống ở nông trại: 5 nhiệm vụ ngày + 3 nhiệm vụ tuần lấy từ nông trại, hồ câu, Trường đua,
// vườn bạn; nhận / đổi / lấy thưởng tại NPC trong vườn. Thưởng là mảnh ghép xe đua (ngẫu nhiên theo bậc), đủ mảnh thì
// ghép ra xe / đồ trang trí của Trường đua, gồm 2 xe và 4 đồ độc quyền chỉ có từ mảnh.
package quests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/games/race"
	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "quests"

const schema = `
CREATE TABLE IF NOT EXISTS quest_slots (
	user_id  INTEGER NOT NULL REFERENCES users(id),
	period   TEXT NOT NULL,             -- "d:2026-11-09" (ngày) / "w:2026-11-09" (tuần, ngày thứ Hai)
	slot     INTEGER NOT NULL,
	kind     TEXT NOT NULL,
	target   INTEGER NOT NULL,
	progress INTEGER NOT NULL DEFAULT 0,
	claimed  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, period, slot)
);
CREATE TABLE IF NOT EXISTS quest_chests (
	user_id INTEGER NOT NULL REFERENCES users(id),
	period  TEXT NOT NULL,
	PRIMARY KEY (user_id, period)
);
CREATE TABLE IF NOT EXISTS quest_rerolls (
	user_id INTEGER NOT NULL REFERENCES users(id),
	day     TEXT NOT NULL,
	n       INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, day)
);
CREATE TABLE IF NOT EXISTS quest_seen (
	user_id INTEGER NOT NULL REFERENCES users(id),
	day     TEXT NOT NULL,
	kind    TEXT NOT NULL,
	key     TEXT NOT NULL,
	PRIMARY KEY (user_id, day, kind, key)
);
CREATE TABLE IF NOT EXISTS race_shards (
	user_id INTEGER NOT NULL REFERENCES users(id),
	item    TEXT NOT NULL,
	qty     INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, item)
);
`

var vietnam = time.FixedZone("ICT", 7*60*60)

// Error là lỗi hiện cho người chơi, kèm mã HTTP.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func bad(msg string) error      { return &Error{http.StatusBadRequest, msg} }
func conflict(msg string) error { return &Error{http.StatusConflict, msg} }

func errStatus(err error) int {
	var e *Error
	var re *race.Error
	switch {
	case errors.As(err, &e):
		return e.Status
	case errors.As(err, &re):
		return re.Status
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		return http.StatusPaymentRequired
	}
	return http.StatusInternalServerError
}

// Quests giữ ví và đồng hồ / bộ số ngẫu nhiên (test thay được).
type Quests struct {
	w    *wallet.Wallet
	now  func() time.Time
	intN func(n int) int
}

func New(ctx context.Context, w *wallet.Wallet) (*Quests, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng nhiệm vụ: %w", err)
	}
	return &Quests{w: w, now: time.Now, intN: rand.IntN}, nil
}

// ---------- Kỳ ngày / tuần (giờ Việt Nam) ----------

type periods struct {
	day, week       string // khoá kỳ: "d:YYYY-MM-DD", "w:YYYY-MM-DD" (thứ Hai)
	dayKey          string // ngày "YYYY-MM-DD" cho bảng đổi nhiệm vụ / sự kiện không trùng
	dayEnd, weekEnd int64  // lúc reset kế tiếp (unix)
}

func periodsAt(t time.Time) periods {
	t = t.In(vietnam)
	y, m, d := t.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, vietnam)
	monday := start.AddDate(0, 0, -((int(start.Weekday()) + 6) % 7))
	return periods{
		day: "d:" + start.Format("2006-01-02"), week: "w:" + monday.Format("2006-01-02"), dayKey: start.Format("2006-01-02"),
		dayEnd: start.AddDate(0, 0, 1).Unix(), weekEnd: monday.AddDate(0, 0, 7).Unix(),
	}
}

// NoelActive: mùa Noel (tháng 12, 1–6/1, riêng năm 2026 bật từ tháng 11) theo giờ Việt Nam — NPC mặc Ông già Noel.
func NoelActive(t time.Time) bool {
	t = t.In(vietnam)
	m := t.Month()
	return m == time.December || (m == time.January && t.Day() <= 6) || (t.Year() == 2026 && m == time.November)
}

// ---------- Sinh nhiệm vụ ----------

// cloudReady: người chơi đã có chậu tầng mây (mới giao nhiệm vụ tầng mây). Đọc thẳng bảng farm_pots; chưa có bảng → false.
func cloudReady(ctx context.Context, tx *wallet.Tx, uid int64) bool {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM farm_pots WHERE user_id = ?`, uid).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// eligible là các mẫu giao được cho người chơi; weekly = chỉ mẫu có nhiệm vụ tuần.
func eligible(cloud, weekly bool) []Template {
	var out []Template
	for _, t := range Templates {
		if (t.Kind == FarmCloud && !cloud) || (weekly && t.Weekly == 0) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// pick rút n mẫu khác loại nhau (trừ các loại trong skip).
func (q *Quests) pick(pool []Template, n int, skip []string) []Template {
	pool = slices.DeleteFunc(slices.Clone(pool), func(t Template) bool { return slices.Contains(skip, t.Kind) })
	for i := len(pool) - 1; i > 0; i-- {
		j := q.intN(i + 1)
		pool[i], pool[j] = pool[j], pool[i]
	}
	return pool[:min(n, len(pool))]
}

// ensure sinh nhiệm vụ ngày / tuần của kỳ hiện tại nếu chưa có.
func (q *Quests) ensure(ctx context.Context, tx *wallet.Tx, uid int64, p periods) error {
	for _, scope := range []struct {
		period string
		weekly bool
		count  int
	}{{p.day, false, DailyCount}, {p.week, true, WeeklyCount}} {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM quest_slots WHERE user_id = ? AND period = ?`, uid, scope.period).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		lv, err := tx.Level(uid)
		if err != nil {
			return err
		}
		for i, t := range q.pick(eligible(cloudReady(ctx, tx, uid), scope.weekly), scope.count, nil) {
			target := t.dailyTarget(lv)
			if scope.weekly {
				target = t.Weekly
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO quest_slots (user_id, period, slot, kind, target) VALUES (?, ?, ?, ?, ?)`,
				uid, scope.period, i, t.Kind, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- Ghi sự kiện ----------

// Record cộng n vào tiến độ các nhiệm vụ loại kind của kỳ ngày và tuần hiện tại. tx = nil thì tự mở giao dịch.
// Lỗi chỉ ghi log — không làm hỏng thao tác gốc của game.
func (q *Quests) Record(ctx context.Context, tx *wallet.Tx, uid int64, kind string, n int) {
	if n <= 0 || uid <= 0 {
		return
	}
	if err := q.inTx(ctx, tx, func(tx *wallet.Tx) error { return q.record(ctx, tx, uid, kind, n) }); err != nil {
		log.Printf("quests: ghi %s cho %d: %v", kind, uid, err)
	}
}

// RecordUnique như Record(1) nhưng mỗi key chỉ tính một lần mỗi ngày (vd thăm cùng một người bạn).
func (q *Quests) RecordUnique(ctx context.Context, tx *wallet.Tx, uid int64, kind, key string) {
	if uid <= 0 {
		return
	}
	err := q.inTx(ctx, tx, func(tx *wallet.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO quest_seen (user_id, day, kind, key) VALUES (?, ?, ?, ?)`,
			uid, periodsAt(q.now()).dayKey, kind, key)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return nil
		}
		return q.record(ctx, tx, uid, kind, 1)
	})
	if err != nil {
		log.Printf("quests: ghi %s cho %d: %v", kind, uid, err)
	}
}

func (q *Quests) record(ctx context.Context, tx *wallet.Tx, uid int64, kind string, n int) error {
	p := periodsAt(q.now())
	if err := q.ensure(ctx, tx, uid, p); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE quest_slots SET progress = MIN(target, progress + ?)
		WHERE user_id = ? AND period IN (?, ?) AND kind = ? AND claimed = 0`, n, uid, p.day, p.week, kind)
	return err
}

// inTx chạy fn trong tx có sẵn, hoặc mở / commit giao dịch mới khi tx = nil.
func (q *Quests) inTx(ctx context.Context, tx *wallet.Tx, fn func(tx *wallet.Tx) error) error {
	if tx != nil {
		return fn(tx)
	}
	tx, err := q.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- Trạng thái gửi giao diện ----------

type Reward struct {
	Shards int   `json:"shards"`
	Parts  int   `json:"parts"` // phụ kiện nâng cấp xe
	XP     int64 `json:"xp"`
}

type Quest struct {
	ID       string `json:"id"` // "d0".."d4", "w0".."w2"
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Progress int    `json:"progress"`
	Target   int    `json:"target"`
	Reward   Reward `json:"reward"`
	Done     bool   `json:"done"`
	Claimed  bool   `json:"claimed"`
}

type ChestState struct {
	Ready   bool `json:"ready"`
	Claimed bool `json:"claimed"`
	Parts   int  `json:"parts"` // số phụ kiện rương cho
}

type ShardView struct {
	Item    string `json:"item"`
	Name    string `json:"name"`
	Tier    string `json:"tier"`
	Kind    string `json:"kind"` // "car" / "item" / "pot"
	Qty     int    `json:"qty"`
	Need    int    `json:"need"`
	Owned   bool   `json:"owned"`             // xe / đồ đã có (chậu VIP luôn false: ghép được nhiều lần)
	Vehicle string `json:"vehicle,omitempty"` // xe: "car" / "moto"
	Count   int    `json:"count,omitempty"`   // chậu VIP: số chậu đã ghép
	Speed   int    `json:"speedPct,omitempty"`
}

type Gain struct {
	Item string `json:"item"`
	Name string `json:"name"`
	Tier string `json:"tier"`
	Kind string `json:"kind"`
	Qty  int    `json:"qty"`
}

// PartGain là phụ kiện vừa nhận (gộp theo loại).
type PartGain struct {
	Part string `json:"part"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

type State struct {
	Daily  []Quest `json:"daily"`
	Weekly []Quest `json:"weekly"`
	Chest  struct {
		Daily  ChestState `json:"daily"`
		Weekly ChestState `json:"weekly"`
	} `json:"chest"`
	Shards      []ShardView    `json:"shards"`
	Rates       map[string]int `json:"rates"`       // tỉ lệ rơi bậc, phần nghìn
	RerollCost  int64          `json:"rerollCost"`  // giá lần đổi kế tiếp (0 = miễn phí)
	RerollsLeft int            `json:"rerollsLeft"` // số lần đổi còn lại hôm nay
	ResetAt     struct {
		Day  int64 `json:"day"`
		Week int64 `json:"week"`
	} `json:"resetAt"`
	NPC     string `json:"npc"`     // "santa" mùa Noel, "farmer" ngoài mùa
	Pending int    `json:"pending"` // số phần thưởng chờ nhận (nhiệm vụ xong + rương)
	// Phụ kiện nâng cấp xe đang có (theo mã) và danh sách phụ kiện (tên, món dùng).
	Parts    map[string]int  `json:"parts"`
	PartInfo []race.PartItem `json:"partInfo"`
}

func (q *Quests) load(ctx context.Context, tx *wallet.Tx, uid int64, p periods) (State, error) {
	var s State
	rows, err := tx.QueryContext(ctx, `SELECT period, slot, kind, target, progress, claimed FROM quest_slots
		WHERE user_id = ? AND period IN (?, ?) ORDER BY slot`, uid, p.day, p.week)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var period, kind string
		var slot int
		var qu Quest
		if err := rows.Scan(&period, &slot, &kind, &qu.Target, &qu.Progress, &qu.Claimed); err != nil {
			rows.Close()
			return s, err
		}
		t, _ := findTemplate(kind)
		qu.Kind, qu.Name, qu.Done = kind, t.title(qu.Target), qu.Progress >= qu.Target
		if period == p.day {
			qu.ID, qu.Reward = fmt.Sprintf("d%d", slot), Reward{DailyShards, DailyParts, DailyXP}
			s.Daily = append(s.Daily, qu)
		} else {
			qu.ID, qu.Reward = fmt.Sprintf("w%d", slot), Reward{WeeklyShards, WeeklyParts, WeeklyXP}
			s.Weekly = append(s.Weekly, qu)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}
	allDone := func(list []Quest) bool {
		return len(list) > 0 && !slices.ContainsFunc(list, func(q Quest) bool { return !q.Done })
	}
	for _, c := range []struct {
		period string
		list   []Quest
		out    *ChestState
	}{{p.day, s.Daily, &s.Chest.Daily}, {p.week, s.Weekly, &s.Chest.Weekly}} {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM quest_chests WHERE user_id = ? AND period = ?`, uid, c.period).Scan(&n); err != nil {
			return s, err
		}
		*c.out = ChestState{Ready: allDone(c.list), Claimed: n > 0}
	}
	s.Chest.Daily.Parts, s.Chest.Weekly.Parts = DailyChestParts, WeeklyChestParts
	if s.Parts, err = race.PartQty(ctx, tx, uid); err != nil {
		return s, err
	}
	s.PartInfo = race.PartItems
	for _, list := range [][]Quest{s.Daily, s.Weekly} {
		for _, qu := range list {
			if qu.Done && !qu.Claimed {
				s.Pending++
			}
		}
	}
	for _, c := range []ChestState{s.Chest.Daily, s.Chest.Weekly} {
		if c.Ready && !c.Claimed {
			s.Pending++
		}
	}
	have, err := shardQty(ctx, tx, uid)
	if err != nil {
		return s, err
	}
	pots, err := farm.VipPotQty(ctx, tx, uid)
	if err != nil {
		return s, err
	}
	for _, sh := range Shards {
		v := ShardView{Item: sh.Item, Name: sh.Name, Tier: sh.Tier, Kind: sh.Kind, Qty: have[sh.Item], Need: sh.Need, Vehicle: sh.Vehicle, Speed: sh.Speed}
		if sh.Kind == "pot" {
			v.Count = pots[sh.Item]
		} else if v.Owned, err = race.Owns(ctx, tx, uid, sh.Item); err != nil {
			return s, err
		}
		s.Shards = append(s.Shards, v)
	}
	s.Rates = map[string]int{}
	for _, t := range Tiers {
		s.Rates[t.ID] = t.Rate
	}
	if NoelActive(q.now()) {
		s.Rates[TierVip] = PotShardRate // mảnh chậu chỉ rơi trong mùa Noel
	}
	used, err := rerollsUsed(ctx, tx, uid, p.dayKey)
	if err != nil {
		return s, err
	}
	s.RerollsLeft = max(0, RerollsPerDay-used)
	if used > 0 {
		s.RerollCost = RerollCost
	}
	s.ResetAt.Day, s.ResetAt.Week = p.dayEnd, p.weekEnd
	s.NPC = "farmer"
	if NoelActive(q.now()) {
		s.NPC = "santa"
	}
	return s, nil
}

func shardQty(ctx context.Context, tx *wallet.Tx, uid int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT item, qty FROM race_shards WHERE user_id = ? AND qty > 0`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var item string
		var n int
		if err := rows.Scan(&item, &n); err != nil {
			return nil, err
		}
		out[item] = n
	}
	return out, rows.Err()
}

func rerollsUsed(ctx context.Context, tx *wallet.Tx, uid int64, day string) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT n FROM quest_rerolls WHERE user_id = ? AND day = ?`, uid, day).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// do chạy thao tác trong giao dịch (sau khi bảo đảm có nhiệm vụ kỳ này) rồi trả trạng thái mới.
func (q *Quests) do(ctx context.Context, uid int64, op func(tx *wallet.Tx, s State, p periods) error) (State, error) {
	tx, err := q.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	p := periodsAt(q.now())
	if err := q.ensure(ctx, tx, uid, p); err != nil {
		return State{}, err
	}
	s, err := q.load(ctx, tx, uid, p)
	if err != nil {
		return State{}, err
	}
	if op != nil {
		if err := op(tx, s, p); err != nil {
			return State{}, err
		}
		if s, err = q.load(ctx, tx, uid, p); err != nil {
			return State{}, err
		}
	}
	return s, tx.Commit()
}

// View trả nhiệm vụ, rương, mảnh của người chơi.
func (q *Quests) View(ctx context.Context, uid int64) (State, error) { return q.do(ctx, uid, nil) }

// ---------- Mảnh ----------

// rollShard rút một mảnh: bậc theo tỉ lệ (tier = "" thì rút bậc), trong bậc chia đều. Rút ngẫu nhiên trong mùa Noel thì
// PotShardRate‰ ra mảnh chậu VIP (theo trọng số từng chậu).
func (q *Quests) rollShard(tier string) Shard {
	if tier == "" && NoelActive(q.now()) && q.intN(1000) < PotShardRate {
		pots := shardsOf(TierVip)
		total := 0
		for _, p := range pots {
			total += p.Weight
		}
		r := q.intN(total)
		for _, p := range pots {
			if r -= p.Weight; r < 0 {
				return p
			}
		}
	}
	if tier == "" {
		r, acc := q.intN(1000), 0
		tier = Tiers[len(Tiers)-1].ID
		for _, t := range Tiers {
			if acc += t.Rate; r < acc {
				tier = t.ID
				break
			}
		}
	}
	list := shardsOf(tier)
	return list[q.intN(len(list))]
}

func addShard(ctx context.Context, tx *wallet.Tx, uid int64, item string, n int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO race_shards (user_id, item, qty) VALUES (?, ?, ?)
		ON CONFLICT(user_id, item) DO UPDATE SET qty = qty + excluded.qty`, uid, item, n)
	return err
}

// give rút n mảnh (bậc tier, "" = ngẫu nhiên theo tỉ lệ), cộng vào kho, gộp kết quả theo món.
func (q *Quests) give(ctx context.Context, tx *wallet.Tx, uid int64, n int, tier string) ([]Gain, error) {
	out := []Gain{}
	for range n {
		sh := q.rollShard(tier)
		if err := addShard(ctx, tx, uid, sh.Item, 1); err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(out, func(g Gain) bool { return g.Item == sh.Item }); i >= 0 {
			out[i].Qty++
		} else {
			out = append(out, Gain{Item: sh.Item, Name: sh.Name, Tier: sh.Tier, Kind: sh.Kind, Qty: 1})
		}
	}
	return out, nil
}

// giveParts rút n phụ kiện (đều 1/3 mỗi loại), cộng vào kho garage, gộp theo loại (thứ tự như race.PartItems).
func (q *Quests) giveParts(ctx context.Context, tx *wallet.Tx, uid int64, n int) ([]PartGain, error) {
	count := make([]int, len(race.PartItems))
	for range n {
		count[q.intN(len(race.PartItems))]++
	}
	out := []PartGain{}
	for i, p := range race.PartItems {
		if count[i] == 0 {
			continue
		}
		if err := race.GrantParts(ctx, tx, uid, p.ID, count[i]); err != nil {
			return nil, err
		}
		out = append(out, PartGain{Part: p.ID, Name: p.Name, Qty: count[i]})
	}
	return out, nil
}

// ---------- Thao tác ----------

func findQuest(s State, id string) (Quest, bool) {
	for _, list := range [][]Quest{s.Daily, s.Weekly} {
		for _, qu := range list {
			if qu.ID == id {
				return qu, true
			}
		}
	}
	return Quest{}, false
}

// slotOf tách id "d3" / "w1" thành kỳ và chỗ.
func slotOf(id string, p periods) (string, int, bool) {
	var slot int
	if len(id) < 2 {
		return "", 0, false
	}
	if _, err := fmt.Sscanf(id[1:], "%d", &slot); err != nil {
		return "", 0, false
	}
	switch id[0] {
	case 'd':
		return p.day, slot, true
	case 'w':
		return p.week, slot, true
	}
	return "", 0, false
}

// Claim nhận thưởng một nhiệm vụ đã xong: mảnh ngẫu nhiên + phụ kiện + XP.
func (q *Quests) Claim(ctx context.Context, uid int64, id string) (State, []Gain, []PartGain, error) {
	var gained []Gain
	var parts []PartGain
	s, err := q.do(ctx, uid, func(tx *wallet.Tx, s State, p periods) error {
		qu, ok := findQuest(s, id)
		if !ok {
			return bad("không có nhiệm vụ này")
		}
		if qu.Claimed {
			return conflict("đã nhận thưởng nhiệm vụ này")
		}
		if !qu.Done {
			return conflict("nhiệm vụ chưa xong")
		}
		period, slot, _ := slotOf(id, p)
		if _, err := tx.ExecContext(ctx, `UPDATE quest_slots SET claimed = 1 WHERE user_id = ? AND period = ? AND slot = ?`, uid, period, slot); err != nil {
			return err
		}
		if err := tx.AddXP(uid, qu.Reward.XP); err != nil {
			return err
		}
		var err error
		if gained, err = q.give(ctx, tx, uid, qu.Reward.Shards, ""); err != nil {
			return err
		}
		parts, err = q.giveParts(ctx, tx, uid, qu.Reward.Parts)
		return err
	})
	return s, gained, parts, err
}

// Chest mở rương ngày ("daily": 1 mảnh ngẫu nhiên) hoặc tuần ("weekly": 1 mảnh độc quyền) khi làm xong mọi nhiệm vụ kỳ đó.
func (q *Quests) Chest(ctx context.Context, uid int64, kind string) (State, []Gain, []PartGain, error) {
	var gained []Gain
	var parts []PartGain
	s, err := q.do(ctx, uid, func(tx *wallet.Tx, s State, p periods) error {
		var c ChestState
		var period, tier string
		switch kind {
		case "daily":
			c, period = s.Chest.Daily, p.day
		case "weekly":
			c, period, tier = s.Chest.Weekly, p.week, TierExclusive
		default:
			return bad("rương không hợp lệ")
		}
		if c.Claimed {
			return conflict("đã mở rương này")
		}
		if !c.Ready {
			return conflict("chưa làm xong mọi nhiệm vụ")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO quest_chests (user_id, period) VALUES (?, ?)`, uid, period); err != nil {
			return err
		}
		var err error
		if gained, err = q.give(ctx, tx, uid, ChestShards, tier); err != nil {
			return err
		}
		parts, err = q.giveParts(ctx, tx, uid, c.Parts)
		return err
	})
	return s, gained, parts, err
}

// Reroll đổi một nhiệm vụ ngày chưa xong lấy loại khác (tiến độ về 0): lần đầu trong ngày miễn phí, sau đó RerollCost xu,
// tối đa RerollsPerDay lần.
func (q *Quests) Reroll(ctx context.Context, uid int64, id string) (State, int64, error) {
	var cost int64
	s, err := q.do(ctx, uid, func(tx *wallet.Tx, s State, p periods) error {
		if len(id) == 0 || id[0] != 'd' {
			return bad("chỉ đổi được nhiệm vụ ngày")
		}
		qu, ok := findQuest(s, id)
		if !ok {
			return bad("không có nhiệm vụ này")
		}
		if qu.Done || qu.Claimed {
			return conflict("nhiệm vụ đã xong, không đổi được")
		}
		if s.RerollsLeft <= 0 {
			return conflict("hôm nay hết lượt đổi nhiệm vụ")
		}
		cost = s.RerollCost
		if cost > 0 {
			if err := tx.Charge(uid, cost, GameID, "Đổi nhiệm vụ"); err != nil {
				return err
			}
		}
		var skip []string
		for _, d := range s.Daily {
			skip = append(skip, d.Kind)
		}
		picked := q.pick(eligible(cloudReady(ctx, tx, uid), false), 1, skip)
		if len(picked) == 0 {
			return conflict("không còn nhiệm vụ khác để đổi")
		}
		lv, err := tx.Level(uid)
		if err != nil {
			return err
		}
		_, slot, _ := slotOf(id, p)
		if _, err := tx.ExecContext(ctx, `UPDATE quest_slots SET kind = ?, target = ?, progress = 0 WHERE user_id = ? AND period = ? AND slot = ?`,
			picked[0].Kind, picked[0].dailyTarget(lv), uid, p.day, slot); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO quest_rerolls (user_id, day, n) VALUES (?, ?, 1)
			ON CONFLICT(user_id, day) DO UPDATE SET n = n + 1`, uid, p.dayKey)
		return err
	})
	return s, cost, err
}

// Granted là món vừa ghép.
type Granted struct {
	Item string `json:"item"`
	Kind string `json:"kind"`
}

// Assemble ghép xe / đồ trang trí khi đủ mảnh và chưa có món đó; không cần đủ cấp nhân vật.
func (q *Quests) Assemble(ctx context.Context, uid int64, item string) (State, Granted, error) {
	var g Granted
	s, err := q.do(ctx, uid, func(tx *wallet.Tx, s State, _ periods) error {
		sh, ok := findShard(item)
		if !ok {
			return bad("không có mảnh của món này")
		}
		i := slices.IndexFunc(s.Shards, func(v ShardView) bool { return v.Item == item })
		v := s.Shards[i]
		if sh.Kind == "pot" { // chậu VIP ghép được nhiều lần, mỗi lần một chậu
			if v.Qty < sh.Need {
				return conflict(fmt.Sprintf("thiếu mảnh: %d/%d", v.Qty, sh.Need))
			}
			if _, err := tx.ExecContext(ctx, `UPDATE race_shards SET qty = qty - ? WHERE user_id = ? AND item = ?`, sh.Need, uid, item); err != nil {
				return err
			}
			g = Granted{Item: item, Kind: sh.Kind}
			return farm.GrantVipPot(ctx, tx, uid, item)
		}
		if v.Owned {
			return conflict("bạn đã có " + sh.Name)
		}
		if v.Qty < sh.Need {
			return conflict(fmt.Sprintf("thiếu mảnh: %d/%d", v.Qty, sh.Need))
		}
		if _, err := tx.ExecContext(ctx, `UPDATE race_shards SET qty = qty - ? WHERE user_id = ? AND item = ?`, sh.Need, uid, item); err != nil {
			return err
		}
		var err error
		if sh.Kind == "car" {
			err = race.GrantCar(ctx, tx, uid, item)
		} else {
			err = race.GrantItem(ctx, tx, uid, item)
		}
		g = Granted{Item: item, Kind: sh.Kind}
		return err
	})
	return s, g, err
}

// surplus là số mảnh thừa của một món: đã có món → mọi mảnh; chưa có → phần vượt số cần ghép.
func surplus(v ShardView) int {
	if v.Owned {
		return v.Qty
	}
	return max(0, v.Qty-v.Need)
}

// Exchange đổi ExchangeCost mảnh thừa bậc tier lấy 1 mảnh ngẫu nhiên bậc trên (Sử thi → Độc quyền; Độc quyền không đổi).
// Lấy mảnh từ món thừa nhiều nhất trước.
func (q *Quests) Exchange(ctx context.Context, uid int64, tier string) (State, []Gain, error) {
	var gained []Gain
	s, err := q.do(ctx, uid, func(tx *wallet.Tx, s State, _ periods) error {
		up := nextTier(tier)
		if up == "" {
			return bad("bậc này không đổi được")
		}
		var list []ShardView
		total := 0
		for _, v := range s.Shards {
			if v.Tier == tier && surplus(v) > 0 {
				list = append(list, v)
				total += surplus(v)
			}
		}
		if total < ExchangeCost {
			return conflict(fmt.Sprintf("cần %d mảnh thừa cùng bậc, mới có %d", ExchangeCost, total))
		}
		sort.SliceStable(list, func(i, j int) bool { return surplus(list[i]) > surplus(list[j]) })
		left := ExchangeCost
		for _, v := range list {
			take := min(left, surplus(v))
			if _, err := tx.ExecContext(ctx, `UPDATE race_shards SET qty = qty - ? WHERE user_id = ? AND item = ?`, take, uid, v.Item); err != nil {
				return err
			}
			if left -= take; left == 0 {
				break
			}
		}
		var err error
		gained, err = q.give(ctx, tx, uid, 1, up)
		return err
	})
	return s, gained, err
}

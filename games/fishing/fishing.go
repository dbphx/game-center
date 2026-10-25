// Package fishing là Hồ câu cá giải trí ở ngoại ô DCity: mua mồi, thả câu, chờ cá cắn câu rồi giật,
// cá câu được vào giỏ để bán lấy xu. Kết quả mỗi lần thả câu do server quay sẵn (giao diện không biết trước),
// giao diện chỉ báo giật thành công hay không, và chỉ được tính khi giật đúng khung thời gian cá cắn câu.
package fishing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "fishing"

// Bait là một loại mồi; Luck càng cao càng dễ ra cá hiếm và cá to.
type Bait struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
	Luck  int    `json:"luck"`
}

var Baits = []Bait{
	{ID: "worm", Name: "Giun", Price: 10, Luck: 0},
	{ID: "shrimp", Name: "Tép", Price: 40, Luck: 1},
	{ID: "special", Name: "Mồi đặc biệt", Price: 150, Luck: 2},
}

// Độ hiếm, từ rác tới huyền thoại.
const (
	Junk      = "junk"
	Common    = "common"
	Uncommon  = "uncommon"
	Rare      = "rare"
	Epic      = "epic"
	Legendary = "legendary"
)

var rarities = []string{Junk, Common, Uncommon, Rare, Epic, Legendary}

// rarityWeights[luck] là tỉ lệ (phần nghìn) ra từng độ hiếm theo thứ tự rarities.
var rarityWeights = [3][6]int{
	{80, 550, 270, 80, 19, 1},  // giun
	{50, 420, 330, 150, 47, 3}, // tép
	{30, 300, 360, 220, 85, 5}, // mồi đặc biệt: huyền thoại 0.5%
}

// difficulty là độ khó mini-game kéo cần (1..5) theo độ hiếm.
var difficulty = map[string]int{Junk: 1, Common: 1, Uncommon: 2, Rare: 3, Epic: 4, Legendary: 5}

// Fish là một loài cá (hoặc rác vớt được); giá = số cm × PricePerCm.
type Fish struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Rarity     string `json:"rarity"`
	MinCm      int    `json:"minCm"`
	MaxCm      int    `json:"maxCm"`
	PricePerCm int64  `json:"pricePerCm"`
	XP         int64  `json:"xp"`
}

var FishList = []Fish{
	{ID: "dep", Name: "Dép cũ", Rarity: Junk, XP: 1},
	{ID: "lon", Name: "Lon nước", Rarity: Junk, XP: 1},
	{ID: "ro", Name: "Cá rô", Rarity: Common, MinCm: 8, MaxCm: 20, PricePerCm: 4, XP: 2},
	{ID: "diec", Name: "Cá diếc", Rarity: Common, MinCm: 8, MaxCm: 18, PricePerCm: 4, XP: 2},
	{ID: "chep", Name: "Cá chép", Rarity: Uncommon, MinCm: 20, MaxCm: 50, PricePerCm: 5, XP: 4},
	{ID: "loc", Name: "Cá lóc", Rarity: Uncommon, MinCm: 25, MaxCm: 60, PricePerCm: 5, XP: 4},
	{ID: "tre", Name: "Cá trê", Rarity: Rare, MinCm: 25, MaxCm: 55, PricePerCm: 8, XP: 8},
	{ID: "chim", Name: "Cá chim", Rarity: Rare, MinCm: 20, MaxCm: 40, PricePerCm: 12, XP: 8},
	{ID: "koi", Name: "Cá koi", Rarity: Epic, MinCm: 30, MaxCm: 70, PricePerCm: 20, XP: 20},
	{ID: "vang", Name: "Cá Vàng Huyền Thoại", Rarity: Legendary, MinCm: 40, MaxCm: 80, PricePerCm: 80, XP: 60},
}

const (
	BasketMax = 30
	WindowMs  = 1500  // khung giật câu sau khi cá cắn
	ReelMs    = 8000  // thời gian chơi mini-game kéo cần sau khung giật
	BiteMinMs = 3000  // cá cắn câu sau 3..10 giây
	BiteMaxMs = 10000 //
	MaxBuy    = 50
)

func findBait(id string) (Bait, bool) {
	for _, b := range Baits {
		if b.ID == id {
			return b, true
		}
	}
	return Bait{}, false
}

func findFish(id string) (Fish, bool) {
	for _, f := range FishList {
		if f.ID == id {
			return f, true
		}
	}
	return Fish{}, false
}

const schema = `
CREATE TABLE IF NOT EXISTS fishing_bait (
	user_id INTEGER NOT NULL REFERENCES users(id),
	bait    TEXT NOT NULL,
	qty     INTEGER NOT NULL,
	PRIMARY KEY (user_id, bait)
);
CREATE TABLE IF NOT EXISTS fishing_casts (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	bait       TEXT NOT NULL,
	created_ms INTEGER NOT NULL,
	bite_ms    INTEGER NOT NULL,
	fish       TEXT NOT NULL,             -- kết quả quay sẵn, không gửi cho giao diện trước khi câu được
	cm         INTEGER NOT NULL,
	open       INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS fishing_casts_open ON fishing_casts(user_id, open);
CREATE TABLE IF NOT EXISTS fishing_basket (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id   INTEGER NOT NULL REFERENCES users(id),
	fish      TEXT NOT NULL,
	cm        INTEGER NOT NULL,
	price     INTEGER NOT NULL,
	caught_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS fishing_basket_user ON fishing_basket(user_id);
CREATE TABLE IF NOT EXISTS fishing_records (
	user_id INTEGER NOT NULL REFERENCES users(id),
	fish    TEXT NOT NULL,
	max_cm  INTEGER NOT NULL,
	PRIMARY KEY (user_id, fish)
);
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
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		return http.StatusPaymentRequired
	case errors.Is(err, wallet.ErrInHospital):
		return http.StatusLocked
	case errors.Is(err, wallet.ErrLowHP):
		return http.StatusForbidden
	case errors.Is(err, wallet.ErrNotFound):
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

// Pond là hồ câu cho mọi người chơi.
type Pond struct {
	w   *wallet.Wallet
	now func() time.Time
	mu  sync.Mutex // giữ rng
	rng *mrand.Rand
	// pick quay kết quả một lần thả câu (mặc định roll; test thay để có kết quả cố định).
	pick func(luck int) (Fish, int, int64)
}

func New(ctx context.Context, w *wallet.Wallet) (*Pond, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng hồ câu: %w", err)
	}
	p := &Pond{w: w, now: time.Now, rng: mrand.New(mrand.NewPCG(mrand.Uint64(), mrand.Uint64()))}
	p.pick = p.roll
	return p, nil
}

// roll quay sẵn kết quả một lần thả câu: loài, cỡ (cm) và thời điểm cá cắn câu (ms sau khi thả).
// Mồi tốt dịch tỉ lệ sang cá hiếm và cá to hơn.
func (p *Pond) roll(luck int) (Fish, int, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w := rarityWeights[min(max(luck, 0), 2)]
	n := p.rng.IntN(1000)
	rarity := Common
	for i, r := range rarities {
		if n < w[i] {
			rarity = r
			break
		}
		n -= w[i]
	}
	var pool []Fish
	for _, f := range FishList {
		if f.Rarity == rarity {
			pool = append(pool, f)
		}
	}
	f := pool[p.rng.IntN(len(pool))]
	cm := 0
	if f.MaxCm > 0 {
		// Lấy cỡ lớn nhất trong (1 + luck) lần quay: mồi tốt cho cá to hơn.
		for range 1 + luck {
			cm = max(cm, f.MinCm+p.rng.IntN(f.MaxCm-f.MinCm+1))
		}
	}
	bite := int64(BiteMinMs + p.rng.IntN(BiteMaxMs-BiteMinMs+1))
	return f, cm, bite
}

// BaitStock là một loại mồi kèm số lượng đang có.
type BaitStock struct {
	Bait
	Qty int `json:"qty"`
}

// Catch là một con cá trong giỏ (hoặc vừa câu được).
type Catch struct {
	ID       int64  `json:"id,omitempty"`
	FishID   string `json:"fishId"`
	Name     string `json:"name"`
	Rarity   string `json:"rarity"`
	Cm       int    `json:"cm"`
	Price    int64  `json:"price"`
	CaughtAt int64  `json:"caughtAt"`
	Record   bool   `json:"record,omitempty"` // con to nhất từng câu được của loài này
}

// Cast là lần thả câu đang mở (không kèm kết quả).
type Cast struct {
	ID         int64  `json:"id"`
	Bait       string `json:"bait"`
	CreatedAt  int64  `json:"createdAt"` // unix ms
	BiteAt     int64  `json:"biteAt"`    // unix ms: cá cắn câu
	WindowMs   int    `json:"windowMs"`  // giật trong khung [biteAt, biteAt + windowMs] rồi kéo cần
	ExpiresAt  int64  `json:"expiresAt"` // unix ms: quá giờ này cá đã ăn mồi chạy mất
	Difficulty int    `json:"difficulty"`
}

// State là hồ câu của một người chơi (GET /api/fishing).
type State struct {
	Now       int64          `json:"now"` // unix ms
	Baits     []BaitStock    `json:"baits"`
	Basket    []Catch        `json:"basket"`
	BasketMax int            `json:"basketMax"`
	Record    map[string]int `json:"record"`
	Cast      *Cast          `json:"cast"`
	Fish      []Fish         `json:"fish"`
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (p *Pond) load(ctx context.Context, q querier, uid int64) (State, error) {
	nowMs := p.now().UnixMilli()
	st := State{Now: nowMs, BasketMax: BasketMax, Record: map[string]int{}, Fish: FishList, Basket: []Catch{}}
	qty := map[string]int{}
	rows, err := q.QueryContext(ctx, `SELECT bait, qty FROM fishing_bait WHERE user_id = ?`, uid)
	if err != nil {
		return State{}, err
	}
	for rows.Next() {
		var b string
		var n int
		if err := rows.Scan(&b, &n); err != nil {
			rows.Close()
			return State{}, err
		}
		qty[b] = n
	}
	rows.Close()
	for _, b := range Baits {
		st.Baits = append(st.Baits, BaitStock{Bait: b, Qty: qty[b.ID]})
	}
	rows, err = q.QueryContext(ctx, `SELECT id, fish, cm, price, caught_at FROM fishing_basket WHERE user_id = ? ORDER BY id`, uid)
	if err != nil {
		return State{}, err
	}
	for rows.Next() {
		var c Catch
		if err := rows.Scan(&c.ID, &c.FishID, &c.Cm, &c.Price, &c.CaughtAt); err != nil {
			rows.Close()
			return State{}, err
		}
		f, _ := findFish(c.FishID)
		c.Name, c.Rarity = f.Name, f.Rarity
		st.Basket = append(st.Basket, c)
	}
	rows.Close()
	rows, err = q.QueryContext(ctx, `SELECT fish, max_cm FROM fishing_records WHERE user_id = ?`, uid)
	if err != nil {
		return State{}, err
	}
	for rows.Next() {
		var f string
		var cm int
		if err := rows.Scan(&f, &cm); err != nil {
			rows.Close()
			return State{}, err
		}
		st.Record[f] = cm
	}
	rows.Close()
	c, _, err := openCast(ctx, q, uid)
	if err != nil {
		return State{}, err
	}
	if c != nil && nowMs <= c.ExpiresAt {
		st.Cast = c
	}
	return st, rows.Err()
}

// openCast đọc lần thả câu đang mở (kèm kết quả ẩn); nil nếu không có.
func openCast(ctx context.Context, q querier, uid int64) (*Cast, Catch, error) {
	var c Cast
	var res Catch
	err := q.QueryRowContext(ctx, `SELECT id, bait, created_ms, bite_ms, fish, cm FROM fishing_casts WHERE user_id = ? AND open = 1 ORDER BY id DESC LIMIT 1`, uid).
		Scan(&c.ID, &c.Bait, &c.CreatedAt, &c.BiteAt, &res.FishID, &res.Cm)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Catch{}, nil
	}
	if err != nil {
		return nil, Catch{}, err
	}
	f, _ := findFish(res.FishID)
	res.Name, res.Rarity, res.Price = f.Name, f.Rarity, int64(res.Cm)*f.PricePerCm
	c.WindowMs = WindowMs
	c.ExpiresAt = c.BiteAt + WindowMs + ReelMs
	c.Difficulty = difficulty[f.Rarity]
	return &c, res, nil
}

// State trả hồ câu của người chơi.
func (p *Pond) State(ctx context.Context, uid int64) (State, error) {
	tx, err := p.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	return p.load(ctx, tx, uid)
}

// do chạy một thao tác trong transaction rồi trả trạng thái mới.
func (p *Pond) do(ctx context.Context, uid int64, op func(tx *wallet.Tx, st State) error) (State, error) {
	tx, err := p.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	st, err := p.load(ctx, tx, uid)
	if err != nil {
		return State{}, err
	}
	if err := op(tx, st); err != nil {
		return State{}, err
	}
	if st, err = p.load(ctx, tx, uid); err != nil {
		return State{}, err
	}
	return st, tx.Commit()
}

// BuyBait mua qty mồi (1..50) vào kho mồi.
func (p *Pond) BuyBait(ctx context.Context, uid int64, baitID string, qty int) (State, error) {
	b, ok := findBait(baitID)
	if !ok {
		return State{}, bad("không có loại mồi này")
	}
	if qty == 0 {
		qty = 1
	}
	if qty < 1 || qty > MaxBuy {
		return State{}, bad(fmt.Sprintf("mỗi lần mua 1–%d mồi", MaxBuy))
	}
	return p.do(ctx, uid, func(tx *wallet.Tx, _ State) error {
		if err := tx.Charge(uid, b.Price*int64(qty), GameID, fmt.Sprintf("Mua %d mồi %s", qty, b.Name)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO fishing_bait (user_id, bait, qty) VALUES (?, ?, ?)
			ON CONFLICT(user_id, bait) DO UPDATE SET qty = qty + excluded.qty`, uid, b.ID, qty)
		return err
	})
}

// CastLine thả câu bằng một mồi: trừ 1 mồi, quay sẵn kết quả, trả thời điểm cá cắn câu.
func (p *Pond) CastLine(ctx context.Context, uid int64, baitID string) (State, error) {
	b, ok := findBait(baitID)
	if !ok {
		return State{}, bad("không có loại mồi này")
	}
	return p.do(ctx, uid, func(tx *wallet.Tx, st State) error {
		if st.Cast != nil {
			return conflict("bạn đang thả câu rồi")
		}
		if len(st.Basket) >= BasketMax {
			return conflict(fmt.Sprintf("giỏ đã đầy %d con, hãy bán bớt", BasketMax))
		}
		for _, s := range st.Baits {
			if s.ID == b.ID && s.Qty < 1 {
				return conflict("hết mồi " + b.Name + ", hãy mua thêm")
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fishing_bait SET qty = qty - 1 WHERE user_id = ? AND bait = ?`, uid, b.ID); err != nil {
			return err
		}
		// Lần thả quá hạn (bỏ dở) coi như cá đã ăn mồi.
		if _, err := tx.ExecContext(ctx, `UPDATE fishing_casts SET open = 0 WHERE user_id = ? AND open = 1`, uid); err != nil {
			return err
		}
		f, cm, bite := p.pick(b.Luck)
		now := p.now().UnixMilli()
		_, err := tx.ExecContext(ctx, `INSERT INTO fishing_casts (user_id, bait, created_ms, bite_ms, fish, cm) VALUES (?, ?, ?, ?, ?, ?)`,
			uid, b.ID, now, now+bite, f.ID, cm)
		return err
	})
}

// Kết quả giật câu.
const (
	OutcomeCaught = "caught" // câu được (cá vào giỏ, hoặc rác bị vứt đi)
	OutcomeMiss   = "miss"   // giật trượt / kéo cần thất bại
	OutcomeEarly  = "early"  // giật khi cá chưa cắn câu
	OutcomeLate   = "late"   // cá đã ăn mồi chạy mất
)

// Reel giật câu. Mọi kết quả đều đóng lần thả câu (mồi đã mất); chỉ success = true trong khung
// [biteAt, biteAt + WindowMs + ReelMs] mới được cá.
func (p *Pond) Reel(ctx context.Context, uid, castID int64, success bool) (State, string, string, *Catch, int64, error) {
	var outcome, msg string
	var got *Catch
	var xp int64
	st, err := p.do(ctx, uid, func(tx *wallet.Tx, _ State) error {
		c, res, err := openCast(ctx, tx, uid)
		if err != nil {
			return err
		}
		if c == nil || c.ID != castID {
			return notFound("không có lần thả câu này")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fishing_casts SET open = 0 WHERE id = ?`, c.ID); err != nil {
			return err
		}
		now := p.now().UnixMilli()
		switch {
		case now < c.BiteAt:
			outcome, msg = OutcomeEarly, "Giật sớm quá, cá chạy mất rồi!"
			return nil
		case now > c.ExpiresAt:
			outcome, msg = OutcomeLate, "Cá đã ăn mồi chạy mất!"
			return nil
		case !success:
			outcome, msg = OutcomeMiss, "Tuột mất rồi, thử lại nhé!"
			return nil
		}
		f, _ := findFish(res.FishID)
		outcome, xp = OutcomeCaught, f.XP
		res.CaughtAt = p.now().Unix()
		got = &res
		if err := tx.AddXP(uid, f.XP); err != nil {
			return err
		}
		if f.Rarity == Junk {
			msg = "Vớt được " + f.Name + "… vứt đi thôi!"
			return nil
		}
		var best int
		err = tx.QueryRowContext(ctx, `SELECT max_cm FROM fishing_records WHERE user_id = ? AND fish = ?`, uid, f.ID).Scan(&best)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if res.Cm > best {
			got.Record = best > 0
			if _, err := tx.ExecContext(ctx, `INSERT INTO fishing_records (user_id, fish, max_cm) VALUES (?, ?, ?)
				ON CONFLICT(user_id, fish) DO UPDATE SET max_cm = excluded.max_cm`, uid, f.ID, res.Cm); err != nil {
				return err
			}
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO fishing_basket (user_id, fish, cm, price, caught_at) VALUES (?, ?, ?, ?, ?)`,
			uid, f.ID, res.Cm, res.Price, res.CaughtAt)
		if err != nil {
			return err
		}
		got.ID, _ = r.LastInsertId()
		msg = fmt.Sprintf("Câu được %s %d cm!", f.Name, res.Cm)
		return nil
	})
	return st, outcome, msg, got, xp, err
}

// Sell bán các con trong giỏ (ids) hoặc cả giỏ (all) lấy xu, một dòng sổ cái.
func (p *Pond) Sell(ctx context.Context, uid int64, ids []int64, all bool) (State, int64, int, error) {
	var coins int64
	var count int
	st, err := p.do(ctx, uid, func(tx *wallet.Tx, st State) error {
		want := map[int64]bool{}
		for _, id := range ids {
			want[id] = true
		}
		names := map[string]int{}
		for _, c := range st.Basket {
			if !all && !want[c.ID] {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM fishing_basket WHERE id = ? AND user_id = ?`, c.ID, uid); err != nil {
				return err
			}
			coins += c.Price
			count++
			names[c.Name]++
		}
		if count == 0 {
			return conflict("không có cá nào để bán")
		}
		parts := make([]string, 0, len(names))
		for name, n := range names {
			parts = append(parts, fmt.Sprintf("%d %s", n, name))
		}
		sort.Strings(parts)
		return tx.Credit(uid, coins, GameID, "Bán cá: "+strings.Join(parts, ", "))
	})
	return st, coins, count, err
}

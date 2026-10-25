package farm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// Bảng của nông trại nằm chung database với ví, nên trừ / cộng xu và đổi nông trại chạy trong cùng transaction.
const schema = `
CREATE TABLE IF NOT EXISTS farm_plots (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	idx        INTEGER NOT NULL,
	crop       TEXT NOT NULL DEFAULT '',
	planted_at INTEGER NOT NULL DEFAULT 0,
	ready_at   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, idx)
);
CREATE TABLE IF NOT EXISTS farm_buildings (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	place      TEXT NOT NULL,           -- 'barn' (chuồng), 'yard' (sân gia cầm) hoặc 'pond' (ao)
	capacity   INTEGER NOT NULL,
	expansions INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, place)
);
CREATE TABLE IF NOT EXISTS farm_stock (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id   INTEGER NOT NULL REFERENCES users(id),
	place     TEXT NOT NULL,
	kind      TEXT NOT NULL,
	bought_at INTEGER NOT NULL,
	ready_at  INTEGER NOT NULL,
	fed_at      INTEGER NOT NULL DEFAULT 0, -- lần cho ăn gần nhất (0 = chưa cho ăn)
	produced_at INTEGER NOT NULL DEFAULT 0, -- lần lấy trứng / sữa / lông gần nhất
	-- Con vật chỉ lớn / cho sản phẩm trong lúc no (tới fed_until); tiến độ tính lười tới progress_at.
	fed_until        INTEGER NOT NULL DEFAULT 0,
	grow_progress    INTEGER NOT NULL DEFAULT 0, -- số giây đã lớn
	progress_at      INTEGER NOT NULL DEFAULT 0,
	product_progress INTEGER NOT NULL DEFAULT 0  -- số giây tích luỹ cho sản phẩm kế tiếp
);
CREATE INDEX IF NOT EXISTS farm_stock_user ON farm_stock(user_id, place);
CREATE TABLE IF NOT EXISTS farm_storage (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	store      TEXT NOT NULL,           -- 'seeds' (kho giống) hoặc 'products' (kho sản phẩm)
	capacity   INTEGER NOT NULL,
	expansions INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, store)
);
CREATE TABLE IF NOT EXISTS farm_items (
	user_id INTEGER NOT NULL REFERENCES users(id),
	store   TEXT NOT NULL,
	item    TEXT NOT NULL,              -- id cây (hạt / nông sản) hoặc id vật nuôi / cá
	qty     INTEGER NOT NULL,
	PRIMARY KEY (user_id, store, item)
);
`

const GameID = "farm"

// Error là lỗi hiện cho người chơi, kèm mã HTTP.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func bad(msg string) error      { return &Error{http.StatusBadRequest, msg} }
func conflict(msg string) error { return &Error{http.StatusConflict, msg} }
func notFound(msg string) error { return &Error{http.StatusNotFound, msg} }
func needLevel(level int) error {
	return &Error{http.StatusForbidden, fmt.Sprintf("cần level %d", level)}
}
func noCoins() error { return &Error{http.StatusPaymentRequired, "không đủ xu"} }
func errStatus(err error) int {
	var fe *Error
	switch {
	case errors.As(err, &fe):
		return fe.Status
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

// Farm là dịch vụ nông trại cho mọi người chơi.
type Farm struct {
	w   *wallet.Wallet
	now func() time.Time
}

func New(ctx context.Context, w *wallet.Wallet) (*Farm, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng nông trại: %w", err)
	}
	if err := migrate(ctx, w, time.Now().Unix()); err != nil {
		return nil, fmt.Errorf("nâng cấp bảng nông trại: %w", err)
	}
	return &Farm{w: w, now: time.Now}, nil
}

// ---------- Trạng thái ----------

type Plot struct {
	Index     int    `json:"index"`
	Crop      string `json:"crop"`
	PlantedAt int64  `json:"plantedAt"`
	ReadyAt   int64  `json:"readyAt"`
}

// Animal là một con trong chuồng / sân / ao. Con vật chỉ lớn và cho sản phẩm trong lúc no (now < fedUntil).
type Animal struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	BoughtAt int64  `json:"boughtAt"`
	// ReadyAt: lúc đã lớn (≤ now), hoặc lúc dự kiến lớn nếu cứ no như hiện tại; 0 nếu đói / hết no trước khi lớn (paused).
	ReadyAt  int64   `json:"readyAt"`
	Grown    bool    `json:"grown"`
	Progress float64 `json:"progress"` // tiến độ lớn 0..1
	Paused   bool    `json:"paused"`   // không kịp lớn / ra sản phẩm với phần no còn lại: cần cho ăn
	FedAt    int64   `json:"fedAt"`
	FedUntil int64   `json:"fedUntil"`
	Hungry   bool    `json:"hungry"` // now ≥ fedUntil
	FullFor  int64   `json:"fullFor"`
	// Trứng / sữa / lông (0 / false nếu con này không cho sản phẩm): lúc dự kiến có, đã có chưa, tiến độ 0..1.
	ProductReadyAt      int64   `json:"productReadyAt"`
	ProductReady        bool    `json:"productReady"`
	ProductProgress     float64 `json:"productProgress"`
	growSec, productSec int64
}

type Pen struct {
	Capacity        int      `json:"capacity"`
	Used            int      `json:"used"`
	MaxCapacity     int      `json:"maxCapacity"`
	NextExpandPrice int64    `json:"nextExpandPrice"`
	Animals         []Animal `json:"animals,omitempty"`
	Fish            []Animal `json:"fish,omitempty"`
	expansions      int
}

type State struct {
	Now           int64   `json:"now"`
	Plots         []Plot  `json:"plots"`
	NextPlotPrice int64   `json:"nextPlotPrice"`
	MaxPlots      int     `json:"maxPlots"`
	Barn          *Pen    `json:"barn"`
	Yard          *Pen    `json:"yard"` // sân gia cầm, luôn có
	Pond          *Pen    `json:"pond"`
	Storage       Storage `json:"storage"`
}

// Gained là phần thưởng của một lần thu hoạch / bán.
type Gained struct {
	Coins int64          `json:"coins"`
	XP    int64          `json:"xp"`
	Count int            `json:"count,omitempty"`
	Items map[string]int `json:"items,omitempty"` // nông sản / sản phẩm vừa vào kho
	Spent int64          `json:"spent,omitempty"` // xu đã tiêu (gieo hàng loạt)
	Used  map[string]int `json:"used,omitempty"`  // thức ăn đã dùng (feed_x → số phần)
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ensurePlots tạo FreePlots ô miễn phí cho người mới vào nông trại.
func ensurePlots(ctx context.Context, q querier, uid int64) error {
	for i := range FreePlots {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO farm_plots (user_id, idx) VALUES (?, ?)`, uid, i); err != nil {
			return err
		}
	}
	return nil
}

func loadPlots(ctx context.Context, q querier, uid int64) ([]Plot, error) {
	rows, err := q.QueryContext(ctx, `SELECT idx, crop, planted_at, ready_at FROM farm_plots WHERE user_id = ? ORDER BY idx`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plots := []Plot{}
	for rows.Next() {
		var p Plot
		if err := rows.Scan(&p.Index, &p.Crop, &p.PlantedAt, &p.ReadyAt); err != nil {
			return nil, err
		}
		plots = append(plots, p)
	}
	return plots, rows.Err()
}

// loadPen đọc chuồng / sân / ao; nil nếu chưa mua (sân gia cầm thì tạo sẵn).
func loadPen(ctx context.Context, q querier, uid int64, place string, now int64) (*Pen, error) {
	b := Buildings[place]
	if place == placeYard {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO farm_buildings (user_id, place, capacity) VALUES (?, ?, ?)`,
			uid, place, b.Capacity); err != nil {
			return nil, err
		}
	}
	pen := &Pen{MaxCapacity: b.MaxCapacity}
	err := q.QueryRowContext(ctx, `SELECT capacity, expansions FROM farm_buildings WHERE user_id = ? AND place = ?`, uid, place).
		Scan(&pen.Capacity, &pen.expansions)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pen.Capacity < b.MaxCapacity {
		pen.NextExpandPrice = b.expandPrice(pen.expansions)
	}
	rows, err := q.QueryContext(ctx, `SELECT id, kind, bought_at, ready_at, fed_at, fed_until, grow_progress, progress_at, product_progress
		FROM farm_stock WHERE user_id = ? AND place = ? ORDER BY id`, uid, place)
	if err != nil {
		return nil, err
	}
	list := []Animal{}
	at := []int64{}
	for rows.Next() {
		var a Animal
		var progressAt int64
		if err := rows.Scan(&a.ID, &a.Kind, &a.BoughtAt, &a.ReadyAt, &a.FedAt, &a.FedUntil, &a.growSec, &progressAt, &a.productSec); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, a)
		at = append(at, progressAt)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Cộng tiến độ tới bây giờ rồi ghi lại (tính lười như HP), sau đó mới đổi fed_until khi cho ăn.
	for i := range list {
		a := &list[i]
		st, ok := findStock(place, a.Kind)
		if !ok {
			continue
		}
		pen.Used += st.Units
		a.settle(st, at[i], now)
		// progress_at không lùi: hai yêu cầu đọc đồng hồ lệch thứ tự thì không cộng trùng một khoảng.
		if _, err := q.ExecContext(ctx, `UPDATE farm_stock SET grow_progress = ?, product_progress = ?, ready_at = ?, progress_at = ? WHERE id = ?`,
			a.growSec, a.productSec, a.ReadyAt, max(at[i], now), a.ID); err != nil {
			return nil, err
		}
		a.derive(st, now)
	}
	if place == placePond {
		pen.Fish = list
	} else {
		pen.Animals = list
	}
	return pen, nil
}

func (f *Farm) load(ctx context.Context, q querier, uid int64) (State, error) {
	if err := ensurePlots(ctx, q, uid); err != nil {
		return State{}, err
	}
	s := State{Now: f.now().Unix(), MaxPlots: MaxPlots}
	var err error
	if s.Plots, err = loadPlots(ctx, q, uid); err != nil {
		return State{}, err
	}
	if len(s.Plots) < MaxPlots {
		s.NextPlotPrice = plotPrice(len(s.Plots))
	}
	if s.Barn, err = loadPen(ctx, q, uid, placeBarn, s.Now); err != nil {
		return State{}, err
	}
	if s.Yard, err = loadPen(ctx, q, uid, placeYard, s.Now); err != nil {
		return State{}, err
	}
	if s.Pond, err = loadPen(ctx, q, uid, placePond, s.Now); err != nil {
		return State{}, err
	}
	// Ao trả mảng fish rỗng chứ không bỏ trường, để giao diện dễ đọc.
	for _, pen := range []*Pen{s.Barn, s.Yard} {
		if pen != nil && pen.Animals == nil {
			pen.Animals = []Animal{}
		}
	}
	if s.Pond != nil && s.Pond.Fish == nil {
		s.Pond.Fish = []Animal{}
	}
	if s.Storage.Seeds, err = loadStore(ctx, q, uid, storeSeeds); err != nil {
		return State{}, err
	}
	if s.Storage.Products, err = loadStore(ctx, q, uid, storeProducts); err != nil {
		return State{}, err
	}
	return s, nil
}

// State trả nông trại của người chơi (tạo 6 ô miễn phí nếu là lần đầu).
func (f *Farm) State(ctx context.Context, uid int64) (State, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	s, err := f.load(ctx, tx, uid)
	if err != nil {
		return State{}, err
	}
	return s, tx.Commit()
}

// do chạy một thao tác trong transaction rồi trả trạng thái mới.
func (f *Farm) do(ctx context.Context, uid int64, op func(tx *wallet.Tx, s State) error) (State, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	s, err := f.load(ctx, tx, uid)
	if err != nil {
		return State{}, err
	}
	if err := op(tx, s); err != nil {
		if errors.Is(err, wallet.ErrNotEnoughCoins) {
			return State{}, noCoins()
		}
		return State{}, err
	}
	if s, err = f.load(ctx, tx, uid); err != nil {
		return State{}, err
	}
	return s, tx.Commit()
}

func requireLevel(tx *wallet.Tx, uid int64, level int) error {
	lv, err := tx.Level(uid)
	if err != nil {
		return err
	}
	if lv < level {
		return needLevel(level)
	}
	return nil
}

// ---------- Ô đất và cây trồng ----------

// BuyPlot mua thêm một ô đất.
func (f *Farm) BuyPlot(ctx context.Context, uid int64) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		n := len(s.Plots)
		if n >= MaxPlots {
			return conflict("đã đủ tối đa 48 ô đất")
		}
		if err := tx.Charge(uid, plotPrice(n), GameID, fmt.Sprintf("Mua ô đất số %d", n+1)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_plots (user_id, idx) VALUES (?, ?)`, uid, n)
		return err
	})
}

// Plant mua hạt và gieo vào một ô trống.
func (f *Farm) Plant(ctx context.Context, uid int64, plot int, cropID string) (State, error) {
	c, ok := findCrop(cropID)
	if !ok {
		return State{}, bad("không có loại cây này")
	}
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if plot < 0 || plot >= len(s.Plots) {
			return bad("ô đất không hợp lệ")
		}
		if s.Plots[plot].Crop != "" {
			return conflict("ô này đang có cây")
		}
		if err := requireLevel(tx, uid, c.Level); err != nil {
			return err
		}
		// Có hạt trong kho giống thì dùng, hết thì mua luôn một hạt.
		if s.Storage.Seeds.Items[c.ID] > 0 {
			if err := addItem(ctx, tx, uid, storeSeeds, c.ID, -1); err != nil {
				return err
			}
		} else if err := tx.Charge(uid, c.SeedPrice, GameID, "Mua hạt "+c.Name); err != nil {
			return err
		}
		now := f.now().Unix()
		_, err := tx.ExecContext(ctx, `UPDATE farm_plots SET crop = ?, planted_at = ?, ready_at = ? WHERE user_id = ? AND idx = ?`,
			c.ID, now, now+c.GrowSeconds, uid, plot)
		return err
	})
}

func clearPlot(ctx context.Context, tx *wallet.Tx, uid int64, plot int) error {
	_, err := tx.ExecContext(ctx, `UPDATE farm_plots SET crop = '', planted_at = 0, ready_at = 0 WHERE user_id = ? AND idx = ?`, uid, plot)
	return err
}

// Harvest thu hoạch một ô đã chín: nông sản vào kho sản phẩm và cộng XP (bán sau ở kho).
func (f *Farm) Harvest(ctx context.Context, uid int64, plot int) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if plot < 0 || plot >= len(s.Plots) {
			return bad("ô đất không hợp lệ")
		}
		p := s.Plots[plot]
		c, ok := findCrop(p.Crop)
		if !ok {
			return conflict("ô này chưa trồng gì")
		}
		if p.ReadyAt > f.now().Unix() {
			return conflict(c.Name + " chưa chín")
		}
		if s.Storage.Products.free() < 1 {
			return productsFull(s.Storage.Products)
		}
		if err := addItem(ctx, tx, uid, storeProducts, c.ID, 1); err != nil {
			return err
		}
		if err := tx.AddXP(uid, c.XP); err != nil {
			return err
		}
		g = Gained{XP: c.XP, Count: 1, Items: map[string]int{c.ID: 1}}
		return clearPlot(ctx, tx, uid, plot)
	})
	return s, g, err
}

// HarvestAll thu hoạch các ô đã chín cho tới khi kho sản phẩm đầy.
func (f *Farm) HarvestAll(ctx context.Context, uid int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		now := f.now().Unix()
		free := s.Storage.Products.free()
		g.Items = map[string]int{}
		ripe := 0
		for _, p := range s.Plots {
			c, ok := findCrop(p.Crop)
			if !ok || p.ReadyAt > now {
				continue
			}
			ripe++
			if g.Count >= free {
				continue
			}
			g.XP += c.XP
			g.Count++
			g.Items[c.ID]++
			if err := clearPlot(ctx, tx, uid, p.Index); err != nil {
				return err
			}
		}
		if ripe == 0 {
			return conflict("chưa có ô nào chín")
		}
		if g.Count == 0 {
			return productsFull(s.Storage.Products)
		}
		for item, n := range g.Items {
			if err := addItem(ctx, tx, uid, storeProducts, item, n); err != nil {
				return err
			}
		}
		return tx.AddXP(uid, g.XP)
	})
	return s, g, err
}

// ---------- Chuồng và ao ----------

func penOf(s State, place string) *Pen {
	switch place {
	case placeBarn:
		return s.Barn
	case placeYard:
		return s.Yard
	}
	return s.Pond
}

func missing(place string) error {
	if place == placeBarn {
		return conflict("chưa có chuồng, hãy mua chuồng trước")
	}
	return conflict("chưa có ao cá, hãy mua ao trước")
}

// findAnimal tìm con id ở place. Gọi /animals/... với gà vịt (giao diện cũ) thì tìm thêm trong sân gia cầm.
func findAnimal(s State, place string, id int64) (string, Animal, Stock, error) {
	places := []string{place}
	if place == placeBarn {
		places = append(places, placeYard)
	}
	for _, pl := range places {
		pen := penOf(s, pl)
		if pen == nil {
			continue
		}
		list := pen.Animals
		if pl == placePond {
			list = pen.Fish
		}
		for _, a := range list {
			if a.ID == id {
				st, _ := findStock(pl, a.Kind)
				return pl, a, st, nil
			}
		}
	}
	switch {
	case place != placeYard && penOf(s, place) == nil:
		return "", Animal{}, Stock{}, missing(place)
	case place == placePond:
		return "", Animal{}, Stock{}, notFound("không tìm thấy con cá này")
	}
	return "", Animal{}, Stock{}, notFound("không tìm thấy con vật này")
}

// BuyBuilding mua chuồng (place = "barn") hoặc ao cá ("pond").
func (f *Farm) BuyBuilding(ctx context.Context, uid int64, place string) (State, error) {
	b := Buildings[place]
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if penOf(s, place) != nil {
			return conflict("bạn đã có " + b.Name + " rồi")
		}
		if err := requireLevel(tx, uid, b.Level); err != nil {
			return err
		}
		if err := tx.Charge(uid, b.Price, GameID, "Mua "+b.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_buildings (user_id, place, capacity) VALUES (?, ?, ?)`, uid, place, b.Capacity)
		return err
	})
}

// ExpandBuilding mở rộng chuồng / ao thêm ExpandUnits đơn vị, giá tăng dần mỗi lần.
func (f *Farm) ExpandBuilding(ctx context.Context, uid int64, place string) (State, error) {
	b := Buildings[place]
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		pen := penOf(s, place)
		if pen == nil {
			return missing(place)
		}
		if pen.Capacity >= b.MaxCapacity {
			return conflict(fmt.Sprintf("%s đã tối đa %d đơn vị", b.Name, b.MaxCapacity))
		}
		if err := tx.Charge(uid, b.expandPrice(pen.expansions), GameID, "Mở rộng "+b.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE farm_buildings SET capacity = ?, expansions = expansions + 1 WHERE user_id = ? AND place = ?`,
			min(b.MaxCapacity, pen.Capacity+b.ExpandUnits), uid, place)
		return err
	})
}

// BuyStock mua một con vật / con cá vào chuồng / ao, chiếm Units đơn vị sức chứa.
func (f *Farm) BuyStock(ctx context.Context, uid int64, place, kind string) (State, error) {
	if _, ok := findStock(placeYard, kind); ok && place == placeBarn {
		place = placeYard // gà vịt mua qua /animals/buy (bản cũ) vào sân gia cầm
	}
	st, ok := findStock(place, kind)
	if !ok {
		return State{}, bad("không có loại này")
	}
	b := Buildings[place]
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		pen := penOf(s, place)
		if pen == nil {
			return missing(place)
		}
		if pen.Used+st.Units > pen.Capacity {
			return conflict(fmt.Sprintf("%s đã đầy (%d/%d đơn vị), %s cần %d đơn vị", b.Name, pen.Used, pen.Capacity, st.Name, st.Units))
		}
		if err := requireLevel(tx, uid, st.Level); err != nil {
			return err
		}
		if err := tx.Charge(uid, st.Price, GameID, "Mua "+st.Name); err != nil {
			return err
		}
		now := f.now().Unix()
		// Con mới mua đã no sẵn FullFor giây.
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at, fed_until, progress_at)
			VALUES (?, ?, ?, ?, 0, ?, ?)`, uid, place, st.ID, now, now+fullFor(st.ID), now)
		return err
	})
}

// SellStock bán một con đã lớn: được xu và XP, giải phóng chỗ trong chuồng / sân / ao.
func (f *Farm) SellStock(ctx context.Context, uid int64, place string, id int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		_, a, st, err := findAnimal(s, place, id)
		if err != nil {
			return err
		}
		if !a.Grown {
			return conflict(st.Name + " chưa lớn")
		}
		if err := tx.Credit(uid, st.SellPrice, GameID, "Bán "+st.Name); err != nil {
			return err
		}
		if err := tx.AddXP(uid, st.XP); err != nil {
			return err
		}
		g = Gained{Coins: st.SellPrice, XP: st.XP}
		_, err = tx.ExecContext(ctx, `DELETE FROM farm_stock WHERE id = ? AND user_id = ?`, id, uid)
		return err
	})
	return s, g, err
}

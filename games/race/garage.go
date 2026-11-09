package race

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Garage: xe đã mua, cấp nâng cấp, đồ trang trí ----------

const garageSchema = `
CREATE TABLE IF NOT EXISTS race_garage (
	user_id     INTEGER PRIMARY KEY REFERENCES users(id),
	current_car TEXT NOT NULL DEFAULT 'starter'
);
CREATE TABLE IF NOT EXISTS race_cars (
	user_id INTEGER NOT NULL REFERENCES users(id),
	car     TEXT NOT NULL,
	engine  INTEGER NOT NULL DEFAULT 0,
	turbo   INTEGER NOT NULL DEFAULT 0,
	tires   INTEGER NOT NULL DEFAULT 0,
	spoiler TEXT NOT NULL DEFAULT '',
	neon    TEXT NOT NULL DEFAULT '',
	paint   TEXT NOT NULL DEFAULT 'paint_red',
	decal   TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (user_id, car)
);
CREATE TABLE IF NOT EXISTS race_items (
	user_id INTEGER NOT NULL REFERENCES users(id),
	item    TEXT NOT NULL,
	PRIMARY KEY (user_id, item)
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
	}
	return http.StatusInternalServerError
}

// Cosmetics là đồ trang trí đang lắp trên một xe ("" = không lắp).
type Cosmetics struct {
	Spoiler string `json:"spoiler"`
	Neon    string `json:"neon"`
	Paint   string `json:"paint"`
	Decal   string `json:"decal"`
}

func (c *Cosmetics) set(kind, item string) {
	switch kind {
	case "spoiler":
		c.Spoiler = item
	case "neon":
		c.Neon = item
	case "paint":
		c.Paint = item
	case "decal":
		c.Decal = item
	}
}

// CarView là một mẫu xe trong garage của người chơi; cũng là xe mang vào phòng đua.
type CarView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"` // "car" hoặc "moto"
	Price     int64     `json:"price"`
	Level     int       `json:"level"` // cấp nhân vật cần để mua
	Owned     bool      `json:"owned"`
	Levels    Levels    `json:"levels"`
	Stats     Stats     `json:"stats"`
	Cosmetics Cosmetics `json:"cosmetics"`
}

// ItemView là một món đồ trang trí; sơn luôn có (miễn phí).
type ItemView struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
	Owned bool   `json:"owned"`
}

// GarageView là garage gửi cho giao diện (GET /api/race/garage).
type GarageView struct {
	Cars    []CarView  `json:"cars"`
	Current string     `json:"current"`
	Items   []ItemView `json:"items"`
	// UpgradePrice[xe][món] là giá nâng món đó lên cấp kế tiếp (0 = đã cấp tối đa hoặc chưa có xe).
	UpgradePrice map[string]map[string]int64 `json:"upgradePrice"`
	MaxLevel     int                         `json:"maxLevel"`
	NitroBoost   float64                     `json:"nitroBoost"`
	NitroMs      int                         `json:"nitroMs"`
	NitroCharge  float64                     `json:"nitroCharge"` // hệ số tốc độ tích nitro
}

// Garage lưu xe và đồ của người chơi trong ví SQLite.
type Garage struct {
	w *wallet.Wallet
}

func NewGarage(ctx context.Context, w *wallet.Wallet) (*Garage, error) {
	if err := w.ExecSchema(ctx, garageSchema); err != nil {
		return nil, fmt.Errorf("tạo bảng trường đua: %w", err)
	}
	return &Garage{w: w}, nil
}

type ownedCar struct {
	levels    Levels
	cosmetics Cosmetics
}

type garageData struct {
	current string
	cars    map[string]ownedCar
	items   map[string]bool
}

// ensure tạo garage mặc định (xe khởi đầu sơn đỏ) nếu người chơi chưa có.
func ensure(ctx context.Context, tx *wallet.Tx, uid int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO race_garage (user_id) VALUES (?)`, uid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO race_cars (user_id, car, paint) VALUES (?, 'starter', ?)`, uid, DefaultPaint)
	return err
}

func load(ctx context.Context, tx *wallet.Tx, uid int64) (garageData, error) {
	d := garageData{cars: map[string]ownedCar{}, items: map[string]bool{}}
	if err := tx.QueryRowContext(ctx, `SELECT current_car FROM race_garage WHERE user_id = ?`, uid).Scan(&d.current); err != nil {
		return d, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT car, engine, turbo, tires, spoiler, neon, paint, decal FROM race_cars WHERE user_id = ?`, uid)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var id string
		var c ownedCar
		if err := rows.Scan(&id, &c.levels.Engine, &c.levels.Turbo, &c.levels.Tires,
			&c.cosmetics.Spoiler, &c.cosmetics.Neon, &c.cosmetics.Paint, &c.cosmetics.Decal); err != nil {
			rows.Close()
			return d, err
		}
		d.cars[id] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT item FROM race_items WHERE user_id = ?`, uid)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return d, err
		}
		d.items[id] = true
	}
	return d, rows.Err()
}

func carView(m CarModel, c ownedCar, owned bool) CarView {
	if !owned {
		c = ownedCar{cosmetics: Cosmetics{Paint: DefaultPaint}}
	}
	return CarView{ID: m.ID, Name: m.Name, Kind: m.Kind, Price: m.Price, Level: m.Level, Owned: owned, Levels: c.levels,
		Stats: StatsOf(m, c.levels), Cosmetics: c.cosmetics}
}

func (d garageData) view() GarageView {
	v := GarageView{Current: d.current, UpgradePrice: map[string]map[string]int64{}, MaxLevel: MaxLevel,
		NitroBoost: NitroBoost, NitroMs: NitroMs, NitroCharge: NitroCharge}
	for _, m := range Cars {
		c, owned := d.cars[m.ID]
		v.Cars = append(v.Cars, carView(m, c, owned))
		prices := map[string]int64{}
		for _, part := range Parts {
			if lv := c.levels.get(part); owned && lv < MaxLevel {
				prices[part] = upgradePrice(m, lv+1)
			} else {
				prices[part] = 0
			}
		}
		v.UpgradePrice[m.ID] = prices
	}
	for _, it := range Items {
		v.Items = append(v.Items, ItemView{ID: it.ID, Kind: it.Kind, Name: it.Name, Price: it.Price,
			Owned: it.Price == 0 || d.items[it.ID]})
	}
	return v
}

// do chạy một thao tác garage trong transaction rồi trả garage mới.
func (g *Garage) do(ctx context.Context, uid int64, op func(tx *wallet.Tx, d garageData) error) (GarageView, error) {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return GarageView{}, err
	}
	defer tx.Rollback()
	if err := ensure(ctx, tx, uid); err != nil {
		return GarageView{}, err
	}
	d, err := load(ctx, tx, uid)
	if err != nil {
		return GarageView{}, err
	}
	if op != nil {
		if err := op(tx, d); err != nil {
			return GarageView{}, err
		}
		if d, err = load(ctx, tx, uid); err != nil {
			return GarageView{}, err
		}
	}
	return d.view(), tx.Commit()
}

// View trả garage của người chơi.
func (g *Garage) View(ctx context.Context, uid int64) (GarageView, error) { return g.do(ctx, uid, nil) }

// Loadout là xe đang dùng (kèm cấp, chỉ số, đồ trang trí) để mang vào phòng đua.
func (g *Garage) Loadout(ctx context.Context, uid int64) (CarView, error) {
	v, err := g.View(ctx, uid)
	if err != nil {
		return CarView{}, err
	}
	for _, c := range v.Cars {
		if c.ID == v.Current && c.Owned {
			return c, nil
		}
	}
	return v.Cars[0], nil
}

// BuyCar mua một mẫu xe và chọn luôn xe đó.
func (g *Garage) BuyCar(ctx context.Context, uid int64, car string) (GarageView, error) {
	return g.do(ctx, uid, func(tx *wallet.Tx, d garageData) error {
		m, ok := findCar(car)
		if !ok {
			return notFound("không có mẫu xe này")
		}
		if _, owned := d.cars[car]; owned {
			return conflict("bạn đã có " + m.Name)
		}
		lv, err := tx.Level(uid)
		if err != nil {
			return err
		}
		if lv < m.Level {
			return &Error{http.StatusForbidden, fmt.Sprintf("Cần cấp %d để mua %s", m.Level, m.Name)}
		}
		if err := tx.Charge(uid, m.Price, GameID, "Mua xe "+m.Name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO race_cars (user_id, car, paint) VALUES (?, ?, ?)`, uid, car, DefaultPaint); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE race_garage SET current_car = ? WHERE user_id = ?`, car, uid)
		return err
	})
}

// SelectCar chọn xe đã có để đua.
func (g *Garage) SelectCar(ctx context.Context, uid int64, car string) (GarageView, error) {
	return g.do(ctx, uid, func(tx *wallet.Tx, d garageData) error {
		if _, ok := findCar(car); !ok {
			return notFound("không có mẫu xe này")
		}
		if _, owned := d.cars[car]; !owned {
			return conflict("bạn chưa có xe này")
		}
		_, err := tx.ExecContext(ctx, `UPDATE race_garage SET current_car = ? WHERE user_id = ?`, car, uid)
		return err
	})
}

// partNames là tên món nâng cấp hiện trong sổ cái.
var partNames = map[string]string{"engine": "động cơ", "turbo": "turbo", "tires": "lốp"}

// Upgrade nâng một món (engine / turbo / tires) của xe lên cấp kế tiếp.
func (g *Garage) Upgrade(ctx context.Context, uid int64, car, part string) (GarageView, error) {
	return g.do(ctx, uid, func(tx *wallet.Tx, d garageData) error {
		m, ok := findCar(car)
		if !ok {
			return notFound("không có mẫu xe này")
		}
		if !slices.Contains(Parts, part) {
			return bad("món nâng cấp không hợp lệ")
		}
		c, owned := d.cars[car]
		if !owned {
			return conflict("bạn chưa có xe này")
		}
		lv := c.levels.get(part)
		if lv >= MaxLevel {
			return conflict(fmt.Sprintf("%s đã cấp tối đa", partNames[part]))
		}
		note := fmt.Sprintf("Nâng %s %s lên cấp %d", partNames[part], m.Name, lv+1)
		if err := tx.Charge(uid, upgradePrice(m, lv+1), GameID, note); err != nil {
			return err
		}
		// part đã được kiểm trong danh sách Parts nên ghép tên cột an toàn.
		_, err := tx.ExecContext(ctx, `UPDATE race_cars SET `+part+` = `+part+` + 1 WHERE user_id = ? AND car = ?`, uid, car)
		return err
	})
}

// BuyItem mua một món đồ trang trí (sơn miễn phí nên không cần mua).
func (g *Garage) BuyItem(ctx context.Context, uid int64, item string) (GarageView, error) {
	return g.do(ctx, uid, func(tx *wallet.Tx, d garageData) error {
		it, ok := findItem(item)
		if !ok {
			return notFound("không có món đồ này")
		}
		if it.Price == 0 || d.items[item] {
			return conflict("bạn đã có " + it.Name)
		}
		if err := tx.Charge(uid, it.Price, GameID, "Mua "+it.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO race_items (user_id, item) VALUES (?, ?)`, uid, item)
		return err
	})
}

// Equip lắp (item = "" thì tháo) một món đồ trang trí loại kind lên xe; xe luôn phải có sơn.
func (g *Garage) Equip(ctx context.Context, uid int64, car, kind, item string) (GarageView, error) {
	return g.do(ctx, uid, func(tx *wallet.Tx, d garageData) error {
		if !slices.Contains(Kinds, kind) {
			return bad("loại đồ không hợp lệ")
		}
		m, ok := findCar(car)
		if !ok {
			return notFound("không có mẫu xe này")
		}
		if _, owned := d.cars[car]; !owned {
			return conflict("bạn chưa có xe này")
		}
		if kind == "spoiler" && item != "" && m.Kind == KindMoto {
			return conflict("Moto không lắp được cánh gió")
		}
		if item == "" {
			if kind == "paint" {
				return bad("xe phải có màu sơn")
			}
		} else {
			it, ok := findItem(item)
			if !ok {
				return notFound("không có món đồ này")
			}
			if it.Kind != kind {
				return bad(it.Name + " không lắp vào chỗ này được")
			}
			if it.Price > 0 && !d.items[item] {
				return conflict("bạn chưa mua " + it.Name)
			}
		}
		// kind đã được kiểm trong danh sách Kinds nên ghép tên cột an toàn.
		_, err := tx.ExecContext(ctx, `UPDATE race_cars SET `+kind+` = ? WHERE user_id = ? AND car = ?`, item, uid, car)
		return err
	})
}

// Garage là nguồn xe mang vào phòng đua.
var _ Loadouts = (*Garage)(nil)

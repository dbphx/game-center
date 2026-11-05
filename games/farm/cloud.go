package farm

import (
	"context"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Đậu thần và tầng mây ----------
//
// Đủ MaxPlots ô đất thì có ô 49 trồng đậu thần, mỗi hạt chín mở vĩnh viễn một tầng mây. Trên mây mua chậu và trồng cây quý.
// Đậu và chậu nằm ở bảng riêng, không nằm trong farm_plots, nên zombie, trộm và thao tác hàng loạt không đụng tới.

const cloudSchema = `
CREATE TABLE IF NOT EXISTS farm_cloud (
	user_id       INTEGER PRIMARY KEY REFERENCES users(id),
	floors        INTEGER NOT NULL DEFAULT 0, -- số tầng đã mở (0–4)
	bean_planted  INTEGER NOT NULL DEFAULT 0, -- 0 = không có hạt đang lớn
	bean_ready_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS farm_pots (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	floor      INTEGER NOT NULL,              -- 1–4
	slot       INTEGER NOT NULL,              -- 0–5
	planted_at INTEGER NOT NULL DEFAULT 0,    -- 0 = chậu trống
	ready_at   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, floor, slot)
);
`

const (
	cloudSlots       = 6
	cloudGrowSeconds = 12 * 3600
	cloudBeanHour    = 3600
)

// CloudFloor là một tầng mây: hạt đậu để mở tầng, loại chậu của tầng và cây mây trồng trong chậu đó.
type CloudFloor struct {
	Floor       int   `json:"floor"`
	BeanPrice   int64 `json:"beanPrice"`
	BeanSeconds int64 `json:"beanSeconds"`
	PotPrice    int64 `json:"potPrice"`
	Slots       int   `json:"slots"`
	Crop        Crop  `json:"crop"`
}

// CloudFloors là các tầng mây, chậu càng đắt thì cây càng quý. Không có điều kiện level, điều kiện duy nhất là đủ 48 ô.
var CloudFloors = []CloudFloor{
	{1, 200_000, 24 * cloudBeanHour, 100_000, cloudSlots, Crop{"cloud_berry", "Dâu mây", 0, 5_000, cloudGrowSeconds, 20_000, 200}},
	{2, 1_000_000, 48 * cloudBeanHour, 500_000, cloudSlots, Crop{"mist_pear", "Lê sương", 0, 20_000, cloudGrowSeconds, 70_000, 300}},
	{3, 3_000_000, 72 * cloudBeanHour, 2_000_000, cloudSlots, Crop{"star_grape", "Nho sao", 0, 50_000, cloudGrowSeconds, 200_000, 450}},
	{4, 10_000_000, 96 * cloudBeanHour, 10_000_000, cloudSlots, Crop{"fairy_peach", "Đào tiên", 0, 100_000, cloudGrowSeconds, 600_000, 700}},
}

func findCloudCrop(id string) (Crop, bool) {
	for _, f := range CloudFloors {
		if f.Crop.ID == id {
			return f.Crop, true
		}
	}
	return Crop{}, false
}

// Pot là một chậu đã mua trên mây; PlantedAt = 0 nghĩa là chậu trống.
type Pot struct {
	Floor     int   `json:"floor"`
	Slot      int   `json:"slot"`
	PlantedAt int64 `json:"plantedAt"`
	ReadyAt   int64 `json:"readyAt"`
}

// Cloud là phần trên mây của nông trại. Available: đã đủ 48 ô, được thấy ô 49.
type Cloud struct {
	Available     bool  `json:"available"`
	Floors        int   `json:"floors"`
	BeanPlantedAt int64 `json:"beanPlantedAt"`
	BeanReadyAt   int64 `json:"beanReadyAt"`
	Pots          []Pot `json:"pots"`
}

func (c Cloud) pot(floor, slot int) (Pot, bool) {
	for _, p := range c.Pots {
		if p.Floor == floor && p.Slot == slot {
			return p, true
		}
	}
	return Pot{}, false
}

func (c Cloud) potsOn(floor int) int {
	n := 0
	for _, p := range c.Pots {
		if p.Floor == floor {
			n++
		}
	}
	return n
}

// settleBean chốt hạt đậu đã chín: mở tầng kế tiếp và trả ô 49 về trống, kể cả khi chủ offline.
func (f *Farm) settleBean(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE farm_cloud SET floors = floors + 1, bean_planted = 0, bean_ready_at = 0
		WHERE user_id = ? AND bean_planted > 0 AND bean_ready_at <= ?`, uid, now)
	return err
}

// loadCloud đọc phần trên mây; plots là số ô đất đang có.
func loadCloud(ctx context.Context, q querier, uid int64, plots int) (Cloud, error) {
	c := Cloud{Available: plots >= MaxPlots, Pots: []Pot{}}
	rows, err := q.QueryContext(ctx, `SELECT floors, bean_planted, bean_ready_at FROM farm_cloud WHERE user_id = ?`, uid)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		if err := rows.Scan(&c.Floors, &c.BeanPlantedAt, &c.BeanReadyAt); err != nil {
			rows.Close()
			return c, err
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return c, err
	}
	rows, err = q.QueryContext(ctx, `SELECT floor, slot, planted_at, ready_at FROM farm_pots WHERE user_id = ? ORDER BY floor, slot`, uid)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Pot
		if err := rows.Scan(&p.Floor, &p.Slot, &p.PlantedAt, &p.ReadyAt); err != nil {
			return c, err
		}
		c.Pots = append(c.Pots, p)
	}
	return c, rows.Err()
}

// cloudFloor trả tầng theo số 1–4.
func cloudFloor(floor int) (CloudFloor, error) {
	if floor < 1 || floor > len(CloudFloors) {
		return CloudFloor{}, bad("tầng mây không hợp lệ")
	}
	return CloudFloors[floor-1], nil
}

// cloudSpot kiểm tra chỗ đặt chậu hợp lệ.
func cloudSpot(floor, slot int) (CloudFloor, error) {
	cf, err := cloudFloor(floor)
	if err != nil {
		return cf, err
	}
	if slot < 0 || slot >= cf.Slots {
		return cf, bad("chỗ đặt chậu không hợp lệ")
	}
	return cf, nil
}

// PlantBean trồng hạt đậu thần ở ô 49 để mở tầng kế tiếp. Trả tiền hạt ngay, một lúc chỉ có một hạt đang lớn.
func (f *Farm) PlantBean(ctx context.Context, uid int64) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if !s.Cloud.Available {
			return conflict(fmt.Sprintf("cần đủ %d ô đất mới trồng được đậu thần", MaxPlots))
		}
		if s.Cloud.BeanPlantedAt > 0 {
			return conflict("đậu thần đang lớn, chờ chín rồi trồng hạt tiếp")
		}
		if s.Cloud.Floors >= len(CloudFloors) {
			return conflict(fmt.Sprintf("đã mở đủ %d tầng mây", len(CloudFloors)))
		}
		cf := CloudFloors[s.Cloud.Floors]
		if err := tx.Charge(uid, cf.BeanPrice, GameID, fmt.Sprintf("Trồng đậu thần lên tầng %d", cf.Floor)); err != nil {
			return err
		}
		now := f.now().Unix()
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_cloud (user_id, bean_planted, bean_ready_at) VALUES (?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET bean_planted = excluded.bean_planted, bean_ready_at = excluded.bean_ready_at`,
			uid, now, now+cf.BeanSeconds)
		return err
	})
}

// BuyPot mua chậu kế tiếp cho một tầng đã mở; chậu thứ n nằm ở chỗ n−1, không bán lại được.
func (f *Farm) BuyPot(ctx context.Context, uid int64, floor int) (State, error) {
	cf, err := cloudFloor(floor)
	if err != nil {
		return State{}, err
	}
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if floor > s.Cloud.Floors {
			return conflict(fmt.Sprintf("tầng %d chưa mở", floor))
		}
		n := s.Cloud.potsOn(floor)
		if n >= cf.Slots {
			return conflict(fmt.Sprintf("tầng %d đã đủ %d chậu", floor, cf.Slots))
		}
		if err := tx.Charge(uid, cf.PotPrice, GameID, fmt.Sprintf("Mua chậu tầng %d", floor)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_pots (user_id, floor, slot) VALUES (?, ?, ?)`, uid, floor, n)
		return err
	})
}

// PlantPot trồng cây mây của tầng vào chậu; tiền hạt trừ thẳng bằng xu, không qua kho giống.
func (f *Farm) PlantPot(ctx context.Context, uid int64, floor, slot int) (State, error) {
	cf, err := cloudSpot(floor, slot)
	if err != nil {
		return State{}, err
	}
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		p, ok := s.Cloud.pot(floor, slot)
		if !ok {
			return conflict("chỗ này chưa có chậu")
		}
		if p.PlantedAt > 0 {
			return conflict("chậu này đang có cây")
		}
		if err := tx.Charge(uid, cf.Crop.SeedPrice, GameID, "Trồng "+cf.Crop.Name); err != nil {
			return err
		}
		now := f.now().Unix()
		_, err := tx.ExecContext(ctx, `UPDATE farm_pots SET planted_at = ?, ready_at = ? WHERE user_id = ? AND floor = ? AND slot = ?`,
			now, now+cf.Crop.GrowSeconds, uid, floor, slot)
		return err
	})
}

// HarvestPot thu cây mây đã chín: 1 nông sản vào kho sản phẩm kèm XP, chậu trống lại. Kho đầy thì cây vẫn còn.
func (f *Farm) HarvestPot(ctx context.Context, uid int64, floor, slot int) (State, Gained, error) {
	cf, err := cloudSpot(floor, slot)
	if err != nil {
		return State{}, Gained{}, err
	}
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		p, ok := s.Cloud.pot(floor, slot)
		if !ok {
			return conflict("chỗ này chưa có chậu")
		}
		if p.PlantedAt == 0 {
			return conflict("chậu này chưa trồng gì")
		}
		if p.ReadyAt > f.now().Unix() {
			return conflict(cf.Crop.Name + " chưa chín")
		}
		if s.Storage.Products.free() < 1 {
			return productsFull(s.Storage.Products)
		}
		if err := addItem(ctx, tx, uid, storeProducts, cf.Crop.ID, 1); err != nil {
			return err
		}
		if err := tx.AddXP(uid, cf.Crop.XP); err != nil {
			return err
		}
		g = Gained{XP: cf.Crop.XP, Count: 1, Items: map[string]int{cf.Crop.ID: 1}}
		_, err := tx.ExecContext(ctx, `UPDATE farm_pots SET planted_at = 0, ready_at = 0 WHERE user_id = ? AND floor = ? AND slot = ?`, uid, floor, slot)
		return err
	})
	return s, g, err
}

// addCloudWorth cộng phần trên mây vào giá trị nông trại: hạt đậu của tầng đã mở và hạt đang lớn, giá chậu, tiền hạt của cây trong chậu.
func addCloudWorth(ctx context.Context, q querier, worth map[int64]int64) error {
	rows, err := q.QueryContext(ctx, `SELECT user_id, floors, bean_planted FROM farm_cloud`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var uid, planted int64
		var floors int
		if err := rows.Scan(&uid, &floors, &planted); err != nil {
			rows.Close()
			return err
		}
		if planted > 0 {
			floors++
		}
		for i := 0; i < floors && i < len(CloudFloors); i++ {
			worth[uid] += CloudFloors[i].BeanPrice
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = q.QueryContext(ctx, `SELECT user_id, floor, planted_at FROM farm_pots`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var uid, planted int64
		var floor int
		if err := rows.Scan(&uid, &floor, &planted); err != nil {
			return err
		}
		cf, err := cloudFloor(floor)
		if err != nil {
			continue
		}
		worth[uid] += cf.PotPrice
		if planted > 0 {
			worth[uid] += cf.Crop.SeedPrice
		}
	}
	return rows.Err()
}

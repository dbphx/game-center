package farm

import (
	"context"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Sản phẩm của vật nuôi: trứng / sữa / lông, và thịt ----------

// Product là sản phẩm con vật đã lớn cho đều đặn: cứ Every giây có 1 món (giữ tối đa 1 món chờ lấy).
type Product struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Every     int64  `json:"every"`
	SellPrice int64  `json:"sellPrice"`
	XP        int64  `json:"xp"`
}

// Meat là thịt lấy được khi thu con vật (con vật mất).
type Meat struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SellPrice int64  `json:"sellPrice"`
	XP        int64  `json:"xp"`
}

type Yield struct {
	Product *Product // nil: con này không cho sản phẩm (heo)
	Meat    Meat
}

// yields theo id vật nuôi; giá / XP của thịt bằng giá bán / XP của con đó.
var yields = map[string]Yield{
	"chicken": {&Product{"egg", "Trứng gà", 600, 60, 2}, Meat{"meat_chicken", "Thịt gà", 400, 5}},
	"duck":    {&Product{"duck_egg", "Trứng vịt", 900, 80, 2}, Meat{"meat_duck", "Thịt vịt", 520, 6}},
	"pig":     {nil, Meat{"meat_pig", "Thịt heo", 1300, 12}},
	"sheep":   {&Product{"wool", "Lông cừu", 1800, 400, 6}, Meat{"meat_sheep", "Thịt cừu", 2000, 20}},
	"cow":     {&Product{"milk", "Sữa bò", 1200, 300, 5}, Meat{"meat_cow", "Thịt bò", 3400, 35}},
}

// animalJSON là một loại vật nuôi trong catalog, kèm sản phẩm và thịt.
type animalJSON struct {
	stockJSON
	Product *Product `json:"product"`
	Meat    *Meat    `json:"meat"`
}

func withYields(list []stockJSON) []animalJSON {
	out := make([]animalJSON, len(list))
	for i, s := range list {
		out[i] = animalJSON{stockJSON: s}
		if y, ok := yields[s.ID]; ok {
			m := y.Meat
			out[i].Product, out[i].Meat = y.Product, &m
		}
	}
	return out
}

// ProduceStock lấy trứng / sữa / lông của một con đã lớn (chuồng hoặc sân) vào kho sản phẩm, rồi chờ thêm Every giây.
func (f *Farm) ProduceStock(ctx context.Context, uid int64, place string, id int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		_, a, st, err := findAnimal(s, place, id)
		if err != nil {
			return err
		}
		now := f.now().Unix()
		p := yields[a.Kind].Product
		if p == nil {
			return conflict(st.Name + " không cho sản phẩm, chỉ lấy thịt được")
		}
		if !a.Grown {
			return conflict(st.Name + " chưa lớn")
		}
		if !a.ProductReady {
			if a.Paused {
				return conflict(fmt.Sprintf("%s chưa có, %s đang đói — cho ăn để %s tiếp", p.Name, st.Name, produceVerb(p)))
			}
			return conflict(fmt.Sprintf("%s chưa có, chờ thêm %s", p.Name, fmtWait(a.ProductReadyAt-now)))
		}
		if s.Storage.Products.free() < 1 {
			return productsFull(s.Storage.Products)
		}
		g = Gained{XP: p.XP, Count: 1, Items: map[string]int{p.ID: 1}}
		return produced(ctx, tx, uid, a.ID, p, now)
	})
	return s, g, err
}

// produced cất 1 sản phẩm của con id vào kho, cộng XP và bắt đầu chu kỳ mới.
func produced(ctx context.Context, tx *wallet.Tx, uid, id int64, p *Product, now int64) error {
	if err := addItem(ctx, tx, uid, storeProducts, p.ID, 1); err != nil {
		return err
	}
	if err := tx.AddXP(uid, p.XP); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE farm_stock SET produced_at = ?, product_progress = 0 WHERE id = ? AND user_id = ?`, now, id, uid)
	return err
}

// produceVerb là động từ ra sản phẩm: đẻ trứng, cho sữa, ra lông.
func produceVerb(p *Product) string {
	switch p.ID {
	case "milk":
		return "cho sữa"
	case "wool":
		return "ra lông"
	}
	return "đẻ trứng"
}

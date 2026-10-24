package wallet

import (
	"context"
	"database/sql"
	"errors"
	"slices"
)

// Shop thời trang có 4 nhóm: quần áo (Catalog), tóc (HairCatalog), cánh (WingsCatalog), gương mặt (FaceCatalog).
// Tóc, cánh, gương mặt dùng chung cho cả hai giới; giao diện vẽ chồng lên nhân vật theo id.

const (
	KindOutfit = "outfit"
	KindHair   = "hair"
	KindWings  = "wings"
	KindFace   = "face"

	DefaultHair = "default" // tóc mặc định, ai cũng có
	DefaultFace = "default" // gương mặt mặc định, ai cũng có
	NoWings     = ""        // không đeo cánh
)

// ItemInfo là một món tóc / cánh (hoặc quần áo, khi gửi chung) trong shop thời trang.
type ItemInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Level   int    `json:"level"`
	Price   int64  `json:"price"`
	Starter bool   `json:"starter"`
}

var (
	HairCatalog = []ItemInfo{
		{ID: DefaultHair, Name: "Tóc mặc định", Kind: KindHair, Level: 1, Starter: true},
		{ID: "short_spiky", Name: "Tóc dựng", Kind: KindHair, Level: 1, Price: 300},
		{ID: "ponytail", Name: "Đuôi ngựa", Kind: KindHair, Level: 1, Price: 300},
		{ID: "long_wave", Name: "Tóc dài xoăn", Kind: KindHair, Level: 2, Price: 700},
		{ID: "bun", Name: "Tóc búi", Kind: KindHair, Level: 2, Price: 700},
		{ID: "mohawk", Name: "Mohican", Kind: KindHair, Level: 3, Price: 1200},
		{ID: "pink_twin", Name: "Hai bím hồng", Kind: KindHair, Level: 4, Price: 2000},
	}

	WingsCatalog = []ItemInfo{
		{ID: "angel", Name: "Cánh thiên thần", Kind: KindWings, Level: 3, Price: 2500},
		{ID: "fairy", Name: "Cánh tiên", Kind: KindWings, Level: 4, Price: 3500},
		{ID: "butterfly", Name: "Cánh bướm", Kind: KindWings, Level: 5, Price: 4500},
		{ID: "bat", Name: "Cánh dơi", Kind: KindWings, Level: 6, Price: 6000},
		{ID: "dragon", Name: "Cánh rồng", Kind: KindWings, Level: 8, Price: 10000},
	}

	FaceCatalog = []ItemInfo{
		{ID: DefaultFace, Name: "Mặt mặc định", Kind: KindFace, Level: 1, Starter: true},
		{ID: "smile", Name: "Cười tươi", Kind: KindFace, Level: 1, Price: 200},
		{ID: "wink", Name: "Nháy mắt", Kind: KindFace, Level: 1, Price: 300},
		{ID: "blush", Name: "Má hồng", Kind: KindFace, Level: 2, Price: 600},
		{ID: "cool", Name: "Kính râm", Kind: KindFace, Level: 2, Price: 800},
		{ID: "cat", Name: "Mèo con", Kind: KindFace, Level: 3, Price: 1200},
		{ID: "star", Name: "Mắt lấp lánh", Kind: KindFace, Level: 4, Price: 2000},
	}

	ErrInvalidItem = errors.New("món đồ không hợp lệ")
)

func itemCatalog(kind string) []ItemInfo {
	switch kind {
	case KindHair:
		return HairCatalog
	case KindWings:
		return WingsCatalog
	case KindFace:
		return FaceCatalog
	}
	return nil
}

// itemInfo tra một món tóc / cánh / gương mặt; quần áo tra trong Catalog qua outfitInfo.
func itemInfo(kind, id string) (ItemInfo, bool) {
	list := itemCatalog(kind)
	i := slices.IndexFunc(list, func(it ItemInfo) bool { return it.ID == id })
	if i < 0 {
		return ItemInfo{}, false
	}
	return list[i], true
}

// ownedItems trả về tóc / cánh / gương mặt người chơi có: món starter và món đã mua, theo thứ tự catalog.
func ownedItems(ctx context.Context, q queryer, id int64, kind string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT item FROM user_items WHERE user_id = ? AND kind = ?`, id, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bought := map[string]bool{}
	for rows.Next() {
		var it string
		if err := rows.Scan(&it); err != nil {
			return nil, err
		}
		bought[it] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []string{}
	for _, it := range itemCatalog(kind) {
		if it.Starter || bought[it.ID] {
			out = append(out, it.ID)
		}
	}
	return out, nil
}

// BuyItem mua một món ở shop thời trang (kind: outfit, hair, wings, face): đủ level, đủ xu thì trừ xu,
// ghi sổ cái và mặc / đeo luôn.
func (w *Wallet) BuyItem(ctx context.Context, id int64, kind, item string) (User, error) {
	if kind == KindOutfit {
		return w.BuyOutfit(ctx, id, item)
	}
	it, ok := itemInfo(kind, item)
	if !ok {
		return User{}, ErrInvalidItem
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	have, err := ownedItems(ctx, tx, id, kind)
	if err != nil {
		return User{}, err
	}
	if slices.Contains(have, item) {
		return User{}, ErrAlreadyOwned
	}
	var balance, xp int64
	if err := tx.QueryRowContext(ctx, `SELECT balance, xp FROM users WHERE id = ?`, id).Scan(&balance, &xp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if LevelFor(xp) < it.Level {
		return User{}, &LevelError{Need: it.Level}
	}
	if balance < it.Price {
		return User{}, ErrNotEnoughCoins
	}

	now := w.now().Unix()
	balance -= it.Price
	// kind chỉ là "hair", "wings" hoặc "face" (đã kiểm ở itemInfo) nên ghép thẳng tên cột được.
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ?, `+kind+` = ? WHERE id = ?`, balance, item, id); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_items (user_id, kind, item, created_at) VALUES (?, ?, ?, ?)`, id, kind, item, now); err != nil {
		return User{}, err
	}
	if it.Price > 0 {
		if err := addEntry(ctx, tx, id, -it.Price, balance, "shop", "Mua "+it.Name, now); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// WearItem mặc / đeo một món đã có. Tóc, mặt "default" luôn dùng được, cánh "" là tháo cánh.
func (w *Wallet) WearItem(ctx context.Context, id int64, kind, item string) (User, error) {
	switch {
	case kind == KindOutfit:
		return w.WearOutfit(ctx, id, item)
	case kind == KindWings && item == NoWings:
	default:
		if _, ok := itemInfo(kind, item); !ok {
			return User{}, ErrInvalidItem
		}
		have, err := ownedItems(ctx, w.db, id, kind)
		if err != nil {
			return User{}, err
		}
		if !slices.Contains(have, item) {
			return User{}, ErrNotOwned
		}
	}
	if _, err := w.db.ExecContext(ctx, `UPDATE users SET `+kind+` = ? WHERE id = ?`, item, id); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

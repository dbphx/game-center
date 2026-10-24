package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// Nhân vật của mỗi tài khoản: giới tính (chọn một lần lúc tạo), trang phục và điểm kinh nghiệm (level).
// Giao diện vẽ nhân vật theo các id này (public/character.js).

// OutfitInfo là một món trong shop thời trang. Starter được chọn miễn phí lúc tạo nhân vật,
// các món khác phải đủ level rồi mua bằng xu.
type OutfitInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Level   int    `json:"level"`
	Price   int64  `json:"price"`
	Starter bool   `json:"starter"`
}

var (
	Genders = []string{"male", "female"}

	Catalog = []OutfitInfo{
		{ID: "casual", Name: "Thường ngày", Level: 1, Starter: true},
		{ID: "sport", Name: "Thể thao", Level: 1, Starter: true},
		{ID: "hoodie", Name: "Áo hoodie", Level: 2, Price: 800},
		{ID: "suit", Name: "Vest lịch lãm", Level: 3, Price: 1500},
		{ID: "aodai", Name: "Áo dài", Level: 4, Price: 2500},
		{ID: "royal", Name: "Hoàng gia", Level: 6, Price: 5000},
	}

	// Outfits là id các trang phục theo thứ tự trong Catalog.
	Outfits = func() []string {
		ids := make([]string, len(Catalog))
		for i, o := range Catalog {
			ids[i] = o.ID
		}
		return ids
	}()

	ErrInvalidCharacter = errors.New("nhân vật không hợp lệ")
	ErrNotOwned         = errors.New("bạn chưa sở hữu trang phục này")
	ErrAlreadyOwned     = errors.New("bạn đã có trang phục này")
	ErrNotEnoughCoins   = errors.New("không đủ xu")
	ErrLevelTooLow      = errors.New("chưa đủ level")
)

// LevelError cho biết cần level bao nhiêu để mua; errors.Is(err, ErrLevelTooLow) vẫn đúng.
type LevelError struct{ Need int }

func (e *LevelError) Error() string        { return fmt.Sprintf("cần level %d để mua", e.Need) }
func (e *LevelError) Is(target error) bool { return target == ErrLevelTooLow }

func outfitInfo(id string) (OutfitInfo, bool) {
	i := slices.IndexFunc(Catalog, func(o OutfitInfo) bool { return o.ID == id })
	if i < 0 {
		return OutfitInfo{}, false
	}
	return Catalog[i], true
}

// Profile là phần người chơi tự chọn khi tạo nhân vật.
type Profile struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
}

// normalize kiểm tra lựa chọn lúc tạo nhân vật: chỉ được chọn trang phục starter.
func (p *Profile) normalize() error {
	if p.Gender == "" {
		p.Gender = Genders[0]
	}
	if p.Outfit == "" {
		p.Outfit = Catalog[0].ID
	}
	o, ok := outfitInfo(p.Outfit)
	if !slices.Contains(Genders, p.Gender) || !ok || !o.Starter {
		return ErrInvalidCharacter
	}
	return nil
}

// LevelStart là tổng XP cần để đạt level: 1 → 0, 2 → 100, 3 → 300, 4 → 600...
// Mỗi level sau cần nhiều hơn level trước 100 XP.
func LevelStart(level int) int64 { return int64(50 * level * (level - 1)) }

func LevelFor(xp int64) int {
	level := 1
	for LevelStart(level+1) <= xp {
		level++
	}
	return level
}

// queryer là *sql.DB hoặc *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// owned trả về trang phục người chơi có: các món starter và các món đã mua, theo thứ tự Catalog.
func owned(ctx context.Context, q queryer, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT outfit FROM user_outfits WHERE user_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bought := map[string]bool{}
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return nil, err
		}
		bought[o] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []string{}
	for _, o := range Catalog {
		if o.Starter || bought[o.ID] {
			out = append(out, o.ID)
		}
	}
	return out, nil
}

// WearOutfit mặc một trang phục đã sở hữu. Giới tính không đổi được sau khi tạo nhân vật.
func (w *Wallet) WearOutfit(ctx context.Context, id int64, outfit string) (User, error) {
	if _, ok := outfitInfo(outfit); !ok {
		return User{}, ErrInvalidCharacter
	}
	have, err := owned(ctx, w.db, id)
	if err != nil {
		return User{}, err
	}
	if !slices.Contains(have, outfit) {
		return User{}, ErrNotOwned
	}
	if _, err := w.db.ExecContext(ctx, `UPDATE users SET outfit = ? WHERE id = ?`, outfit, id); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// BuyOutfit mua trang phục ở shop: đủ level, đủ xu thì trừ xu, ghi sổ cái và mặc luôn.
func (w *Wallet) BuyOutfit(ctx context.Context, id int64, outfit string) (User, error) {
	o, ok := outfitInfo(outfit)
	if !ok {
		return User{}, ErrInvalidCharacter
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	have, err := owned(ctx, tx, id)
	if err != nil {
		return User{}, err
	}
	if slices.Contains(have, outfit) {
		return User{}, ErrAlreadyOwned
	}
	var balance, xp int64
	if err := tx.QueryRowContext(ctx, `SELECT balance, xp FROM users WHERE id = ?`, id).Scan(&balance, &xp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if LevelFor(xp) < o.Level {
		return User{}, &LevelError{Need: o.Level}
	}
	if balance < o.Price {
		return User{}, ErrNotEnoughCoins
	}

	now := w.now().Unix()
	balance -= o.Price
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ?, outfit = ? WHERE id = ?`, balance, outfit, id); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_outfits (user_id, outfit, created_at) VALUES (?, ?, ?)`, id, outfit, now); err != nil {
		return User{}, err
	}
	if o.Price > 0 {
		if err := addEntry(ctx, tx, id, -o.Price, balance, "shop", "Mua trang phục "+o.Name, now); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// AwardXP cộng kinh nghiệm cho nhiều người (ví dụ sau một ván). Trả về level mới của từng người.
func (w *Wallet) AwardXP(ctx context.Context, xp map[int64]int64) (map[int64]int, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	levels := map[int64]int{}
	for id, add := range xp {
		var total int64
		if err := tx.QueryRowContext(ctx, `UPDATE users SET xp = xp + ? WHERE id = ? RETURNING xp`, max(0, add), id).Scan(&total); err != nil {
			return nil, err
		}
		levels[id] = LevelFor(total)
	}
	return levels, tx.Commit()
}

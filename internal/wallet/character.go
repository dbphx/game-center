package wallet

import (
	"context"
	"errors"
	"slices"
)

// Nhân vật của mỗi tài khoản: giới tính, trang phục và điểm kinh nghiệm (level).
// Giao diện vẽ nhân vật theo các id này (public/character.js).

var (
	Genders = []string{"male", "female"}
	Outfits = []string{"casual", "suit", "sport", "aodai"}

	ErrInvalidCharacter = errors.New("nhân vật không hợp lệ")
)

// Profile là phần người chơi tự chọn khi tạo nhân vật.
type Profile struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
}

func (p *Profile) normalize() error {
	if p.Gender == "" {
		p.Gender = Genders[0]
	}
	if p.Outfit == "" {
		p.Outfit = Outfits[0]
	}
	if !slices.Contains(Genders, p.Gender) || !slices.Contains(Outfits, p.Outfit) {
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

// SetCharacter đổi giới tính / trang phục (ở shop thời trang).
func (w *Wallet) SetCharacter(ctx context.Context, id int64, gender, outfit string) (User, error) {
	p := Profile{Gender: gender, Outfit: outfit}
	if err := p.normalize(); err != nil {
		return User{}, err
	}
	if _, err := w.db.ExecContext(ctx, `UPDATE users SET gender = ?, outfit = ? WHERE id = ?`, p.Gender, p.Outfit, id); err != nil {
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

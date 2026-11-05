package farm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	mrand "math/rand/v2"
	"slices"

	"github.com/dbphx/game-center/internal/wallet"
)

// Đất tốt: nâng một lần mỗi ô, cây gieo sau đó chín nhanh hơn 30%.
const (
	GoodSoilPrice   = 8000
	GoodSoilSpeedup = 0.3
)

// growSeconds là thời gian chín của cây trên một ô đất.
func growSeconds(c Crop, good bool) int64 {
	if good {
		return c.GrowSeconds * 7 / 10
	}
	return c.GrowSeconds
}

// Zombie: đúng đầu mỗi giờ server có một đợt chung cho mọi nông trại, chủ có ZombieWindow giây để đập.
// Không có tiến trình nền: settleZombies tính bù các đợt đã qua mỗi khi nông trại được đọc, ngẫu nhiên có seed
// (uid, đợt) nên tính lại (vd transaction bị huỷ) vẫn ra đúng kết quả cũ.
const (
	ZombieWindow      = 300  // giây để đập zombie, tính từ đầu giờ
	ZombieHitHP       = 2    // máu mất mỗi nhát đập
	zombieWaveSeconds = 3600 // một đợt mỗi giờ
	zombieMaxCatchup  = 720  // vắng lâu chỉ tính bù tối đa 30 ngày gần nhất
)

// Súng đậu: mua một lần, mỗi viên đạn tự diệt một zombie từ giây GunDelay của đợt, kể cả khi chủ offline.
const (
	GunPrice    = 399000
	AmmoPrice   = 500
	GunDelay    = 30   // giây sau đầu giờ súng mới bắn, để người đang online thấy zombie và đạn bay
	maxAmmoSale = 1000 // số đạn tối đa mỗi lần mua
)

// zombieRisk là nguy cơ mỗi giờ của một cây không được canh: mất đúng 50% sau 12 giờ.
var zombieRisk = 1 - math.Pow(0.5, 1.0/12)

const zombieSchema = `
CREATE TABLE IF NOT EXISTS farm_zombies (
	user_id INTEGER NOT NULL REFERENCES users(id),
	idx     INTEGER NOT NULL,          -- ô đất
	wave    INTEGER NOT NULL,          -- giờ của đợt: unix / 3600
	bites   INTEGER NOT NULL,          -- 1 = hết hạn sẽ ăn cây, 0 = bỏ đi; không gửi cho giao diện
	PRIMARY KEY (user_id, idx)
);
CREATE TABLE IF NOT EXISTS farm_zombie_progress (
	user_id   INTEGER PRIMARY KEY REFERENCES users(id),
	last_wave INTEGER NOT NULL,        -- đợt mới nhất đã tính
	eaten     INTEGER NOT NULL DEFAULT 0 -- số cây bị ăn chưa báo cho chủ
);
CREATE TABLE IF NOT EXISTS farm_guns (
	user_id INTEGER PRIMARY KEY REFERENCES users(id), -- có dòng = đã mua súng
	ammo    INTEGER NOT NULL DEFAULT 0, -- đạn còn lại
	killed  INTEGER NOT NULL DEFAULT 0  -- số zombie súng đã diệt chưa báo cho chủ
);
`

// zombieCount trả số zombie của một đợt và xác suất mỗi con ăn cây nếu không bị đập, để nguy cơ của mỗi cây
// đúng bằng zombieRisk ở mọi cỡ nhà.
func zombieCount(crops int) (count int, biteChance float64) {
	if crops <= 0 {
		return 0, 0
	}
	risk := zombieRisk * float64(crops)
	count = min(crops, max(1, int(math.Ceil(risk))))
	return count, min(1, risk/float64(count))
}

// zombiePick là một zombie được chọn cho một đợt.
type zombiePick struct {
	idx   int
	bites bool
}

// pickZombies chọn mục tiêu trong các ô đủ điều kiện; cùng (uid, đợt) và cùng danh sách thì ra cùng kết quả.
func pickZombies(uid, wave int64, eligible []int) []zombiePick {
	count, chance := zombieCount(len(eligible))
	if count == 0 {
		return nil
	}
	rng := mrand.New(mrand.NewPCG(uint64(uid), uint64(wave)))
	plots := slices.Clone(eligible)
	rng.Shuffle(len(plots), func(i, j int) { plots[i], plots[j] = plots[j], plots[i] })
	out := make([]zombiePick, count)
	for i := range out {
		out[i] = zombiePick{idx: plots[i], bites: rng.Float64() < chance}
	}
	return out
}

// settleZombies tính bù zombie của uid tới thời điểm now: cho zombie quá hạn ăn cây hoặc bỏ đi, thêm các đợt mới.
// Lần đầu thấy một nông trại thì không tính ngược, đợt đầu tiên là đầu giờ kế tiếp.
func (f *Farm) settleZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	if f.noZombies {
		return nil
	}
	cur := now / zombieWaveSeconds
	var last int64
	err := tx.QueryRowContext(ctx, `SELECT last_wave FROM farm_zombie_progress WHERE user_id = ?`, uid).Scan(&last)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO farm_zombie_progress (user_id, last_wave) VALUES (?, ?)`, uid, cur)
		return err
	}
	if err != nil {
		return err
	}
	if err := shootThenExpire(ctx, tx, uid, now); err != nil {
		return err
	}
	if last >= cur {
		return nil
	}
	for w := max(last+1, cur-zombieMaxCatchup+1); w <= cur; w++ {
		start := w * zombieWaveSeconds
		eligible, err := zombieTargets(ctx, tx, uid, start)
		if err != nil {
			return err
		}
		for _, z := range pickZombies(uid, w, eligible) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO farm_zombies (user_id, idx, wave, bites) VALUES (?, ?, ?, ?)`,
				uid, z.idx, w, z.bites); err != nil {
				return err
			}
		}
		// Tính bù theo thứ tự thời gian: đợt đã qua vẫn tốn đạn trước khi tới đợt sau.
		if len(eligible) > 0 && now >= start+GunDelay {
			if err := shootThenExpire(ctx, tx, uid, now); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_zombie_progress SET last_wave = ? WHERE user_id = ?`, cur, uid)
	return err
}

// zombieTargets liệt kê các ô có cây gieo trước lúc đợt bắt đầu và chưa có zombie.
func zombieTargets(ctx context.Context, tx *wallet.Tx, uid, start int64) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.idx FROM farm_plots p WHERE p.user_id = ? AND p.crop != '' AND p.planted_at < ?
		AND NOT EXISTS (SELECT 1 FROM farm_zombies z WHERE z.user_id = p.user_id AND z.idx = p.idx) ORDER BY p.idx`, uid, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		out = append(out, idx)
	}
	return out, rows.Err()
}

// shootThenExpire cho súng bắn trước, rồi mới xử lý zombie hết hạn.
func shootThenExpire(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	if err := shootZombies(ctx, tx, uid, now); err != nil {
		return err
	}
	return expireZombies(ctx, tx, uid, now)
}

// shootZombies dùng đạn diệt các zombie đã quá giây GunDelay, đợt cũ trước, ô nhỏ trước; hết đạn thì dừng.
func shootZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	var ammo int
	err := tx.QueryRowContext(ctx, `SELECT ammo FROM farm_guns WHERE user_id = ?`, uid).Scan(&ammo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil || ammo <= 0 {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM farm_zombies WHERE user_id = ? AND idx IN (
		SELECT idx FROM farm_zombies WHERE user_id = ? AND wave * ? + ? <= ? ORDER BY wave, idx LIMIT ?)`,
		uid, uid, zombieWaveSeconds, GunDelay, now, ammo)
	if err != nil {
		return err
	}
	shot, err := res.RowsAffected()
	if err != nil || shot == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_guns SET ammo = ammo - ?, killed = killed + ? WHERE user_id = ?`, shot, shot, uid)
	return err
}

// Gun là súng đậu của một nông trại.
type Gun struct {
	Owned bool `json:"owned"`
	Ammo  int  `json:"ammo"`
}

// loadGun đọc súng và số zombie súng đã diệt chưa báo.
func loadGun(ctx context.Context, q querier, uid int64) (g Gun, killed int, err error) {
	err = q.QueryRowContext(ctx, `SELECT ammo, killed FROM farm_guns WHERE user_id = ?`, uid).Scan(&g.Ammo, &killed)
	if errors.Is(err, sql.ErrNoRows) {
		return Gun{}, 0, nil
	}
	g.Owned = err == nil
	return g, killed, err
}

// BuyGun mua súng đậu; mỗi nông trại một khẩu.
func (f *Farm) BuyGun(ctx context.Context, uid int64) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if s.Gun.Owned {
			return conflict("bạn đã có súng đậu")
		}
		if err := tx.Charge(uid, GunPrice, GameID, "Mua súng đậu"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_guns (user_id) VALUES (?)`, uid)
		return err
	})
}

// BuyAmmo mua qty viên đạn đậu cho súng đã có.
func (f *Farm) BuyAmmo(ctx context.Context, uid int64, qty int) (State, error) {
	if qty < 1 || qty > maxAmmoSale {
		return State{}, bad(fmt.Sprintf("mỗi lần mua từ 1 tới %d viên đạn", maxAmmoSale))
	}
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if !s.Gun.Owned {
			return conflict("hãy mua súng đậu trước")
		}
		if err := tx.Charge(uid, int64(qty)*AmmoPrice, GameID, fmt.Sprintf("Mua %d đạn đậu", qty)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE farm_guns SET ammo = ammo + ? WHERE user_id = ?`, qty, uid)
		return err
	})
}

// expireZombies xử lý zombie đã hết hạn đập: con bites = 1 ăn mất cây (mất cả hạt), con còn lại bỏ đi.
func expireZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT idx, bites FROM farm_zombies WHERE user_id = ? AND wave * ? + ? <= ?`,
		uid, zombieWaveSeconds, ZombieWindow, now)
	if err != nil {
		return err
	}
	var picks []zombiePick
	for rows.Next() {
		var z zombiePick
		if err := rows.Scan(&z.idx, &z.bites); err != nil {
			rows.Close()
			return err
		}
		picks = append(picks, z)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	eaten := 0
	for _, z := range picks {
		if z.bites {
			eaten++
			if err := clearPlot(ctx, tx, uid, z.idx); err != nil { // clearPlot xoá luôn zombie của ô
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM farm_zombies WHERE user_id = ? AND idx = ?`, uid, z.idx); err != nil {
			return err
		}
	}
	if eaten == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_zombie_progress SET eaten = eaten + ? WHERE user_id = ?`, eaten, uid)
	return err
}

// loadZombieInfo đọc hạn đập của đợt đang có (0 nếu không có zombie) và số cây bị ăn chưa báo.
func loadZombieInfo(ctx context.Context, q querier, uid int64) (until int64, eaten int, err error) {
	var wave sql.NullInt64
	err = q.QueryRowContext(ctx, `SELECT (SELECT MAX(wave) FROM farm_zombies WHERE user_id = ?),
		COALESCE((SELECT eaten FROM farm_zombie_progress WHERE user_id = ?), 0)`, uid, uid).Scan(&wave, &eaten)
	if err != nil {
		return 0, 0, err
	}
	if wave.Valid {
		until = wave.Int64*zombieWaveSeconds + ZombieWindow
	}
	return until, eaten, nil
}

// UpgradePlot nâng một ô lên đất tốt; cây đang trồng giữ nguyên giờ chín, áp dụng từ lần gieo sau.
func (f *Farm) UpgradePlot(ctx context.Context, uid int64, plot int) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if plot < 0 || plot >= len(s.Plots) {
			return bad("ô đất không hợp lệ")
		}
		if s.Plots[plot].Good {
			return conflict("ô này đã là đất tốt")
		}
		if err := tx.Charge(uid, GoodSoilPrice, GameID, fmt.Sprintf("Nâng ô đất số %d lên đất tốt", plot+1)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE farm_plots SET good = 1 WHERE user_id = ? AND idx = ?`, uid, plot)
		return err
	})
}

// HitZombie đập zombie trên một ô: một nhát chết, trừ ZombieHitHP máu. Đang nằm viện thì không đập được.
func (f *Farm) HitZombie(ctx context.Context, uid int64, plot int) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if plot < 0 || plot >= len(s.Plots) {
			return bad("ô đất không hợp lệ")
		}
		if !s.Plots[plot].Zombie {
			return conflict("ô này không có zombie")
		}
		lost, err := tx.Damage(uid, ZombieHitHP)
		if err != nil {
			return err
		}
		if lost == 0 { // Damage không trừ khi đang nằm viện
			return conflict("đang nằm viện, không đập được zombie")
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM farm_zombies WHERE user_id = ? AND idx = ?`, uid, plot)
		return err
	})
}

// ZombieStatus trả số zombie đang có và hạn đập, cho bản đồ thành phố.
func (f *Farm) ZombieStatus(ctx context.Context, uid int64) (count int, until int64, err error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if err := f.settleZombies(ctx, tx, uid, f.now().Unix()); err != nil {
		return 0, 0, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM farm_zombies WHERE user_id = ?`, uid).Scan(&count); err != nil {
		return 0, 0, err
	}
	if until, _, err = loadZombieInfo(ctx, tx, uid); err != nil {
		return 0, 0, err
	}
	return count, until, tx.Commit()
}

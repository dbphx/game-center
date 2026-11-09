package farm

import (
	"cmp"
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
	GoodSoilPrice   = 30000
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
	AmmoPrice   = 1500
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
CREATE TABLE IF NOT EXISTS farm_stock_zombies (
	stock_id INTEGER PRIMARY KEY,      -- farm_stock.id
	user_id  INTEGER NOT NULL REFERENCES users(id),
	wave     INTEGER NOT NULL,
	bites    INTEGER NOT NULL          -- không gửi cho giao diện
);
CREATE INDEX IF NOT EXISTS farm_stock_zombies_user ON farm_stock_zombies(user_id);
CREATE TABLE IF NOT EXISTS farm_stock_zombie_progress (
	user_id   INTEGER PRIMARY KEY REFERENCES users(id),
	last_wave INTEGER NOT NULL,        -- đợt vật nuôi mới nhất đã tính
	eaten     INTEGER NOT NULL DEFAULT 0 -- số con bị ăn chưa báo cho chủ
);
`

// stockZones là các khu vật nuôi; mỗi khu có đợt riêng với seed riêng. Chó không nằm trong farm_stock nên không bị tấn công.
var stockZones = []struct {
	place string
	zone  uint64
}{{placeBarn, 1}, {placeYard, 2}, {placePond, 3}}

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

// zombieTarget là một mục tiêu đủ điều kiện: idx là ô đất hoặc id con vật, progress là tiến độ lớn 0..1.
type zombieTarget struct {
	idx      int
	progress float64
}

// pickTargets chọn mục tiêu cho một đợt: zombie lựa cây / con gần trưởng thành nhất, bằng tiến độ thì ngẫu nhiên.
// Xáo trước rồi sắp xếp ổn định; cùng (uid, seed) và cùng danh sách thì ra cùng kết quả. Số zombie và xác suất ăn
// không phụ thuộc mục tiêu nên tỉ lệ mất chung không đổi, rủi ro chỉ dồn vào cây / con gần lớn.
func pickTargets(uid int64, seed uint64, targets []zombieTarget) []zombiePick {
	count, chance := zombieCount(len(targets))
	if count == 0 {
		return nil
	}
	rng := mrand.New(mrand.NewPCG(uint64(uid), seed))
	list := slices.Clone(targets)
	rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
	slices.SortStableFunc(list, func(a, b zombieTarget) int { return cmp.Compare(b.progress, a.progress) })
	out := make([]zombiePick, count)
	for i := range out {
		out[i] = zombiePick{idx: list[i].idx, bites: rng.Float64() < chance}
	}
	return out
}

// growthOf là tiến độ done / total, chặn ở 1: đã chín hoặc đã lớn đều ở mức cao nhất, để lâu không an toàn hơn.
func growthOf(done, total int64) float64 {
	if total <= 0 || done >= total {
		return 1
	}
	return float64(max(0, done)) / float64(total)
}

// settleZombies tính bù zombie của uid tới thời điểm now: cho zombie quá hạn ăn cây hoặc bỏ đi, thêm các đợt mới.
// Lần đầu thấy một nông trại thì không tính ngược, đợt đầu tiên là đầu giờ kế tiếp.
func (f *Farm) settleZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	if f.noZombies {
		return nil
	}
	cur := now / zombieWaveSeconds
	// Vật nuôi có tiến độ riêng: người offline lâu từ trước khi có tính năng không bị tính bù đợt quá khứ.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO farm_stock_zombie_progress (user_id, last_wave) VALUES (?, ?)`, uid, cur); err != nil {
		return err
	}
	var last, stockLast int64
	if err := tx.QueryRowContext(ctx, `SELECT last_wave FROM farm_stock_zombie_progress WHERE user_id = ?`, uid).Scan(&stockLast); err != nil {
		return err
	}
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
	if last >= cur && stockLast >= cur {
		return nil
	}
	for w := max(min(last, stockLast)+1, cur-zombieMaxCatchup+1); w <= cur; w++ {
		start := w * zombieWaveSeconds
		spawned := false
		if w > last {
			targets, err := zombieTargets(ctx, tx, uid, start)
			if err != nil {
				return err
			}
			for _, z := range pickTargets(uid, uint64(w), targets) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO farm_zombies (user_id, idx, wave, bites) VALUES (?, ?, ?, ?)`,
					uid, z.idx, w, z.bites); err != nil {
					return err
				}
				spawned = true
			}
		}
		if w > stockLast {
			for _, zone := range stockZones {
				targets, err := stockTargets(ctx, tx, uid, zone.place, start)
				if err != nil {
					return err
				}
				for _, z := range pickTargets(uid, uint64(w)*4+zone.zone, targets) {
					if _, err := tx.ExecContext(ctx, `INSERT INTO farm_stock_zombies (stock_id, user_id, wave, bites) VALUES (?, ?, ?, ?)`,
						z.idx, uid, w, z.bites); err != nil {
						return err
					}
					spawned = true
				}
			}
		}
		// Tính bù theo thứ tự thời gian: đợt đã qua vẫn tốn đạn trước khi tới đợt sau.
		if spawned && now >= start+GunDelay {
			if err := shootThenExpire(ctx, tx, uid, now); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE farm_stock_zombie_progress SET last_wave = ? WHERE user_id = ?`, cur, uid); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_zombie_progress SET last_wave = ? WHERE user_id = ?`, cur, uid)
	return err
}

// zombieTargets liệt kê các ô có cây gieo trước lúc đợt bắt đầu và chưa có zombie, kèm tiến độ tại lúc đó.
func zombieTargets(ctx context.Context, tx *wallet.Tx, uid, start int64) ([]zombieTarget, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.idx, p.planted_at, p.ready_at FROM farm_plots p WHERE p.user_id = ? AND p.crop != '' AND p.planted_at < ?
		AND NOT EXISTS (SELECT 1 FROM farm_zombies z WHERE z.user_id = p.user_id AND z.idx = p.idx) ORDER BY p.idx`, uid, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []zombieTarget{}
	for rows.Next() {
		var idx int
		var planted, ready int64
		if err := rows.Scan(&idx, &planted, &ready); err != nil {
			return nil, err
		}
		out = append(out, zombieTarget{idx, growthOf(start-planted, ready-planted)})
	}
	return out, rows.Err()
}

// stockTargets liệt kê các con ở place mua trước lúc đợt bắt đầu và chưa có zombie, kèm tiến độ lớn đã lưu.
func stockTargets(ctx context.Context, tx *wallet.Tx, uid int64, place string, start int64) ([]zombieTarget, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.kind, s.grow_progress FROM farm_stock s WHERE s.user_id = ? AND s.place = ? AND s.bought_at < ?
		AND NOT EXISTS (SELECT 1 FROM farm_stock_zombies z WHERE z.stock_id = s.id) ORDER BY s.id`, uid, place, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []zombieTarget{}
	for rows.Next() {
		var id int
		var kind string
		var grow int64
		if err := rows.Scan(&id, &kind, &grow); err != nil {
			return nil, err
		}
		st, _ := findStock(place, kind)
		out = append(out, zombieTarget{id, growthOf(grow, st.GrowSeconds)})
	}
	return out, rows.Err()
}

// errStockZombie là lỗi khi đụng vào con đang bị tấn công: bán đi không phải là cách né zombie.
func errStockZombie() error { return conflict("Phải đập zombie trước") }

// deleteStock xoá một con vật cùng zombie của nó; mọi đường xoá con vật đều đi qua đây.
func deleteStock(ctx context.Context, tx *wallet.Tx, uid, id int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM farm_stock_zombies WHERE stock_id = ? AND user_id = ?`, id, uid); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM farm_stock WHERE id = ? AND user_id = ?`, id, uid)
	return err
}

// shootThenExpire cho súng bắn trước, rồi mới xử lý zombie hết hạn.
func shootThenExpire(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	if err := shootZombies(ctx, tx, uid, now); err != nil {
		return err
	}
	if err := expireZombies(ctx, tx, uid, now); err != nil {
		return err
	}
	return expireStockZombies(ctx, tx, uid, now)
}

// shootZombies dùng đạn diệt các zombie đã quá giây GunDelay: đợt cũ trước; trong một đợt, ruộng theo ô rồi vật nuôi
// theo id; hết đạn thì dừng. Súng bỏ qua zombie nhắm vào cây / con có giá bán rẻ hơn một viên đạn (không tốn đạn,
// zombie đó vẫn có thể ăn như khi không có súng).
func shootZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	var ammo int
	err := tx.QueryRowContext(ctx, `SELECT ammo FROM farm_guns WHERE user_id = ?`, uid).Scan(&ammo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil || ammo <= 0 {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT 0 AS stock, z.idx AS id, z.wave, '' AS place, COALESCE(p.crop, '') AS kind
			FROM farm_zombies z LEFT JOIN farm_plots p ON p.user_id = z.user_id AND p.idx = z.idx
			WHERE z.user_id = ? AND z.wave * ? + ? <= ?
		UNION ALL SELECT 1, z.stock_id, z.wave, COALESCE(st.place, ''), COALESCE(st.kind, '')
			FROM farm_stock_zombies z LEFT JOIN farm_stock st ON st.id = z.stock_id
			WHERE z.user_id = ? AND z.wave * ? + ? <= ?
		ORDER BY 3, 1, 2`,
		uid, zombieWaveSeconds, GunDelay, now, uid, zombieWaveSeconds, GunDelay, now)
	if err != nil {
		return err
	}
	type shot struct {
		stock bool
		id    int64
	}
	var shots []shot
	for rows.Next() && len(shots) < ammo {
		var z shot
		var wave int64
		var place, kind string
		if err := rows.Scan(&z.stock, &z.id, &wave, &place, &kind); err != nil {
			rows.Close()
			return err
		}
		if zombieTargetValue(z.stock, place, kind) < AmmoPrice {
			continue
		}
		shots = append(shots, z)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(shots) == 0 {
		return nil
	}
	for _, z := range shots {
		query := `DELETE FROM farm_zombies WHERE user_id = ? AND idx = ?`
		if z.stock {
			query = `DELETE FROM farm_stock_zombies WHERE user_id = ? AND stock_id = ?`
		}
		if _, err := tx.ExecContext(ctx, query, uid, z.id); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_guns SET ammo = ammo - ?, killed = killed + ? WHERE user_id = ?`, len(shots), len(shots), uid)
	return err
}

// zombieTargetValue là giá bán của cây / con mà zombie nhắm tới; 0 nếu ô đã trống hoặc con đã mất.
func zombieTargetValue(stock bool, place, kind string) int64 {
	if !stock {
		if c, ok := findCrop(kind); ok {
			return c.SellPrice
		}
		return 0
	}
	if st, ok := findStock(place, kind); ok {
		return st.SellPrice
	}
	return 0
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

// expireStockZombies xử lý zombie vật nuôi đã hết hạn đập: con bites = 1 ăn mất con vật, con còn lại bỏ đi.
func expireStockZombies(ctx context.Context, tx *wallet.Tx, uid, now int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT stock_id, bites FROM farm_stock_zombies WHERE user_id = ? AND wave * ? + ? <= ?`,
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
			if err := deleteStock(ctx, tx, uid, int64(z.idx)); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM farm_stock_zombies WHERE stock_id = ? AND user_id = ?`, z.idx, uid); err != nil {
			return err
		}
	}
	if eaten == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_stock_zombie_progress SET eaten = eaten + ? WHERE user_id = ?`, eaten, uid)
	return err
}

// loadZombieInfo đọc hạn đập của đợt đang có (0 nếu không có zombie), số cây và số con bị ăn chưa báo.
func loadZombieInfo(ctx context.Context, q querier, uid int64) (until int64, eaten, eatenStock int, err error) {
	var wave sql.NullInt64
	err = q.QueryRowContext(ctx, `SELECT (SELECT MAX(wave) FROM (
			SELECT wave FROM farm_zombies WHERE user_id = ? UNION ALL SELECT wave FROM farm_stock_zombies WHERE user_id = ?)),
		COALESCE((SELECT eaten FROM farm_zombie_progress WHERE user_id = ?), 0),
		COALESCE((SELECT eaten FROM farm_stock_zombie_progress WHERE user_id = ?), 0)`, uid, uid, uid, uid).Scan(&wave, &eaten, &eatenStock)
	if err != nil {
		return 0, 0, 0, err
	}
	if wave.Valid {
		until = wave.Int64*zombieWaveSeconds + ZombieWindow
	}
	return until, eaten, eatenStock, nil
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

// HitStockZombie đập zombie trên một con vật / cá của chủ: một nhát chết, trừ ZombieHitHP máu. Đang nằm viện thì không đập được.
func (f *Farm) HitStockZombie(ctx context.Context, uid, id int64) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		var target *Animal
		for _, pen := range []*Pen{s.Barn, s.Yard, s.Pond} {
			if pen == nil {
				continue
			}
			for _, list := range [][]Animal{pen.Animals, pen.Fish} {
				for i := range list {
					if list[i].ID == id {
						target = &list[i]
					}
				}
			}
		}
		if target == nil {
			return notFound("không tìm thấy con này")
		}
		if !target.Zombie {
			return conflict("con này không có zombie")
		}
		lost, err := tx.Damage(uid, ZombieHitHP)
		if err != nil {
			return err
		}
		if lost == 0 { // Damage không trừ khi đang nằm viện
			return conflict("đang nằm viện, không đập được zombie")
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM farm_stock_zombies WHERE stock_id = ? AND user_id = ?`, id, uid)
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
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM farm_zombies WHERE user_id = ?) +
		(SELECT count(*) FROM farm_stock_zombies WHERE user_id = ?)`, uid, uid).Scan(&count); err != nil {
		return 0, 0, err
	}
	if until, _, _, err = loadZombieInfo(ctx, tx, uid); err != nil {
		return 0, 0, err
	}
	return count, until, tx.Commit()
}

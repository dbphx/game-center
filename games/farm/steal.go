package farm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Trộm nông trại bạn bè ----------
//
// Bạn bè được vào nông trại của nhau trộm ô cây đã chín. Mỗi ô mỗi vụ chỉ bị trộm thành công một lần, và mỗi người
// chỉ được thử một ô một lần mỗi vụ (bị chó bắt cũng mất lượt thử ô đó). Mỗi ô chỉ ra 1 nông sản nên kẻ trộm lấy đúng
// nông sản đó (chủ thu hoạch ô bị trộm chỉ còn XP): đồ không tự sinh thêm, hai tài khoản trộm qua lại cũng không lời hơn.
//
// Chó đói thì trộm được ngay. Chó no (đang canh) thì lần trộm thành cuộc đuổi: Steal trả một Chase, giao diện cho chó chạy
// theo nhân vật; chạy ra tới cổng thì FinishSteal(escaped=true) lấy được đồ, bị chó chạm (escaped=false) hoặc quá hạn thì
// bị cắn mất ChaseBiteHP máu (hết máu thì nằm viện), không lấy được gì. Trong lúc đuổi, ô đó khoá với người khác.
// Kiểm tra "có phải bạn bè" do package friends làm trước khi gọi Visit / Steal / FinishSteal.

const (
	StealsPerDay = 5  // số lần thử trộm (cả lần bị bắt) mỗi ngày trên một nhà
	ChaseBiteHP  = 50 // máu bị trừ khi chó đuổi kịp
	// Thời gian tối thiểu từ lúc bắt đầu đuổi tới lúc báo thoát (chạy từ ô ra cổng): báo sớm hơn coi như bị bắt.
	ChaseMinMs = 1500
	ChaseTTLMs = 20_000 // quá hạn mà chưa báo kết quả thì coi như bị bắt; ô khoá với người khác tới lúc này
	// Tốc độ chó đuổi (đơn vị bản đồ nông trại / giây): nhanh hơn người chơi (230) khoảng 10%, giao diện cho chó sủa
	// một chút rồi mới chạy nên ở gần cổng vẫn thoát được.
	DogSpeed     = 255
	theftLogSize = 50
)

// vnZone là múi giờ Việt Nam để tính "hôm nay" cho lượt trộm.
var vnZone = time.FixedZone("ICT", 7*3600)

func gone(msg string) error { return &Error{http.StatusNotFound, msg} }

// Person là tên hiển thị của người trong nhật ký trộm.
type Person struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Theft là một dòng nhật ký: ai vào trộm gì, có bị chó bắt không (Chasing: đang bị đuổi, chưa có kết quả).
type Theft struct {
	At      int64  `json:"at"`
	Thief   Person `json:"thief"`
	Item    string `json:"item"`
	Qty     int    `json:"qty"`
	Caught  bool   `json:"caught"`
	HPLost  int    `json:"hpLost"`
	Chasing bool   `json:"chasing"`
}

func loadThefts(ctx context.Context, q querier, owner int64) ([]Theft, error) {
	rows, err := q.QueryContext(ctx, `SELECT t.at, t.thief_id, u.name, t.item, t.qty, t.caught, t.hp_lost, t.pending
		FROM farm_thefts t JOIN users u ON u.id = t.thief_id WHERE t.owner_id = ? ORDER BY t.id DESC LIMIT ?`, owner, theftLogSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Theft{}
	for rows.Next() {
		var t Theft
		if err := rows.Scan(&t.At, &t.Thief.ID, &t.Thief.Name, &t.Item, &t.Qty, &t.Caught, &t.HPLost, &t.Chasing); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// VisitPlot là một ô đất khi xem nông trại của bạn.
type VisitPlot struct {
	Index     int    `json:"index"`
	Crop      string `json:"crop"`
	PlantedAt int64  `json:"plantedAt"`
	ReadyAt   int64  `json:"readyAt"`
	Ripe      bool   `json:"ripe"`
	Stolen    bool   `json:"stolen"`
	Tried     bool   `json:"tried"`  // người xem đã thử trộm ô này trong vụ này (thành công hay bị bắt)
	Locked    bool   `json:"locked"` // người khác đang bị chó đuổi vì ô này
}

// Visit là nông trại của bạn bè (chỉ xem): ô đất, chó và số lượt trộm còn lại hôm nay của người xem.
type Visit struct {
	Now        int64       `json:"now"`
	Plots      []VisitPlot `json:"plots"`
	Dog        Dog         `json:"dog"`
	StealsLeft int         `json:"stealsLeft"`
}

// Summary là tóm tắt nông trại cho danh sách bạn bè.
type Summary struct {
	Ripe        int  `json:"ripe"` // số ô chín chưa bị trộm và người xem chưa thử
	DogGuarding bool `json:"dogGuarding"`
}

// Chase là một cuộc chó đuổi: giao diện vẽ chó chạy theo nhân vật rồi báo kết quả bằng FinishSteal(ID).
type Chase struct {
	ID        string `json:"id"`
	Plot      int    `json:"plot"`
	Item      string `json:"item"`
	Qty       int    `json:"qty"`
	StartedAt int64  `json:"startedAt"` // ms
	DogSpeed  int    `json:"dogSpeed"`
	MinMs     int64  `json:"minMs"`
	ExpiresAt int64  `json:"expiresAt"` // ms
}

// StealResult là kết quả một lần trộm: "stolen" (lấy được Qty Item vào kho), "chase" (chó đang đuổi, chờ FinishSteal),
// "caught" (bị chó cắn mất HPLost máu, không lấy được gì) hoặc "gone" (thoát nhưng chủ đã thu hoạch trong lúc đuổi).
type StealResult struct {
	Result string `json:"result"`
	Item   string `json:"item"`
	Qty    int    `json:"qty"`
	HPLost int    `json:"hpLost"`
	Chase  *Chase `json:"chase,omitempty"`
	Visit  Visit  `json:"-"`
}

func (f *Farm) dayStart(now int64) int64 {
	t := time.Unix(now, 0).In(vnZone)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, vnZone).Unix()
}

func (f *Farm) stealsLeft(ctx context.Context, q querier, thief, owner, now int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM farm_thefts WHERE thief_id = ? AND owner_id = ? AND at >= ?`,
		thief, owner, f.dayStart(now)).Scan(&n)
	return max(0, StealsPerDay-n), err
}

// plotMarks: các (ô, lúc gieo) của owner mà viewer đã thử trộm, và các ô người khác đang bị đuổi (khoá).
func plotMarks(ctx context.Context, q querier, owner, viewer, nowMs int64) (tried, locked map[[2]int64]bool, err error) {
	rows, err := q.QueryContext(ctx, `SELECT t.idx, t.planted_at, t.thief_id = ?, t.pending = 1 AND t.expires_ms > ? FROM farm_thefts t
		JOIN farm_plots p ON p.user_id = t.owner_id AND p.idx = t.idx AND p.planted_at = t.planted_at
		WHERE t.owner_id = ?`, viewer, nowMs, owner)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	tried, locked = map[[2]int64]bool{}, map[[2]int64]bool{}
	for rows.Next() {
		var k [2]int64
		var mine, chasing bool
		if err := rows.Scan(&k[0], &k[1], &mine, &chasing); err != nil {
			return nil, nil, err
		}
		if mine {
			tried[k] = true
		} else if chasing {
			locked[k] = true
		}
	}
	return tried, locked, rows.Err()
}

func (f *Farm) visit(ctx context.Context, q querier, owner, viewer int64, now time.Time) (Visit, error) {
	if err := ensurePlots(ctx, q, owner); err != nil {
		return Visit{}, err
	}
	plots, err := loadPlots(ctx, q, owner)
	if err != nil {
		return Visit{}, err
	}
	tried, locked, err := plotMarks(ctx, q, owner, viewer, now.UnixMilli())
	if err != nil {
		return Visit{}, err
	}
	sec := now.Unix()
	v := Visit{Now: sec, Plots: make([]VisitPlot, len(plots))}
	for i, p := range plots {
		k := [2]int64{int64(p.Index), p.PlantedAt}
		planted := p.Crop != ""
		v.Plots[i] = VisitPlot{Index: p.Index, Crop: p.Crop, PlantedAt: p.PlantedAt, ReadyAt: p.ReadyAt,
			Ripe: planted && p.ReadyAt <= sec, Stolen: p.Stolen, Tried: planted && tried[k], Locked: planted && locked[k]}
	}
	if v.Dog, err = loadDog(ctx, q, owner, sec); err != nil {
		return Visit{}, err
	}
	v.StealsLeft, err = f.stealsLeft(ctx, q, viewer, owner, sec)
	return v, err
}

// settleChases: cuộc đuổi quá hạn mà kẻ trộm không báo kết quả (đóng trang, mất mạng...) thì coi như bị chó cắn.
// Gọi đầu mỗi lần xem / thao tác nông trại và trộm, nên máu bị trừ chậm nhất ở lần kế tiếp có người mở nông trại.
func settleChases(ctx context.Context, tx *wallet.Tx, nowMs int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, thief_id FROM farm_thefts WHERE pending = 1 AND expires_ms <= ?`, nowMs)
	if err != nil {
		return err
	}
	type row struct{ id, thief int64 }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.thief); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range list {
		if _, err := bite(ctx, tx, r.id, r.thief); err != nil {
			return err
		}
	}
	return nil
}

// bite kết thúc cuộc đuổi id là bị bắt: trừ máu kẻ trộm (luật máu ở internal/wallet), ghi nhật ký.
func bite(ctx context.Context, tx *wallet.Tx, id, thief int64) (int, error) {
	lost, err := tx.Damage(thief, ChaseBiteHP)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE farm_thefts SET pending = 0, caught = 1, qty = 0, hp_lost = ? WHERE id = ?`, lost, id)
	return lost, err
}

// Visit trả nông trại của owner cho viewer xem.
func (f *Farm) Visit(ctx context.Context, owner, viewer int64) (Visit, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return Visit{}, err
	}
	defer tx.Rollback()
	now := f.now()
	if err := settleChases(ctx, tx, now.UnixMilli()); err != nil {
		return Visit{}, err
	}
	v, err := f.visit(ctx, tx, owner, viewer, now)
	if err != nil {
		return Visit{}, err
	}
	return v, tx.Commit()
}

// Summaries tóm tắt nông trại của nhiều người cho viewer (danh sách bạn bè): số ô chín chưa bị trộm mà viewer chưa thử,
// chó có canh không.
func (f *Farm) Summaries(ctx context.Context, viewer int64, ids []int64) (map[int64]Summary, error) {
	out := make(map[int64]Summary, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := f.now().Unix()
	for _, id := range ids {
		var s Summary
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM farm_plots p WHERE p.user_id = ? AND p.crop != '' AND p.ready_at <= ?
			AND NOT EXISTS (SELECT 1 FROM farm_stolen s WHERE s.user_id = p.user_id AND s.idx = p.idx AND s.planted_at = p.planted_at)
			AND NOT EXISTS (SELECT 1 FROM farm_thefts t WHERE t.owner_id = p.user_id AND t.idx = p.idx AND t.planted_at = p.planted_at
				AND t.thief_id = ?)`,
			id, now, viewer).Scan(&s.Ripe); err != nil {
			return nil, err
		}
		d, err := loadDog(ctx, tx, id, now)
		if err != nil {
			return nil, err
		}
		s.DogGuarding = d.Guarding
		out[id] = s
	}
	return out, nil
}

// take cho kẻ trộm lấy nông sản của ô (đã kiểm tra ô chín, chưa bị trộm): đánh dấu ô, cất vào kho kẻ trộm.
func take(ctx context.Context, tx *wallet.Tx, thief, owner int64, plot int, plantedAt int64, item string) error {
	r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO farm_stolen (user_id, idx, planted_at, thief_id) VALUES (?, ?, ?, ?)`,
		owner, plot, plantedAt, thief)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return conflict("Ô này đã bị trộm rồi")
	}
	return addItem(ctx, tx, thief, storeProducts, item, 1)
}

func newToken() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Steal: thief trộm ô plot đã chín của owner (đã kiểm tra là bạn bè). Chó đói thì lấy được ngay;
// chó đang canh thì bắt đầu cuộc đuổi (result "chase"), kết quả chốt ở FinishSteal.
func (f *Farm) Steal(ctx context.Context, thief, owner int64, plot int) (StealResult, error) {
	if thief == owner {
		return StealResult{}, bad("không thể trộm nhà mình")
	}
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return StealResult{}, err
	}
	defer tx.Rollback()
	now := f.now()
	nowMs := now.UnixMilli()
	if err := settleChases(ctx, tx, nowMs); err != nil {
		return StealResult{}, err
	}
	v, err := f.visit(ctx, tx, owner, thief, now)
	if err != nil {
		return StealResult{}, err
	}
	if plot < 0 || plot >= len(v.Plots) {
		return StealResult{}, bad("ô đất không hợp lệ")
	}
	p := v.Plots[plot]
	c, ok := findCrop(p.Crop)
	switch {
	case !ok:
		return StealResult{}, conflict("ô này chưa trồng gì")
	case !p.Ripe:
		return StealResult{}, conflict(c.Name + " chưa chín")
	case p.Stolen:
		return StealResult{}, conflict("Ô này đã bị trộm rồi")
	case p.Tried:
		return StealResult{}, conflict("Bạn đã thử trộm ô này rồi")
	case p.Locked:
		return StealResult{}, conflict("Có người đang trộm ô này, chó đang đuổi")
	case v.StealsLeft <= 0:
		return StealResult{}, conflict(fmt.Sprintf("hôm nay bạn đã vào nhà này %d lần, mai quay lại nhé", StealsPerDay))
	}
	products, err := loadStore(ctx, tx, thief, storeProducts)
	if err != nil {
		return StealResult{}, err
	}
	if products.free() < 1 {
		return StealResult{}, productsFull(products)
	}
	res := StealResult{Result: "stolen", Item: c.ID, Qty: 1}
	if v.Dog.Guarding {
		// Chó đang canh: chưa lấy đồ, giữ chỗ ô này tới khi có kết quả đuổi (đã tính một lượt thử).
		ch := &Chase{ID: newToken(), Plot: plot, Item: c.ID, Qty: 1, StartedAt: nowMs, DogSpeed: DogSpeed,
			MinMs: ChaseMinMs, ExpiresAt: nowMs + ChaseTTLMs}
		res = StealResult{Result: "chase", Item: c.ID, Qty: 1, Chase: ch}
		if _, err := tx.ExecContext(ctx, `INSERT INTO farm_thefts (owner_id, thief_id, idx, planted_at, item, qty, at, pending, token,
			started_ms, expires_ms) VALUES (?, ?, ?, ?, ?, 0, ?, 1, ?, ?, ?)`,
			owner, thief, plot, p.PlantedAt, c.ID, now.Unix(), ch.ID, ch.StartedAt, ch.ExpiresAt); err != nil {
			return StealResult{}, err
		}
	} else {
		if err := take(ctx, tx, thief, owner, plot, p.PlantedAt, c.ID); err != nil {
			return StealResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO farm_thefts (owner_id, thief_id, idx, planted_at, item, qty, at)
			VALUES (?, ?, ?, ?, ?, 1, ?)`, owner, thief, plot, p.PlantedAt, c.ID, now.Unix()); err != nil {
			return StealResult{}, err
		}
	}
	if res.Visit, err = f.visit(ctx, tx, owner, thief, now); err != nil {
		return StealResult{}, err
	}
	return res, tx.Commit()
}

// FinishSteal chốt cuộc đuổi token: escaped = chạy ra tới cổng (lấy được đồ), không thì bị chó cắn.
// Báo thoát sớm hơn ChaseMinMs kể từ lúc bắt đầu, hoặc khi đã quá hạn, cũng tính là bị cắn. Mỗi token dùng một lần.
func (f *Farm) FinishSteal(ctx context.Context, thief, owner int64, token string, escaped bool) (StealResult, error) {
	if token == "" {
		return StealResult{}, bad("thiếu mã cuộc đuổi")
	}
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return StealResult{}, err
	}
	defer tx.Rollback()
	now := f.now()
	nowMs := now.UnixMilli()
	var id, plantedAt, startedMs, expiresMs int64
	var plot int
	var item string
	var pending bool
	if err := tx.QueryRowContext(ctx, `SELECT id, idx, planted_at, item, pending, started_ms, expires_ms FROM farm_thefts
		WHERE token = ? AND thief_id = ? AND owner_id = ?`, token, thief, owner).
		Scan(&id, &plot, &plantedAt, &item, &pending, &startedMs, &expiresMs); err != nil {
		return StealResult{}, gone("không có cuộc đuổi này")
	}
	if !pending {
		return StealResult{}, conflict("lần trộm này đã kết thúc")
	}
	res := StealResult{Result: "caught", Item: item}
	if escaped && nowMs >= startedMs+ChaseMinMs && nowMs < expiresMs {
		// Ô vẫn là vụ cũ (chủ chưa thu hoạch trong lúc đuổi) thì mới lấy được.
		var cur int64
		if err := tx.QueryRowContext(ctx, `SELECT planted_at FROM farm_plots WHERE user_id = ? AND idx = ?`, owner, plot).Scan(&cur); err != nil {
			return StealResult{}, err
		}
		if cur == plantedAt {
			products, err := loadStore(ctx, tx, thief, storeProducts)
			if err != nil {
				return StealResult{}, err
			}
			if products.free() < 1 {
				return StealResult{}, productsFull(products) // còn hạn: bán bớt rồi báo lại
			}
			if err := take(ctx, tx, thief, owner, plot, plantedAt, item); err != nil {
				return StealResult{}, err
			}
			res = StealResult{Result: "stolen", Item: item, Qty: 1}
		} else {
			res = StealResult{Result: "gone", Item: item}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE farm_thefts SET pending = 0, qty = ? WHERE id = ?`, res.Qty, id); err != nil {
			return StealResult{}, err
		}
	} else if res.HPLost, err = bite(ctx, tx, id, thief); err != nil {
		return StealResult{}, err
	}
	if err := settleChases(ctx, tx, nowMs); err != nil {
		return StealResult{}, err
	}
	if res.Visit, err = f.visit(ctx, tx, owner, thief, now); err != nil {
		return StealResult{}, err
	}
	return res, tx.Commit()
}

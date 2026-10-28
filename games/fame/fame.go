// Package fame là Khu danh vọng của DCity: bảng xếp hạng người chơi theo tổng tài sản
// (xu trong ví + đồ thời trang đã mua + giá trị nông trại).
package fame

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "fame"

const (
	TopN     = 10               // số người trên bảng
	CacheTTL = 30 * time.Second // bảng tính lại sau khoảng này (không quét database mỗi lần xem)
)

// Entry là một dòng xếp hạng: tên hiển thị, ngoại hình và tài sản (xu). Không có tên đăng nhập.
type Entry struct {
	Rank   int    `json:"rank"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Hair   string `json:"hair"`
	Wings  string `json:"wings"`
	Face   string `json:"face"`
	Total  int64  `json:"total"`
	Coins  int64  `json:"coins"`
	Items  int64  `json:"items"`
	Farm   int64  `json:"farm"`
}

// Board là bảng gửi cho giao diện; Me chỉ có khi người xem đã đăng nhập.
type Board struct {
	UpdatedAt int64   `json:"updatedAt"` // lúc tính bảng (ms)
	Top       []Entry `json:"top"`
	Me        *Entry  `json:"me,omitempty"`
}

type Fame struct {
	w    *wallet.Wallet
	farm *farm.Farm
	now  func() time.Time

	mu      sync.Mutex
	ranking []Entry       // toàn bộ người chơi đã xếp hạng, lần tính gần nhất
	index   map[int64]int // id → vị trí trong ranking
	at      time.Time
}

func New(w *wallet.Wallet, f *farm.Farm) *Fame {
	return &Fame{w: w, farm: f, now: time.Now}
}

// compute xếp hạng mọi người chơi: tổng giảm dần, bằng nhau thì ai vào trước (id nhỏ) đứng trên.
func (f *Fame) compute(ctx context.Context) ([]Entry, error) {
	assets, err := f.w.Assets(ctx)
	if err != nil {
		return nil, err
	}
	farms, err := f.farm.Worth(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]Entry, len(assets))
	for i, a := range assets {
		list[i] = Entry{ID: a.ID, Name: a.Name, Level: a.Level, Gender: a.Gender, Outfit: a.Outfit, Hair: a.Hair, Wings: a.Wings,
			Face: a.Face, Coins: a.Coins, Items: a.Items, Farm: farms[a.ID]}
		list[i].Total = a.Coins + a.Items + list[i].Farm
	}
	slices.SortFunc(list, func(a, b Entry) int {
		if c := cmp.Compare(b.Total, a.Total); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	for i := range list {
		list[i].Rank = i + 1
	}
	return list, nil
}

// Board trả bảng xếp hạng (tính lại nếu đã cũ hơn CacheTTL); uid > 0 thì kèm hạng của người đó.
// Người vừa tạo tài khoản sau lần tính gần nhất thì tính lại ngay để có hạng.
func (f *Fame) Board(ctx context.Context, uid int64) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	_, known := f.index[uid]
	if f.ranking == nil || now.Sub(f.at) >= CacheTTL || (uid > 0 && !known) {
		list, err := f.compute(ctx)
		if err != nil {
			return Board{}, err
		}
		f.ranking, f.at = list, now
		f.index = make(map[int64]int, len(list))
		for i, e := range list {
			f.index[e.ID] = i
		}
	}
	b := Board{UpdatedAt: f.at.UnixMilli(), Top: slices.Clone(f.ranking[:min(TopN, len(f.ranking))])}
	if i, ok := f.index[uid]; ok && uid > 0 {
		me := f.ranking[i]
		b.Me = &me
	}
	return b, nil
}

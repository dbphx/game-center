package fishing

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := errStatus(err)
	msg := err.Error()
	switch status {
	case http.StatusInternalServerError:
		log.Printf("fishing: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// Gained là phần thưởng của một thao tác (bán cá: xu; câu được: XP).
type Gained struct {
	Coins int64 `json:"coins"`
	XP    int64 `json:"xp"`
	Count int   `json:"count,omitempty"`
}

// reply là trạng thái hồ câu kèm user và kết quả thao tác (nếu có).
type reply struct {
	State
	User    wallet.User `json:"user"`
	Outcome string      `json:"outcome,omitempty"` // POST reel: caught | miss | early | late
	Message string      `json:"message,omitempty"`
	Caught  *Catch      `json:"caught,omitempty"` // con vừa câu được (rác thì rarity "junk", không vào giỏ)
	Gained  *Gained     `json:"gained,omitempty"`
}

// Handler trả API hồ câu, gắn ở /api/fishing và /api/fishing/... (cần đăng nhập).
func (p *Pond) Handler() http.Handler {
	mux := http.NewServeMux()
	type body struct {
		Bait    string  `json:"bait"`
		Qty     int     `json:"qty"`
		CastID  int64   `json:"castId"`
		Success bool    `json:"success"`
		IDs     []int64 `json:"ids"`
		All     bool    `json:"all"`
	}
	handle := func(pattern string, op func(r *http.Request, uid int64, b body) (reply, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := p.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if err != nil {
				writeErr(w, err)
				return
			}
			var b body
			if r.Method == http.MethodPost {
				// Câu cá cần đủ máu và không nằm viện; bán cá trong giỏ thì lúc nào cũng được.
				if !strings.HasSuffix(r.URL.Path, "/sell") {
					if err := wallet.CheckZone(u, GameID); err != nil {
						writeErr(w, err)
						return
					}
				}
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b)
			}
			out, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			// Đọc user sau khi transaction đã đóng (database chỉ có một kết nối).
			if out.User, err = p.w.User(r.Context(), u.ID); err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		})
	}
	handle("GET /api/fishing", func(r *http.Request, uid int64, _ body) (reply, error) {
		st, err := p.State(r.Context(), uid)
		return reply{State: st}, err
	})
	handle("POST /api/fishing/bait/buy", func(r *http.Request, uid int64, b body) (reply, error) {
		st, err := p.BuyBait(r.Context(), uid, b.Bait, b.Qty)
		return reply{State: st}, err
	})
	handle("POST /api/fishing/cast", func(r *http.Request, uid int64, b body) (reply, error) {
		st, err := p.CastLine(r.Context(), uid, b.Bait)
		return reply{State: st}, err
	})
	handle("POST /api/fishing/reel", func(r *http.Request, uid int64, b body) (reply, error) {
		st, outcome, msg, got, xp, err := p.Reel(r.Context(), uid, b.CastID, b.Success)
		out := reply{State: st, Outcome: outcome, Message: msg, Caught: got}
		if got != nil {
			out.Gained = &Gained{XP: xp, Count: 1}
		}
		return out, err
	})
	handle("POST /api/fishing/sell", func(r *http.Request, uid int64, b body) (reply, error) {
		st, coins, n, err := p.Sell(r.Context(), uid, b.IDs, b.All)
		return reply{State: st, Gained: &Gained{Coins: coins, Count: n}}, err
	})
	return mux
}

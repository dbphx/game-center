package baucua

import (
	"encoding/json"
	"errors"
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
		log.Printf("baucua: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusPaymentRequired:
		msg = "không đủ xu"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// Handler trả API bầu cua, gắn ở /api/baucua/... Xem trạng thái / danh sách bàn không cần đăng nhập; các thao tác
// khác cần đăng nhập, riêng mở bàn / vào bàn / đặt cược / làm cái cần đủ máu vào Casino.
func (g *Game) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/baucua/state", func(w http.ResponseWriter, r *http.Request) {
		var uid int64
		if tok := bearer(r); tok != "" {
			// Token hỏng thì vẫn cho xem như khách.
			if u, err := g.w.UserByToken(r.Context(), tok); err == nil {
				uid = u.ID
			} else if !errors.Is(err, wallet.ErrNotFound) {
				writeErr(w, err)
				return
			}
		}
		s, err := g.State(r.Context(), r.URL.Query().Get("room"), uid)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s)
	})
	mux.HandleFunc("GET /api/baucua/rooms", func(w http.ResponseWriter, r *http.Request) {
		list, err := g.Rooms(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	type body struct {
		Room     string `json:"room"`
		Face     string `json:"face"`
		Amount   int64  `json:"amount"`
		Bankroll int64  `json:"bankroll"`
	}
	// post bọc xác thực, kiểm máu (nếu cần) và đọc body.
	post := func(pattern string, needHP bool, op func(r *http.Request, uid int64, b body) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := g.w.UserByToken(r.Context(), bearer(r))
			if err != nil {
				writeErr(w, err)
				return
			}
			if needHP {
				if err := wallet.CheckZone(u, "casino"); err != nil {
					writeErr(w, err)
					return
				}
			}
			var b body
			json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			out, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		})
	}
	post("POST /api/baucua/rooms", true, func(r *http.Request, uid int64, _ body) (any, error) {
		code, err := g.Create(r.Context(), uid)
		return map[string]string{"code": code}, err
	})
	post("POST /api/baucua/join", true, func(r *http.Request, uid int64, b body) (any, error) {
		return g.Join(r.Context(), b.Room, uid)
	})
	post("POST /api/baucua/leave", false, func(r *http.Request, uid int64, b body) (any, error) {
		return map[string]bool{"ok": true}, g.Leave(r.Context(), b.Room, uid)
	})
	post("POST /api/baucua/bet", true, func(r *http.Request, uid int64, b body) (any, error) {
		return g.Bet(r.Context(), b.Room, uid, b.Face, b.Amount)
	})
	post("POST /api/baucua/clear", false, func(r *http.Request, uid int64, b body) (any, error) {
		return g.Clear(r.Context(), b.Room, uid)
	})
	post("POST /api/baucua/bank", true, func(r *http.Request, uid int64, b body) (any, error) {
		return g.Bank(r.Context(), b.Room, uid, b.Bankroll)
	})
	post("POST /api/baucua/unbank", false, func(r *http.Request, uid int64, b body) (any, error) {
		return g.Unbank(r.Context(), b.Room, uid)
	})
	return mux
}

package friends

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errStatus(err error) int {
	var fe *Error
	var ge *farm.Error
	switch {
	case errors.As(err, &fe):
		return fe.Status
	case errors.As(err, &ge):
		return ge.Status
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		return http.StatusPaymentRequired
	case errors.Is(err, wallet.ErrInHospital):
		return http.StatusLocked
	case errors.Is(err, wallet.ErrLowHP):
		return http.StatusForbidden
	case errors.Is(err, wallet.ErrNotFound):
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, err error) {
	status := errStatus(err)
	msg := err.Error()
	switch status {
	case http.StatusInternalServerError:
		log.Printf("friends: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// withUser gửi kèm người chơi (máu sau khi bị chó cắn, kho...) để giao diện cập nhật thanh máu.
func (f *Friends) withUser(r *http.Request, uid int64, res StealReply, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	me, err := f.w.User(r.Context(), uid)
	if err != nil {
		return nil, err
	}
	return struct {
		StealReply
		User wallet.User `json:"user"`
	}{res, me}, nil
}

// Handler trả API bạn bè, gắn ở /api/friends và /api/friends/... (cần đăng nhập).
func (f *Friends) Handler() http.Handler {
	mux := http.NewServeMux()
	type body struct {
		Username string `json:"username"`
		ID       int64  `json:"id"`
		Plot     *int   `json:"plot"`
		Chase    string `json:"chase"`
		Escaped  bool   `json:"escaped"`
	}
	// handle bọc xác thực và đọc body; op trả dữ liệu gửi về.
	handle := func(pattern string, op func(r *http.Request, u wallet.User, b body) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := f.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if err != nil {
				writeErr(w, err)
				return
			}
			var b body
			if r.Method == http.MethodPost {
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			}
			out, err := op(r, u, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		})
	}
	other := func(r *http.Request) (int64, error) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			return 0, bad("mã người chơi không hợp lệ")
		}
		return id, nil
	}

	handle("GET /api/friends", func(r *http.Request, u wallet.User, _ body) (any, error) {
		return f.List(r.Context(), u.ID)
	})
	handle("POST /api/friends/request", func(r *http.Request, u wallet.User, b body) (any, error) {
		return f.Request(r.Context(), u.ID, b.Username, b.ID)
	})
	handle("POST /api/friends/accept", func(r *http.Request, u wallet.User, b body) (any, error) {
		return f.Accept(r.Context(), u.ID, b.ID)
	})
	handle("POST /api/friends/decline", func(r *http.Request, u wallet.User, b body) (any, error) {
		return f.Decline(r.Context(), u.ID, b.ID)
	})
	handle("POST /api/friends/remove", func(r *http.Request, u wallet.User, b body) (any, error) {
		return f.Remove(r.Context(), u.ID, b.ID)
	})
	handle("GET /api/friends/{id}/farm", func(r *http.Request, u wallet.User, _ body) (any, error) {
		id, err := other(r)
		if err != nil {
			return nil, err
		}
		return f.Farm(r.Context(), u.ID, id)
	})
	handle("POST /api/friends/{id}/steal", func(r *http.Request, u wallet.User, b body) (any, error) {
		id, err := other(r)
		if err != nil {
			return nil, err
		}
		// Vào trộm cũng là làm nông: cần đủ máu như vào nông trại, không nằm viện.
		if err := wallet.CheckZone(u, farm.GameID); err != nil {
			return nil, err
		}
		plot := -1
		if b.Plot != nil {
			plot = *b.Plot
		}
		res, err := f.Steal(r.Context(), u.ID, id, plot)
		return f.withUser(r, u.ID, res, err)
	})
	// Kết quả cuộc chó đuổi: không kiểm tra máu (đang chạy mà bị cắn hết máu vẫn phải báo được).
	handle("POST /api/friends/{id}/steal/finish", func(r *http.Request, u wallet.User, b body) (any, error) {
		id, err := other(r)
		if err != nil {
			return nil, err
		}
		res, err := f.FinishSteal(r.Context(), u.ID, id, b.Chase, b.Escaped)
		return f.withUser(r, u.ID, res, err)
	})
	return mux
}

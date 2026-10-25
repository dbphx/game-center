package site

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
		log.Printf("site: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	case http.StatusUnauthorized:
		msg = "phiên đăng nhập không hợp lệ"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// reply là trạng thái công trường kèm user (số dư, HP, level) và kết quả ca (nếu có).
type reply struct {
	State
	User  wallet.User `json:"user"`
	Shift *Shift      `json:"shift,omitempty"` // ca vừa nhận (POST start)
	Pay   *int64      `json:"pay,omitempty"`   // POST finish
	XP    *int64      `json:"xp,omitempty"`
	Score *int        `json:"score,omitempty"`
}

// Handler trả API công trường, gắn ở /api/site và /api/site/... (cần đăng nhập).
func (s *Site) Handler() http.Handler {
	mux := http.NewServeMux()
	type body struct {
		Job     string `json:"job"`
		ShiftID int64  `json:"shiftId"`
		Score   int    `json:"score"`
	}
	handle := func(pattern string, op func(r *http.Request, uid int64, b body) (reply, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := s.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if err != nil {
				writeErr(w, err)
				return
			}
			var b body
			if r.Method == http.MethodPost {
				// Nộp ca luôn được (tiền công của ca đã làm); nhận ca mới cần đủ máu và không nằm viện.
				if strings.HasSuffix(r.URL.Path, "/start") {
					if err := wallet.CheckZone(u, GameID); err != nil {
						writeErr(w, err)
						return
					}
				}
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b)
			}
			out, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			// Đọc user sau khi transaction đã đóng (database chỉ có một kết nối).
			if out.User, err = s.w.User(r.Context(), u.ID); err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, out)
		})
	}
	handle("GET /api/site", func(r *http.Request, uid int64, _ body) (reply, error) {
		st, err := s.State(r.Context(), uid)
		return reply{State: st}, err
	})
	handle("POST /api/site/start", func(r *http.Request, uid int64, b body) (reply, error) {
		st, err := s.Start(r.Context(), uid, b.Job)
		return reply{State: st, Shift: st.Active}, err
	})
	handle("POST /api/site/finish", func(r *http.Request, uid int64, b body) (reply, error) {
		if b.ShiftID == 0 {
			return reply{}, bad("thiếu shiftId")
		}
		st, res, err := s.Finish(r.Context(), uid, b.ShiftID, b.Score)
		return reply{State: st, Pay: &res.Pay, XP: &res.XP, Score: &res.Score}, err
	})
	return mux
}

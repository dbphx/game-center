package fame

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Handler trả API Khu danh vọng, gắn ở /api/fame. Ai cũng xem được; gửi kèm token thì có thêm hạng của mình
// (token sai / hết hạn thì bỏ qua, không báo lỗi).
func (f *Fame) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/fame", func(w http.ResponseWriter, r *http.Request) {
		var uid int64
		if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); token != "" {
			if u, err := f.w.UserByToken(r.Context(), token); err == nil {
				uid = u.ID
			}
		}
		b, err := f.Board(r.Context(), uid)
		if err != nil {
			log.Printf("fame: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lỗi máy chủ, thử lại sau"})
			return
		}
		writeJSON(w, http.StatusOK, b)
	})
	return mux
}

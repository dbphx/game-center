package server

import (
	"errors"
	"net"
	"net/http"

	"github.com/dbphx/game-center/internal/wallet"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Gender   string `json:"gender"`
	Outfit   string `json:"outfit"`
}

// authError đổi lỗi đăng nhập / đăng ký thành mã HTTP; lỗi khác là lỗi máy chủ.
func (s *Server) authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wallet.ErrBadCredentials), errors.Is(err, wallet.ErrWrongPassword):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, wallet.ErrNotOwned), errors.Is(err, wallet.ErrLevelTooLow), errors.Is(err, wallet.ErrLowHP):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, wallet.ErrInHospital):
		writeError(w, http.StatusLocked, err.Error())
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		writeError(w, http.StatusPaymentRequired, err.Error())
	case errors.Is(err, wallet.ErrUsernameTaken), errors.Is(err, wallet.ErrHasCredentials), errors.Is(err, wallet.ErrAlreadyOwned),
		errors.Is(err, wallet.ErrFullHP):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, wallet.ErrInvalidUsername), errors.Is(err, wallet.ErrWeakPassword), errors.Is(err, wallet.ErrLongPassword),
		errors.Is(err, wallet.ErrInvalidCharacter), errors.Is(err, wallet.ErrBadSource):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.internal(w, err)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	readJSON(w, r, &c)
	ip := clientIP(r)
	if s.registers.blocked(ip) {
		writeError(w, http.StatusTooManyRequests, "tạo quá nhiều tài khoản, thử lại sau")
		return
	}
	u, token, err := s.wallet.Register(r.Context(), c.Username, c.Password,
		wallet.Profile{Name: c.Name, Gender: c.Gender, Outfit: c.Outfit})
	if err != nil {
		s.authError(w, err)
		return
	}
	s.registers.add(ip)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	readJSON(w, r, &c)
	key := wallet.NormalizeUsername(c.Username)
	if s.loginFails.blocked(key) {
		writeError(w, http.StatusTooManyRequests, "sai mật khẩu quá nhiều lần, thử lại sau 15 phút")
		return
	}
	u, token, err := s.wallet.Login(r.Context(), c.Username, c.Password)
	if errors.Is(err, wallet.ErrBadCredentials) {
		s.loginFails.add(key)
	}
	if err != nil {
		s.authError(w, err)
		return
	}
	s.loginFails.reset(key)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ wallet.User) {
	if err := s.wallet.Logout(r.Context(), bearer(r)); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// setCredentials cho tài khoản khách cũ đặt tên đăng nhập + mật khẩu.
func (s *Server) setCredentials(w http.ResponseWriter, r *http.Request, u wallet.User) {
	var c credentials
	readJSON(w, r, &c)
	u, err := s.wallet.SetCredentials(r.Context(), u.ID, c.Username, c.Password)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u wallet.User) {
	var body struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	readJSON(w, r, &body)
	if err := s.wallet.ChangePassword(r.Context(), u.ID, body.Old, body.New); err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// wearOutfit mặc một trang phục đã có. Giới tính không đổi được nên trường gender (nếu gửi) bị bỏ qua.
func (s *Server) wearOutfit(w http.ResponseWriter, r *http.Request, u wallet.User) {
	if err := wallet.CheckZone(u, "shop"); err != nil {
		s.authError(w, err)
		return
	}
	var c credentials
	readJSON(w, r, &c)
	u, err := s.wallet.WearOutfit(r.Context(), u.ID, c.Outfit)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// buyOutfit mua trang phục ở shop thời trang (đủ level, trừ xu).
func (s *Server) buyOutfit(w http.ResponseWriter, r *http.Request, u wallet.User) {
	if err := wallet.CheckZone(u, "shop"); err != nil {
		s.authError(w, err)
		return
	}
	var c credentials
	readJSON(w, r, &c)
	u, err := s.wallet.BuyOutfit(r.Context(), u.ID, c.Outfit)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// hit trừ máu khi nhân vật bị xe đụng trên bản đồ.
func (s *Server) hit(w http.ResponseWriter, r *http.Request, u wallet.User) {
	var body struct {
		Source string `json:"source"`
	}
	readJSON(w, r, &body)
	u, err := s.wallet.Hit(r.Context(), u.ID, body.Source)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// heal chữa ở bệnh viện: xuất viện ngay hoặc chữa phần HP thiếu.
func (s *Server) heal(w http.ResponseWriter, r *http.Request, u wallet.User) {
	u, err := s.wallet.Heal(r.Context(), u.ID)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

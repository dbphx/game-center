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
	case errors.Is(err, wallet.ErrUsernameTaken), errors.Is(err, wallet.ErrHasCredentials):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, wallet.ErrInvalidUsername), errors.Is(err, wallet.ErrWeakPassword), errors.Is(err, wallet.ErrLongPassword),
		errors.Is(err, wallet.ErrInvalidCharacter):
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

// setCharacter đổi giới tính / trang phục nhân vật (shop thời trang).
func (s *Server) setCharacter(w http.ResponseWriter, r *http.Request, u wallet.User) {
	var c credentials
	readJSON(w, r, &c)
	u, err := s.wallet.SetCharacter(r.Context(), u.ID, c.Gender, c.Outfit)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

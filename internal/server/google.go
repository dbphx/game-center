package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dbphx/game-center/internal/googleauth"
	"github.com/dbphx/game-center/internal/wallet"
)

// Đăng nhập bằng Google (nút Google Identity Services trên trình duyệt gửi ID token — "credential"):
//   - POST /api/login/google: tài khoản Google đã liên kết → đăng nhập; chưa liên kết → trả vé đăng ký (signup)
//     để giao diện cho chọn tên / nhân vật rồi gọi POST /api/register/google, chưa tạo tài khoản ngay;
//   - POST /api/me/google: liên kết Google vào tài khoản đang đăng nhập; DELETE /api/me/google: bỏ liên kết.
// Không cấu hình Client ID thì tính năng tắt (503).

// GoogleVerifier xác minh ID token của Google (googleauth.Verifier; test dùng bản giả).
type GoogleVerifier interface {
	Verify(ctx context.Context, token string) (googleauth.Claims, error)
	ClientID() string
}

// SignupTTL là thời gian sống của vé đăng ký bằng Google.
const SignupTTL = 10 * time.Minute

// EnableGoogle bật đăng nhập Google với v (gọi trước khi phục vụ request).
func (s *Server) EnableGoogle(v GoogleVerifier) {
	s.google = v
	s.signupKey = make([]byte, 32)
	rand.Read(s.signupKey)
}

func (s *Server) googleClientID() string {
	if s.google == nil {
		return ""
	}
	return s.google.ClientID()
}

// signupTicket là nội dung vé đăng ký: ký HMAC bằng khoá ngẫu nhiên của tiến trình (khởi động lại thì vé cũ hết hiệu lực).
type signupTicket struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

func (s *Server) signTicket(t signupTicket) string {
	b, _ := json.Marshal(t)
	body := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, s.signupKey)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var errBadTicket = errors.New("phiên đăng ký bằng Google đã hết hạn, bấm đăng nhập Google lại")

func (s *Server) readTicket(v string) (signupTicket, error) {
	body, sig, ok := strings.Cut(v, ".")
	if !ok {
		return signupTicket{}, errBadTicket
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	mac := hmac.New(sha256.New, s.signupKey)
	mac.Write([]byte(body))
	if err != nil || !hmac.Equal(got, mac.Sum(nil)) {
		return signupTicket{}, errBadTicket
	}
	b, err := base64.RawURLEncoding.DecodeString(body)
	var t signupTicket
	if err != nil || json.Unmarshal(b, &t) != nil || t.Sub == "" || time.Now().Unix() > t.Exp {
		return signupTicket{}, errBadTicket
	}
	return t, nil
}

// verifyGoogle kiểm credential; trả false (đã trả lời lỗi) khi tắt tính năng, bị giới hạn hoặc token sai.
func (s *Server) verifyGoogle(w http.ResponseWriter, r *http.Request, credential string) (googleauth.Claims, bool) {
	if s.google == nil {
		writeError(w, http.StatusServiceUnavailable, "chưa bật đăng nhập Google")
		return googleauth.Claims{}, false
	}
	ip := clientIP(r)
	if s.googleTries.blocked(ip) {
		writeError(w, http.StatusTooManyRequests, "thử đăng nhập Google quá nhiều lần, thử lại sau")
		return googleauth.Claims{}, false
	}
	s.googleTries.add(ip)
	c, err := s.google.Verify(r.Context(), credential)
	if errors.Is(err, googleauth.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, err.Error())
		return googleauth.Claims{}, false
	}
	if err != nil {
		s.internal(w, err)
		return googleauth.Claims{}, false
	}
	return c, true
}

type googleBody struct {
	Credential string `json:"credential"`
	Signup     string `json:"signup"`
	Name       string `json:"name"`
	Gender     string `json:"gender"`
	Outfit     string `json:"outfit"`
}

func (s *Server) loginGoogle(w http.ResponseWriter, r *http.Request) {
	var b googleBody
	readJSON(w, r, &b)
	c, ok := s.verifyGoogle(w, r, b.Credential)
	if !ok {
		return
	}
	u, token, err := s.wallet.LoginGoogle(r.Context(), c.Sub, c.Email)
	if errors.Is(err, wallet.ErrGoogleNotLinked) {
		writeJSON(w, http.StatusOK, map[string]any{"needSignup": true, "email": c.Email, "name": wallet.CleanName(c.Name),
			"signup": s.signTicket(signupTicket{Sub: c.Sub, Email: c.Email, Exp: time.Now().Add(SignupTTL).Unix()})})
		return
	}
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *Server) registerGoogle(w http.ResponseWriter, r *http.Request) {
	if s.google == nil {
		writeError(w, http.StatusServiceUnavailable, "chưa bật đăng nhập Google")
		return
	}
	var b googleBody
	readJSON(w, r, &b)
	t, err := s.readTicket(b.Signup)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	ip := clientIP(r)
	if s.registers.blocked(ip) {
		writeError(w, http.StatusTooManyRequests, "tạo quá nhiều tài khoản, thử lại sau")
		return
	}
	u, token, err := s.wallet.RegisterGoogle(r.Context(), t.Sub, t.Email, wallet.Profile{Name: b.Name, Gender: b.Gender, Outfit: b.Outfit})
	if err != nil {
		s.authError(w, err)
		return
	}
	s.registers.add(ip)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": u})
}

func (s *Server) linkGoogle(w http.ResponseWriter, r *http.Request, u wallet.User) {
	var b googleBody
	readJSON(w, r, &b)
	c, ok := s.verifyGoogle(w, r, b.Credential)
	if !ok {
		return
	}
	u, err := s.wallet.LinkGoogle(r.Context(), u.ID, c.Sub, c.Email)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) unlinkGoogle(w http.ResponseWriter, r *http.Request, u wallet.User) {
	u, err := s.wallet.UnlinkGoogle(r.Context(), u.ID)
	if err != nil {
		s.authError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

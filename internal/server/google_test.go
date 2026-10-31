package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dbphx/game-center/internal/googleauth"
	"github.com/dbphx/game-center/internal/wallet"
)

// fakeVerifier nhận credential dạng "ok:<sub>:<email>"; còn lại là token sai.
type fakeVerifier struct{}

func (fakeVerifier) ClientID() string { return "test.apps.googleusercontent.com" }
func (fakeVerifier) Verify(_ context.Context, token string) (googleauth.Claims, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "ok" {
		return googleauth.Claims{}, fmt.Errorf("%w: giả", googleauth.ErrInvalidToken)
	}
	return googleauth.Claims{Sub: parts[1], Email: parts[2], Name: "Google " + parts[1]}, nil
}

func newGoogleServer(t *testing.T, enable bool) (*httptest.Server, *Server) {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	s := New(w, fstest.MapFS{"index.html": {Data: []byte("hub")}}, nil)
	if enable {
		s.EnableGoogle(fakeVerifier{})
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv, s
}

type googleReply struct {
	Token      string      `json:"token"`
	User       wallet.User `json:"user"`
	NeedSignup bool        `json:"needSignup"`
	Email      string      `json:"email"`
	Name       string      `json:"name"`
	Signup     string      `json:"signup"`
	Error      string      `json:"error"`
}

func TestGoogleDisabled(t *testing.T) {
	srv, _ := newGoogleServer(t, false)
	var cfg map[string]any
	call(t, srv, "GET", "/api/config", "", nil, &cfg)
	if cfg["googleClientId"] != "" {
		t.Fatalf("tắt thì googleClientId rỗng: %v", cfg["googleClientId"])
	}
	if code := call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "ok:1:a@b.c"}, nil); code != 503 {
		t.Fatalf("tắt tính năng phải 503, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": "x"}, nil); code != 503 {
		t.Fatalf("đăng ký khi tắt phải 503, được %d", code)
	}
}

func TestGoogleSignupLoginLink(t *testing.T) {
	srv, s := newGoogleServer(t, true)
	var cfg map[string]any
	call(t, srv, "GET", "/api/config", "", nil, &cfg)
	if cfg["googleClientId"] != "test.apps.googleusercontent.com" {
		t.Fatalf("config: %v", cfg["googleClientId"])
	}
	if code := call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "rac"}, nil); code != 401 {
		t.Fatalf("token sai phải 401, được %d", code)
	}

	// Chưa liên kết: trả vé đăng ký, chưa tạo tài khoản.
	var g googleReply
	if code := call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "ok:sub-an:an@gmail.com"}, &g); code != 200 ||
		!g.NeedSignup || g.Signup == "" || g.Email != "an@gmail.com" || g.Token != "" {
		t.Fatalf("chưa liên kết: %d %+v", code, g)
	}
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": g.Signup + "x", "name": "An"}, nil); code != 401 {
		t.Fatalf("vé sửa chữ ký phải 401, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": g.Signup, "outfit": "khong-co"}, nil); code != 400 {
		t.Fatalf("nhân vật sai phải 400, được %d", code)
	}
	var reg googleReply
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": g.Signup, "name": "An Google", "gender": "female"}, &reg); code != 200 ||
		reg.Token == "" || reg.User.Name != "An Google" || reg.User.Google == nil || reg.User.Google.Email != "an@gmail.com" || reg.User.Balance != wallet.StartBalance {
		t.Fatalf("đăng ký Google: %d %+v", code, reg)
	}
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": g.Signup}, nil); code != 409 {
		t.Fatalf("dùng lại vé (sub đã liên kết) phải 409, được %d", code)
	}
	// Vé hết hạn.
	old := s.signTicket(signupTicket{Sub: "sub-cu", Email: "cu@gmail.com", Exp: time.Now().Add(-time.Second).Unix()})
	if code := call(t, srv, "POST", "/api/register/google", "", map[string]string{"signup": old}, nil); code != 401 {
		t.Fatalf("vé hết hạn phải 401, được %d", code)
	}

	// Đã liên kết: đăng nhập thẳng, phiên cũ bị đá ra.
	var in googleReply
	if code := call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "ok:sub-an:an@gmail.com"}, &in); code != 200 ||
		in.Token == "" || in.User.ID != reg.User.ID || in.NeedSignup {
		t.Fatalf("đăng nhập Google: %d %+v", code, in)
	}
	if code := call(t, srv, "GET", "/api/me", reg.Token, nil, nil); code != 401 {
		t.Fatalf("phiên cũ phải bị đá ra, được %d", code)
	}
	var me wallet.User
	call(t, srv, "GET", "/api/me", in.Token, nil, &me)
	if me.Google == nil || me.Google.Email != "an@gmail.com" || me.Username != "dcityan" || me.HasPassword {
		t.Fatalf("/api/me: %+v", me)
	}
	// Tài khoản Google chưa có mật khẩu: đổi mật khẩu / bỏ liên kết báo 409.
	if code := call(t, srv, "POST", "/api/password", in.Token, map[string]string{"old": "", "new": "matkhau456"}, nil); code != 409 {
		t.Fatalf("đổi mật khẩu khi chưa có phải 409, được %d", code)
	}
	if code := call(t, srv, "DELETE", "/api/me/google", in.Token, nil, nil); code != 409 {
		t.Fatalf("bỏ liên kết khi chưa có mật khẩu phải 409, được %d", code)
	}

	// Tài khoản cũ (tên đăng nhập + mật khẩu) liên kết Google rồi đăng nhập bằng Google vào đúng tài khoản đó.
	b := newAccount(t, srv, "binh")
	if code := call(t, srv, "POST", "/api/me/google", b.Token, map[string]string{"credential": "ok:sub-an:an@gmail.com"}, nil); code != 409 {
		t.Fatalf("Google đã gắn tài khoản khác phải 409, được %d", code)
	}
	var linked struct {
		User wallet.User `json:"user"`
	}
	if code := call(t, srv, "POST", "/api/me/google", b.Token, map[string]string{"credential": "ok:sub-binh:binh@gmail.com"}, &linked); code != 200 ||
		linked.User.Google == nil || linked.User.ID != b.User.ID {
		t.Fatalf("liên kết: %d %+v", code, linked)
	}
	if code := call(t, srv, "POST", "/api/me/google", b.Token, map[string]string{"credential": "ok:sub-khac:khac@gmail.com"}, nil); code != 409 {
		t.Fatalf("đã có Google khác phải 409, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/me/google", "", map[string]string{"credential": "ok:sub-x:x@gmail.com"}, nil); code != 401 {
		t.Fatalf("liên kết khi chưa đăng nhập phải 401, được %d", code)
	}
	var bl googleReply
	if code := call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "ok:sub-binh:binh@gmail.com"}, &bl); code != 200 || bl.User.ID != b.User.ID {
		t.Fatalf("đăng nhập Google vào tài khoản cũ: %d %+v", code, bl)
	}
	var un struct {
		User wallet.User `json:"user"`
	}
	if code := call(t, srv, "DELETE", "/api/me/google", bl.Token, nil, &un); code != 200 || un.User.Google != nil {
		t.Fatalf("bỏ liên kết: %d %+v", code, un)
	}
}

func TestGoogleRateLimit(t *testing.T) {
	srv, _ := newGoogleServer(t, true)
	codes := map[int]int{}
	for range 35 {
		codes[call(t, srv, "POST", "/api/login/google", "", map[string]string{"credential": "rac"}, nil)]++
	}
	if codes[429] == 0 || codes[401] != 30 {
		t.Fatalf("giới hạn 30 lần / 15 phút theo IP: %v", codes)
	}
}

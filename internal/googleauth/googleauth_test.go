package googleauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const clientID = "dcity-test.apps.googleusercontent.com"

// fakeGoogle phục vụ JWKS với các khoá RSA tự sinh và ký token như Google.
type fakeGoogle struct {
	keys    map[string]*rsa.PrivateKey
	fetches atomic.Int32
	srv     *httptest.Server
}

func newFakeGoogle(t *testing.T, kids ...string) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range kids {
		g.addKey(t, kid)
	}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		g.fetches.Add(1)
		var set struct {
			Keys []map[string]string `json:"keys"`
		}
		for kid, k := range g.keys {
			set.Keys = append(set.Keys, map[string]string{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": kid,
				"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())})
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(set)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) addKey(t *testing.T, kid string) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g.keys[kid] = k
}

func sign(t *testing.T, k *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"})
	p, _ := json.Marshal(claims)
	head := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	sum := sha256.Sum256([]byte(head))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return head + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func goodClaims(now time.Time) map[string]any {
	return map[string]any{"iss": "https://accounts.google.com", "aud": clientID, "sub": "1234567890",
		"email": "An@Gmail.com", "email_verified": true, "name": "An Nguyễn", "picture": "https://x/y.png",
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
}

func TestVerify(t *testing.T) {
	g := newFakeGoogle(t, "k1")
	now := time.Unix(1_800_000_000, 0)
	clock := now
	v := NewForTest(clientID, g.srv.URL, g.srv.Client(), func() time.Time { return clock })
	ctx := context.Background()

	c, err := v.Verify(ctx, sign(t, g.keys["k1"], "k1", goodClaims(now)))
	if err != nil {
		t.Fatal(err)
	}
	if c.Sub != "1234567890" || c.Email != "an@gmail.com" || c.Name != "An Nguyễn" || c.Picture == "" {
		t.Fatalf("claims: %+v", c)
	}

	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	bad := func(name string, mut func(map[string]any), key *rsa.PrivateKey, kid string) {
		t.Helper()
		cl := goodClaims(now)
		if mut != nil {
			mut(cl)
		}
		if key == nil {
			key = g.keys["k1"]
		}
		if _, err := v.Verify(ctx, sign(t, key, kid, cl)); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: phải ErrInvalidToken, được %v", name, err)
		}
	}
	bad("sai chữ ký", nil, other, "k1")
	bad("sai aud", func(c map[string]any) { c["aud"] = "khac.apps.googleusercontent.com" }, nil, "k1")
	bad("sai iss", func(c map[string]any) { c["iss"] = "https://evil.example" }, nil, "k1")
	bad("hết hạn", func(c map[string]any) { c["exp"] = now.Add(-2 * time.Minute).Unix() }, nil, "k1")
	bad("email chưa xác minh", func(c map[string]any) { c["email_verified"] = false }, nil, "k1")
	bad("thiếu sub", func(c map[string]any) { delete(c, "sub") }, nil, "k1")
	if _, err := v.Verify(ctx, "abc.def"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("rác: %v", err)
	}

	// Lệch giờ dưới 60 giây vẫn nhận; email_verified dạng chuỗi "true" cũng nhận.
	cl := goodClaims(now)
	cl["exp"] = now.Add(-30 * time.Second).Unix()
	cl["email_verified"] = "true"
	if _, err := v.Verify(ctx, sign(t, g.keys["k1"], "k1", cl)); err != nil {
		t.Fatalf("lệch giờ nhỏ: %v", err)
	}
	if n := g.fetches.Load(); n != 1 {
		t.Fatalf("khoá còn hạn không được tải lại, đã tải %d lần", n)
	}

	// Google xoay khoá: kid lạ → tải lại ngay (đã quá 1 phút kể từ lần tải trước).
	g.addKey(t, "k2")
	clock = now.Add(2 * time.Minute)
	cl = goodClaims(clock)
	if _, err := v.Verify(ctx, sign(t, g.keys["k2"], "k2", cl)); err != nil {
		t.Fatalf("kid mới: %v", err)
	}
	if n := g.fetches.Load(); n != 2 {
		t.Fatalf("kid lạ phải tải lại khoá, đã tải %d lần", n)
	}
	// Kid không có thật: tải lại tối đa mỗi phút một lần (chống bị spam làm gọi Google liên tục).
	for range 5 {
		if _, err := v.Verify(ctx, sign(t, other, "k-gia", cl)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("kid giả: %v", err)
		}
	}
	if n := g.fetches.Load(); n != 2 {
		t.Fatalf("kid giả trong cùng phút không được tải lại, đã tải %d lần", n)
	}
	// Hết max-age thì tải lại.
	clock = now.Add(2 * time.Hour)
	if _, err := v.Verify(ctx, sign(t, g.keys["k1"], "k1", goodClaims(clock))); err != nil {
		t.Fatal(err)
	}
	if n := g.fetches.Load(); n != 3 {
		t.Fatalf("hết hạn khoá phải tải lại, đã tải %d lần", n)
	}
}

func TestMaxAge(t *testing.T) {
	for cc, want := range map[string]time.Duration{
		"public, max-age=19800, must-revalidate": 19800 * time.Second,
		"":                                       defaultKeyTTL,
		"no-cache":                               defaultKeyTTL,
	} {
		if got := maxAge(cc); got != want {
			t.Errorf("%q: %v, muốn %v", cc, got, want)
		}
	}
}

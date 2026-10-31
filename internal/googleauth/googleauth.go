// Package googleauth xác minh ID token (JWT) mà nút "Đăng nhập với Google" (Google Identity Services)
// trả về cho trình duyệt: chữ ký RS256 theo khoá công khai của Google (JWKS), aud = Client ID của DCity,
// iss của Google, còn hạn, email đã xác minh. Chỉ dùng thư viện chuẩn.
package googleauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CertsURL là JWKS khoá công khai của Google.
const CertsURL = "https://www.googleapis.com/oauth2/v3/certs"

const (
	defaultKeyTTL = time.Hour        // không có Cache-Control thì giữ khoá 1 giờ
	refetchGap    = time.Minute      // gặp kid lạ: tải lại khoá tối đa mỗi phút một lần
	clockSkew     = 60 * time.Second // cho phép lệch giờ khi kiểm exp / iat
)

// ErrInvalidToken là mọi lỗi của token (chữ ký, hạn, aud, iss, email chưa xác minh...).
var ErrInvalidToken = errors.New("token Google không hợp lệ hoặc đã hết hạn")

// Claims là thông tin lấy từ ID token.
type Claims struct {
	Sub     string // mã tài khoản Google, không đổi
	Email   string
	Name    string
	Picture string
}

// Verifier xác minh ID token cho một Client ID. An toàn khi dùng từ nhiều goroutine.
type Verifier struct {
	clientID string
	certsURL string
	client   *http.Client
	now      func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	expires   time.Time // lúc bộ khoá hết hạn theo Cache-Control
	lastFetch time.Time
}

// New tạo Verifier cho clientID, tải khoá từ CertsURL.
func New(clientID string) *Verifier {
	return &Verifier{clientID: clientID, certsURL: CertsURL, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
}

// NewForTest dùng nguồn khoá / đồng hồ riêng (test với khoá RSA tự sinh).
func NewForTest(clientID, certsURL string, client *http.Client, now func() time.Time) *Verifier {
	v := New(clientID)
	v.certsURL, v.client, v.now = certsURL, client, now
	return v
}

// ClientID trả Client ID đang dùng (giao diện cần để vẽ nút Google).
func (v *Verifier) ClientID() string { return v.clientID }

type header struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type payload struct {
	Iss           string          `json:"iss"`
	Aud           string          `json:"aud"`
	Sub           string          `json:"sub"`
	Email         string          `json:"email"`
	EmailVerified json.RawMessage `json:"email_verified"` // Google gửi bool, đôi khi chuỗi "true"
	Name          string          `json:"name"`
	Picture       string          `json:"picture"`
	Exp           int64           `json:"exp"`
	Iat           int64           `json:"iat"`
}

// Verify kiểm token và trả Claims; lỗi về token luôn bọc ErrInvalidToken.
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, fmt.Errorf("%w: sai định dạng", ErrInvalidToken)
	}
	var h header
	if err := decodePart(parts[0], &h); err != nil || h.Alg != "RS256" || h.Kid == "" {
		return Claims{}, fmt.Errorf("%w: header", ErrInvalidToken)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, fmt.Errorf("%w: chữ ký", ErrInvalidToken)
	}
	key, err := v.key(ctx, h.Kid)
	if err != nil {
		return Claims{}, err
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
		return Claims{}, fmt.Errorf("%w: sai chữ ký", ErrInvalidToken)
	}
	var p payload
	if err := decodePart(parts[1], &p); err != nil {
		return Claims{}, fmt.Errorf("%w: payload", ErrInvalidToken)
	}
	now := v.now()
	switch {
	case p.Aud != v.clientID:
		return Claims{}, fmt.Errorf("%w: aud", ErrInvalidToken)
	case p.Iss != "accounts.google.com" && p.Iss != "https://accounts.google.com":
		return Claims{}, fmt.Errorf("%w: iss", ErrInvalidToken)
	case p.Exp == 0 || now.After(time.Unix(p.Exp, 0).Add(clockSkew)):
		return Claims{}, fmt.Errorf("%w: hết hạn", ErrInvalidToken)
	case p.Iat != 0 && time.Unix(p.Iat, 0).After(now.Add(clockSkew)):
		return Claims{}, fmt.Errorf("%w: iat", ErrInvalidToken)
	case p.Sub == "" || p.Email == "":
		return Claims{}, fmt.Errorf("%w: thiếu sub / email", ErrInvalidToken)
	case !verified(p.EmailVerified):
		return Claims{}, fmt.Errorf("%w: email chưa xác minh", ErrInvalidToken)
	}
	return Claims{Sub: p.Sub, Email: strings.ToLower(p.Email), Name: p.Name, Picture: p.Picture}, nil
}

func verified(raw json.RawMessage) bool {
	s := strings.Trim(string(raw), `"`)
	return s == "true"
}

func decodePart(s string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// key trả khoá theo kid: dùng bộ đệm còn hạn; hết hạn hoặc gặp kid lạ thì tải lại (kid lạ: tối đa mỗi phút một lần).
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	if k, ok := v.keys[kid]; ok && now.Before(v.expires) {
		return k, nil
	}
	stale := now.After(v.expires)
	if stale || now.Sub(v.lastFetch) >= refetchGap {
		if err := v.fetch(ctx, now); err != nil && v.keys == nil {
			return nil, err
		}
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("%w: không có khoá %q", ErrInvalidToken, kid)
}

// fetch tải JWKS; gọi khi đang giữ mu. Lỗi mạng thì giữ bộ khoá cũ.
func (v *Verifier) fetch(ctx context.Context, now time.Time) error {
	v.lastFetch = now
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("tải khoá Google: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("tải khoá Google: HTTP %d", res.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return fmt.Errorf("đọc khoá Google: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return errors.New("khoá Google rỗng")
	}
	v.keys = keys
	v.expires = now.Add(maxAge(res.Header.Get("Cache-Control")))
	return nil
}

// maxAge đọc max-age trong Cache-Control, mặc định defaultKeyTTL.
func maxAge(cc string) time.Duration {
	for _, part := range strings.Split(cc, ",") {
		part = strings.TrimSpace(part)
		if s, ok := strings.CutPrefix(part, "max-age="); ok {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return time.Duration(n) * time.Second
			}
		}
	}
	return defaultKeyTTL
}

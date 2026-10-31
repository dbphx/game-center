package friends

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t      *testing.T
	w      *wallet.Wallet
	f      *Friends
	tokens map[int64]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "friends.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	fm, err := farm.New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	f, err := New(ctx, w, fm)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, w: w, f: f, tokens: map[int64]string{}}
}

func (e *env) user(username, name string) int64 {
	e.t.Helper()
	u, token, err := e.w.Register(ctx, username, "matkhau123", wallet.Profile{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	e.tokens[u.ID] = token
	return u.ID
}

func (e *env) ok(l List, err error) List {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
	return l
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("muốn lỗi %d, không có lỗi", status)
	}
	if got := errStatus(err); got != status {
		t.Fatalf("muốn mã %d, được %d (%v)", status, got, err)
	}
}

func ids(people []Person) []int64 {
	out := []int64{}
	for _, p := range people {
		out = append(out, p.ID)
	}
	return out
}

func TestRequestAcceptRemove(t *testing.T) {
	e := newEnv(t)
	an, binh := e.user("an01", "An"), e.user("binh", "Bình")

	// Mời theo tên đăng nhập, không phân biệt hoa thường.
	l := e.ok(e.f.Request(ctx, an, "  BINH ", 0))
	if len(l.Outgoing) != 1 || l.Outgoing[0].ID != binh || l.Outgoing[0].Name != "Bình" || l.Outgoing[0].Level != 1 || len(l.Friends) != 0 {
		t.Fatalf("sau khi mời: %+v", l)
	}
	_, err := e.f.Request(ctx, an, "binh", 0)
	wantStatus(t, err, http.StatusConflict) // mời trùng
	_, err = e.f.Request(ctx, an, "an01", 0)
	wantStatus(t, err, http.StatusBadRequest) // tự kết bạn
	_, err = e.f.Request(ctx, an, "khongco", 0)
	wantStatus(t, err, http.StatusNotFound)
	_, err = e.f.Request(ctx, an, "", 0)
	wantStatus(t, err, http.StatusBadRequest)

	l = e.ok(e.f.List(ctx, binh))
	if len(l.Incoming) != 1 || l.Incoming[0].ID != an {
		t.Fatalf("lời mời đến: %+v", l)
	}
	l = e.ok(e.f.Accept(ctx, binh, an))
	if got := l.Friends; len(got) != 1 || got[0].ID != an || len(l.Incoming) != 0 {
		t.Fatalf("sau khi chấp nhận: %+v", l)
	}
	if l := e.ok(e.f.List(ctx, an)); len(l.Friends) != 1 || l.Friends[0].ID != binh || len(l.Outgoing) != 0 {
		t.Fatalf("phía người mời: %+v", l)
	}
	_, err = e.f.Request(ctx, binh, "an01", 0)
	wantStatus(t, err, http.StatusConflict) // đã là bạn
	_, err = e.f.Accept(ctx, binh, an)
	wantStatus(t, err, http.StatusNotFound)

	e.ok(e.f.Remove(ctx, an, binh))
	if l := e.ok(e.f.List(ctx, binh)); len(l.Friends) != 0 {
		t.Fatalf("huỷ kết bạn một chiều: %+v", l)
	}
	_, err = e.f.Remove(ctx, an, binh)
	wantStatus(t, err, http.StatusNotFound)
}

func TestCrossRequestBefriends(t *testing.T) {
	e := newEnv(t)
	an, binh := e.user("an01", "An"), e.user("binh", "Bình")
	e.ok(e.f.Request(ctx, an, "", binh)) // mời theo id
	l := e.ok(e.f.Request(ctx, binh, "an01", 0))
	if len(l.Friends) != 1 || l.Friends[0].ID != an || len(l.Incoming)+len(l.Outgoing) != 0 {
		t.Fatalf("mời chéo: %+v", l)
	}
	if ok, _ := e.f.AreFriends(ctx, an, binh); !ok {
		t.Fatal("mời chéo mà chưa thành bạn")
	}
}

func TestDecline(t *testing.T) {
	e := newEnv(t)
	an, binh, chi := e.user("an01", "An"), e.user("binh", "Bình"), e.user("chi", "Chi")
	e.ok(e.f.Request(ctx, an, "binh", 0))
	e.ok(e.f.Request(ctx, an, "chi", 0))
	e.ok(e.f.Decline(ctx, binh, an)) // người nhận từ chối
	e.ok(e.f.Decline(ctx, an, chi))  // người mời rút lại
	if l := e.ok(e.f.List(ctx, an)); len(l.Outgoing) != 0 || len(l.Friends) != 0 {
		t.Fatalf("sau khi từ chối: %+v", l)
	}
	_, err := e.f.Decline(ctx, binh, an)
	wantStatus(t, err, http.StatusNotFound)
	// Từ chối rồi vẫn mời lại được.
	e.ok(e.f.Request(ctx, an, "binh", 0))
}

func TestMaxFriends(t *testing.T) {
	e := newEnv(t)
	an := e.user("an01", "An")
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Chèn sẵn MaxFriends người bạn cho nhanh.
	for i := range MaxFriends {
		r, err := tx.ExecContext(ctx, `INSERT INTO users (username, name, balance, created_at) VALUES (?, ?, 0, 0)`, "u"+strconv.Itoa(i), "U")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := r.LastInsertId()
		if err := befriend(ctx, tx, an, id, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.user("binh", "Bình")
	_, err = e.f.Request(ctx, an, "binh", 0)
	wantStatus(t, err, http.StatusConflict)
}

func TestFarmAndStealOnlyFriends(t *testing.T) {
	e := newEnv(t)
	an, binh := e.user("an01", "An"), e.user("binh", "Bình")
	// Ai cũng xem được nông trại (vd từ Khu danh vọng), nhưng không phải bạn thì không có lượt trộm.
	if v, err := e.f.Farm(ctx, an, binh); err != nil || v.Friend || v.StealsLeft != 0 || v.Owner.Name != "Bình" {
		t.Fatalf("xem nhà người lạ: %+v %v", v, err)
	}
	_, err := e.f.Farm(ctx, an, an)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.Steal(ctx, an, binh, 0)
	wantStatus(t, err, http.StatusForbidden)

	e.ok(e.f.Request(ctx, an, "binh", 0))
	if v, err := e.f.Farm(ctx, an, binh); err != nil || v.Friend || v.StealsLeft != 0 { // mới mời, chưa là bạn
		t.Fatalf("mới mời: %+v %v", v, err)
	}
	e.ok(e.f.Accept(ctx, binh, an))
	v, err := e.f.Farm(ctx, an, binh)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Friend || v.Owner.ID != binh || v.Owner.Name != "Bình" || len(v.Plots) != farm.FreePlots || v.StealsLeft != farm.StealsPerDay || v.Dog.Guarding {
		t.Fatalf("nông trại bạn: %+v", v)
	}
	_, err = e.f.Steal(ctx, an, binh, 0) // ô trống
	wantStatus(t, err, http.StatusConflict)
}

// Gọi API thật qua HTTP: tên trường đúng hợp đồng với giao diện.
func TestHandler(t *testing.T) {
	e := newEnv(t)
	an, binh := e.user("an01", "An"), e.user("binh", "Bình")
	srv := httptest.NewServer(e.f.Handler())
	t.Cleanup(srv.Close)
	call := func(uid int64, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		if uid != 0 {
			req.Header.Set("Authorization", "Bearer "+e.tokens[uid])
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out := map[string]any{}
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if code, _ := call(0, "GET", "/api/friends", nil); code != http.StatusUnauthorized {
		t.Fatalf("không token: %d", code)
	}
	if code, out := call(an, "POST", "/api/friends/request", map[string]any{"username": "binh"}); code != 200 || len(out["outgoing"].([]any)) != 1 {
		t.Fatalf("mời: %d %v", code, out)
	}
	if code, out := call(an, "POST", "/api/friends/request", map[string]any{"username": "binh"}); code != http.StatusConflict || out["error"] == "" {
		t.Fatalf("mời trùng: %d %v", code, out)
	}
	code, out := call(binh, "POST", "/api/friends/accept", map[string]any{"id": an})
	if code != 200 {
		t.Fatalf("chấp nhận: %d %v", code, out)
	}
	fr := out["friends"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "name", "level", "gender", "outfit", "hair", "wings", "face", "online", "ripe", "dogGuarding"} {
		if _, ok := fr[k]; !ok {
			t.Fatalf("bạn thiếu trường %q: %v", k, fr)
		}
	}
	if _, ok := fr["username"]; ok {
		t.Fatal("không được lộ tên đăng nhập")
	}
	code, out = call(an, "GET", "/api/friends/"+strconv.FormatInt(binh, 10)+"/farm", nil)
	if code != 200 || out["owner"].(map[string]any)["name"] != "Bình" || out["dog"] == nil || out["stealsLeft"] == nil {
		t.Fatalf("xem nông trại: %d %v", code, out)
	}
	p := out["plots"].([]any)[0].(map[string]any)
	for _, k := range []string{"index", "crop", "plantedAt", "readyAt", "ripe", "stolen", "tried", "locked"} {
		if _, ok := p[k]; !ok {
			t.Fatalf("ô thiếu trường %q: %v", k, p)
		}
	}
	if code, out := call(an, "POST", "/api/friends/"+strconv.FormatInt(binh, 10)+"/steal", map[string]any{"plot": 0}); code != http.StatusConflict {
		t.Fatalf("trộm ô trống: %d %v", code, out)
	}
	if code, out := call(an, "POST", "/api/friends/"+strconv.FormatInt(binh, 10)+"/steal/finish", map[string]any{"chase": "khongco", "escaped": true}); code != http.StatusNotFound {
		t.Fatalf("kết thúc cuộc đuổi không có: %d %v", code, out)
	}
	if code, _ := call(an, "GET", "/api/friends/abc/farm", nil); code != http.StatusBadRequest {
		t.Fatalf("id sai: %d", code)
	}
	if code, _ := call(an, "POST", "/api/friends/remove", map[string]any{"id": binh}); code != 200 {
		t.Fatalf("huỷ kết bạn: %d", code)
	}
	if code, out := call(an, "GET", "/api/friends/"+strconv.FormatInt(binh, 10)+"/farm", nil); code != 200 || out["friend"] != false || out["stealsLeft"] != 0.0 {
		t.Fatalf("hết là bạn: vẫn xem được nhưng không trộm được: %d %v", code, out)
	}
	if code, _ := call(an, "POST", "/api/friends/"+strconv.FormatInt(binh, 10)+"/steal", map[string]any{"plot": 0}); code != http.StatusForbidden {
		t.Fatalf("không phải bạn mà trộm được: %d", code)
	}
}

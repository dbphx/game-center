package baucua

import (
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// player tạo thêm một người chơi có sẵn xu.
func (e *env) player(username string, balance int64) int64 {
	e.t.Helper()
	u, _, err := e.w.Register(ctx, username, "matkhau123", wallet.Profile{Name: username})
	if err != nil {
		e.t.Fatal(err)
	}
	e.setBalance(u.ID, balance)
	return u.ID
}

func (e *env) create(uid int64) string {
	e.t.Helper()
	code, err := e.g.Create(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return code
}

func (e *env) state(code string, uid int64) State {
	e.t.Helper()
	s, err := e.g.State(ctx, code, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) betIn(code string, uid int64, face string, amount int64) {
	e.t.Helper()
	if _, err := e.g.Bet(ctx, code, uid, face, amount); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) rooms() map[string]RoomSummary {
	e.t.Helper()
	list, err := e.g.Rooms(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if len(list) == 0 || list[0].Code != Lobby {
		e.t.Fatalf("sòng chung phải đứng đầu: %+v", list)
	}
	out := map[string]RoomSummary{}
	for _, r := range list {
		out[r.Code] = r
	}
	return out
}

func TestRoomCreateJoinLeaveClose(t *testing.T) {
	e := newEnv(t, 1000)
	ban := e.player("ban", 1000)
	code := e.create(e.uid)
	if len(code) != 6 {
		t.Fatalf("mã bàn 6 ký tự, được %q", code)
	}
	if r := e.rooms()[code]; r.Host == nil || r.Host.Name != "Người chơi" || r.Members != 1 || r.Banker != nil {
		t.Fatalf("bàn mới: %+v", r)
	}
	s, err := e.g.Join(ctx, code, ban)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Room.Members) != 2 || s.Room.Code != code || s.Invite != "/baucua/?room="+code || s.Mine.Host || s.Room.Host == nil {
		t.Fatalf("vào bàn: %+v", s.Room)
	}
	if !e.state(code, e.uid).Mine.Host {
		t.Fatal("người tạo là chủ bàn")
	}
	if err := e.g.Leave(ctx, code, ban); err != nil {
		t.Fatal(err)
	}
	if n := e.rooms()[code].Members; n != 1 {
		t.Fatalf("rời bàn: còn %d người", n)
	}
	if _, err := e.g.State(ctx, "KHONGCO", e.uid); status(err) != http.StatusNotFound {
		t.Fatalf("bàn không tồn tại phải 404, được %v", err)
	}
	if s := e.state("", 0); s.Room.Code != Lobby || s.Room.Host != nil {
		t.Fatalf("mặc định là sòng chung: %+v", s.Room)
	}

	// Không ai poll 15 s thì coi như rời; bỏ trống 5 phút thì bàn đóng.
	e.clock = e.clock.Add(16 * time.Second)
	if n := e.rooms()[code].Members; n != 0 {
		t.Fatalf("không poll 15 s vẫn còn %d người", n)
	}
	e.clock = e.clock.Add(roomIdle)
	if _, ok := e.rooms()[code]; ok {
		t.Fatal("bàn bỏ trống 5 phút phải đóng")
	}
	if _, err := e.g.State(ctx, code, e.uid); status(err) != http.StatusNotFound {
		t.Fatalf("bàn đã đóng phải 404, được %v", err)
	}
}

func TestRoomsAreIndependent(t *testing.T) {
	e := newEnv(t, 1000)
	rolls := [][3]string{{"bau", "bau", "bau"}, {"nai", "nai", "nai"}}
	e.g.roll = func() [3]string { d := rolls[e.rolls%2]; e.rolls++; return d }
	code := e.create(e.uid)
	e.betIn(code, e.uid, "tom", 100)
	e.betIn(Lobby, e.uid, "ca", 50)
	if s := e.state(Lobby, e.uid); s.Mine.Total != 50 || s.Totals["tom"] != 0 || s.Room.RoundTotal != 50 {
		t.Fatalf("sòng chung lẫn cược bàn riêng: %+v", s.Mine)
	}
	if s := e.state(code, e.uid); s.Mine.Total != 100 || s.Totals["ca"] != 0 {
		t.Fatalf("bàn riêng lẫn cược sòng chung: %+v", s.Mine)
	}
	e.clock = e.clock.Add(BettingTime + RollingTime)
	a, b := e.state(Lobby, e.uid), e.state(code, e.uid)
	if e.rolls != 2 || a.Dice[0] == b.Dice[0] || len(a.History) != 1 || len(b.History) != 1 {
		t.Fatalf("mỗi bàn quay riêng: %d lần, %v / %v", e.rolls, a.Dice, b.Dice)
	}
}

func TestBankerSettlesFromBankroll(t *testing.T) {
	e := newEnv(t, 10000) // e.uid làm cái
	p := e.player("khach", 5000)
	code := e.create(e.uid)
	s, err := e.g.Bank(ctx, code, e.uid, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if s.Room.Banker == nil || s.Room.Banker.Bankroll != 3000 || !s.Mine.Banker || s.Room.MaxRoundTotal != 1000 || *s.Balance != 7000 {
		t.Fatalf("làm cái: %+v số dư %d", s.Room, *s.Balance)
	}
	if _, err := e.g.Bet(ctx, code, e.uid, "cua", 10); status(err) != http.StatusConflict {
		t.Fatalf("người làm cái đặt cược phải 409, được %v", err)
	}
	e.betIn(code, p, "cua", 500) // ra 2 lần: nhận 1500
	e.betIn(code, p, "nai", 500) // không ra
	_, err = e.g.Bet(ctx, code, p, "ga", 10)
	if status(err) != http.StatusConflict || err.Error() != "Quỹ cái chỉ nhận thêm 0 xu" {
		t.Fatalf("vượt quỹ / 3 phải 409, được %v", err)
	}
	if s := e.state(code, p); s.Room.RoundTotal != 1000 || s.Room.MaxRoundTotal != 1000 {
		t.Fatalf("tổng cược bàn: %+v", s.Room)
	}

	e.clock = e.clock.Add(BettingTime + RollingTime)
	s = e.state(code, e.uid)
	if b := s.Room.Banker; b == nil || b.Bankroll != 2500 || b.Net == nil || *b.Net != -500 {
		t.Fatalf("quỹ sau vòng: %+v", s.Room.Banker)
	}
	if got := e.user(p).Balance; got != 5000-1000+1500 {
		t.Fatalf("khách nhận sai: %d", got)
	}
	if u := e.user(e.uid); u.Balance != 7000 || u.XP != XPPlay {
		t.Fatalf("người làm cái: ví %d, XP %d", u.Balance, u.XP)
	}

	// Vòng sau khách thua hết: quỹ lời, người làm cái thêm XP.
	e.clock = e.clock.Add(ResultTime)
	e.betIn(code, p, "bau", 500)
	e.clock = e.clock.Add(BettingTime + RollingTime)
	if b := e.state(code, e.uid).Room.Banker; b.Bankroll != 3000 || *b.Net != 500 {
		t.Fatalf("quỹ lời: %+v", b)
	}
	if xp := e.user(e.uid).XP; xp != 2*XPPlay+XPWin {
		t.Fatalf("XP người làm cái: %d", xp)
	}
}

func TestBankRules(t *testing.T) {
	e := newEnv(t, 10000)
	p := e.player("khach", 5000)
	code := e.create(e.uid)
	if _, err := e.g.Bank(ctx, Lobby, e.uid, 3000); status(err) != http.StatusConflict {
		t.Fatalf("sòng chung không cho làm cái, được %v", err)
	}
	if _, err := e.g.Bank(ctx, code, e.uid, 2999); status(err) != http.StatusBadRequest {
		t.Fatalf("quỹ dưới 3000 phải 400, được %v", err)
	}
	if _, err := e.g.Bank(ctx, code, p, 6000); status(err) != http.StatusPaymentRequired {
		t.Fatalf("không đủ xu nộp quỹ phải 402, được %v", err)
	}
	e.betIn(code, p, "cua", 100)
	if _, err := e.g.Bank(ctx, code, e.uid, 3000); status(err) != http.StatusConflict {
		t.Fatalf("bàn đã có cược thì chưa được làm cái, được %v", err)
	}
	e.clock = e.clock.Add(BettingTime)
	if _, err := e.g.Bank(ctx, code, e.uid, 3000); status(err) != http.StatusConflict {
		t.Fatalf("đang lắc không nhận làm cái, được %v", err)
	}
	e.clock = e.clock.Add(RollingTime)
	if _, err := e.g.Bank(ctx, code, e.uid, 3000); err != nil {
		t.Fatalf("lúc mở bát được làm cái (từ vòng sau): %v", err)
	}
	if _, err := e.g.Bank(ctx, code, p, 3000); status(err) != http.StatusConflict {
		t.Fatalf("bàn đã có người làm cái, được %v", err)
	}
	if e.user(p).Balance != 5000-100+300 {
		t.Fatal("vòng trước bàn vẫn do máy trả thưởng")
	}
}

func TestUnbankTiming(t *testing.T) {
	e := newEnv(t, 10000)
	p := e.player("khach", 5000)
	code := e.create(e.uid)
	if _, err := e.g.Bank(ctx, code, e.uid, 3000); err != nil {
		t.Fatal(err)
	}
	if _, err := e.g.Unbank(ctx, code, p); status(err) != http.StatusConflict {
		t.Fatalf("không làm cái thì không thôi được, được %v", err)
	}
	e.betIn(code, p, "nai", 500) // thua: quỹ +600
	e.betIn(code, p, "nai", 100)
	s, err := e.g.Unbank(ctx, code, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if s.Room.Banker == nil || !s.Room.Banker.Leaving || e.user(e.uid).Balance != 7000 {
		t.Fatalf("còn cược thì thôi sau khi chốt: %+v", s.Room.Banker)
	}
	e.clock = e.clock.Add(BettingTime + RollingTime)
	if s := e.state(code, p); s.Room.Banker != nil || s.Room.MaxRoundTotal != 0 {
		t.Fatalf("chốt xong phải thôi làm cái: %+v", s.Room.Banker)
	}
	if b := e.user(e.uid).Balance; b != 10000+600 {
		t.Fatalf("hoàn quỹ kèm lãi: %d", b)
	}

	// Không có cược thì thôi ngay.
	e.clock = e.clock.Add(ResultTime)
	if _, err := e.g.Bank(ctx, code, e.uid, 4000); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.g.Unbank(ctx, code, e.uid); s.Room.Banker != nil || *s.Balance != 10600 {
		t.Fatalf("thôi ngay: %+v số dư %d", s.Room.Banker, *s.Balance)
	}
}

func TestBankerAutoQuit(t *testing.T) {
	t.Run("vắng mặt", func(t *testing.T) {
		e := newEnv(t, 10000)
		p := e.player("khach", 5000)
		code := e.create(e.uid)
		if _, err := e.g.Bank(ctx, code, e.uid, 3000); err != nil {
			t.Fatal(err)
		}
		// Khách vẫn ở bàn, người làm cái không poll nữa.
		for range 8 {
			e.clock = e.clock.Add(10 * time.Second)
			e.state(code, p)
		}
		if s := e.state(code, p); s.Room.Banker != nil {
			t.Fatalf("vắng quá 60 s phải thôi làm cái: %+v", s.Room.Banker)
		}
		if b := e.user(e.uid).Balance; b != 10000 {
			t.Fatalf("hoàn quỹ: %d", b)
		}
	})
	t.Run("hết quỹ", func(t *testing.T) {
		e := newEnv(t, 10000)
		p := e.player("khach", 5000)
		e.dice = [3]string{"cua", "cua", "cua"}
		code := e.create(e.uid)
		if _, err := e.g.Bank(ctx, code, e.uid, 3000); err != nil {
			t.Fatal(err)
		}
		e.betIn(code, p, "cua", 1000) // ra 3 lần: nhận 4000, quỹ còn 0
		e.clock = e.clock.Add(BettingTime + RollingTime)
		s := e.state(code, e.uid)
		if s.Room.Banker != nil || e.user(p).Balance != 8000 || e.user(e.uid).Balance != 7000 {
			t.Fatalf("hết quỹ: %+v, khách %d, cái %d", s.Room.Banker, e.user(p).Balance, e.user(e.uid).Balance)
		}
	})
}

func TestRestartRefundsBank(t *testing.T) {
	e := newEnv(t, 10000)
	code := e.create(e.uid)
	if _, err := e.g.Bank(ctx, code, e.uid, 5000); err != nil {
		t.Fatal(err)
	}
	e.w.Close()
	e.open()
	if b := e.user(e.uid).Balance; b != 10000 {
		t.Fatalf("khởi động lại phải hoàn quỹ làm cái: %d", b)
	}
	if _, err := e.g.State(ctx, code, e.uid); status(err) != http.StatusNotFound {
		t.Fatal("bàn riêng không còn sau khi khởi động lại")
	}
}

// Nhiều người đặt và poll cùng lúc qua mốc mở bát: tiền vào ra cân bằng, quỹ không âm.
func TestBankerConcurrent(t *testing.T) {
	e := newEnv(t, 100000)
	code := e.create(e.uid)
	if _, err := e.g.Bank(ctx, code, e.uid, 30000); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, name := range []string{"khach1", "khach2", "khach3", "khach4", "khach5"} {
		ids = append(ids, e.player(name, 20000))
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		for _, face := range []string{"cua", "ga", "bau"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 3 {
					e.g.Bet(ctx, code, id, face, 1000) // một số sẽ bị 409 vì vượt quỹ / 3
				}
			}()
		}
	}
	wg.Wait()
	if s := e.state(code, e.uid); s.Room.RoundTotal > 10000 {
		t.Fatalf("tổng cược %d vượt quỹ / 3", s.Room.RoundTotal)
	}
	e.clock = e.clock.Add(BettingTime + RollingTime)
	for range 10 {
		wg.Add(1)
		go func() { defer wg.Done(); e.g.State(ctx, code, ids[0]) }()
	}
	wg.Wait()
	b := e.state(code, e.uid).Room.Banker
	var players int64
	for _, id := range ids {
		players += e.user(id).Balance
	}
	if b == nil || b.Bankroll < 0 || players+b.Bankroll != 5*20000+30000 {
		t.Fatalf("tiền không cân: khách %d, quỹ %+v", players, b)
	}
}

// Database của bản chỉ có sòng chung (bảng cược chưa có cột room): mở lên vẫn chạy, cược treo được hoàn.
func TestUpgradeOldBetsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	w, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	u, _, err := w.Register(ctx, "nguoicu", "matkhau123", wallet.Profile{Name: "Cũ"})
	if err != nil {
		t.Fatal(err)
	}
	old := `CREATE TABLE baucua_bets (id INTEGER PRIMARY KEY AUTOINCREMENT, round INTEGER NOT NULL, user_id INTEGER NOT NULL,
		face TEXT NOT NULL, amount INTEGER NOT NULL, settled INTEGER NOT NULL DEFAULT 0);
		INSERT INTO baucua_bets (round, user_id, face, amount) VALUES (1, ` + strconv.FormatInt(u.ID, 10) + `, 'cua', 100);`
	if err := w.ExecSchema(ctx, old); err != nil {
		t.Fatal(err)
	}
	g, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := w.User(ctx, u.ID); after.Balance != u.Balance+100 {
		t.Fatalf("cược treo bản cũ phải được hoàn: %d → %d", u.Balance, after.Balance)
	}
	if _, err := g.Bet(ctx, Lobby, u.ID, "cua", 10); err != nil && status(err) != http.StatusConflict {
		t.Fatalf("đặt cược sau khi nâng cấp: %v", err)
	}
}

// Chat bàn: chỉ người có mặt mới chat được, tin được lọc và mang seq tăng dần, chat dày bị chặn, bàn khác không thấy.
func TestRoomChat(t *testing.T) {
	e := newEnv(t, 1000)
	ban := e.player("ban", 1000)
	code := e.create(e.uid)
	if _, err := e.g.Chat(ctx, code, ban, "chào"); status(err) != http.StatusConflict {
		t.Fatalf("chưa vào bàn mà chat phải 409, được %v", err)
	}
	e.state(code, ban)
	s, err := e.g.Chat(ctx, code, ban, "  chào   cả nhà  đm ")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Chat) != 1 || s.Chat[0].Text != "chào cả nhà ***" || s.Chat[0].Name != "ban" || s.Chat[0].Seq != 1 {
		t.Fatalf("tin chat sai: %+v", s.Chat)
	}
	if _, err := e.g.Chat(ctx, code, ban, "nữa"); status(err) != http.StatusConflict {
		t.Fatalf("chat dày phải 409, được %v", err)
	}
	e.clock = e.clock.Add(2 * time.Second)
	e.g.Chat(ctx, code, e.uid, "ok")
	if s := e.state(code, e.uid); len(s.Chat) != 2 || s.Chat[1].Seq != 2 || s.Chat[1].Name != "Người chơi" {
		t.Fatalf("chủ bàn phải thấy 2 tin: %+v", s.Chat)
	}
	if s := e.state(Lobby, e.uid); len(s.Chat) != 0 {
		t.Fatalf("sòng chung không thấy chat bàn riêng: %+v", s.Chat)
	}
}

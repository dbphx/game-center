// DCity: tổ hợp game bài online dùng chung một ví xu.
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/dbphx/game-center/games/arena"
	"github.com/dbphx/game-center/games/baucua"
	"github.com/dbphx/game-center/games/cangua"
	"github.com/dbphx/game-center/games/caro"
	"github.com/dbphx/game-center/games/fame"
	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/games/fishing"
	"github.com/dbphx/game-center/games/friends"
	"github.com/dbphx/game-center/games/plaza"
	"github.com/dbphx/game-center/games/site"
	"github.com/dbphx/game-center/games/tienlen/online"
	"github.com/dbphx/game-center/internal/googleauth"
	"github.com/dbphx/game-center/internal/server"
	"github.com/dbphx/game-center/internal/wallet"
)

//go:embed public
var publicFiles embed.FS

func main() {
	addr := flag.String("addr", ":"+envOr("PORT", "8080"), "địa chỉ lắng nghe")
	dbPath := flag.String("db", envOr("DB_PATH", "data/gamehub.db"), "file SQLite lưu tài khoản và ví")
	googleID := flag.String("google-client-id", os.Getenv("GOOGLE_CLIENT_ID"), "OAuth Client ID cho đăng nhập Google (rỗng: tắt)")
	flag.Parse()

	w, err := wallet.Open(*dbPath)
	if err != nil {
		log.Fatalf("mở ví %s: %v", *dbPath, err)
	}
	defer w.Close()

	static, _ := fs.Sub(publicFiles, "public")
	srv := server.New(w, static, games(w))
	if *googleID != "" {
		srv.EnableGoogle(googleauth.New(*googleID))
		log.Printf("đăng nhập Google: bật (client %s)", *googleID)
	}

	printAddresses(*addr)
	log.Fatal(http.ListenAndServe(*addr, srv))
}

// games liệt kê các game trong hub. Thêm game mới thì thêm vào đây.
func games(w *wallet.Wallet) []server.Game {
	auth := func(ctx context.Context, token string) (online.User, error) {
		u, err := w.UserByToken(ctx, token)
		return online.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face, Level: u.Level,
			HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	opts := online.DefaultOptions
	opts.MinHP = wallet.ZoneMinHP["casino"]
	tienlen := online.NewHub(w, opts)
	farmGame, err := farm.New(context.Background(), w)
	if err != nil {
		log.Fatalf("nông trại: %v", err)
	}
	friendsGame, err := friends.New(context.Background(), w, farmGame)
	if err != nil {
		log.Fatalf("bạn bè: %v", err)
	}
	caroAuth := func(ctx context.Context, token string) (caro.User, error) {
		u, err := w.UserByToken(ctx, token)
		return caro.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face,
			Level: u.Level, HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	caroOpts := caro.DefaultOptions
	caroOpts.MinHP = wallet.ZoneMinHP["casino"]
	caroHub := caro.NewHub(w, caroOpts)
	canguaAuth := func(ctx context.Context, token string) (cangua.User, error) {
		u, err := w.UserByToken(ctx, token)
		return cangua.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face,
			Level: u.Level, HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	canguaOpts := cangua.DefaultOptions
	canguaOpts.MinHP = wallet.ZoneMinHP["casino"]
	canguaHub := cangua.NewHub(w, canguaOpts)
	armory, err := arena.New(context.Background(), w)
	if err != nil {
		log.Fatalf("đấu trường: %v", err)
	}
	arenaAuth := func(ctx context.Context, token string) (arena.User, error) {
		u, err := w.UserByToken(ctx, token)
		return arena.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face,
			Level: u.Level, HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	arenaOpts := arena.DefaultOptions
	arenaOpts.MinHP = wallet.ZoneMinHP[arena.GameID]
	arenaHub := arena.NewHub(w, arenaOpts, armory)
	bauCua, err := baucua.New(context.Background(), w)
	if err != nil {
		log.Fatalf("bầu cua: %v", err)
	}
	siteGame, err := site.New(context.Background(), w)
	if err != nil {
		log.Fatalf("công trường: %v", err)
	}
	pond, err := fishing.New(context.Background(), w)
	if err != nil {
		log.Fatalf("hồ câu cá: %v", err)
	}
	// Quảng trường: người chơi thấy nhau đi lại, trò chuyện, tặng xu (WebSocket /ws/plaza).
	square := plaza.NewHub(w)
	go square.Run(context.Background())

	return []server.Game{
		{
			ID:          online.GameID,
			Name:        "Tiến Lên Miền Nam",
			Description: "Đánh bài 13 lá: chặt heo, đôi thông, tứ quý. Chơi với bạn bè hoặc với máy.",
			Players:     "2–4 người",
			Path:        "/tienlen/",
			Stats:       func() any { return tienlen.Stats() },
			WS:          tienlen.Handler(auth),
			Rooms:       tienlen.RoomsHandler(),
		},
		{
			ID:          farm.GameID,
			Name:        "Nông trại",
			Description: "Trồng cây, nuôi gia súc, nuôi cá; thu hoạch đổi xu và XP.",
			Players:     "1 người",
			Path:        "/farm/",
			API:         farmGame.Handler(),
		},
		{
			ID:          baucua.GameID,
			Name:        "Bầu Cua Tôm Cá",
			Description: "Đặt cược vào Bầu, Cua, Tôm, Cá, Gà, Nai; nhà cái lắc 3 xúc xắc mỗi vòng 30 giây.",
			Players:     "Sòng chung",
			Path:        "/baucua/",
			API:         bauCua.Handler(),
		},
		{
			ID:          caro.GameID,
			Name:        "Cờ Caro",
			Description: "Bàn 15×15, ai có 5 quân liền trước thì thắng. Chơi với bạn hoặc với máy, có cược.",
			Players:     "2 người",
			Path:        "/caro/",
			Stats:       func() any { return caroHub.Stats() },
			WS:          caroHub.Handler(caroAuth),
			Rooms:       caroHub.RoomsHandler(),
		},
		{
			ID:          cangua.GameID,
			Name:        "Cờ Cá Ngựa",
			Description: "2–4 người đổ xúc xắc đua 4 ngựa về chuồng, đá ngựa đối thủ. Chơi với bạn hoặc với máy, có cược.",
			Players:     "2–4 người",
			Path:        "/cangua/",
			Stats:       func() any { return canguaHub.Stats() },
			WS:          canguaHub.Handler(canguaAuth),
			Rooms:       canguaHub.RoomsHandler(),
		},
		{
			ID:          arena.GameID,
			Name:        "Đấu trường",
			Description: "Bắn theo lượt: canh góc, lực và hướng gió; 6 vũ khí (dép lào, chổi chà, cây lau nhà, TV, tủ lạnh…), cường hoá, chuyển cấp, khảm ngọc; đấu boss rơi ngọc; 5 sân đấu.",
			Players:     "2–4 người",
			Path:        "/arena/",
			Stats:       func() any { return arenaHub.Stats() },
			WS:          arenaHub.Handler(arenaAuth),
			Rooms:       arenaHub.RoomsHandler(),
			API:         armory.Handler(),
		},
		{
			ID:          site.GameID,
			Name:        "Công trường",
			Description: "Làm thuê theo ca ở ngoại ô: khuân gạch, trộn vữa, lái cần cẩu để kiếm xu (tốn máu).",
			Players:     "1 người",
			Path:        "/site/",
			API:         siteGame.Handler(),
		},
		{
			ID:          fishing.GameID,
			Name:        "Hồ câu cá",
			Description: "Mua mồi, thả câu, giật đúng lúc cá cắn câu; bán cá trong giỏ lấy xu.",
			Players:     "1 người",
			Path:        "/fishing/",
			API:         pond.Handler(),
		},
		{
			ID:          fame.GameID,
			Name:        "Khu danh vọng",
			Description: "Bảng vàng top 10 người giàu nhất DCity theo tổng tài sản: xu, đồ thời trang, nông trại.",
			Players:     "Bảng xếp hạng",
			Path:        "/",
			API:         fame.New(w, farmGame).Handler(),
		},
		{
			ID:          friends.GameID,
			Name:        "Bạn bè",
			Description: "Kết bạn theo tên đăng nhập, ghé nông trại của bạn và trộm nông sản chín (coi chừng chó giữ nhà).",
			Players:     "Bạn bè",
			Path:        "/",
			API:         friendsGame.Handler(),
		},
		{
			ID:          plaza.GameID,
			Name:        "Quảng trường",
			Description: "Gặp gỡ người chơi khác trên bản đồ: trò chuyện, biểu cảm, tặng xu, mời chơi.",
			Players:     "Mọi người",
			Path:        "/",
			Stats:       func() any { return map[string]int{"players": square.Online()} },
			WS:          square.Handler(),
		},
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// printAddresses in ra địa chỉ LAN để bạn bè cùng mạng vào chơi.
func printAddresses(addr string) {
	_, port, _ := net.SplitHostPort(addr)
	log.Printf("DCity đang chạy: http://localhost:%s", port)
	ifaces, _ := net.InterfaceAddrs()
	for _, a := range ifaces {
		if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			log.Printf("  trong mạng LAN: http://%s:%s", ip.IP, port)
		}
	}
}

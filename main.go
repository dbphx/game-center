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

	"github.com/dbphx/game-center/games/baucua"
	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/games/tienlen/online"
	"github.com/dbphx/game-center/internal/server"
	"github.com/dbphx/game-center/internal/wallet"
)

//go:embed public
var publicFiles embed.FS

func main() {
	addr := flag.String("addr", ":"+envOr("PORT", "8080"), "địa chỉ lắng nghe")
	dbPath := flag.String("db", envOr("DB_PATH", "data/gamehub.db"), "file SQLite lưu tài khoản và ví")
	flag.Parse()

	w, err := wallet.Open(*dbPath)
	if err != nil {
		log.Fatalf("mở ví %s: %v", *dbPath, err)
	}
	defer w.Close()

	static, _ := fs.Sub(publicFiles, "public")
	srv := server.New(w, static, games(w))

	printAddresses(*addr)
	log.Fatal(http.ListenAndServe(*addr, srv))
}

// games liệt kê các game trong hub. Thêm game mới thì thêm vào đây.
func games(w *wallet.Wallet) []server.Game {
	auth := func(ctx context.Context, token string) (online.User, error) {
		u, err := w.UserByToken(ctx, token)
		return online.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Level: u.Level,
			HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	opts := online.DefaultOptions
	opts.MinHP = wallet.ZoneMinHP["casino"]
	tienlen := online.NewHub(w, opts)
	farmGame, err := farm.New(context.Background(), w)
	if err != nil {
		log.Fatalf("nông trại: %v", err)
	}
	bauCua, err := baucua.New(context.Background(), w)
	if err != nil {
		log.Fatalf("bầu cua: %v", err)
	}

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

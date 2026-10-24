// Server game Tiến Lên Miền Nam: phục vụ giao diện web và WebSocket cho các phòng chơi.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dbphx/game-center/room"
)

//go:embed public
var publicFiles embed.FS

const (
	pingInterval = 25 * time.Second
	readTimeout  = 60 * time.Second
	sendBuffer   = 64
)

var upgrader = websocket.Upgrader{
	// Cho phép mở trang từ địa chỉ khác (IP LAN, tên miền tunnel...).
	CheckOrigin: func(*http.Request) bool { return true },
}

func main() {
	addr := flag.String("addr", ":"+envOr("PORT", "8080"), "địa chỉ lắng nghe")
	flag.Parse()

	printAddresses(*addr)
	log.Fatal(http.ListenAndServe(*addr, newMux(room.NewHub(room.DefaultOptions))))
}

func newMux(hub *room.Hub) *http.ServeMux {
	static, _ := fs.Sub(publicFiles, "public")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		newClient(ws, hub).run()
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	return mux
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
	log.Printf("Tiến Lên đang chạy: http://localhost:%s", port)
	ifaces, _ := net.InterfaceAddrs()
	for _, a := range ifaces {
		if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			log.Printf("  trong mạng LAN: http://%s:%s", ip.IP, port)
		}
	}
}

// client là một kết nối WebSocket. Gorilla chỉ cho một goroutine ghi, nên mọi tin gửi đi qua kênh out.
type client struct {
	ws      *websocket.Conn
	hub     *room.Hub
	out     chan []byte
	done    chan struct{}
	once    sync.Once
	session *room.Session
}

func newClient(ws *websocket.Conn, hub *room.Hub) *client {
	return &client{ws: ws, hub: hub, out: make(chan []byte, sendBuffer), done: make(chan struct{})}
}

// Send không chặn: client quá chậm thì bị ngắt kết nối, sau đó tự vào lại.
func (c *client) Send(msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("json: %v", err)
		return
	}
	select {
	case c.out <- b:
	case <-c.done:
	default:
		c.shutdown()
	}
}

func (c *client) shutdown() {
	c.once.Do(func() {
		close(c.done)
		c.ws.Close()
	})
}

func (c *client) run() {
	go c.writeLoop()
	defer func() {
		if c.session != nil {
			c.session.Room.Disconnect(c.session.Player, c)
		}
		c.shutdown()
	}()

	c.ws.SetReadLimit(4096)
	c.ws.SetReadDeadline(time.Now().Add(readTimeout))
	c.ws.SetPongHandler(func(string) error { return c.ws.SetReadDeadline(time.Now().Add(readTimeout)) })

	for {
		var m room.Message
		if err := c.ws.ReadJSON(&m); err != nil {
			return
		}
		if err := c.handle(m); err != nil {
			c.Send(map[string]string{"t": "error", "msg": err.Error()})
		}
	}
}

func (c *client) handle(m room.Message) error {
	if c.session == nil {
		var s *room.Session
		var err error
		switch m.T {
		case "create":
			s, err = c.hub.Create(m.Name, c)
		case "join":
			s, err = c.hub.Join(m.Code, m.Name, c)
		case "resume":
			s, err = c.hub.Resume(m.Token, c)
			if err != nil {
				c.Send(map[string]string{"t": "left"})
				return nil
			}
		default:
			return nil // tin nhắn tới trễ sau khi rời phòng
		}
		if err != nil {
			return err
		}
		c.session = s
		c.Send(map[string]string{"t": "session", "token": s.Token, "code": s.Room.Code})
		return nil
	}

	err := c.session.Room.Handle(c.session.Player, m)
	if m.T == "leave" {
		c.session = nil
	}
	return err
}

func (c *client) writeLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case b := <-c.out:
			c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.TextMessage, b); err != nil {
				c.shutdown()
				return
			}
		case <-ping.C:
			c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.shutdown()
				return
			}
		case <-c.done:
			return
		}
	}
}

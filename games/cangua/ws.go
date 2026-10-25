package cangua

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 25 * time.Second
	readTimeout  = 60 * time.Second
	sendBuffer   = 64
)

var upgrader = websocket.Upgrader{
	// Cho phép mở trang từ địa chỉ khác (IP LAN, tên miền tunnel...).
	CheckOrigin: func(*http.Request) bool { return true },
}

// Auth đổi token tài khoản hub thành người dùng.
type Auth func(ctx context.Context, token string) (User, error)

// Handler trả về handler WebSocket cho Cờ Cá Ngựa.
func (h *Hub) Handler(auth Auth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &client{ws: ws, hub: h, auth: auth, out: make(chan []byte, sendBuffer), done: make(chan struct{})}
		c.run()
	})
}

// client là một kết nối WebSocket. Gorilla chỉ cho một goroutine ghi, nên mọi tin gửi đi qua kênh out.
type client struct {
	ws       *websocket.Conn
	hub      *Hub
	auth     Auth
	out      chan []byte
	done     chan struct{}
	once     sync.Once
	session  *Session
	watching bool // đã theo dõi việc tài khoản đăng nhập ở nơi khác
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
		var m Message
		if err := c.ws.ReadJSON(&m); err != nil {
			return
		}
		if err := c.handle(m); err != nil {
			c.Send(map[string]string{"t": "error", "msg": err.Error()})
		}
	}
}

func (c *client) handle(m Message) error {
	if c.session == nil {
		if m.T != "create" && m.T != "join" {
			return nil // tin nhắn tới trễ sau khi rời phòng
		}
		u, err := c.auth(context.Background(), m.Token)
		if err != nil {
			c.Send(authFailed(err))
			return nil
		}
		var s *Session
		if m.T == "create" {
			s, err = c.hub.Create(u, c, m.Bet)
		} else {
			s, err = c.hub.Join(m.Code, u, c)
		}
		if err != nil {
			if m.T == "join" {
				c.Send(map[string]string{"t": "left"})
			}
			return err
		}
		c.session = s
		c.watchRevoke(u.ID)
		c.Send(map[string]string{"t": "session", "code": s.Room.Code})
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

// authFailed là tin báo token không dùng được; reason "elsewhere" khi tài khoản đã đăng nhập ở nơi khác.
func authFailed(err error) map[string]string {
	msg := map[string]string{"t": "auth"}
	var e interface{ Elsewhere() bool }
	if errors.As(err, &e) && e.Elsewhere() {
		msg["reason"] = "elsewhere"
	}
	return msg
}

// watchRevoke ngắt kết nối khi tài khoản đăng nhập ở nơi khác (mỗi tài khoản một phiên): gửi
// {t:"auth", reason:"elsewhere"} rồi đóng. Ví thật (wallet.Wallet) có WatchRevoke; ví giả trong test thì bỏ qua.
func (c *client) watchRevoke(userID int64) {
	wr, ok := c.hub.bank.(interface {
		WatchRevoke(userID int64) (<-chan struct{}, func())
	})
	if !ok || c.watching {
		return
	}
	c.watching = true
	revoked, stop := wr.WatchRevoke(userID)
	go func() {
		defer stop()
		select {
		case <-revoked:
			c.Send(map[string]string{"t": "auth", "reason": "elsewhere"})
			// Chờ một chút cho tin kịp gửi rồi mới đóng kết nối.
			select {
			case <-time.After(500 * time.Millisecond):
				c.shutdown()
			case <-c.done:
			}
		case <-c.done:
		}
	}()
}

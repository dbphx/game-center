package plaza

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 25 * time.Second
	readTimeout  = 60 * time.Second
	sendBuffer   = 128
)

var upgrader = websocket.Upgrader{
	// Cho phép mở trang từ địa chỉ khác (IP LAN, tên miền tunnel...).
	CheckOrigin: func(*http.Request) bool { return true },
}

// Handler trả handler WebSocket của quảng trường (gắn ở /ws/plaza).
func (h *Hub) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &client{ws: ws, hub: h, out: make(chan []byte, sendBuffer), done: make(chan struct{})}
		c.run()
	})
}

// client là một kết nối; Gorilla chỉ cho một goroutine ghi nên mọi tin đi qua kênh out.
type client struct {
	ws     *websocket.Conn
	hub    *Hub
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	player *Player
}

// Send không chặn: client quá chậm thì bị ngắt, giao diện tự nối lại.
func (c *client) Send(msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		log.Printf("plaza json: %v", err)
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
		if c.player != nil {
			c.hub.Leave(c.player)
		}
		c.shutdown()
	}()
	c.ws.SetReadLimit(2048)
	c.ws.SetReadDeadline(time.Now().Add(readTimeout))
	c.ws.SetPongHandler(func(string) error { return c.ws.SetReadDeadline(time.Now().Add(readTimeout)) })
	ctx := context.Background()
	for {
		var m Message
		if err := c.ws.ReadJSON(&m); err != nil {
			return
		}
		c.ws.SetReadDeadline(time.Now().Add(readTimeout))
		if c.player == nil {
			if m.T != "hello" {
				continue
			}
			p, err := c.hub.Hello(ctx, m.Token, c)
			if err != nil {
				c.Send(map[string]string{"t": "auth"})
				return
			}
			c.player = p
			continue
		}
		if err := c.hub.Handle(ctx, c.player, m); err != nil {
			c.Send(map[string]string{"t": "error", "msg": err.Error()})
		}
	}
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

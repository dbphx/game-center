package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dbphx/game-center/room"
)

type testClient struct {
	t  *testing.T
	ws *websocket.Conn
}

func dial(t *testing.T, srv *httptest.Server) *testClient {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return &testClient{t: t, ws: ws}
}

func (c *testClient) send(m any) {
	c.t.Helper()
	if err := c.ws.WriteJSON(m); err != nil {
		c.t.Fatal(err)
	}
}

// next đọc tới khi gặp tin nhắn loại want, trả về tin đó dạng map.
func (c *testClient) next(want string) map[string]any {
	c.t.Helper()
	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var m map[string]any
		if err := c.ws.ReadJSON(&m); err != nil {
			c.t.Fatalf("đợi %q: %v", want, err)
		}
		if m["t"] == want {
			return m
		}
		if m["t"] == "error" && want != "error" {
			c.t.Fatalf("server báo lỗi: %v", m["msg"])
		}
	}
}

func (c *testClient) state() room.State {
	c.t.Helper()
	b, _ := json.Marshal(c.next("state"))
	var s room.State
	json.Unmarshal(b, &s)
	return s
}

func TestServesClient(t *testing.T) {
	srv := httptest.NewServer(newMux(room.NewHub(room.DefaultOptions)))
	defer srv.Close()
	for _, path := range []string{"/", "/js/client.js", "/js/rules.js", "/style.css"} {
		res, err := http.Get(srv.URL + path)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("GET %s: %v %v", path, err, res.Status)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
}

// Hai người + một máy chơi trọn một ván qua WebSocket thật, mỗi người đánh theo gợi ý.
func TestTwoPlayersAndBotPlayFullGame(t *testing.T) {
	hub := room.NewHub(room.Options{BotDelay: time.Millisecond, TurnTimeout: time.Minute, Grace: time.Minute})
	srv := httptest.NewServer(newMux(hub))
	defer srv.Close()

	host := dial(t, srv)
	host.send(room.Message{T: "create", Name: "An"})
	code := host.next("session")["code"].(string)

	guest := dial(t, srv)
	guest.send(room.Message{T: "join", Code: strings.ToLower(code), Name: "Bình"})
	guest.next("session")

	host.send(room.Message{T: "addBot"})
	host.send(room.Message{T: "start"})

	s := host.state()
	for s.Phase == room.PhaseLobby {
		s = host.state()
	}
	clients := map[string]*testClient{s.Players[0].ID: host, s.Players[1].ID: guest}

	for moves := 0; ; moves, s = moves+1, host.state() {
		if moves > 300 {
			t.Fatal("ván không kết thúc")
		}
		if s.Phase == room.PhaseOver {
			sum := 0
			for _, p := range s.Players {
				sum += p.Score
			}
			if sum != 0 {
				t.Fatalf("tổng điểm %d khác 0", sum)
			}
			return
		}
		cur := clients[s.Players[s.Turn].ID]
		if cur == nil {
			continue // lượt máy, đợi trạng thái tiếp theo
		}
		cur.send(room.Message{T: "hint"})
		hint := cur.next("hint")
		if hint["cards"] == nil {
			cur.send(room.Message{T: "pass"})
		} else {
			var cards []int
			for _, v := range hint["cards"].([]any) {
				cards = append(cards, int(v.(float64)))
			}
			cur.send(room.Message{T: "play", Cards: cards})
		}
	}
}

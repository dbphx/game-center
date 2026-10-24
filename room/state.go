package room

import (
	"slices"
	"time"

	"github.com/dbphx/game-center/game"
)

type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Bot    bool   `json:"bot"`
	Online bool   `json:"online"`
	Left   bool   `json:"left"`
	Host   bool   `json:"host"`
	Score  int    `json:"score"`
	Cards  int    `json:"cards"`
	Passed bool   `json:"passed"`
	Place  int    `json:"place"`            // -1 khi chưa về
	Reveal []int  `json:"reveal,omitempty"` // bài còn lại, chỉ lộ khi hết ván
}

// State là toàn bộ những gì một người chơi được thấy. Bài của người khác chỉ có số lá.
type State struct {
	T           string         `json:"t"`
	Code        string         `json:"code"`
	You         string         `json:"you"`
	Phase       string         `json:"phase"`
	Players     []PlayerView   `json:"players"`
	Hand        []int          `json:"hand"`
	Turn        int            `json:"turn"`
	TurnMs      int            `json:"turnMs"` // thời gian còn lại của lượt người đang online, 0 nếu không đếm giờ
	LastPlay    *game.LastPlay `json:"lastPlay"`
	MustInclude int            `json:"mustInclude"`
	History     []game.Event   `json:"history"`
	Instant     *game.Instant  `json:"instant"`
	Delta       []int          `json:"delta"`
	GameNo      int            `json:"gameNo"`
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "state", Code: r.Code, You: viewer.ID, Phase: r.phase, Turn: -1, MustInclude: -1, Hand: []int{}}
	g := r.game
	inGame := r.phase != PhaseLobby && g != nil
	if inGame {
		s.Turn, s.LastPlay, s.MustInclude, s.History = g.Turn, g.LastPlay, g.MustInclude, g.History
		s.Instant, s.GameNo = g.Instant, g.GameNo
		if g.Over {
			s.Delta = g.Delta
		}
		if !r.deadline.IsZero() {
			s.TurnMs = max(0, int(time.Until(r.deadline).Milliseconds()))
		}
	}
	for i, p := range r.players {
		v := PlayerView{ID: p.ID, Name: p.Name, Bot: p.Bot, Online: p.Online, Left: p.Left, Host: p.ID == r.hostID, Score: p.Score, Place: -1}
		if inGame {
			v.Cards = len(g.Hands[i])
			v.Passed = g.Passed[i] && g.LastPlay != nil
			v.Place = slices.Index(g.Finished, i)
			if p == viewer {
				s.Hand = g.Hands[i]
			} else if g.Over {
				v.Reveal = g.Hands[i]
			}
		}
		s.Players = append(s.Players, v)
	}
	return s
}

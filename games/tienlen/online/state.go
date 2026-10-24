package online

import (
	"slices"
	"time"

	"github.com/dbphx/game-center/games/tienlen"
)

type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Gender string `json:"gender,omitempty"`
	Outfit string `json:"outfit,omitempty"`
	Level  int    `json:"level,omitempty"`
	Bot    bool   `json:"bot"`
	Online bool   `json:"online"`
	Left   bool   `json:"left"`
	Host   bool   `json:"host"`
	Coins  int64  `json:"coins"` // số dư ví chung; máy không có ví
	Cong   bool   `json:"cong"`  // bị cóng
	Cards  int    `json:"cards"`
	Passed bool   `json:"passed"`
	Place  int    `json:"place"`            // -1 khi chưa về
	Reveal []int  `json:"reveal,omitempty"` // bài còn lại, chỉ lộ khi hết ván
}

// State là toàn bộ những gì một người chơi được thấy. Bài của người khác chỉ có số lá.
type State struct {
	T           string             `json:"t"`
	Code        string             `json:"code"`
	You         string             `json:"you"`
	Phase       string             `json:"phase"`
	Players     []PlayerView       `json:"players"`
	Hand        []int              `json:"hand"`
	Turn        int                `json:"turn"`
	TurnMs      int                `json:"turnMs"` // thời gian còn lại của lượt người đang online, 0 nếu không đếm giờ
	LastPlay    *tienlen.LastPlay  `json:"lastPlay"`
	MustInclude int                `json:"mustInclude"`
	History     []tienlen.Event    `json:"history"`
	Instant     *tienlen.Instant   `json:"instant"`
	Delta       []int              `json:"delta"`     // tiền thứ hạng (mức cược)
	Transfers   []tienlen.Transfer `json:"transfers"` // tiền chặt / thối (mức cược)
	Money       []int              `json:"money"`     // tổng xu từng người được/mất trong ván, khi hết ván
	Bet         int                `json:"bet"`       // mức cược cho ván tới
	GameBet     int                `json:"gameBet"`   // mức cược của ván đang hiển thị
	BetOptions  []int              `json:"betOptions"`
	GameNo      int                `json:"gameNo"`
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "state", Code: r.Code, You: viewer.ID, Phase: r.phase, Turn: -1, MustInclude: -1, Hand: []int{},
		Bet: r.bet, GameBet: r.gameBet, BetOptions: BetOptions}
	g := r.game
	inGame := r.phase != PhaseLobby && g != nil
	if inGame {
		s.Turn, s.LastPlay, s.MustInclude, s.History = g.Turn, g.LastPlay, g.MustInclude, g.History
		s.Instant, s.GameNo = g.Instant, g.GameNo
		s.Transfers = g.Transfers
		if g.Over {
			s.Delta = g.Delta
			for _, units := range g.Money() {
				s.Money = append(s.Money, units*r.gameBet)
			}
		}
		if !r.deadline.IsZero() {
			s.TurnMs = max(0, int(time.Until(r.deadline).Milliseconds()))
		}
	}
	for i, p := range r.players {
		v := PlayerView{ID: p.ID, Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Level: p.Level, Bot: p.Bot, Online: p.Online, Left: p.Left, Host: p.ID == r.hostID, Coins: p.Coins, Place: -1}
		if inGame {
			v.Cards = len(g.Hands[i])
			v.Passed = g.Passed[i] && g.LastPlay != nil
			v.Place = slices.Index(g.Finished, i)
			v.Cong = slices.Contains(g.Cong, i)
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

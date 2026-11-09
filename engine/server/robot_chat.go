package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	mywant "mywant/engine/core"
)

// The robot's conversation is one record: its want's chat — cc_messages, what
// was said to it (a question from the phone, from the GUI's chat), and
// cc_responses, what it said (an answer, an offer asked with はい / いいえ, news
// it brings unasked). The GUI's chat shows it from the want; the phone shows
// it from here, the same lines in the same order (seq) from the last クリア
// (cc_session_resets) on. The board's bubble says each of the robot's lines
// as it is written (sayTurnAnswer, robotSayNotice).

// robotChatLine is one line of the robot's conversation.
type robotChatLine struct {
	// Seq orders the conversation (core's chat_seq.go), and names the line.
	Seq int64 `json:"seq"`
	// Who said it: "person" (cc_messages) or "robot" (cc_responses).
	Who       string `json:"who"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
	// For the robot's: answer (fm), notice (the app's own word), success /
	// error (its agent's).
	Subtype string      `json:"subtype,omitempty"`
	Cards   []robotCard `json:"cards,omitempty"`
	Choices []string    `json:"choices,omitempty"`
	// The turn it asked or answered; "" for a line said unasked.
	Turn   string `json:"turn,omitempty"`
	Sender string `json:"sender,omitempty"`
}

type robotChatResponse struct {
	Lines []robotChatLine `json:"lines"`
	// The last クリア: the conversation shown starts after it.
	ResetAt string `json:"resetAt,omitempty"`
}

// robotChat is the robot's conversation, in order, after the last reset; only
// the lines after seq `after` when it is given.
func (s *Server) robotChat(after int64) (robotChatResponse, error) {
	robot, err := s.robotWantNow()
	if err != nil {
		return robotChatResponse{}, err
	}
	var out robotChatResponse
	resets := mywant.GetCurrent(robot, "cc_session_resets", []any{})
	var resetAt time.Time
	if len(resets) > 0 {
		out.ResetAt = fmt.Sprint(resets[len(resets)-1])
		resetAt, _ = time.Parse(time.RFC3339, out.ResetAt)
	}
	read := func(key, who string) {
		raw, _ := json.Marshal(mywant.GetCurrent(robot, key, []any{}))
		var lines []robotChatLine
		_ = json.Unmarshal(raw, &lines)
		for _, l := range lines {
			if l.Text == "" {
				continue
			}
			l.Who = who
			at, perr := time.Parse(time.RFC3339, l.Timestamp)
			if l.Seq == 0 && perr == nil {
				l.Seq = at.Unix() * 1_000_000 // written before lines had seq
			}
			if !resetAt.IsZero() && perr == nil && at.Before(resetAt) {
				continue
			}
			if l.Seq <= after {
				continue
			}
			out.Lines = append(out.Lines, l)
		}
	}
	read("cc_messages", "person")
	read("cc_responses", "robot")
	// Asked before answered, where two fall in the same place.
	sort.SliceStable(out.Lines, func(i, j int) bool {
		if out.Lines[i].Seq != out.Lines[j].Seq {
			return out.Lines[i].Seq < out.Lines[j].Seq
		}
		return out.Lines[i].Who == "person" && out.Lines[j].Who == "robot"
	})
	if out.Lines == nil {
		out.Lines = []robotChatLine{}
	}
	return out, nil
}

// GET /api/v1/robot/chat[?after=<seq>]
func (s *Server) handleRobotChat(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	out, err := s.robotChat(after)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	s.JSONResponse(w, http.StatusOK, out)
}

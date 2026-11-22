package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// A want's new answer is its notification.
//
// There used to be a watcher per kind of news — one polling spotify wants for
// a changed track and composing "♪ <track>" — and nothing at all for the rest:
// a weather want that fetched new weather told nobody. But every want that
// declares a finalResultField already keeps its answers, shaped once, in its
// result history. A new one of those IS the news, for every want alike, so the
// engine announces it (mywant.OnWantOutput) and this records it as the want's
// alert carrying that same entry. The notice, the card's history and the dots
// on the canvas are then three views of one Output, not three data models that
// have to be kept in step.

// RegisterOutputCallback wires the engine's OnWantOutput to the per-want alert
// store and push. Called once during server startup.
func (s *Server) RegisterOutputCallback() {
	mywant.OnWantOutput = func(w *mywant.Want, out mywant.ResultHistoryEntry) {
		defer func() {
			// A notice must never take down the server.
			if r := recover(); r != nil {
				log.Printf("[want-output] recovered: %v", r)
			}
		}()
		if w == nil || w.Metadata.ID == "" {
			return
		}
		// Its first answer enters it in its character's ask ledger.
		s.noteHomeAsk(w)
		if s.notifications == nil {
			return
		}
		// Told of at the moments its type names and the want chose
		// (core/result_timings.go): murmured, notified, both or neither.
		tell := w.TellingOf(out)
		if w.HasTimings() {
			s.noteTelling(w, out, tell)
		}
		if !tell.Murmur && !tell.Notify {
			return
		}
		// Its type's advice on it (want_advice.go), told with it.
		{
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			out.Advice = mywant.AdviceOn(ctx, w, out, s.sideScript(mywant.SideScriptAdvice, w.Metadata.Type))
			cancel()
		}
		voiced := s.newsVoiceLine(w, mywant.VoiceEvent{Kind: mywant.VoiceEventOutput, Result: out.Result}, outputMessage(out))
		msg, mood := voiced.Say, voiced.Mood
		// One id for the murmur and the notice of the same answer: what a
		// want that heard it names in its answer, and what a rating of the
		// notice is about.
		murmurID := "notif-" + time.Now().UTC().Format("20060102T150405.000000000")
		ev := initiativeEvent{
			Kind: initiativeOutput, WantID: w.Metadata.ID, WantName: w.Metadata.Name,
			WantType: w.Metadata.Type, Message: msg, Moment: w.TimingMoments(tell.Timings),
			NoticeID: murmurID,
		}
		// What it is about (time, place) and why it was said (its timing).
		labels := s.murmurLabels(w, out, voiced.Labels)
		for k, v := range timingReasonLabels(w, out, tell, ev.Moment) {
			if labels == nil {
				labels = map[string]string{}
			}
			labels[k] = v
		}
		if tell.Murmur {
			// The murmur: the want's alert, its tile's "♪".
			entry := out
			if err := s.notifications.Record(NotificationEntry{
				ID:         murmurID,
				Message:    msg,
				Mood:       mood,
				Kind:       "alert",
				TargetType: "want",
				TargetID:   w.Metadata.ID,
				Output:     &entry,
				Labels:     labels,
			}); err != nil {
				log.Printf("[want-output] failed to record alert for %s: %v", w.Metadata.Name, err)
				return
			}
			// Heard by the wants that listen for it (spec.hear, hear.go), with
			// what it is about in set words (its time and place).
			s.hearMurmur(w, murmurID, msg, out, labels)
		}
		// Pushed at its notify timings: as the want chose, or — the type's
		// default — unless Laya is sure it is routine (push_news.go).
		push := tell.Notify
		if push && !tell.NotifyChosen {
			if a, by, ok := s.outputNews(w, msg, ev.Moment); ok {
				ev.News, ev.NewsBy = &a, by
				push = !pushHeld(a, pushNewsThreshold())
			}
		}
		if tell.Notify {
			ev.Push = "sent"
			if !push {
				ev.Push = "held"
			}
		}
		if tell.Murmur && !characterWantTypes[w.Metadata.Type] {
			s.robotInitiativeNotify(ev)
		}
		if !push {
			return
		}
		title := w.Metadata.Name
		if title == "" {
			title = "MyWant"
		}
		s.sendWantPush(w.Metadata.ID, title, msg, "/w/"+w.Metadata.ID)
		// And to every page open on it — a browser's tab, the app's — which
		// shows it as the device's own notification while it is open: no
		// push service to sign up to, nothing to set but the permission.
		// Kept, for a rating of it (👍 / 👎, notices.go) to find what it was.
		noteNotice(noticeRecord{
			ID: murmurID, WantID: w.Metadata.ID, Want: w.Metadata.Name, WantType: w.Metadata.Type,
			Text: msg, Moment: ev.Moment, Timings: tell.Timings, Labels: labels, News: ev.News,
		})
		broadcastSSE(noticeEvent, wantNotice{
			ID: murmurID, WantID: w.Metadata.ID, WantType: w.Metadata.Type, Title: title, Body: msg,
			Off: mywant.GetDeviceStore().Settings().NoticesOff, Labels: labels,
		})
	}
}

// noticeEvent is the event stream's name for a want's answer it notifies of
// (spec.voice.notify, or its type's say, and not held back by Laya).
const noticeEvent = "notice"

// wantNotice is one such answer, for an open page to show as a notification.
// ID is the same for the same answer, so a device with several pages open
// shows it once.
type wantNotice struct {
	ID     string `json:"id"`
	WantID string `json:"wantId"`
	// WantType: what kind of want said it — a device's notifications of the
	// same kind stack together (the app's thread).
	WantType string `json:"wantType,omitempty"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	// Off: the devices that do not show notices (Devices' 通知, off) — a
	// device finding itself here leaves it.
	Off []string `json:"off,omitempty"`
	// Labels: why it was said (reason, timings, trigger…) and what about.
	Labels map[string]string `json:"labels,omitempty"`
}

// newsVoice is a want's news said in its own voice (mywant.WantNewsVoice) —
// what its type's voice script says to it, and how it feels — or plain, as
// given, when the type has no script or the script has nothing to say. For
// the notice and the push alike: the news is told as the tile would tell it.
func (s *Server) newsVoice(w *mywant.Want, ev mywant.VoiceEvent, plain string) (string, string) {
	line := s.newsVoiceLine(w, ev, plain)
	return line.Say, line.Mood
}

// newsVoiceLine is newsVoice with the labels its voice gave the line (time,
// place — for the wants that hear it).
func (s *Server) newsVoiceLine(w *mywant.Want, ev mywant.VoiceEvent, plain string) mywant.VoiceLine {
	script := ""
	if s.wantTypeLoader != nil {
		script = s.wantTypeLoader.GetVoiceScript(w.GetMetadata().Type)
	}
	line := mywant.WantNewsVoice(context.Background(), w, script, ev)
	if line.Say != "" {
		return line
	}
	// No line of its own: the plain words, still with its labels.
	return mywant.VoiceLine{Say: plain, Labels: line.Labels}
}

// outputMessage says an answer in one line, for the places that can only show
// text — a push notification, the notice log.
//
// A string answer is its own sentence. A structured one says it with the
// fields people name things by, and only falls back to its JSON when it has
// none of them.
func outputMessage(out mywant.ResultHistoryEntry) string {
	switch v := out.Result.(type) {
	case string:
		return truncateRunes(strings.TrimSpace(v), 120)
	case map[string]any:
		for _, k := range []string{"summary", "title", "name", "datetime", "text"} {
			if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
				return truncateRunes(strings.TrimSpace(s), 120)
			}
		}
	case []any:
		return fmt.Sprintf("%d件", len(v))
	}
	// An answer with no words of its own (a position) said by what its want
	// wrote beside it.
	if s, ok := out.Fields["summary"].(string); ok && strings.TrimSpace(s) != "" {
		return truncateRunes(strings.TrimSpace(s), 120)
	}
	b, err := json.Marshal(out.Result)
	if err != nil {
		return "新しい結果"
	}
	return truncateRunes(string(b), 120)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// noteTelling shows in the robot's chat, as a processing line, how a want
// told of its answer — the timings it came at, and whether it murmured and
// notified at them:
//
//	timing want=Spotify at=album murmur=yes notify=no(chosen)
func (s *Server) noteTelling(w *mywant.Want, out mywant.ResultHistoryEntry, tell mywant.Telling) {
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	notify := yes(tell.Notify)
	if tell.NotifyChosen {
		notify += "(chosen)"
	}
	at := strings.Join(tell.Timings, ",")
	if at == "" {
		at = "-"
	}
	s.robotChatTrace("timing", map[string]any{
		"want": w.Metadata.Name, "wantId": w.Metadata.ID, "at": at,
		"murmur": yes(tell.Murmur), "notify": notify,
	}, []string{"want", "at", "murmur", "notify"})
}

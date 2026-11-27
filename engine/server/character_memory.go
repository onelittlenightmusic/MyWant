package server

// character_memory.go — what a character remembers hearing, and how they feel.
//
// A want murmurs when it has news (its last_murmur, said in its own words and
// with its own mood). The characters standing near it hear it. Not everything
// heard is kept: a character remembers only what moved them, and only a few
// things at a time (memorySlots) — the least moving of those forgotten first.
//
// How much a murmur moves a character is the want's mood (worried moves more
// than calm) times how much the want is theirs (one they went to, opened, or
// had the robot make, or were sent to — their story says so) times how near
// they stood. The same delay is news to the one who rides that line and
// nothing to the one who does not, so the same board leaves each a different
// day.
//
// The character's mood is the mood of what moved them most, fading with time
// (memoryMoodHalfLife). Memories fade too (memoryHalfLife), so an old one is
// the first to make room.
//
//	GET /api/v1/characters/{id}/memory   their mood and what they remember
//
// Kept at ~/.mywant/stories/<character>/memory.json; each memory kept or
// forgotten is in their story too (memory.keep, memory.forget).

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	// memorySlots is how many things a character can remember at once.
	memorySlots = 5
	// memoryHearRange: a want this many cells away (or nearer) is heard.
	memoryHearRange = 2
	// memoryMoved: a murmur moving a character less than this is not kept.
	memoryMoved = 0.15
	// memoryHalfLife: how fast a memory fades, for making room.
	memoryHalfLife = 12 * time.Hour
	// memoryMoodHalfLife: how fast a mood fades.
	memoryMoodHalfLife = 2 * time.Hour
	// memoryOwnDays: a want touched this recently is the character's own.
	memoryOwnDays = 7
	// memoryHearDelay: a murmur is weighed this long after it is said. What a
	// character did about the want as it spoke — the robot's errand there —
	// reaches their story only as that finishes, just after the news itself.
	memoryHearDelay = 3 * time.Second

	storyKindMemoryKeep   = "memory.keep"
	storyKindMemoryForget = "memory.forget"
)

// moodWeight is how much a want's mood moves whoever hears it.
var moodWeight = map[string]float64{
	"worried": 0.9, "sad": 0.7, "joy": 0.6, "eager": 0.6, "waiting": 0.4, "calm": 0.2,
}

// Memory is one thing a character remembers hearing.
type Memory struct {
	WantID   string  `json:"wantId"`
	WantName string  `json:"wantName,omitempty"`
	WantType string  `json:"wantType,omitempty"`
	Say      string  `json:"say"`
	Mood     string  `json:"mood"`
	Impact   float64 `json:"impact"`
	At       string  `json:"at"`
	// Where the want was heard from.
	X int `json:"x"`
	Y int `json:"y"`
}

// CharacterMemory is a character's mood and what they remember.
type CharacterMemory struct {
	Character string `json:"character"`
	Mood      string `json:"mood"`
	// Intensity is how strongly, 0..1, faded to now when read.
	Intensity float64  `json:"intensity"`
	MoodAt    string   `json:"moodAt,omitempty"`
	Memories  []Memory `json:"memories"`
}

type characterMemories struct {
	mu   sync.Mutex
	dir  string // "" is the story recorder's root
	now  func() time.Time
	held map[string]*CharacterMemory
}

var memories = &characterMemories{now: time.Now, held: map[string]*CharacterMemory{}}

func (m *characterMemories) root() string {
	if m.dir != "" {
		return m.dir
	}
	return story.root()
}

func (m *characterMemories) path(character string) string {
	return filepath.Join(m.root(), character, "memory.json")
}

// loadLocked is a character's memory, read from disk the first time.
func (m *characterMemories) loadLocked(character string) *CharacterMemory {
	if cm, ok := m.held[character]; ok {
		return cm
	}
	cm := &CharacterMemory{Character: character, Mood: "calm"}
	if data, err := os.ReadFile(m.path(character)); err == nil {
		_ = json.Unmarshal(data, cm)
		cm.Character = character
	}
	m.held[character] = cm
	return cm
}

func (m *characterMemories) saveLocked(cm *CharacterMemory) {
	path := m.path(cm.Character)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// fade is how much of something felt at `at` is left now.
func fade(at string, now time.Time, halfLife time.Duration) float64 {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return 0
	}
	age := now.Sub(t)
	if age <= 0 {
		return 1
	}
	return math.Pow(0.5, float64(age)/float64(halfLife))
}

// hearing is one murmur reaching one character.
type hearing struct {
	character string
	mem       Memory
}

// impactOf is how much a murmur moves someone: the want's mood, how much the
// want is theirs (0..1), how near they stood (cells).
func impactOf(mood string, own float64, dist int) float64 {
	w, ok := moodWeight[mood]
	if !ok {
		w = moodWeight["calm"]
	}
	near := 1.0 - 0.2*float64(dist)
	return w * own * near
}

// hear takes in what one character heard: their mood moves, and it is
// remembered if it moved them enough and there is room — or something less
// moving to forget. Returns what was kept and what was forgotten for it.
func (m *characterMemories) hear(h hearing) (kept bool, forgot *Memory) {
	if h.mem.Impact < memoryMoved {
		return false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	cm := m.loadLocked(h.character)

	// The mood: what moves them more than what they feel now takes over.
	left := cm.Intensity * fade(cm.MoodAt, now, memoryMoodHalfLife)
	if h.mem.Impact >= left {
		cm.Mood, cm.Intensity, cm.MoodAt = h.mem.Mood, h.mem.Impact, h.mem.At
	}

	// The same want again: the same words are not news; new words replace.
	for i, old := range cm.Memories {
		if old.WantID != h.mem.WantID {
			continue
		}
		if old.Say == h.mem.Say {
			m.saveLocked(cm)
			return false, nil
		}
		gone := old
		cm.Memories[i] = h.mem
		m.saveLocked(cm)
		return true, &gone
	}
	if len(cm.Memories) < memorySlots {
		cm.Memories = append(cm.Memories, h.mem)
		m.saveLocked(cm)
		return true, nil
	}
	// Full: the least moving now (faded) makes room, if it is less than this.
	weakest, weakestAt := math.Inf(1), -1
	for i, old := range cm.Memories {
		if v := old.Impact * fade(old.At, now, memoryHalfLife); v < weakest {
			weakest, weakestAt = v, i
		}
	}
	if h.mem.Impact <= weakest {
		m.saveLocked(cm)
		return false, nil
	}
	gone := cm.Memories[weakestAt]
	cm.Memories[weakestAt] = h.mem
	m.saveLocked(cm)
	return true, &gone
}

// get is a character's memory as it is now: the mood faded to now, newest first.
func (m *characterMemories) get(character string) CharacterMemory {
	m.mu.Lock()
	defer m.mu.Unlock()
	cm := m.loadLocked(character)
	out := *cm
	out.Memories = append([]Memory{}, cm.Memories...)
	out.Intensity = cm.Intensity * fade(cm.MoodAt, m.now(), memoryMoodHalfLife)
	if out.Intensity < memoryMoved/2 {
		out.Mood, out.Intensity = "calm", 0
	}
	sort.SliceStable(out.Memories, func(i, j int) bool { return out.Memories[i].At > out.Memories[j].At })
	return out
}

// ── Who hears a murmur ─────────────────────────────────────────────────────

// wantCell is where a want stands on the board, if it does.
func wantCell(w *mywant.Want) (int, int, bool) {
	labels := w.GetLabels()
	x, errX := strconv.Atoi(labels["mywant.io/canvas-x"])
	y, errY := strconv.Atoi(labels["mywant.io/canvas-y"])
	return x, y, errX == nil && errY == nil
}

// characterCell is where a character stands: the robot where its want is,
// anyone else where they were last seen.
func characterCell(c mywant.Character, wants []*mywant.Want) (int, int, bool) {
	if drawnFromOwnWant(c.ID) {
		for _, w := range wants {
			if w.Metadata.ID == c.AuraCardWantID {
				return wantCell(w)
			}
		}
		return 0, 0, false
	}
	return resolveCanvasNear(c.ID, wants)
}

// ownness is how much a want is a character's: theirs when their story has
// them at it lately (stood on, opened, made for them, or — the robot — sent
// to it), a little otherwise.
func ownness(character, wantID string, now time.Time) float64 {
	base := 0.35
	events, err := story.read(character, storyQuery{
		since: now.AddDate(0, 0, -memoryOwnDays), until: now.Add(time.Second),
		kinds: map[string]bool{
			ws.StoryKindWantArrive: true, ws.StoryKindWantOpen: true,
			ws.StoryKindWantExpand: true, ws.StoryKindActDone: true,
			storyKindRobotTool: true,
		},
	})
	if err != nil {
		return base
	}
	for _, ev := range events {
		if ev.To != nil && ev.To.ID == wantID {
			return 1
		}
	}
	return base
}

// hearMurmurNearby lets the characters near a murmuring want hear it.
func (s *Server) hearMurmurNearby(w *mywant.Want, say, mood string) {
	if say == "" || characterWantTypes[w.Metadata.Type] {
		return
	}
	wx, wy, ok := wantCell(w)
	if !ok {
		return
	}
	if mood == "" {
		mood = "calm"
	}
	time.Sleep(memoryHearDelay)
	wants := s.globalBuilder.GetWants()
	now := memories.now()
	for _, c := range mywant.ListCharacters() {
		cx, cy, ok := characterCell(c, wants)
		if !ok {
			continue
		}
		dist := max(absInt(cx-wx), absInt(cy-wy))
		if dist > memoryHearRange {
			continue
		}
		s.heardBy(c.ID, w, say, mood, dist, now)
	}
}

// heardBy is one character hearing a want say something from dist cells
// away: weighed, and remembered if it moved them.
func (s *Server) heardBy(character string, w *mywant.Want, say, mood string, dist int, now time.Time) {
	wx, wy, _ := wantCell(w)
	mem := Memory{
		WantID: w.Metadata.ID, WantName: w.Metadata.Name, WantType: w.Metadata.Type,
		Say: say, Mood: mood, At: now.Format(time.RFC3339), X: wx, Y: wy,
	}
	mem.Impact = math.Round(impactOf(mood, ownness(character, w.Metadata.ID, now), dist)*100) / 100
	kept, forgot := memories.hear(hearing{character: character, mem: mem})
	place := &ws.StoryPlace{Kind: "want", ID: mem.WantID, Name: mem.WantName, Type: mem.WantType, X: &mem.X, Y: &mem.Y}
	if forgot != nil {
		story.add(ws.StoryEvent{Character: character, Kind: storyKindMemoryForget,
			From:   &ws.StoryPlace{Kind: "want", ID: forgot.WantID, Name: forgot.WantName, Type: forgot.WantType},
			Detail: map[string]string{"say": forgot.Say}})
	}
	if kept {
		story.add(ws.StoryEvent{Character: character, Kind: storyKindMemoryKeep, To: place,
			Detail: map[string]string{"say": say, "mood": mood, "impact": strconv.FormatFloat(mem.Impact, 'f', 2, 64)}})
	}
}

// standingWords is what a want says to whoever stands on it and asks — as
// its tile says it on the board: its news not yet read, else the last news
// it murmured, else its type's voice for how it is.
func (s *Server) standingWords(w *mywant.Want) (string, string) {
	if s.notifications != nil {
		if e := s.notifications.LatestUnreadAlert(w.Metadata.ID); e != nil && e.Message != "" {
			mood := e.Mood
			if mood == "" {
				mood = mywant.MoodOf(e.Message)
			}
			return e.Message, mood
		}
	}
	if said, _ := w.GetCurrent("last_murmur"); said != nil {
		if text, ok := said.(string); ok && text != "" {
			mood, _ := w.GetCurrent("mood")
			m, _ := mood.(string)
			if m == "" {
				m = mywant.MoodOf(text)
			}
			return text, m
		}
	}
	script := ""
	if s.wantTypeLoader != nil {
		script = s.wantTypeLoader.GetVoiceScript(w.Metadata.Type)
	}
	line := mywant.WantVoice(context.Background(), w, script)
	return line.Say, line.Mood
}

// hearAsked: a character who asked something of a want, standing at it,
// hears what it says to them — news or not. An errand comes back with what
// it was told, not only with what happened to be new.
func (s *Server) hearAsked(character string, w *mywant.Want) {
	if characterWantTypes[w.Metadata.Type] {
		return
	}
	say, mood := s.standingWords(w)
	if say == "" {
		return
	}
	if mood == "" {
		mood = "calm"
	}
	s.heardBy(character, w, say, mood, 0, memories.now())
}

// getCharacterMemory handles GET /api/v1/characters/{id}/memory.
func (s *Server) getCharacterMemory(w http.ResponseWriter, r *http.Request) {
	character := mux.Vars(r)["id"]
	if !storyCharacterOK.MatchString(character) {
		http.Error(w, "bad character", http.StatusBadRequest)
		return
	}
	writeDoc(w, r, http.StatusOK, memories.get(character))
}

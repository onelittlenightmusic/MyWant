package server

// character_seen.go — the board as one character has seen it today.
//
// GET /api/v1/characters/{id}/seen   ?since= RFC 3339 or a duration (default: today)
//
//	{id, at, place, near, radius, now:{x,y}, seen:[{x,y,at}], tiles:[…], others:[…], tilesSeen, tilesAll}
//
// A character sees the cells around where they stand (seenRadius). Where they
// stood is in their story: the cells they stopped on (canvas.move), the tiles
// they stepped onto and off (want.arrive, want.leave), the wants the robot
// was sent to (robot.tool) — and, between two of them, the way they went (a
// straight line; the steps between stops are not kept). Each cell seen is
// kept with when it was seen last.
//
// Tiles: what is on the board on a seen cell, with what they remember
// hearing from it (its mood then) and whether they see it now. Others: the
// characters they see now — only now, nothing of where others were.

import (
	"net/http"
	"sort"
	"time"

	mywant "mywant/engine/core"

	"github.com/gorilla/mux"
	ws "github.com/onelittlenightmusic/want-spec"
)

// seenRadius is how far a character sees from where they stand, in cells.
const seenRadius = 2

// seenStepsMax: a gap between two positions longer than this is a jump (a
// device picked up elsewhere), not a walk — its way is not filled in.
const seenStepsMax = 40

type SeenCell struct {
	X  int    `json:"x"`
	Y  int    `json:"y"`
	At string `json:"at"`
}

type SeenTile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	// Kind: want or thing.
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	// At: when its cell was seen last. Now: seen from where they stand now.
	At  string `json:"at"`
	Now bool   `json:"now,omitempty"`
	// Mood and Say: what they remember hearing from it, if anything.
	Mood string `json:"mood,omitempty"`
	Say  string `json:"say,omitempty"`
}

type SeenOther struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
	Color  string `json:"color,omitempty"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

type SeenPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type CharacterSeen struct {
	ID     string      `json:"id"`
	Since  string      `json:"since"`
	Radius int         `json:"radius"`
	Now    *SeenPoint  `json:"now,omitempty"`
	Place  string      `json:"place,omitempty"`
	Near   bool        `json:"near,omitempty"`
	Mood   string      `json:"mood"`
	Seen   []SeenCell  `json:"seen"`
	Tiles  []SeenTile  `json:"tiles"`
	Others []SeenOther `json:"others"`
	// TilesSeen of TilesAll on the board.
	TilesSeen int `json:"tilesSeen"`
	TilesAll  int `json:"tilesAll"`
}

// seenMoveKinds: the story events that say where the character stood.
var seenMoveKinds = map[string]bool{
	ws.StoryKindCanvasMove: true, ws.StoryKindWantArrive: true,
	ws.StoryKindWantLeave: true, storyKindRobotTool: true,
}

type seenStop struct {
	x, y int
	at   time.Time
}

// seenStops are where a character stood, oldest first, from their story.
func seenStops(events []ws.StoryEvent) []seenStop {
	var out []seenStop
	add := func(p *ws.StoryPlace, at time.Time) {
		if p == nil || p.X == nil || p.Y == nil {
			return
		}
		if n := len(out); n > 0 && out[n-1].x == *p.X && out[n-1].y == *p.Y {
			out[n-1].at = at
			return
		}
		out = append(out, seenStop{*p.X, *p.Y, at})
	}
	for _, ev := range events {
		if !seenMoveKinds[ev.Kind] {
			continue
		}
		at, err := time.Parse(time.RFC3339, ev.At)
		if err != nil {
			continue
		}
		// Stepping off a tile is standing on it until then.
		add(ev.From, at)
		add(ev.To, at)
	}
	return out
}

// seenCells is each cell seen from the stops and the way between them, with
// when it was seen last.
func seenCells(stops []seenStop, radius int) map[[2]int]time.Time {
	cells := map[[2]int]time.Time{}
	look := func(x, y int, at time.Time) {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				k := [2]int{x + dx, y + dy}
				if at.After(cells[k]) {
					cells[k] = at
				}
			}
		}
	}
	for i, s := range stops {
		if i > 0 {
			p := stops[i-1]
			steps := max(absInt(s.x-p.x), absInt(s.y-p.y))
			if steps > 1 && steps <= seenStepsMax {
				for k := 1; k < steps; k++ {
					x := p.x + (s.x-p.x)*k/steps
					y := p.y + (s.y-p.y)*k/steps
					// On the way: seen as they left the last stop.
					look(x, y, p.at)
				}
			}
		}
		look(s.x, s.y, s.at)
	}
	return cells
}

// getCharacterSeen handles GET /api/v1/characters/{id}/seen.
func (s *Server) getCharacterSeen(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	c, ok := mywant.GetCharacter(id)
	if !ok {
		s.JSONError(w, r, http.StatusNotFound, "Character not found", id)
		return
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	since, ok := storyTime(r.URL.Query().Get("since"), now, today)
	if !ok {
		s.JSONError(w, r, http.StatusBadRequest, "Invalid since", r.URL.Query().Get("since"))
		return
	}
	events, err := story.read(c.ID, storyQuery{since: since, until: now.Add(time.Second), kinds: seenMoveKinds})
	if err != nil {
		s.JSONError(w, r, http.StatusInternalServerError, "Story not read", err.Error())
		return
	}
	stops := seenStops(events)

	out := CharacterSeen{ID: c.ID, Since: since.Format(time.RFC3339), Radius: seenRadius,
		Seen: []SeenCell{}, Tiles: []SeenTile{}, Others: []SeenOther{}}
	m := memories.get(c.ID)
	out.Mood = m.Mood
	var wants []*mywant.Want
	if s.globalBuilder != nil {
		wants = s.globalBuilder.GetWants()
		if x, y, ok := characterCell(*c, wants); ok {
			out.Now = &SeenPoint{x, y}
			stops = append(stops, seenStop{x, y, now})
		}
		out.Place, out.Near = s.placeOf(*c)
	}
	cells := seenCells(stops, seenRadius)
	for k, at := range cells {
		out.Seen = append(out.Seen, SeenCell{k[0], k[1], at.Format(time.RFC3339)})
	}
	sort.Slice(out.Seen, func(i, j int) bool {
		if out.Seen[i].Y != out.Seen[j].Y {
			return out.Seen[i].Y < out.Seen[j].Y
		}
		return out.Seen[i].X < out.Seen[j].X
	})

	seesNow := func(x, y int) bool {
		return out.Now != nil && absInt(x-out.Now.X) <= seenRadius && absInt(y-out.Now.Y) <= seenRadius
	}
	heard := map[string]Memory{}
	for i := len(m.Memories) - 1; i >= 0; i-- {
		heard[m.Memories[i].WantID] = m.Memories[i]
	}
	if tiles, err := s.robotTiles(); err == nil {
		for _, t := range tiles {
			if characterWantTypes[t.typ] {
				continue // a person's mouth, not a place
			}
			out.TilesAll++
			at, ok := cells[[2]int{t.x, t.y}]
			if !ok {
				continue
			}
			out.TilesSeen++
			st := SeenTile{ID: t.id, Name: t.name, Type: t.typ, Kind: t.cardKind, X: t.x, Y: t.y,
				At: at.Format(time.RFC3339), Now: seesNow(t.x, t.y)}
			if h, ok := heard[t.id]; ok {
				st.Mood, st.Say = h.Mood, h.Say
			}
			out.Tiles = append(out.Tiles, st)
		}
	}
	if out.Now != nil {
		for _, o := range mywant.ListCharacters() {
			if o.ID == c.ID {
				continue
			}
			if x, y, ok := characterCell(o, wants); ok && seesNow(x, y) {
				out.Others = append(out.Others, SeenOther{ID: o.ID, Name: o.Name, Avatar: o.Avatar, Color: o.Color, X: x, Y: y})
			}
		}
	}
	s.JSONResponse(w, http.StatusOK, out)
}

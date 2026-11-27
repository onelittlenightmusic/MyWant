package server

// story_robot.go — the robot's story, kept as every character's is.
//
// A person's character is moved by a device (PUT /cursors), and that is where
// their story hears of each step. The robot is moved by the server itself —
// it stands on its own want, and pointing, following and its errands write
// that want's cell — so nothing ever told its story anything, and it had none.
// It moves differently; where it went and what it did are told the same way:
//
//   - where it stands, read off its want every storyTickEvery: a move, a stop
//     (canvas.move), a want stepped onto or off (want.arrive, want.leave),
//     as the intersections tell them for a character;
//   - what it asked of a want — a capability, a want made or brought back, a
//     look — as robot.tool, with the want it was about, from the one place
//     every robot tool runs (robotRunTool), an act's steps and the board's
//     tools alike.
//
// So the wants it was sent to are its own as any character's are
// (character_memory.go's ownness), with no rule of its own.

import (
	"strings"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
)

const (
	storyKindRobotTool = "robot.tool"
)

// robotStoryTools are the tools that do something with a want — what the
// robot was sent to do. point is a walk, told by where it stands.
var robotStoryTools = map[string]bool{
	"use_capability": true, "deploy_want": true, "restore_want": true, "look": true,
}

// robotPeekCard is the want a tool's run named (its card), left in place for
// whoever takes it after.
func robotPeekCard(args map[string]string) *robotCard {
	if _, ok := args[robotCardArg]; !ok {
		return nil
	}
	copied := map[string]string{robotCardArg: args[robotCardArg]}
	return robotTakeCard(copied)
}

// storyRobotTool puts what a robot just did with a want into its story — the
// robot's, or the character's it ran as (robotBodyArg).
func (s *Server) storyRobotTool(name string, args map[string]string, out string) {
	if !robotStoryTools[name] || strings.HasPrefix(out, "Error:") {
		return
	}
	card := robotPeekCard(args)
	if card == nil || card.Kind != "want" || card.ID == "" {
		return
	}
	who := bodyFromArgs(args).Character
	to := s.storyWant(card.ID)
	var asked *mywant.Want
	if s.globalBuilder != nil {
		if w, _, ok := s.globalBuilder.FindWantByID(card.ID); ok && w != nil {
			to = storyWantPlace(w)
			asked = w
		}
	}
	detail := map[string]string{"tool": name, "said": firstLine(out, 160)}
	if c := args["capability"]; c != "" {
		detail["capability"] = c
	}
	story.add(ws.StoryEvent{Character: who, Kind: storyKindRobotTool, To: to, Detail: detail})
	// Asked, it is told: what the want says to it is heard there and then.
	if (name == "use_capability" || name == "look") && asked != nil {
		go s.hearAsked(who, asked)
	}
}

// followRobotStory reads where each robot stands — the robot and every
// character acting as one (robot_body.go) — every storyTickEvery, into its
// story, as a cursor PUT and the intersections do for a character.
func (s *Server) followRobotStory() {
	on := map[string]map[string]*ws.StoryPlace{}
	t := time.NewTicker(storyTickEvery)
	defer t.Stop()
	for range t.C {
		seen := map[string]bool{}
		for _, b := range robotBodies() {
			seen[b.Character] = true
			on[b.Character] = s.robotStoryStep(b, on[b.Character])
		}
		for c := range on {
			if !seen[c] {
				delete(on, c) // a person again: their device tells their steps
			}
		}
	}
}

// robotStoryStep is one reading: a robot's cell to its story, and the wants
// on that cell compared with those it stood on last time. Returns what it
// stands on now.
func (s *Server) robotStoryStep(b robotBody, was map[string]*ws.StoryPlace) map[string]*ws.StoryPlace {
	if s.globalBuilder == nil {
		return was
	}
	wants := s.globalBuilder.GetWants()
	x, y, found := 0, 0, false
	for _, w := range wants {
		if w.Metadata.ID == b.Want || w.Metadata.Name == b.Want {
			x, y, found = wantCell(w)
			break
		}
	}
	if !found {
		return was
	}
	now := map[string]*ws.StoryPlace{}
	for _, w := range wants {
		if characterWantTypes[w.Metadata.Type] || w.Metadata.IsSystemWant {
			continue
		}
		if w.GetLabels()["mywant.io/archived"] == "true" {
			continue
		}
		if wx, wy, ok := wantCell(w); ok && wx == x && wy == y {
			now[w.Metadata.ID] = storyWantPlace(w)
		}
	}
	story.cursor(b.Character, x, y, "", "")
	// The first reading is where it already was, not somewhere it went.
	if was == nil {
		return now
	}
	for id, p := range was {
		if _, still := now[id]; !still {
			story.leave(b.Character, p)
		}
	}
	for id, p := range now {
		if _, before := was[id]; !before {
			story.arrive(b.Character, p)
		}
	}
	return now
}

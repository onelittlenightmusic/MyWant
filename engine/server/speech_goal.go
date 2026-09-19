package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	mywant "mywant/engine/core"
)

// What somebody asks the robot becomes a want.
//
// It used to become a prompt: the sentence went into the robot want's mailbox
// and the on-device agent was left to understand it, choose the commands, fill
// their arguments, notice what came back and stop at the right moment — all of
// it inside a model with an 8k window, and none of it visible afterwards. Asked
// to delete one want, it ran `gui tile set` forty-eight times.
//
// A free_goal want holds the request instead (see the want type of that name).
// The sentence is its parameter, the plan is its state, each command and what it
// printed is kept, and anything that cannot be undone stops and waits for a
// person. The model is still asked what to do next — that is the part it is good
// at — but one short question at a time, and this side decides what runs.
//
// The robot still speaks the answer, because that is where the person is
// looking.

// freeGoalWantType is the type that carries a request said in plain words.
const freeGoalWantType = "free_goal"

// startFreeGoal turns one request into a want on the board, and says where it
// went. An empty id means nothing was started, and the caller says why.
func (s *Server) startFreeGoal(request, askedBy, context string) (string, error) {
	if s.globalBuilder == nil {
		return "", fmt.Errorf("no builder")
	}
	request = strings.TrimSpace(request)
	if request == "" {
		return "", fmt.Errorf("nothing was asked")
	}

	name := fmt.Sprintf("goal-%s", time.Now().Format("150405"))
	want := &mywant.Want{
		Metadata: mywant.Metadata{
			// Its own id, because tracking a want that has none is refused —
			// and the name is already unique to the second it was asked in.
			ID:   name,
			Name: name,
			Type: freeGoalWantType,
			Labels: map[string]string{
				// Beside whoever asked, so the work appears where they are
				// looking rather than wherever the next free cell happened to
				// be. CanvasNearHook turns this into a cell.
				canvasLabelNear: askedBy,
			},
		},
		Spec: mywant.WantSpec{
			Params: map[string]any{
				"request":  request,
				"asked_by": askedBy,
				"context":  context,
			},
		},
	}
	ids, err := s.globalBuilder.AddWantsAsyncWithTracking([]*mywant.Want{want})
	if err != nil {
		return "", err
	}
	id := name
	if len(ids) > 0 {
		id = ids[0]
	}
	log.Printf("[Speech] %s asked for %q — goal %s", askedBy, request, id)
	return id, nil
}

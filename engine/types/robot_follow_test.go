package types

import (
	"strconv"
	"testing"
	"time"

	. "mywant/engine/core"

	"github.com/stretchr/testify/assert"
)

func newFollowingRobot(id string, params map[string]any) *RobotWant {
	return &RobotWant{Want: Want{
		Metadata: Metadata{ID: id, Name: id, Type: "robot", Labels: map[string]string{
			"mywant.io/canvas-x": "0",
			"mywant.io/canvas-y": "0",
		}},
		Spec: WantSpec{Params: params},
	}}
}

func robotCell(w *RobotWant) (int, int) {
	x, _ := strconv.Atoi(w.GetLabel("mywant.io/canvas-x"))
	y, _ := strconv.Atoi(w.GetLabel("mywant.io/canvas-y"))
	return x, y
}

func withTargetAt(t *testing.T, x, y int) {
	t.Helper()
	prev := LocateCharacter
	LocateCharacter = func(string) (int, int, bool) { return x, y, true }
	t.Cleanup(func() { LocateCharacter = prev })
}

func TestRobotFollow_StaysPutWithoutFollow(t *testing.T) {
	withTargetAt(t, 5, 5)
	w := newFollowingRobot("robot-still", map[string]any{"follow_delay_ms": 0, "follow_step_ms": 0})
	for range 5 {
		w.follow()
	}
	x, y := robotCell(w)
	assert.Equal(t, [2]int{0, 0}, [2]int{x, y})
}

func TestRobotFollow_WaitsThenStepsOneCellAtATime(t *testing.T) {
	withTargetAt(t, 3, 1)
	w := newFollowingRobot("robot-follow", map[string]any{
		"follow": "hero", "follow_delay_ms": 40, "follow_step_ms": 0,
	})

	// Out of reach, but the delay has not passed yet: it hesitates.
	w.follow()
	x, y := robotCell(w)
	assert.Equal(t, [2]int{0, 0}, [2]int{x, y})

	time.Sleep(50 * time.Millisecond)
	w.follow()
	x, y = robotCell(w)
	assert.Equal(t, [2]int{1, 1}, [2]int{x, y}, "one diagonal step toward the target")

	w.follow()
	x, y = robotCell(w)
	assert.Equal(t, [2]int{2, 1}, [2]int{x, y})

	// Beside them now: it stops rather than stepping onto their cell.
	w.follow()
	x, y = robotCell(w)
	assert.Equal(t, [2]int{2, 1}, [2]int{x, y})
}

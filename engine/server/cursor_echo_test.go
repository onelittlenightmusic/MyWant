package server

import "testing"

// The ways a cursor position can be contradicted by something that is not
// news, and what stops each of them.

// A broadcast has to say WHICH write put the character where it is, or the
// browser that made that write cannot tell its own echo from somebody else's
// news — which is what made a character pace between two cells on a slow link.
func TestABroadcastSaysWhichWritePlacedThem(t *testing.T) {
	resetCursorState()
	putCursorSeq(t, &Server{}, "chr-a", 3, 4, 100)

	if got := cursors["chr-a"].Seq; got != 100 {
		t.Fatalf("position should carry the seq that set it: got %d, want 100", got)
	}
}

// A PUT the client has already moved past changes nothing about where they
// are, so it must not claim to have placed them there either.
func TestAnOverruledWriteDoesNotClaimThePosition(t *testing.T) {
	resetCursorState()
	s := &Server{}
	putCursorSeq(t, s, "chr-a", 3, 4, 100)
	putCursorSeq(t, s, "chr-a", 9, 9, 50) // sent earlier, arrived later

	e := cursors["chr-a"]
	if e.X != 3 || e.Y != 4 {
		t.Fatalf("stale write moved them: got (%v,%v), want (3,4)", e.X, e.Y)
	}
	if e.Seq != 100 {
		t.Fatalf("stale write claimed the position: seq %d, want 100", e.Seq)
	}
}

// Two devices signed in as one character. The one being played says where the
// character is; the other goes on proving it is alive without overruling that.
func TestOnlyTheDeviceBeingPlayedPlacesTheCharacter(t *testing.T) {
	resetCursorState()
	s := &Server{}
	putCursorFrom(t, s, "chr-a", "laptop", 1, 1, 10) // sitting where it has been
	putCursorFrom(t, s, "chr-a", "phone", 5, 5, 20)  // picked up, walks
	putCursorFrom(t, s, "chr-a", "laptop", 1, 1, 30) // keepalive: same cell as before

	e := cursors["chr-a"]
	if e.X != 5 || e.Y != 5 {
		t.Fatalf("an idle device dragged the character back: got (%v,%v), want (5,5)", e.X, e.Y)
	}
}

// Taking the wheel is done by moving, not by asking.
func TestMovingTakesTheWheel(t *testing.T) {
	resetCursorState()
	s := &Server{}
	putCursorFrom(t, s, "chr-a", "laptop", 1, 1, 10)
	putCursorFrom(t, s, "chr-a", "phone", 5, 5, 20)
	putCursorFrom(t, s, "chr-a", "laptop", 2, 1, 30) // a step, not a keepalive
	putCursorFrom(t, s, "chr-a", "phone", 5, 5, 40)  // now the phone is the idle one

	e := cursors["chr-a"]
	if e.X != 2 || e.Y != 1 {
		t.Fatalf("the device that started walking did not take over: got (%v,%v), want (2,1)", e.X, e.Y)
	}
}

// A move the server made of its own accord is news to everybody, including the
// browser whose character it is — so it is stamped past the shared mark every
// client compares against, without raising that mark and blocking their steps.
func TestAServerMoveIsNewsToTheOwningClient(t *testing.T) {
	resetCursorState()
	putCursorSeq(t, &Server{}, "chr-a", 3, 4, 100)

	cursorsMu.Lock()
	seq := serverAuthoredSeq("chr-a")
	mark := cursorSeq["chr-a"]
	cursorsMu.Unlock()

	if seq <= 100 {
		t.Fatalf("a server move must outrank every seq a client has sent: got %d, want > 100", seq)
	}
	if mark != 100 {
		t.Fatalf("stamping a server move must not block the client's next PUT: mark is %d, want 100", mark)
	}
}

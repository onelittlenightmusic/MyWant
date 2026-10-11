package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBoardChangesAreOneEvent(t *testing.T) {
	ch, cancel := globalSSEHub.subscribe()
	defer cancel()

	boardWantsChanged("w1")
	boardThingsChanged("t1")
	boardThingsChanged("t1", "t2")

	deadline := time.After(2 * time.Second)
	var got []string
	for {
		select {
		case msg := <-ch:
			if !strings.Contains(msg, "board_changed") {
				continue
			}
			got = append(got, msg)
		case <-time.After(400 * time.Millisecond):
			if len(got) != 1 {
				t.Fatalf("want one board_changed for the burst, got %d: %v", len(got), got)
			}
			data := got[0][strings.Index(got[0], "data: ")+6:]
			var c boardChange
			if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &c); err != nil {
				t.Fatalf("decode %q: %v", data, err)
			}
			if strings.Join(c.Wants, ",") != "w1" || strings.Join(c.Things, ",") != "t1,t2" || c.All {
				t.Fatalf("got %+v", c)
			}
			return
		case <-deadline:
			t.Fatal("no event")
		}
	}
}

package types

import (
	"testing"

	. "mywant/engine/core"
)

func TestResolveRobotSubject(t *testing.T) {
	want := func(name, typ string) *Want {
		return &Want{Metadata: Metadata{ID: "id-" + name, Name: name, Type: typ}}
	}
	photo := want("picture-instance", "picture")
	photo2 := want("picture-instance-2", "picture")
	weather := want("weather-nakano", "weather")
	board := []*Want{photo, weather}

	onPhoto := "\n\n(context: the person asking is standing at (9, 7) on the canvas; on that cell: picture-instance (want picture); \"this\"/\"これ\"/\"here\"/\"ここ\" most likely means one of those)"
	besidePhoto := "\n\n(context: the person asking is standing at (8, 7) on the canvas; next to them: 新宿 (thing station), picture-instance (want picture); \"this\"/\"これ\"/\"here\"/\"ここ\" most likely means one of those)"

	cases := []struct {
		name    string
		request string
		wants   []*Want
		expect  *Want
	}{
		{"named", "picture-instanceの写真のスコアは？", board, photo},
		{"named, whatever the case", "What is the score in Picture-Instance?", board, photo},
		{"longest name wins", "picture-instance-2 のスコアは？", []*Want{photo, photo2}, photo2},
		{"only pictures are subjects", "weather-nakanoはどう？", board, nil},
		{"pointed at, standing on it", "この写真のスコアは？" + onPhoto, board, photo},
		{"pointed at, beside it", "これのスコアは？" + besidePhoto, board, photo},
		{"no pointing word, even beside a photo", "新宿はどこ？" + besidePhoto, board, nil},
		{"pointing word but nothing there", "これは何？", board, nil},
		{"the position line's own これ does not count", "スコアは？" + onPhoto, board, nil},
	}
	for _, c := range cases {
		got := resolveRobotSubject(c.request, c.wants)
		if got != c.expect {
			name := func(w *Want) string {
				if w == nil {
					return "nil"
				}
				return w.Metadata.Name
			}
			t.Errorf("%s: got %s, want %s", c.name, name(got), name(c.expect))
		}
	}
}

func TestResolveRobotSubjectTwoPhotosUnderfoot(t *testing.T) {
	a := &Want{Metadata: Metadata{Name: "photo-a", Type: "picture"}}
	b := &Want{Metadata: Metadata{Name: "photo-b", Type: "picture"}}
	req := "これのスコアは？\n\n(context: the person asking is standing at (1, 1) on the canvas; next to them: photo-a (want picture), photo-b (want picture))"
	if got := resolveRobotSubject(req, []*Want{a, b}); got != nil {
		t.Fatalf("two photos beside and a これ should be nobody's guess, got %s", got.Metadata.Name)
	}
}

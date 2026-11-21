package mywant

import "testing"

func TestHeardInbox(t *testing.T) {
	TakeHeard("hearer")
	DeliverHeard("hearer", HeardMurmur{MurmurID: "m1", FromName: "golf"})
	DeliverHeard("hearer", HeardMurmur{MurmurID: "m2", FromName: "golf"})
	w := &Want{Metadata: Metadata{ID: "hearer"}}
	if got := w.Hear(); len(got) != 2 || got[0].MurmurID != "m1" {
		t.Fatalf("heard %+v", got)
	}
	if len(w.Hear()) != 2 {
		t.Fatal("reading used it up")
	}
	w.SayInReply(HeardMurmur{MurmurID: "m2"}, "晴れだよ")
	if s, _ := w.GetStateString("say", ""); s != "晴れだよ" {
		t.Fatalf("say %q", s)
	}
	if r, _ := w.GetStateString(SayReplyToState, ""); r != "m2" {
		t.Fatalf("reply to %q", r)
	}
	if got := TakeHeard("hearer"); len(got) != 2 || len(PeekHeard("hearer")) != 0 {
		t.Fatal("taking did not empty it")
	}
}

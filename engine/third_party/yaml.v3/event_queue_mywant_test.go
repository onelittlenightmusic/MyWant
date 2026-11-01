package yaml

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// mywant patch (PATCHED.md): the emitter's event queue must not keep every
// event of the document once they have been written, and the text must be
// what it always was.
func TestMywantEmitterEventQueueStaysSmall(t *testing.T) {
	doc := make(map[string]any)
	var want strings.Builder
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("k%06d", i)
		doc[key] = map[string]any{"empty": map[string]any{}, "list": []any{i, "x", []any{}}, "name": key}
		fmt.Fprintf(&want, "%s:\n    empty: {}\n    list:\n        - %d\n        - x\n        - []\n    name: %s\n", key, i, key)
	}
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	if err := e.Encode(doc); err != nil {
		t.Fatal(err)
	}
	n := cap(e.encoder.emitter.events)
	e.Close()
	if buf.String() != want.String() {
		t.Fatal("output differs from the expected document")
	}
	if n > initial_queue_size {
		t.Fatalf("event queue grew to %d events, want at most %d", n, initial_queue_size)
	}
}

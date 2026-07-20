package mywant

import (
	"sync"
	"testing"
)

// Reproduces the crash seen in production: GET /api/v1/labels iterating a
// want's labels (GetLabels) while a want goroutine moves it (SetLabel).
// Before the locked mutators this aborted the process with
// "fatal error: concurrent map iteration and map write".
func TestLabelsConcurrentReadWrite(t *testing.T) {
	w := &Want{}
	w.Metadata.Labels = map[string]string{"mywant.io/canvas-x": "0"}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // writer: robot movement
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			w.SetLabel("mywant.io/canvas-x", string(rune('0'+i%10)))
			w.SetLabel("mywant.io/canvas-y", string(rune('0'+i%10)))
			w.DeleteLabel("scratch")
			w.SetLabel("scratch", "v")
		}
	}()

	wg.Add(1)
	go func() { // reader: the /labels handler
		defer wg.Done()
		for i := 0; i < 200000; i++ {
			for range w.GetLabels() {
			}
			_ = w.GetLabel("mywant.io/canvas-x")
		}
		close(stop)
	}()

	wg.Wait()
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// resetModelWork empties the queue and forgets the devices, for one test.
func resetModelWork(t *testing.T) {
	t.Helper()
	modelQueue = &claimQueue[modelClaim]{}
	modelDevicesMu.Lock()
	modelDevices = map[string]modelDevice{}
	modelDevicesMu.Unlock()
	keep := serverHasModel
	t.Cleanup(func() { serverHasModel = keep })
}

func TestModelWorkGoesOnlyToADeviceThatCan(t *testing.T) {
	resetModelWork(t)
	modelQueue.enqueue(modelClaim{RequestID: "g", Op: modelOpGenerate})
	modelQueue.enqueue(modelClaim{RequestID: "d", Op: modelOpDraw, Device: "mac"})

	if _, ok := takeModelWork("old", nil); ok {
		t.Errorf("a poller that says nothing of what it can do was handed model work")
	}
	if _, ok := takeModelWork("phone", parseModelCan("draw")); ok {
		t.Errorf("a drawing meant for the mac was handed to the phone")
	}
	if c, ok := takeModelWork("phone", parseModelCan("generate, draw")); !ok || c.RequestID != "g" {
		t.Errorf("phone took %+v, %v; want the generate job", c, ok)
	}
	if c, ok := takeModelWork("mac", parseModelCan("draw")); !ok || c.RequestID != "d" {
		t.Errorf("mac took %+v, %v; want its drawing", c, ok)
	}
}

func TestModelDeviceArrivesOnceUntilItGoesQuiet(t *testing.T) {
	resetModelWork(t)
	now := time.Now()
	if !noteModelDevice("phone", parseModelCan("generate"), now) {
		t.Errorf("first ask: want arrived")
	}
	if noteModelDevice("phone", parseModelCan("generate"), now.Add(10*time.Second)) {
		t.Errorf("asked again soon after: want not arrived")
	}
	if !noteModelDevice("phone", parseModelCan("generate"), now.Add(10*time.Second+modelDeviceFresh+time.Second)) {
		t.Errorf("asked after going quiet: want arrived")
	}
	if !modelDeviceCan(modelOpGenerate, "", now.Add(20*time.Second+modelDeviceFresh)) {
		t.Errorf("want a device that can generate")
	}
	if modelDeviceCan(modelOpDraw, "", now) {
		t.Errorf("no device said it can draw")
	}
}

// A device that can takes the job and answers through POST model-result.
func TestModelJobDoneByADevice(t *testing.T) {
	resetModelWork(t)
	serverHasModel = func() bool { t.Error("the server's model was asked although a device answered"); return true }
	noteModelDevice("phone", parseModelCan("generate"), time.Now())
	s := &Server{}
	go func() {
		for i := 0; i < 200; i++ {
			if c, ok := takeModelWork("phone", parseModelCan("generate")); ok {
				var in generateInput
				_ = json.Unmarshal(c.Input, &in)
				out, _ := json.Marshal(generateOutput{Text: "read: " + in.Prompt})
				body, _ := json.Marshal(modelResult{RequestID: c.RequestID, Output: out})
				s.modelResultHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	text, by, err := generateText(context.Background(), "score", false, 2*time.Second)
	if err != nil || text != "read: score" || by != modelByDevice {
		t.Fatalf("got %q by %s, %v", text, by, err)
	}
}

// No device answers in time: the job leaves the queue, and the server's own
// model does it — or, with none, nobody does.
func TestModelJobFallsBackToTheServerThenToNone(t *testing.T) {
	resetModelWork(t)
	noteModelDevice("phone", parseModelCan("generate"), time.Now())
	serverHasModel = func() bool { return true }
	here := func(context.Context) (json.RawMessage, error) { return json.Marshal(generateOutput{Text: "by the server"}) }

	out, by, err := runModelJob(context.Background(), modelOpGenerate, generateInput{Prompt: "p"},
		modelJobOptions{Wait: 20 * time.Millisecond, Here: here})
	if err != nil || by != modelByServer || !bytes.Contains(out, []byte("by the server")) {
		t.Fatalf("got %s by %s, %v", out, by, err)
	}
	if _, ok := takeModelWork("phone", parseModelCan("generate")); ok {
		t.Errorf("a job nobody waits for any more was left in the queue")
	}

	serverHasModel = func() bool { return false }
	if _, _, err := runModelJob(context.Background(), modelOpGenerate, generateInput{Prompt: "p"},
		modelJobOptions{Wait: 20 * time.Millisecond, Here: here}); !errors.Is(err, errNoModel) {
		t.Errorf("err = %v, want errNoModel", err)
	}
}

// No device has asked lately: nothing is queued, and the job goes straight to
// the server rather than waiting for nobody.
func TestModelJobSkipsTheQueueWithNoDevice(t *testing.T) {
	resetModelWork(t)
	serverHasModel = func() bool { return true }
	start := time.Now()
	_, by, err := runModelJob(context.Background(), modelOpGenerate, generateInput{Prompt: "p"},
		modelJobOptions{Wait: time.Minute, Here: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"text":"x"}`), nil }})
	if err != nil || by != modelByServer || time.Since(start) > time.Second {
		t.Fatalf("by %s, %v, after %v", by, err, time.Since(start))
	}
}

// pending-action hands model work to a poller that can, home browser or not,
// and other work as before.
func TestPendingActionHandsModelWork(t *testing.T) {
	resetModelWork(t)
	s := &Server{}
	modelQueue.enqueue(modelClaim{RequestID: "g", Op: modelOpGenerate, Input: json.RawMessage(`{"prompt":"p"}`)})
	w := httptest.NewRecorder()
	s.pendingBrowserAction(w, httptest.NewRequest(http.MethodGet, "/?device=phone&can=generate", nil))
	var got pendingActionResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Kind != "model" || got.Model == nil || got.Model.RequestID != "g" || got.Model.Op != modelOpGenerate {
		t.Fatalf("got %s", w.Body.String())
	}
}

// only=model: nothing but model work, even for the home browser.
func TestPendingActionOnlyModel(t *testing.T) {
	resetModelWork(t)
	s := &Server{}
	navLaunchQueue = &claimQueue[navLaunchClaim]{}
	t.Cleanup(func() { navLaunchQueue = &claimQueue[navLaunchClaim]{} })
	navLaunchQueue.enqueue(navLaunchClaim{TargetURL: "https://anyone.example/"})
	w := httptest.NewRecorder()
	s.pendingBrowserAction(w, httptest.NewRequest(http.MethodGet, "/?device=phone&can=generate&only=model", nil))
	var got pendingActionResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Kind != "" {
		t.Fatalf("got %s, want nothing", w.Body.String())
	}
	if _, ok := navLaunchQueue.dequeue(); !ok {
		t.Errorf("the tab asked for was taken by a poller that only wanted model work")
	}
}

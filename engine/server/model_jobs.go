package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	types "mywant/engine/types"
)

// Model work — a job for a language model (or Apple's picture and text
// recognisers beside it), done by whichever device has one, else by this
// server.
//
// A Mac's Chrome with the MyWant extension reaches fmtool (Apple's model on
// that Mac); the iPhone app has Apple's model on the phone; this server has
// one only when it runs on a Mac with fmtool — never on Fly. What the model is
// asked, and what becomes of its answer, is worked out here either way, as for
// the robot (fm.go): a device only runs the model on what it is handed.
//
// The queue is the web errands' (handlers_web_wants.go): the same claimQueue,
// handed out by the same polled GET /api/v1/web-wants/pending-action — kind
// "model" — with the same "pending_action" nudge on the event stream, and the
// answer posted back as a web errand's is (POST /web-wants/model-result).
// Two things are its own:
//
//   - A poller says what it can do (pending-action?can=generate,draw), and is
//     handed only work it can do. A poller that says nothing — every one from
//     before this — is handed none.
//   - No home browser. The home rule is about where a tab opens, so a person
//     can see it; model work shows nothing anywhere, and a phone that is not
//     the home browser has the same model as one that is. A job meant for one
//     device (Device: drawing, where the person who asked picks the picture)
//     goes to that device only.
//
// The order (runModelJob): a device that can, if one has asked for work
// lately — it gets the job and the caller waits for its answer, up to `Wait`;
// else, or when none answered in time, this server's own model; else none,
// and the caller does what it did before there was any of this (Claude, or
// saying it cannot).

// The ops a device may do. generate: {prompt, warm} → {text}.
const (
	modelOpGenerate = "generate"
	modelOpReadText = "read-text"
	modelOpLook     = "look"
	modelOpConverse = "converse"
	modelOpDraw     = "draw"
)

// modelClaim is one job waiting for a device.
type modelClaim struct {
	RequestID string          `json:"request_id"`
	Op        string          `json:"op"`
	Input     json.RawMessage `json:"input"`
	// Device, when set: only this device is handed it.
	Device string `json:"-"`
}

// modelResult is what a device posts back, and what the waiter is woken with.
type modelResult struct {
	RequestID string          `json:"request_id"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
}

var modelQueue = &claimQueue[modelClaim]{}

var (
	modelPendingMu sync.Mutex
	modelPending   = map[string]chan modelResult{}
)

// modelDevices remembers, per device, what it said it can do and when it last
// asked — so a job is queued only while some device could take it, rather
// than waited on for nothing.
var (
	modelDevicesMu sync.Mutex
	modelDevices   = map[string]modelDevice{}
)

type modelDevice struct {
	can map[string]bool
	at  time.Time
}

// modelDeviceFresh is how long a device counts as there after it last asked:
// the extension asks on every nudge and on its minute alarm, the phone every
// ten seconds while the app is in front.
const modelDeviceFresh = 2 * time.Minute

// noteModelDevice records a poller's can, and says whether it is a device
// that can do something and had not asked lately — work already waiting may
// now have someone to do it.
func noteModelDevice(device string, can map[string]bool, now time.Time) (arrived bool) {
	if len(can) == 0 {
		return false
	}
	if device == "" {
		device = "anonymous"
	}
	modelDevicesMu.Lock()
	defer modelDevicesMu.Unlock()
	prev, seen := modelDevices[device]
	modelDevices[device] = modelDevice{can: can, at: now}
	return !seen || now.Sub(prev.at) > modelDeviceFresh
}

// modelDeviceCan says whether a device that can do op has asked lately — when
// device is set, that one.
func modelDeviceCan(op, device string, now time.Time) bool {
	modelDevicesMu.Lock()
	defer modelDevicesMu.Unlock()
	for id, d := range modelDevices {
		if device != "" && id != device {
			continue
		}
		if d.can[op] && now.Sub(d.at) <= modelDeviceFresh {
			return true
		}
	}
	return false
}

// parseModelCan reads pending-action's can=generate,draw.
func parseModelCan(raw string) map[string]bool {
	out := map[string]bool{}
	for _, op := range strings.Split(raw, ",") {
		if op = strings.TrimSpace(op); op != "" {
			out[op] = true
		}
	}
	return out
}

// takeModelWork hands a poller the oldest job it can do: one meant for it, or
// one meant for anyone.
func takeModelWork(device string, can map[string]bool) (modelClaim, bool) {
	if len(can) == 0 {
		return modelClaim{}, false
	}
	return modelQueue.dequeueWhere(func(c modelClaim) bool {
		return can[c.Op] && (c.Device == "" || c.Device == device)
	})
}

// modelJobOptions is how long to wait for a device, and what this server does
// itself when none takes the job.
type modelJobOptions struct {
	// Wait: how long a device has to answer once the job is queued — short for
	// a person waiting on it, longer for work in the background.
	Wait time.Duration
	// Device: only this device may do it.
	Device string
	// Here: this server doing the job with its own model; nil when it cannot
	// do this op. Asked only when serverHasModel.
	Here func(ctx context.Context) (json.RawMessage, error)
}

// errNoModel: neither a device nor this server could do the job.
var errNoModel = errors.New("no model to do this: no device that can has asked for work lately, and this server has none")

// modelBy says who did a job, for the log.
type modelBy string

const (
	modelByDevice modelBy = "device"
	modelByServer modelBy = "server"
)

// runModelJob does a job by the order above, and says who did it. An error
// from a device that took the job is that device's; it is not done again here,
// as a web errand that failed in a browser is not run again elsewhere.
func runModelJob(ctx context.Context, op string, input any, opts modelJobOptions) (json.RawMessage, modelBy, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, "", err
	}
	if modelDeviceCan(op, opts.Device, time.Now()) {
		res, ok := waitForModelDevice(ctx, modelClaim{RequestID: generateWantID(), Op: op, Input: raw, Device: opts.Device}, opts.Wait)
		if ok {
			if res.Error != "" {
				return nil, modelByDevice, fmt.Errorf("%s", res.Error)
			}
			return res.Output, modelByDevice, nil
		}
	}
	if opts.Here != nil && serverHasModel() {
		out, err := opts.Here(ctx)
		return out, modelByServer, err
	}
	return nil, "", errNoModel
}

// waitForModelDevice queues a job and waits for a device's answer; false when
// none came in time — the job is then taken out of the queue, so no device
// does it after nobody is waiting for it.
func waitForModelDevice(ctx context.Context, claim modelClaim, wait time.Duration) (modelResult, bool) {
	ch := make(chan modelResult, 1)
	modelPendingMu.Lock()
	modelPending[claim.RequestID] = ch
	modelPendingMu.Unlock()
	modelQueue.enqueue(claim)
	go broadcastSSE("pending_action", nil)

	gone := func() {
		modelQueue.dequeueWhere(func(c modelClaim) bool { return c.RequestID == claim.RequestID })
		modelPendingMu.Lock()
		delete(modelPending, claim.RequestID)
		modelPendingMu.Unlock()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case res := <-ch:
		return res, true
	case <-timer.C:
		gone()
		return modelResult{}, false
	case <-ctx.Done():
		gone()
		return modelResult{}, false
	}
}

// serverHasModel: this server has its own model (fmtool on a Mac). Turned off
// with MYWANT_SERVER_MODEL=off, to have devices do everything — for trying
// the devices' half on a Mac that could do it all itself. A variable so tests
// can say either.
var serverHasModel = func() bool {
	if os.Getenv("MYWANT_SERVER_MODEL") == "off" {
		return false
	}
	return types.HasOnDeviceModel()
}

// generateInput and generateOutput are op generate's.
type generateInput struct {
	Prompt string `json:"prompt"`
	Warm   bool   `json:"warm,omitempty"`
}

type generateOutput struct {
	Text string `json:"text"`
}

// generateText runs op generate: a prompt in, the model's words out — a
// device's model, else this server's (fmtool, plain).
func generateText(ctx context.Context, prompt string, warm bool, wait time.Duration) (string, modelBy, error) {
	out, by, err := runModelJob(ctx, modelOpGenerate, generateInput{Prompt: prompt, Warm: warm}, modelJobOptions{
		Wait: wait,
		Here: func(context.Context) (json.RawMessage, error) {
			text, ok := types.AskPlain(prompt, warm)
			if !ok {
				return nil, errors.New("this server's model did not answer")
			}
			return json.Marshal(generateOutput{Text: text})
		},
	})
	if err != nil {
		return "", by, err
	}
	var g generateOutput
	if err := json.Unmarshal(out, &g); err != nil {
		return "", by, fmt.Errorf("generate: %v", err)
	}
	return g.Text, by, nil
}

// modelResultHandler is POST /api/v1/web-wants/model-result — a device's
// answer to a job it took. Wakes the job's waiter, if it is still waiting; a
// late answer, after the waiter gave up, is dropped.
func (s *Server) modelResultHandler(w http.ResponseWriter, r *http.Request) {
	var res modelResult
	if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	modelPendingMu.Lock()
	ch, ok := modelPending[res.RequestID]
	if ok {
		delete(modelPending, res.RequestID)
	}
	modelPendingMu.Unlock()
	if ok {
		ch <- res
	}
	s.JSONResponse(w, http.StatusOK, map[string]bool{"delivered": ok})
}

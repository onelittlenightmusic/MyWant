package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	mywant "mywant/engine/core"
)

// A token budget for GET responses.
//
// A reader that can only take so much — the robot's model on an iPhone has
// 4,096 tokens for everything — asks with ?token-limit=4k, and the response is
// cut down until it fits: the parts that weigh most and mean least go first.
// The order comes from measuring real boards (docs/payload-profile.md): a
// want's history is two thirds of it; bookkeeping, display hints and the
// framework's own state fields come next; a type's description and each
// value's meaning stay to the end.
//
// Deciding how far to cut does not measure anything on the request. Every
// ordinary GET that hands out wants or want types is measured afterwards, off
// the request path, into a ledger (budgetLedger): for each want and type, how
// many tokens it comes to after each stage. A request with a limit reads the
// ledger, picks the first stage at which the whole response fits, and cuts
// once. Only an object the ledger has never seen is measured there and then.

// budgetStage is one step of cutting, applied to every want and want type in
// a response. Stages apply cumulatively, in the order of budgetStages.
type budgetStage struct {
	name  string
	apply func(m map[string]any, kind string)
}

var budgetStages = []budgetStage{
	// What the want has been, not what it is: two thirds of an average want.
	{"history", func(m map[string]any, kind string) {
		if kind == "want" {
			delete(m, "history")
		}
	}},
	// How the system keeps it.
	{"bookkeeping", func(m map[string]any, kind string) {
		switch kind {
		case "want":
			for _, k := range []string{"state_timestamps", "connectivity_metadata", "hash", "exposable_fields", "hidden_state"} {
				delete(m, k)
			}
			if md, _ := m["metadata"].(map[string]any); md != nil {
				for _, k := range []string{"correlation", "series", "updatedAt", "createdAt"} {
					delete(md, k)
				}
			}
		case "type":
			for _, k := range []string{"source", "connectivity", "monitorCapabilities"} {
				delete(m, k)
			}
		}
	}},
	// How a GUI draws it: colours, gradients, icons, where its tile stands.
	{"display", func(m map[string]any, _ string) {
		md, _ := m["metadata"].(map[string]any)
		if md == nil {
			return
		}
		labels, _ := md["labels"].(map[string]any)
		for k := range labels {
			if budgetDisplayLabel(k) {
				delete(labels, k)
			}
		}
	}},
	// The fields the framework puts in every want and every type: the same
	// everywhere, so they say nothing about this one.
	{"shared-state", func(m map[string]any, kind string) {
		shared := budgetSharedState()
		switch kind {
		case "want":
			if st, _ := m["state"].(map[string]any); st != nil {
				if cur, _ := st["current"].(map[string]any); cur != nil {
					for k := range shared {
						delete(cur, k)
					}
				}
			}
		case "type":
			if defs, ok := m["state"].([]any); ok {
				kept := defs[:0]
				for _, d := range defs {
					if dm, _ := d.(map[string]any); dm != nil && shared[fmt.Sprint(dm["name"])] {
						continue
					}
					kept = append(kept, d)
				}
				m["state"] = kept
			}
		}
	}},
	// How a type is made and run, as opposed to what it is.
	{"how-made", func(m map[string]any, kind string) {
		switch kind {
		case "type":
			for _, k := range []string{"examples", "onInitialize", "constraints", "agents", "inlineAgents",
				"finalizeWhen", "requires", "require", "relatedTypes", "seeAlso"} {
				delete(m, k)
			}
			budgetKeepFields(m["state"], "name", "description", "type")
			budgetKeepFields(m["parameters"], "name", "description", "type", "required", "example")
		case "want":
			if spec, _ := m["spec"].(map[string]any); spec != nil {
				for k := range spec {
					if k != "params" {
						delete(spec, k)
					}
				}
			}
		}
	}},
	// Long single values: a chat record, a recognized page. Cut, not dropped.
	{"long-values", func(m map[string]any, _ string) {
		for k, v := range m {
			m[k] = budgetTruncate(v, 1000, 20)
		}
	}},
	{"short-values", func(m map[string]any, _ string) {
		for k, v := range m {
			m[k] = budgetTruncate(v, 200, 5)
		}
	}},
}

func budgetStageNames() []string {
	out := make([]string, len(budgetStages))
	for i, s := range budgetStages {
		out[i] = s.name
	}
	return out
}

// budgetDisplayLabel: a label that only tells a GUI how to draw.
func budgetDisplayLabel(k string) bool {
	for _, p := range []string{"category-bg", "category-icon", "type-icon", "tile-", "mywant.io/canvas", "form-style"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	for _, s := range []string{"-icon", "-icons", "-color", "-colour", "-bg", "-bg-light", "-bg-dark"} {
		if strings.HasSuffix(k, s) {
			return true
		}
	}
	return false
}

func budgetSharedState() map[string]bool {
	out := map[string]bool{"final_result": true, "say": true}
	for _, f := range mywant.SystemReservedStateFields() {
		out[f] = true
	}
	return out
}

// budgetKeepFields leaves only the named keys in every object of a list.
func budgetKeepFields(v any, keep ...string) {
	list, _ := v.([]any)
	for _, item := range list {
		m, _ := item.(map[string]any)
		for k := range m {
			if !containsString(keep, k) {
				delete(m, k)
			}
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// budgetTruncate shortens every string past maxRunes and every list past
// maxItems, at any depth, saying how much was left out.
func budgetTruncate(v any, maxRunes, maxItems int) any {
	switch x := v.(type) {
	case string:
		if utf8.RuneCountInString(x) > maxRunes {
			r := []rune(x)
			return string(r[:maxRunes]) + "…"
		}
		return x
	case []any:
		if len(x) > maxItems {
			cut := make([]any, 0, maxItems+1)
			for _, c := range x[:maxItems] {
				cut = append(cut, budgetTruncate(c, maxRunes, maxItems))
			}
			return append(cut, fmt.Sprintf("… %d more", len(x)-maxItems))
		}
		for i, c := range x {
			x[i] = budgetTruncate(c, maxRunes, maxItems)
		}
		return x
	case map[string]any:
		for k, c := range x {
			x[k] = budgetTruncate(c, maxRunes, maxItems)
		}
		return x
	}
	return v
}

// estimateTokens is what a language model makes of JSON: measured on the
// robot's model, about 2.8 characters a token for ASCII, and a token a
// character for anything else (Japanese) — on the safe side.
func estimateTokens(b []byte) int {
	ascii, other := 0, 0
	for _, r := range string(b) {
		if r < 128 {
			ascii++
		} else {
			other++
		}
	}
	return int(float64(ascii)/2.8+0.999) + other
}

// ── what in a response is a want, or a want type ─────────────────────────────

type budgetObject struct {
	m    map[string]any
	kind string // "want" | "type"
	key  string
	fp   string // what says it has not changed since it was measured
}

func budgetKind(m map[string]any) string {
	md, ok := m["metadata"].(map[string]any)
	if !ok {
		return ""
	}
	if _, ok := m["spec"]; ok {
		if _, ok := md["type"]; ok {
			return "want"
		}
	}
	if _, ok := m["state"].([]any); ok {
		return "type"
	}
	if _, ok := m["parameters"]; ok {
		return "type"
	}
	return ""
}

// budgetObjects finds every want and want type in a decoded response, not
// looking inside one once it is found.
func budgetObjects(v any) []budgetObject {
	var out []budgetObject
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if kind := budgetKind(x); kind != "" {
				md := x["metadata"].(map[string]any)
				o := budgetObject{m: x, kind: kind}
				if kind == "want" {
					o.key = "want:" + fmt.Sprint(md["id"])
					o.fp = fmt.Sprint(x["hash"])
				} else {
					o.key = "type:" + fmt.Sprint(md["name"])
					if src, _ := x["source"].(map[string]any); src != nil {
						o.fp = fmt.Sprint(src["digest"], src["commit"])
					}
					o.fp += fmt.Sprint(md["version"])
				}
				if o.fp == "" || o.fp == "<nil>" {
					raw, _ := json.Marshal(x)
					o.fp = strconv.Itoa(len(raw))
				}
				out = append(out, o)
				return
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
	return out
}

// budgetMeasure is an object's size, in tokens, untouched and after each stage.
func budgetMeasure(o budgetObject) []int {
	raw, _ := json.Marshal(o.m)
	sizes := []int{estimateTokens(raw)}
	var copyM map[string]any
	if err := json.Unmarshal(raw, &copyM); err != nil {
		return sizes
	}
	for _, st := range budgetStages {
		st.apply(copyM, o.kind)
		b, _ := json.Marshal(copyM)
		sizes = append(sizes, estimateTokens(b))
	}
	return sizes
}

// ── the ledger: what each object measured to ───────────────────────────────────

type ledgerEntry struct {
	Kind        string    `json:"kind"`
	Name        string    `json:"name,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Tokens      []int     `json:"tokens"` // untouched, then after each stage
	MeasuredAt  time.Time `json:"measured_at"`
}

type budgetLedgerStore struct {
	mu      sync.Mutex
	entries map[string]ledgerEntry
	dirty   bool
	path    string
	once    sync.Once

	queue    chan []byte
	lastSeen map[string]time.Time
}

var budgetLedger = &budgetLedgerStore{
	entries:  map[string]ledgerEntry{},
	queue:    make(chan []byte, 2),
	lastSeen: map[string]time.Time{},
}

// budgetRecordEvery is how often one path's responses are measured at most:
// the GUI polls the want list every few seconds, and nothing in it changes
// that fast that matters to a budget.
const budgetRecordEvery = 30 * time.Second

func (l *budgetLedgerStore) start() {
	l.once.Do(func() {
		l.path = thingPath("payload-ledger.json")
		if raw, err := os.ReadFile(l.path); err == nil {
			_ = json.Unmarshal(raw, &l.entries)
		}
		go l.work()
		go l.persist()
	})
}

// work measures queued responses, one at a time, off every request's path.
func (l *budgetLedgerStore) work() {
	for body := range l.queue {
		var v any
		if json.Unmarshal(body, &v) != nil {
			continue
		}
		for _, o := range budgetObjects(v) {
			l.lookup(o)
		}
	}
}

func (l *budgetLedgerStore) persist() {
	for range time.Tick(time.Minute) {
		l.mu.Lock()
		if !l.dirty {
			l.mu.Unlock()
			continue
		}
		raw, err := json.Marshal(l.entries)
		l.dirty = false
		l.mu.Unlock()
		if err == nil {
			if err := os.WriteFile(l.path, raw, 0o644); err != nil {
				log.Printf("[budget] writing the ledger: %v", err)
			}
		}
	}
}

// lookup is the object's measurement: the ledger's, if it has measured this
// version of it; otherwise measured now and kept.
func (l *budgetLedgerStore) lookup(o budgetObject) []int {
	l.mu.Lock()
	e, ok := l.entries[o.key]
	l.mu.Unlock()
	if ok && e.Fingerprint == o.fp && len(e.Tokens) == len(budgetStages)+1 {
		return e.Tokens
	}
	tokens := budgetMeasure(o)
	name := ""
	if md, _ := o.m["metadata"].(map[string]any); md != nil {
		name = fmt.Sprint(md["name"])
	}
	l.mu.Lock()
	l.entries[o.key] = ledgerEntry{Kind: o.kind, Name: name, Fingerprint: o.fp, Tokens: tokens, MeasuredAt: time.Now()}
	l.dirty = true
	l.mu.Unlock()
	return tokens
}

// offer hands a response to the worker, unless this path was measured lately
// or the worker is busy — then this one is simply not measured.
func (l *budgetLedgerStore) offer(path string, body []byte) {
	l.mu.Lock()
	if time.Since(l.lastSeen[path]) < budgetRecordEvery {
		l.mu.Unlock()
		return
	}
	l.lastSeen[path] = time.Now()
	l.mu.Unlock()
	select {
	case l.queue <- body:
	default:
	}
}

// ── cutting a response to a budget ─────────────────────────────────────────────

type budgetResult struct {
	Estimate int      // tokens, as cut
	Stages   []string // the stages applied, plus "items" if a list was shortened
	Fits     bool
}

// budgetFit cuts a decoded response to limit tokens, in place where it can,
// and returns it with what was done.
func budgetFit(v any, limit int) (any, budgetResult) {
	raw, _ := json.Marshal(v)
	total := estimateTokens(raw)
	if total <= limit {
		return v, budgetResult{Estimate: total, Fits: true}
	}
	objs := budgetObjects(v)
	measured := make([][]int, len(objs))
	for i, o := range objs {
		measured[i] = budgetLedger.lookup(o)
	}
	// The first stage at which the ledger says the whole response fits.
	k := len(budgetStages)
	for stage := 1; stage <= len(budgetStages); stage++ {
		saved := 0
		for _, t := range measured {
			if stage < len(t) {
				saved += t[0] - t[stage]
			}
		}
		if total-saved <= limit {
			k = stage
			break
		}
	}
	res := budgetResult{}
	apply := func(stage int) {
		for _, o := range objs {
			budgetStages[stage].apply(o.m, o.kind)
		}
		res.Stages = append(res.Stages, budgetStages[stage].name)
	}
	for stage := 0; stage < k; stage++ {
		apply(stage)
	}
	raw, _ = json.Marshal(v)
	res.Estimate = estimateTokens(raw)
	// The ledger guessed short (the objects changed since): one stage more at a time.
	for res.Estimate > limit && k < len(budgetStages) {
		apply(k)
		k++
		raw, _ = json.Marshal(v)
		res.Estimate = estimateTokens(raw)
	}
	// Still over: a list of many — keep its first items.
	if res.Estimate > limit {
		if cut, ok := budgetShortenList(v, limit); ok {
			v = cut
			res.Stages = append(res.Stages, "items")
			raw, _ = json.Marshal(v)
			res.Estimate = estimateTokens(raw)
		}
	}
	res.Fits = res.Estimate <= limit
	return v, res
}

// budgetShortenList keeps the head of the largest list at the top of a
// response (the "wants" of GET /wants), as long as will fit, at least one.
func budgetShortenList(v any, limit int) (any, bool) {
	set := func(list []any) any { return list }
	var list []any
	switch x := v.(type) {
	case []any:
		list = x
	case map[string]any:
		best := ""
		for k, c := range x {
			if l, ok := c.([]any); ok && len(l) > len(list) {
				best, list = k, l
			}
		}
		if best != "" {
			set = func(l []any) any { x[best] = l; return x }
		}
	}
	if len(list) < 2 {
		return v, false
	}
	n := len(list)
	for n > 1 {
		n = n * 3 / 4
		if n < 1 {
			n = 1
		}
		out := set(list[:n])
		raw, _ := json.Marshal(out)
		if estimateTokens(raw) <= limit {
			return out, true
		}
	}
	return set(list[:1]), true
}

// parseTokenLimit reads ?token-limit= (or token_limit): 4096, 4k or 4K.
func parseTokenLimit(r *http.Request) (int, bool) {
	q := r.URL.Query()
	raw := q.Get("token-limit")
	if raw == "" {
		raw = q.Get("token_limit")
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, false
	}
	mult := 1
	if strings.HasSuffix(raw, "k") {
		mult, raw = 1024, strings.TrimSuffix(raw, "k")
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return int(n * float64(mult)), true
}

// ── the middleware ───────────────────────────────────────────────────────────────

// budgetRecordPath: the GETs whose responses hold wants or want types.
func budgetRecordPath(p string) bool {
	return strings.HasPrefix(p, "/api/v1/wants") || strings.HasPrefix(p, "/api/v1/want-types")
}

// budgetMaxRecord: a response bigger than this is not kept to be measured.
const budgetMaxRecord = 4 << 20

type budgetWriter struct {
	http.ResponseWriter
	buf    bytes.Buffer
	status int
	hold   bool // a limit: hold everything, write it cut
	over   bool // recording, and it grew past budgetMaxRecord
}

func (w *budgetWriter) WriteHeader(code int) {
	w.status = code
	if !w.hold {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *budgetWriter) Write(b []byte) (int, error) {
	if w.hold {
		return w.buf.Write(b)
	}
	if !w.over {
		if w.buf.Len()+len(b) > budgetMaxRecord {
			w.over = true
			w.buf.Reset()
		} else {
			w.buf.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// tokenBudgetMiddleware cuts a GET's response to ?token-limit= when asked,
// and otherwise passes it through, measuring want and type responses into the
// ledger afterwards.
func tokenBudgetMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		budgetLedger.start()
		limit, limited := parseTokenLimit(r)
		if !limited && !budgetRecordPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		bw := &budgetWriter{ResponseWriter: w, status: http.StatusOK, hold: limited}
		next.ServeHTTP(bw, r)

		if !limited {
			if !bw.over && bw.status == http.StatusOK && bw.buf.Len() > 0 {
				budgetLedger.offer(r.URL.Path, append([]byte(nil), bw.buf.Bytes()...))
			}
			return
		}

		body := bw.buf.Bytes()
		isJSON := strings.Contains(w.Header().Get("Content-Type"), "json")
		var v any
		if bw.status != http.StatusOK || !isJSON || json.Unmarshal(body, &v) != nil {
			w.WriteHeader(bw.status)
			_, _ = w.Write(body)
			return
		}
		cut, res := budgetFit(v, limit)
		out, _ := json.Marshal(cut)
		h := w.Header()
		h.Del("Content-Length")
		h.Del("ETag") // the cut body is not the one the tag names
		h.Set("X-Token-Limit", strconv.Itoa(limit))
		h.Set("X-Token-Estimate", strconv.Itoa(res.Estimate))
		h.Set("X-Token-Trimmed", strings.Join(res.Stages, ","))
		h.Set("X-Token-Fits", strconv.FormatBool(res.Fits))
		w.WriteHeader(bw.status)
		_, _ = w.Write(out)
	})
}

// ── reading the ledger ──────────────────────────────────────────────────────────

// GET /api/v1/payload-ledger
// What every want and type measured to, heaviest first, and the stages the
// sizes are after.
func (s *Server) getPayloadLedger(w http.ResponseWriter, _ *http.Request) {
	budgetLedger.start()
	budgetLedger.mu.Lock()
	type row struct {
		Key string `json:"key"`
		ledgerEntry
	}
	rows := make([]row, 0, len(budgetLedger.entries))
	for k, e := range budgetLedger.entries {
		rows = append(rows, row{k, e})
	}
	budgetLedger.mu.Unlock()
	sort.Slice(rows, func(i, j int) bool {
		if len(rows[i].Tokens) == 0 || len(rows[j].Tokens) == 0 {
			return len(rows[i].Tokens) > len(rows[j].Tokens)
		}
		return rows[i].Tokens[0] > rows[j].Tokens[0]
	})
	s.JSONResponse(w, http.StatusOK, map[string]any{
		"stages":  append([]string{"untouched"}, budgetStageNames()...),
		"entries": rows,
	})
}

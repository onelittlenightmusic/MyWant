package server

// tools_capability.go — what a want can answer beyond its own result.
//
// A want type says it in its YAML (capabilities): each in the person's words
// ("最新のゴルフのスコアを検索できる"), for the robot to choose by, and how it
// is done — a prompt over data the server gathers (reads: answers,
// answer_photos, state), or a Machine-Readable Skill of the type's own. The
// robot's model only chooses one, by its description; this side does it: it
// gathers exactly the data named and nothing else, or runs exactly the skill
// named, inside the type's own directory, and hands what came back — with the
// type's prompt — to the model that words the answer. So the knowledge of how
// a want's photos are read, or which skill finds what, is the want type's, and
// the robot's instructions know none of it.
//
// One tool for the device's tool loop (use_capability, its choices the board's
// capabilities as they are now), and the same step for the plan (plan.go:
// a capability is chosen as a ref, "want#capability").

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "embed"

	"gopkg.in/yaml.v3"

	mywant "mywant/engine/core"
)

const (
	// fmCapSep joins a want's name and its capability's into the choice.
	fmCapSep = "#"
	// How long a capability's skill may run.
	fmCapSkillTimeout = 90 * time.Second
	// How much of each kind of data a prompt is given.
	fmCapAnswers = 6
	fmCapPhotos  = 4
	fmCapLines   = 12
)

// fmStdCap is a standard capability: one every want has, done the same way
// for every type — so its type need not say it, and a person can ask any want
// for it (「smartgolfを再実行」「再チェック」「最新を取得」). They are kept in
// standard_capabilities.yaml, beside this file.
type fmStdCap struct {
	mywant.WantCapabilityDef `yaml:",inline"`
	// Action is an operation of the server's own (fmStdActions), done instead
	// of reading data.
	Action string `yaml:"action,omitempty"`
}

//go:embed standard_capabilities.yaml
var fmStdCapsYAML []byte

// fmStdActions are the operations a standard capability may name.
var fmStdActions = map[string]func(s *Server, w *mywant.Want, args map[string]string) (string, error){
	"restart": (*Server).fmStdRestart,
}

var fmStdCaps = func() []fmStdCap {
	var file struct {
		Capabilities []fmStdCap `yaml:"capabilities"`
	}
	if err := yaml.Unmarshal(fmStdCapsYAML, &file); err != nil {
		panic("standard_capabilities.yaml: " + err.Error())
	}
	for _, c := range file.Capabilities {
		if c.Action != "" && fmStdActions[c.Action] == nil {
			panic("standard_capabilities.yaml: " + c.Name + " names no action the server has: " + c.Action)
		}
	}
	return file.Capabilities
}()

func fmStdCapNamed(name string) (fmStdCap, bool) {
	for _, c := range fmStdCaps {
		if c.Name == name {
			return c, true
		}
	}
	return fmStdCap{}, false
}

// fmStdRestart runs a want again, walks to it, and — when it finds what it
// finds within seconds — says what it found; else that it is running.
func (s *Server) fmStdRestart(w *mywant.Want, args map[string]string) (string, error) {
	if err := s.backend("POST", "/api/v1/wants/"+w.Metadata.ID+"/restart", nil, nil); err != nil {
		return "", fmt.Errorf("could not run %q again: %v", w.Metadata.Name, err)
	}
	out := fmt.Sprintf("Ran %q (%s) again.", w.Metadata.Name, w.Metadata.Type)
	if s.fmWaitSettled(w.Metadata.ID, fmPlanResultWait) {
		look := map[string]string{"name": w.Metadata.Name, fmShortArg: "true"}
		if said, err := s.fmLook(look); err == nil {
			return out + " What it says now:\n" + said, nil
		}
	}
	return out + " It is still running; what it finds will show on its tile — say so.", nil
}

// fmWantNamed is the want a name means: its own name (or close to it), else
// the want of the type it is an alias of — the one on the board before one put
// away. More than one meant, they are said instead.
func (s *Server) fmWantNamed(name string) (*mywant.Want, error) {
	if s.globalBuilder == nil {
		return nil, fmt.Errorf("no board")
	}
	all := s.globalBuilder.GetWants()
	var wants []*mywant.Want
	for _, w := range all {
		if w.Metadata.Labels[fmArchived] != "true" && w.Metadata.Name != fmRobotWant {
			wants = append(wants, w)
		}
	}
	loose := fmLooseName
	var near []*mywant.Want
	for _, w := range wants {
		if w.Metadata.Name == name || loose(w.Metadata.Name) == loose(name) {
			return w, nil
		}
		if n := loose(name); n != "" && strings.Contains(loose(w.Metadata.Name), n) {
			near = append(near, w)
		}
	}
	if len(near) == 0 {
		typeOf := make([]string, len(wants))
		for i, w := range wants {
			typeOf[i] = w.Metadata.Type
		}
		for _, i := range fmByAlias(name, typeOf, s.fmTypeAliases()) {
			near = append(near, wants[i])
		}
	}
	switch len(near) {
	case 0:
		return nil, fmt.Errorf("no want named %q on the board; use board to see the names", name)
	case 1:
		return near[0], nil
	}
	names := make([]string, len(near))
	for i, w := range near {
		names[i] = w.Metadata.Name
	}
	return nil, fmt.Errorf("%q could be: %s — ask the person which", name, strings.Join(names, ", "))
}

// fmCap is one capability of one want on the board.
type fmCap struct {
	want *mywant.Want
	def  mywant.WantCapabilityDef
	// The directory the type was loaded from, where its skill must be.
	dir string
}

func (c fmCap) ref() string { return c.want.Metadata.Name + fmCapSep + c.def.Name }

// fmCapabilities is every capability of every want on the board (not those
// put away), in the board's order.
func (s *Server) fmCapabilities() []fmCap {
	if s.globalBuilder == nil || s.wantTypeLoader == nil {
		return nil
	}
	var out []fmCap
	for _, w := range s.globalBuilder.GetWants() {
		if w.Metadata.Labels[fmArchived] == "true" {
			continue
		}
		def := s.wantTypeLoader.GetDefinition(w.Metadata.Type)
		if def == nil || len(def.Capabilities) == 0 {
			continue
		}
		dir := ""
		if o := s.wantTypeLoader.GetOrigin(w.Metadata.Type); o != nil && o.Path != "" {
			dir = filepath.Dir(o.Path)
		}
		for _, c := range def.Capabilities {
			if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Description) == "" {
				continue
			}
			out = append(out, fmCap{want: w, def: c, dir: dir})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ref() < out[j].ref() })
	return out
}

// fmCapabilityChoices is what use_capability's capability may be: the
// standard ones by name, then each want's own as want#capability.
func (s *Server) fmCapabilityChoices() []string {
	var out []string
	for _, c := range fmStdCaps {
		out = append(out, c.Name)
	}
	return append(out, s.fmCapabilityRefs()...)
}

func (s *Server) fmCapabilityRefs() []string {
	var refs []string
	for _, c := range s.fmCapabilities() {
		refs = append(refs, c.ref())
	}
	return refs
}

// fmCapabilitiesText is the board's capabilities as the robot's instructions
// list them: what each can look up, by its choice.
func (s *Server) fmCapabilitiesText() string {
	var b strings.Builder
	b.WriteString("\n\nWhat every want can do, asked of it by name — call use_capability with the capability and want set to the want's name:\n")
	for _, c := range fmStdCaps {
		fmt.Fprintf(&b, "- %s: %s\n", c.Name, firstLine(c.Description, 160))
	}
	if caps := s.fmCapabilities(); len(caps) > 0 {
		b.WriteString("What wants on the board can look up beyond what they show — when the person asks exactly this, call use_capability with it:\n")
		for _, c := range caps {
			fmt.Fprintf(&b, "- %s: %s\n", c.ref(), firstLine(c.def.Description, 120))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// fmUseCapability does one capability: its data gathered (or its skill run)
// and handed back with the type's way of answering from it.
func (s *Server) fmUseCapability(args map[string]string) (string, error) {
	ref := strings.TrimSpace(args["capability"])
	// A standard one: of the want named (want#restart, or want= beside it).
	stdName, wantName := ref, strings.TrimSpace(args["want"])
	if i := strings.LastIndex(ref, fmCapSep); i >= 0 {
		stdName, wantName = ref[i+len(fmCapSep):], ref[:i]
	}
	if std, ok := fmStdCapNamed(stdName); ok {
		if wantName == "" {
			return "", fmt.Errorf("%s needs the want: set want to its name", std.Name)
		}
		w, err := s.fmWantNamed(wantName)
		if err != nil {
			return "", err
		}
		args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
		if std.Action != "" {
			return fmStdActions[std.Action](s, w, args)
		}
		// Data to read, as a type's own capability: of the want named.
		return s.fmCapAnswer(fmCap{want: w, def: std.WantCapabilityDef}, args)
	}
	// A type's own, named beside its want (the plan's capability field).
	if !strings.Contains(ref, fmCapSep) && wantName != "" {
		w, err := s.fmWantNamed(wantName)
		if err != nil {
			return "", err
		}
		ref = w.Metadata.Name + fmCapSep + ref
	}
	var c *fmCap
	for _, x := range s.fmCapabilities() {
		if x.ref() == ref {
			x := x
			c = &x
			break
		}
	}
	if c == nil {
		return "", fmt.Errorf("no capability %q; choose one of: %s", ref, strings.Join(s.fmCapabilityChoices(), ", "))
	}
	return s.fmCapAnswer(*c, args)
}

// fmCapAnswer does a capability that reads (or runs a skill): its data
// gathered and handed back with the way of answering from it.
func (s *Server) fmCapAnswer(c fmCap, args map[string]string) (string, error) {
	question := args[fmQuestionArg]
	w := c.want
	args[fmCardArg] = fmCardValue("want", w.Metadata.ID, w.Metadata.Name)
	go s.walkRobotToEvidence(w.Metadata.ID)

	var b strings.Builder
	fmt.Fprintf(&b, "%s can %s.\n", w.Metadata.Name, firstLine(c.def.Description, 160))
	// First, so a long run of data cut short never loses it.
	b.WriteString("Answer exactly what the person asked — when, where, a score, whatever it was: only that, not everything below — from these lines only. If they do not hold it, say so; never make it up.\n")
	if p := strings.TrimSpace(c.def.Prompt); p != "" {
		// What the data means, in the type's words — not what to say: the
		// question decides that (「練習場所は？」 asked of a want whose
		// capability also holds scores is answered with the place).
		b.WriteString("How its data reads (the want type's own words): " + strings.Join(strings.Fields(p), " ") + "\n")
	}
	if c.def.Skill != "" {
		out, err := s.fmRunCapabilitySkill(c, question)
		if err != nil {
			return "", err
		}
		b.WriteString("\nWhat its skill found:\n" + out + "\n")
	}
	for _, kind := range c.def.Reads {
		switch kind {
		case "answers":
			b.WriteString("\n" + fmCapAnswersText(w))
		case "answer_photos":
			b.WriteString("\n" + s.fmCapPhotosText(w, args))
		case "state":
			b.WriteString("\nWhat it says now:\n" + s.fmLookFull(w.Metadata.Name, w.Metadata.Type) + "\n")
		}
	}
	return b.String(), nil
}

// fmCapAnswersText is a want's kept answers, newest first.
func fmCapAnswersText(w *mywant.Want) string {
	h := w.BuildHistory().ResultHistory
	if len(h) == 0 {
		return "Its answers: none kept.\n"
	}
	var b strings.Builder
	b.WriteString("Its answers, newest first:\n")
	for i := len(h) - 1; i >= 0 && len(h)-1-i < fmCapAnswers; i-- {
		e := h[i]
		r, _ := json.Marshal(e.Result)
		now := ""
		if e.Current {
			now = " (its answer now)"
		}
		fmt.Fprintf(&b, "- %s%s: %s\n", answerWhen(e), now, firstLine(string(r), 300))
	}
	return b.String()
}

// fmCapPhotosText is the photos joined to a want's answers, newest answer
// first, with what was read off each.
func (s *Server) fmCapPhotosText(w *mywant.Want, args map[string]string) string {
	photos := s.answerPhotosOf([]*mywant.Want{w}, w.Metadata.Name, nil)
	if len(photos) == 0 {
		return "Photos joined to its answers: none.\n"
	}
	// Those nobody has read yet, or not read into their fields: handed to the
	// asker's device to read (the iPhone app's PhotoReader), and queued here.
	var unread []fmUnreadPhoto
	for _, p := range photos {
		switch {
		case p.Read == nil && p.ImageURL != "":
			unread = append(unread, fmUnreadPhoto{ID: p.ID, Image: p.ImageURL})
		case len(p.FieldSpec) > 0 && len(p.Fields) == 0:
			unread = append(unread, fmUnreadPhoto{ID: p.ID, Image: p.ImageURL, Read: true})
		}
	}
	if len(unread) > 0 {
		s.readPhotoFieldsSoon()
		raw, _ := json.Marshal(unread)
		args[fmUnreadArg] = string(raw)
	}
	var b strings.Builder
	if photos[0].Rules != "" {
		b.WriteString("How such a photo is read: " + photos[0].Rules + "\n")
	}
	b.WriteString("Photos joined to its answers, newest answer first:\n")
	for i, p := range photos {
		if i == fmCapPhotos {
			break
		}
		fmt.Fprintf(&b, "- photo %q, for the answer of %s\n", p.Name, p.About)
		if len(p.Fields) > 0 {
			keys := make([]string, 0, len(p.Fields))
			for k := range p.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "  %s: %s\n", k, p.Fields[k])
			}
		}
		switch {
		case p.Read != nil && len(p.Read.Lines) > 0:
			lines := p.Read.Lines
			if len(lines) > fmCapLines {
				lines = lines[:fmCapLines]
			}
			b.WriteString("  its words: " + strings.Join(lines, " / ") + "\n")
		case len(p.Fields) == 0:
			b.WriteString("  (not read yet)\n")
		}
	}
	return b.String()
}

// fmRunCapabilitySkill runs a capability's skill — only one inside its type's
// own directory — with the question, and says what it printed.
func (s *Server) fmRunCapabilitySkill(c fmCap, question string) (string, error) {
	if c.dir == "" {
		return "", fmt.Errorf("%s's type has no directory of its own to run %s from", c.want.Metadata.Name, c.def.Skill)
	}
	path, ok := fmCapSkillPath(c.dir, c.def.Skill)
	if !ok {
		return "", fmt.Errorf("%s's skill %s is outside its type's directory", c.want.Metadata.Name, c.def.Skill)
	}
	arg, _ := json.Marshal(map[string]string{"question": question})
	ctx, cancel := context.WithTimeout(context.Background(), fmCapSkillTimeout)
	defer cancel()
	out, err := mywant.RunMRSScript(ctx, path, mywant.MRSRunOptions{Args: []string{string(arg)}})
	if err != nil {
		return "", fmt.Errorf("%s's skill failed: %v", c.want.Metadata.Name, err)
	}
	data, _ := json.Marshal(out)
	return firstLine(string(data), 2000), nil
}

// fmCapSkillPath is a skill's path in its type's directory — or not ok, when
// it would leave it (../, an absolute path).
func fmCapSkillPath(dir, skill string) (string, bool) {
	if dir == "" || skill == "" || filepath.IsAbs(skill) {
		return "", false
	}
	path := filepath.Clean(filepath.Join(dir, skill))
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

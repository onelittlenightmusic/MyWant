package server

// robot_resources.go — where the robot's Plans and Acts are kept.
//
// In memory, newest last, and on disk one file each (mywant.ResourceDir):
// ~/.mywant/plans/<name>.yaml and ~/.mywant/acts/<name>.yaml, read back the
// first time either is asked for, so they outlive the server. An act left
// Pending or Running when the server stopped is Failed when read back — what
// it was doing stopped with the server.
//
// Kept: the latest plansKept plans and actsKept acts, none older than
// resourcesKeptFor. An act not yet finished is never let go, nor a plan an
// act kept names.

import (
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mywant "mywant/engine/core"

	ws "github.com/onelittlenightmusic/want-spec"
	"gopkg.in/yaml.v3"
)

const (
	plansKept        = 500
	actsKept         = 500
	resourcesKeptFor = 30 * 24 * time.Hour
)

type actEntry struct {
	doc  ws.Act
	done chan struct{} // closed when the act has finished

	// saveMu orders a save of this act against its file being removed;
	// deleted (under robotRes.mu) says it was let go, and is not saved again.
	saveMu  sync.Mutex
	deleted bool
}

var robotRes struct {
	mu     sync.Mutex
	loaded bool
	// home is where the plans/ and acts/ directories are; "" is ~/.mywant.
	home     string
	plansDir *mywant.ResourceDir
	actsDir  *mywant.ResourceDir
	plans    []*ws.Plan  // oldest first
	acts     []*actEntry // oldest first
}

// lockRobotRes takes the lock, reading what is on disk the first time.
func lockRobotRes() {
	robotRes.mu.Lock()
	if robotRes.loaded {
		return
	}
	robotRes.loaded = true
	if robotRes.home == "" {
		robotRes.plansDir, robotRes.actsDir = mywant.NewResourceDir("plans"), mywant.NewResourceDir("acts")
	} else {
		robotRes.plansDir = &mywant.ResourceDir{Path: robotRes.home + "/plans"}
		robotRes.actsDir = &mywant.ResourceDir{Path: robotRes.home + "/acts"}
	}
	robotRes.plansDir.Load(func(data []byte) error {
		var p ws.Plan
		if err := yaml.Unmarshal(data, &p); err != nil {
			return err
		}
		if p.Metadata.Name != "" {
			robotRes.plans = append(robotRes.plans, &p)
		}
		return nil
	})
	now := time.Now().Format(time.RFC3339)
	robotRes.actsDir.Load(func(data []byte) error {
		var a ws.Act
		if err := yaml.Unmarshal(data, &a); err != nil {
			return err
		}
		if a.Metadata.Name == "" {
			return nil
		}
		e := &actEntry{doc: a, done: make(chan struct{})}
		close(e.done)
		if !a.Finished() {
			e.doc.Status.Phase = ws.ActFailed
			e.doc.Status.Message = "the server stopped before it was done"
			e.doc.Status.CompletionTime = now
			if err := robotRes.actsDir.Save(a.Metadata.Name, &e.doc); err != nil {
				log.Printf("[WARN] acts: %v", err)
			}
		}
		robotRes.acts = append(robotRes.acts, e)
		return nil
	})
	sort.SliceStable(robotRes.plans, func(i, j int) bool {
		return resourceOrder(robotRes.plans[i].Metadata) < resourceOrder(robotRes.plans[j].Metadata)
	})
	sort.SliceStable(robotRes.acts, func(i, j int) bool {
		return resourceOrder(robotRes.acts[i].doc.Metadata) < resourceOrder(robotRes.acts[j].doc.Metadata)
	})
	if len(robotRes.plans)+len(robotRes.acts) > 0 {
		log.Printf("[robot] %d plans, %d acts read back", len(robotRes.plans), len(robotRes.acts))
	}
}

// resourceOrder is when it was made: the nanoseconds in its name
// (plan-<ns>), else its createdAt.
func resourceOrder(m ws.ResourceMeta) int64 {
	if i := strings.LastIndexByte(m.Name, '-'); i >= 0 {
		if n, err := strconv.ParseInt(m.Name[i+1:], 10, 64); err == nil {
			return n
		}
	}
	if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
		return t.UnixNano()
	}
	return 0
}

// pruneRobotResLocked lets go of what is past keeping, from memory; the
// files are removed by the caller after unlocking (removeRobotRes).
func pruneRobotResLocked(now time.Time) (plans []string, acts []*actEntry) {
	old := now.Add(-resourcesKeptFor).UnixNano()

	keptActs := robotRes.acts[:0]
	over := len(robotRes.acts) - actsKept
	for _, a := range robotRes.acts {
		if a.doc.Finished() && (over > 0 || resourceOrder(a.doc.Metadata) < old) {
			over--
			a.deleted = true
			acts = append(acts, a)
			continue
		}
		keptActs = append(keptActs, a)
	}
	robotRes.acts = keptActs

	named := map[string]bool{}
	for _, a := range robotRes.acts {
		named[a.doc.Spec.Plan] = true
	}
	keptPlans := robotRes.plans[:0]
	over = len(robotRes.plans) - plansKept
	for _, p := range robotRes.plans {
		if !named[p.Metadata.Name] && (over > 0 || resourceOrder(p.Metadata) < old) {
			over--
			plans = append(plans, p.Metadata.Name)
			continue
		}
		keptPlans = append(keptPlans, p)
	}
	robotRes.plans = keptPlans
	return plans, acts
}

// removeRobotRes removes the files of what was let go.
func removeRobotRes(plans []string, acts []*actEntry) {
	for _, name := range plans {
		if err := robotRes.plansDir.Remove(name); err != nil {
			log.Printf("[WARN] plans: %v", err)
		}
	}
	for _, a := range acts {
		a.saveMu.Lock()
		if err := robotRes.actsDir.Remove(a.doc.Metadata.Name); err != nil {
			log.Printf("[WARN] acts: %v", err)
		}
		a.saveMu.Unlock()
	}
}

// save writes the act as it stands, unless it was let go.
func (a *actEntry) save() {
	a.saveMu.Lock()
	defer a.saveMu.Unlock()
	robotRes.mu.Lock()
	if a.deleted {
		robotRes.mu.Unlock()
		return
	}
	snap := a.snapshotLocked()
	robotRes.mu.Unlock()
	if err := robotRes.actsDir.Save(snap.Metadata.Name, &snap); err != nil {
		log.Printf("[WARN] acts: %v", err)
	}
}

// useRobotResHome keeps the plans and acts under home from now on, read
// again from there (tests).
func useRobotResHome(home string) {
	robotRes.mu.Lock()
	defer robotRes.mu.Unlock()
	robotRes.home, robotRes.loaded = home, false
	robotRes.plans, robotRes.acts = nil, nil
}

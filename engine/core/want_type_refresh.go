package mywant

// RefreshWantTypeDefinition hands a type's definition, loaded again (a custom
// installed over an older version, POST /want-types/reload), to the wants of
// that type already running. They kept the one they were made with: a state
// field the new version added was then nobody's — its fetchFrom never filled
// it and its label was unknown — until the server was restarted (fly,
// 2026-11-01: smartgolf's goto_template, so a slot's way could not be built).
//
// Only what the definition decides is changed: the fields, their labels and
// what fetchFrom fills. The want's state is left as it is — a field it has
// keeps its value, a new one gets its initial value — and its spec, status
// and agents are not touched.
func (cb *ChainBuilder) RefreshWantTypeDefinition(def *WantTypeDefinition) int {
	if def == nil {
		return 0
	}
	cb.wantsMu.RLock()
	var wants []*Want
	for _, rw := range cb.wants {
		if rw.want != nil && rw.want.GetMetadata().Type == def.Metadata.Name {
			wants = append(wants, rw.want)
		}
	}
	cb.wantsMu.RUnlock()

	for _, w := range wants {
		w.refreshTypeDefinition(def)
	}
	return len(wants)
}

func (n *Want) refreshTypeDefinition(def *WantTypeDefinition) {
	n.metadataMutex.RLock()
	same := n.WantTypeDefinition == def
	n.metadataMutex.RUnlock()
	if same {
		return
	}

	// The state fields a plugin agent declares for a capability the want
	// requires are merged into its definition, as SetWantTypeDefinition does —
	// into a copy here, the new definition being the registry's.
	typeDef := *def
	typeDef.State = append([]StateDef(nil), def.State...)
	if n.agentRegistry != nil {
		have := make(map[string]bool, len(typeDef.State))
		for _, sd := range typeDef.State {
			have[sd.Name] = true
		}
		for _, capName := range n.Spec.Requires {
			for _, sd := range n.agentRegistry.GetPluginStateUpdatesForCapability(capName) {
				if !have[sd.Name] {
					typeDef.State = append(typeDef.State, sd)
					have[sd.Name] = true
				}
			}
		}
	}

	// Labels built aside and put in whole: the want's agents read StateLabels
	// while this runs, and SetStateLabels fills the map in place.
	shadow := &Want{}
	shadow.SetStateLabels(&typeDef)

	provided := make([]string, 0, len(typeDef.State))
	for _, sd := range typeDef.State {
		provided = append(provided, sd.Name)
		if sd.InitialValue == nil {
			continue
		}
		if _, ok := n.State.Load(sd.Name); !ok {
			n.State.Store(sd.Name, sd.InitialValue)
		}
	}

	n.metadataMutex.Lock()
	n.WantTypeDefinition = &typeDef
	n.StateLabels = shadow.StateLabels
	n.ProvidedStateFields = provided
	n.metadataMutex.Unlock()
	// Derive again (want.go's fetchFrom pass): a new fetchFrom field fills from
	// the source already there, without waiting for the next run. Its cached
	// filter follows the definition by itself (fetchFromFor).
	n.stateRevision.Add(1)
	bumpStateEpoch()
}

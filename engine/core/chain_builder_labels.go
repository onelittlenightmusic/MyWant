package mywant

import "sort"

// AddLabelToRegistry explicitly registers a label in the global registry
func (cb *ChainBuilder) AddLabelToRegistry(key, value string) {
	cb.labelRegistryMutex.Lock()
	defer cb.labelRegistryMutex.Unlock()

	if cb.labelRegistry == nil {
		cb.labelRegistry = make(map[string]map[string]bool)
	}
	if cb.labelRegistry[key] == nil {
		cb.labelRegistry[key] = make(map[string]bool)
	}
	cb.labelRegistry[key][value] = true
}

// registerLabelsFromWant extracts all labels from a want and registers them
func (cb *ChainBuilder) registerLabelsFromWant(want *Want) {
	if want == nil {
		return
	}
	labels := want.GetLabels()
	if len(labels) == 0 {
		return
	}

	for k, v := range labels {
		cb.AddLabelToRegistry(k, v)
	}
}

// GetRegisteredLabels returns a snapshot of all registered labels (both manual and from wants)
func (cb *ChainBuilder) GetRegisteredLabels() (keys []string, values map[string][]string) {
	cb.labelRegistryMutex.RLock()
	defer cb.labelRegistryMutex.RUnlock()

	keys = make([]string, 0, len(cb.labelRegistry))
	values = make(map[string][]string)

	for k, vals := range cb.labelRegistry {
		keys = append(keys, k)
		vList := make([]string, 0, len(vals))
		for v := range vals {
			vList = append(vList, v)
		}
		sort.Strings(vList)
		values[k] = vList
	}
	sort.Strings(keys)

	return keys, values
}

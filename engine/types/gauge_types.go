package types

import . "mywant/engine/core"

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[GaugeWant, GaugeLocals]("gauge")
	})
}

type GaugeLocals struct{}

// GaugeWant displays a numeric value received from a connected provider want.
// Connect via config `using` selector; the provider must expose fields with exposable: true.
type GaugeWant struct{ Want }

func (g *GaugeWant) GetLocals() *GaugeLocals {
	return CheckLocalsInitialized[GaugeLocals](&g.Want)
}

func (g *GaugeWant) Initialize() {
	g.SetCurrent("source_field", g.GetStringParam("source_field", ""))
	g.SetCurrent("min", g.GetFloatParam("min", 0))
	g.SetCurrent("max", g.GetFloatParam("max", 100))
	g.SetCurrent("unit", g.GetStringParam("unit", "%"))
	g.SetCurrent("gauge_label", g.GetStringParam("label", ""))
	g.SetCurrent("value", 0.0)
}

func (g *GaugeWant) IsAchieved() bool { return false }

func (g *GaugeWant) Progress() {
	_, raw, done, ok := g.Use(100)
	if !ok || done {
		return
	}

	snapshot, ok := raw.(map[string]any)
	if !ok {
		return
	}

	sourceField := GetCurrent(&g.Want, "source_field", "")
	if sourceField == "" {
		return
	}

	fieldVal, exists := snapshot[sourceField]
	if !exists {
		return
	}

	var value float64
	switch v := fieldVal.(type) {
	case float64:
		value = v
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	default:
		return
	}

	g.SetCurrent("value", value)

	min := GetCurrent(&g.Want, "min", 0.0)
	max := GetCurrent(&g.Want, "max", 100.0)
	if max > min {
		pct := int((value - min) / (max - min) * 100)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		g.SetCurrent("achieving_percentage", pct)
	}
}

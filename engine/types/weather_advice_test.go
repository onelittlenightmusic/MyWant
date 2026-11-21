package types

import (
	"testing"

	. "mywant/engine/core"
)

func TestWeatherAdvice(t *testing.T) {
	got := weatherAdvice(AdviceInput{
		Result: "霧雨 1°C (湿度 96%) (最高 6° / 最低 1°)",
		Fields: map[string]any{"weather_condition": "rain"},
		Change: []Change{{Kind: ChangeChanged, Field: "weather_condition", From: "sunny", To: "rain"}},
	})
	if len(got) != 2 || got[0].Kind != AdviceBring || got[0].Item != "傘" || got[0].Why != "晴れから雨になりそう" {
		t.Fatalf("rain, from sunny: an umbrella first: %+v", got)
	}
	if got[1].Kind != AdviceWear || got[1].Item != "上着" || got[1].Why != "最低 1°" {
		t.Errorf("cold: a coat: %+v", got[1])
	}
	if got := weatherAdvice(AdviceInput{Result: "快晴 18°C (最高 22° / 最低 12°)", Fields: map[string]any{"weather_condition": "sunny"}}); len(got) != 0 {
		t.Errorf("a fine, mild day needs nothing: %+v", got)
	}
}

package mywant

import (
	"testing"
	"time"
)

func TestConvertTimeToDatetimeTakesTheNextOne(t *testing.T) {
	now := time.Date(2026, 9, 11, 23, 50, 0, 0, time.Local)

	// Later today.
	at, ok := nextOccurrenceOf("23:55", now)
	if !ok || at.Day() != 11 || at.Hour() != 23 || at.Minute() != 55 {
		t.Errorf("23:55 at 23:50 is five minutes away, got %v (ok=%v)", at, ok)
	}
	// Past today, so tomorrow — a departure at 00:20 read at 23:50 is twenty
	// minutes away, and reading it as this morning's would fire the alarm at
	// once and sixteen hours late.
	at, ok = nextOccurrenceOf("00:20", now)
	if !ok || at.Day() != 12 || at.Hour() != 0 || at.Minute() != 20 {
		t.Errorf("00:20 at 23:50 is tomorrow's, got %v (ok=%v)", at, ok)
	}
	if _, ok := nextOccurrenceOf("not a time", now); ok {
		t.Error("nonsense is not a clock reading")
	}
}

func TestConvertSubTypeValueOnlyDoesWhatItKnows(t *testing.T) {
	// Built in the local zone, because that is what a clock reading is: the
	// same instant is 22:47 in Tokyo and 13:47 in UTC, and hard-coding either
	// makes the test pass in one place and fail in the other.
	instant := time.Date(2026, 9, 11, 22, 47, 0, 0, time.Local).Format(time.RFC3339)
	v, ok := ConvertSubTypeValue(instant, "datetime", "time")
	if !ok || v != "22:47" {
		t.Errorf("datetime → time = %v (ok=%v), want 22:47", v, ok)
	}
	if v, ok := ConvertSubTypeValue("22:47", "time", "datetime"); !ok {
		t.Errorf("time → datetime should convert, got %v", v)
	}
	// Nothing it knows: passed through untouched, so the mismatch stays visible
	// rather than being papered over with a guess.
	if v, ok := ConvertSubTypeValue("22:47", "time", "station"); ok || v != "22:47" {
		t.Errorf("time → station is not a conversion, got %v (ok=%v)", v, ok)
	}
	if _, ok := ConvertSubTypeValue("22:47", "time", "time"); ok {
		t.Error("same kind is not a conversion")
	}
	if _, ok := ConvertSubTypeValue(1234, "time", "datetime"); ok {
		t.Error("only strings carry these kinds")
	}
}

// The hook itself: a parameter reads what the wire feeds it, in its own kind.
func TestParamConversionAppliesOnRead(t *testing.T) {
	w := &Want{}
	w.Spec.Params = map[string]any{"event_time": "22:47"}
	if got := w.GetStringParam("event_time", ""); got != "22:47" {
		t.Errorf("untouched without a conversion, got %q", got)
	}
	w.SetParamConversion("event_time", "time", "datetime")
	got := w.GetStringParam("event_time", "")
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("with the conversion registered the parameter reads as an instant, got %q", got)
	}
	w.SetParamConversion("event_time", "", "")
	if got := w.GetStringParam("event_time", ""); got != "22:47" {
		t.Errorf("cleared, got %q", got)
	}
}

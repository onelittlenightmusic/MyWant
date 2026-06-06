package types

import (
	"fmt"
	"time"

	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[DateAndTimeWant, DateAndTimeLocals]("date_and_time")
	})
}

// DateAndTimeLocals holds type-specific local state to detect changes.
type DateAndTimeLocals struct{}

// DateAndTimeWant is a persistent user-control that lets the user pick a date
// and time (hour/minute/second). The selected value is stored as a datetime
// ISO-8601 string ("YYYY-MM-DDTHH:MM:SS") in the "datetime" current-state key,
// which is exposable so downstream wants can consume it.
//
// The individual parts (date, hour, minute, second) are also stored as separate
// current-state keys so the frontend card can read/write them individually via
// PUT /api/v1/states/{id}/{key}.
//
// Example expose entry to propagate the selected datetime:
//
//	exposes:
//	  - currentState: datetime
//	    asGoal: scheduled_at
type DateAndTimeWant struct{ Want }

func (d *DateAndTimeWant) GetLocals() *DateAndTimeLocals {
	return CheckLocalsInitialized[DateAndTimeLocals](&d.Want)
}

func (d *DateAndTimeWant) Initialize() {
	// Preserve user-set values across restarts.
	// On first initialization the flag is absent (false), so defaults are applied.
	// On subsequent restarts the flag is persisted (true), so we keep whatever the
	// user has selected and skip the defaults — preventing a clock_type change from
	// wiping the chosen date/time.
	//
	// When now=true we always skip persistence and let Progress() set the time.
	if GetCurrent[bool](d, "_initialized", false) {
		return
	}

	isNow := d.GetBoolParam("now", false)

	var date string
	var hour, minute, second int

	if isNow {
		// Seed initial state from the current time; Progress() will keep it live.
		t := currentTimeInZone(d.GetStringParam("time_zone", ""))
		date = t.Format("2006-01-02")
		hour = t.Hour()
		minute = t.Minute()
		second = t.Second()
	} else {
		date = d.GetStringParam("default_date", "")
		hour = int(d.GetFloatParam("default_hour", 0))
		minute = int(d.GetFloatParam("default_minute", 0))
		second = int(d.GetFloatParam("default_second", 0))
	}

	d.SetCurrent("date", date)
	d.SetCurrent("hour", hour)
	d.SetCurrent("minute", minute)
	d.SetCurrent("second", second)
	d.SetCurrent("datetime", composeDatetime(date, hour, minute, second))
	d.SetCurrent("_initialized", true)
}

// IsAchieved always returns false — date_and_time is a persistent control want.
func (d *DateAndTimeWant) IsAchieved() bool { return false }

// Progress recomposes and re-emits the datetime string whenever any part changes.
// When now=true, it overwrites date/hour/minute/second with the current time on
// every tick, effectively running as a live clock.
func (d *DateAndTimeWant) Progress() {
	if d.GetBoolParam("now", false) {
		t := currentTimeInZone(d.GetStringParam("time_zone", ""))
		date := t.Format("2006-01-02")
		hour := t.Hour()
		minute := t.Minute()
		second := t.Second()
		d.SetCurrent("date", date)
		d.SetCurrent("hour", hour)
		d.SetCurrent("minute", minute)
		d.SetCurrent("second", second)
		d.SetCurrent("datetime", composeDatetime(date, hour, minute, second))
		return
	}

	date, _ := d.GetStateString("date", "")
	hour := int(GetCurrent[float64](d, "hour", 0))
	minute := int(GetCurrent[float64](d, "minute", 0))
	second := int(GetCurrent[float64](d, "second", 0))

	dt := composeDatetime(date, hour, minute, second)
	d.SetCurrent("datetime", dt)
}

// currentTimeInZone returns time.Now() in the given IANA timezone.
// Falls back to local time when tz is empty or invalid.
func currentTimeInZone(tz string) time.Time {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Now().In(loc)
		}
	}
	return time.Now()
}

// composeDatetime combines date + h/m/s into an ISO-8601 local datetime string.
// Returns an empty string when date is unset.
func composeDatetime(date string, hour, minute, second int) string {
	if date == "" {
		return ""
	}
	return fmt.Sprintf("%sT%02d:%02d:%02d", date, hour, minute, second)
}

package mywant

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseTimeExpression parses natural language time expressions into hour and minute
// Supports formats like "7am", "9pm", "17:30", "midnight", "noon"
func ParseTimeExpression(expr string) (int, int, error) {
	if expr == "" {
		return 0, 0, fmt.Errorf("empty time expression")
	}

	expr = strings.ToLower(strings.TrimSpace(expr))

	// Handle special cases
	switch expr {
	case "midnight":
		return 0, 0, nil
	case "noon":
		return 12, 0, nil
	}

	// Handle 24-hour format like "17:30"
	if strings.Contains(expr, ":") {
		parts := strings.Split(expr, ":")
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid time format: %s", expr)
		}
		hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid hour: %s", parts[0])
		}
		minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid minute: %s", parts[1])
		}
		if hour < 0 || hour > 23 {
			return 0, 0, fmt.Errorf("hour out of range: %d", hour)
		}
		if minute < 0 || minute > 59 {
			return 0, 0, fmt.Errorf("minute out of range: %d", minute)
		}
		return hour, minute, nil
	}

	// Handle 12-hour format with am/pm like "7am", "9pm"
	ampmRegex := regexp.MustCompile(`^(\d{1,2})\s*(am|pm)$`)
	matches := ampmRegex.FindStringSubmatch(expr)
	if matches != nil {
		hour, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid hour: %s", matches[1])
		}
		if hour < 1 || hour > 12 {
			return 0, 0, fmt.Errorf("hour out of range for 12-hour format: %d", hour)
		}

		period := matches[2]
		if period == "pm" && hour != 12 {
			hour += 12
		} else if period == "am" && hour == 12 {
			hour = 0
		}
		return hour, 0, nil
	}

	return 0, 0, fmt.Errorf("unsupported time format: %s", expr)
}

// ParseFrequencyExpression parses natural language frequency expressions into time.Duration
// Supports formats like "5 minutes", "2 hours", "day", "week", "30 seconds"
func ParseFrequencyExpression(expr string) (time.Duration, error) {
	if expr == "" {
		return 0, fmt.Errorf("empty frequency expression")
	}

	expr = strings.ToLower(strings.TrimSpace(expr))

	// Handle special cases
	switch expr {
	case "day":
		return 24 * time.Hour, nil
	case "week":
		return 7 * 24 * time.Hour, nil
	case "hour":
		return time.Hour, nil
	case "minute":
		return time.Minute, nil
	case "second":
		return time.Second, nil
	}

	// Handle expressions like "5 minutes", "2 hours", "30 seconds"
	parts := strings.Fields(expr)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid frequency format: %s", expr)
	}

	value, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid frequency value: %s", parts[0])
	}

	unit := strings.ToLower(parts[1])

	// Normalize unit names (handle both singular and plural)
	switch {
	case strings.HasPrefix(unit, "second"):
		return time.Duration(value) * time.Second, nil
	case strings.HasPrefix(unit, "minute"):
		return time.Duration(value) * time.Minute, nil
	case strings.HasPrefix(unit, "hour"):
		return time.Duration(value) * time.Hour, nil
	case strings.HasPrefix(unit, "day"):
		return time.Duration(value) * 24 * time.Hour, nil
	case strings.HasPrefix(unit, "week"):
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported time unit: %s", unit)
	}
}

// CalculateNextExecution calculates the next execution time based on a WhenSpec
func CalculateNextExecution(spec WhenSpec, now time.Time) (time.Time, error) {
	if err := ValidateWhenSpec(spec); err != nil {
		return time.Time{}, err
	}

	// Parse frequency
	interval, err := ParseFrequencyExpression(spec.Every)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse frequency: %w", err)
	}

	// If 'at' is specified, calculate time-based schedule
	if spec.At != "" {
		hour, minute, err := ParseTimeExpression(spec.At)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse time: %w", err)
		}

		// Create a time for today at the specified time
		next := now.Local()
		next = time.Date(next.Year(), next.Month(), next.Day(), hour, minute, 0, 0, time.Local)

		// If the time has already passed today, move to next interval
		if next.Before(now) || next.Equal(now) {
			next = next.Add(interval)
		}

		return next, nil
	}

	// If no 'at' is specified, calculate based on daily 00:00:00 as reference
	// For example, {every: "30 seconds"} executes at 00:00:00, 00:00:30, 00:01:00, 00:01:30, ...
	nowLocal := now.Local()
	startOfDay := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.Local)

	// Calculate elapsed time since start of day
	elapsedSinceStartOfDay := nowLocal.Sub(startOfDay)

	// Calculate how many intervals have passed
	intervalsPassedFloat := float64(elapsedSinceStartOfDay) / float64(interval)
	intervalsPassed := int64(intervalsPassedFloat)

	// Calculate the next scheduled execution time
	nextExecution := startOfDay.Add(time.Duration(intervalsPassed+1) * interval)

	return nextExecution, nil
}

// ValidateWhenSpec validates a WhenSpec configuration
func ValidateWhenSpec(spec WhenSpec) error {
	if spec.Every == "" {
		return fmt.Errorf("'every' field is required in WhenSpec")
	}

	// Validate that 'every' can be parsed
	_, err := ParseFrequencyExpression(spec.Every)
	if err != nil {
		return fmt.Errorf("invalid 'every' value: %w", err)
	}

	// If 'at' is specified, validate it
	if spec.At != "" {
		_, _, err := ParseTimeExpression(spec.At)
		if err != nil {
			return fmt.Errorf("invalid 'at' value: %w", err)
		}
	}

	return nil
}

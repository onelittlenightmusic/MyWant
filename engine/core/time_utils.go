package mywant

import (
	"math/rand"
	"time"
)

// TimeRange defines a range of hours for time generation
type TimeRange struct {
	StartHour int
	EndHour   int
}

// GenerateRandomTimeInRange generates a random time within the specified hour range on the given base date
func GenerateRandomTimeInRange(baseDate time.Time, hourRange TimeRange) time.Time {
	hourSpan := hourRange.EndHour - hourRange.StartHour
	randomHour := hourRange.StartHour + rand.Intn(hourSpan)
	randomMinute := rand.Intn(60)

	return time.Date(
		baseDate.Year(),
		baseDate.Month(),
		baseDate.Day(),
		randomHour,
		randomMinute,
		0, 0,
		time.Local,
	)
}

// GenerateRandomDuration generates a random duration between min and max hours
func GenerateRandomDuration(minHours, maxHours float64) float64 {
	return minHours + rand.Float64()*(maxHours-minHours)
}

// Common time ranges for different reservation types
var (
	LunchTimeRange  = TimeRange{StartHour: 11, EndHour: 14} // 11 AM - 2 PM
	DinnerTimeRange = TimeRange{StartHour: 18, EndHour: 21} // 6 PM - 9 PM
	CheckInRange    = TimeRange{StartHour: 14, EndHour: 16} // 2 PM - 4 PM
	CheckOutRange   = TimeRange{StartHour: 11, EndHour: 13} // 11 AM - 1 PM
)

// GenerateRandomTimeWithOptions generates a random time with multiple time range options
// Example: lunch or dinner with 50/50 probability
func GenerateRandomTimeWithOptions(baseDate time.Time, options ...TimeRange) time.Time {
	if len(options) == 0 {
		return baseDate
	}

	selectedRange := options[rand.Intn(len(options))]
	return GenerateRandomTimeInRange(baseDate, selectedRange)
}

// CalculateRebookingDelay calculates a random delay for flight rebooking (2-4 hours)
func CalculateRebookingDelay() time.Duration {
	delayHours := 2 + time.Duration(rand.Intn(3))
	return delayHours * time.Hour
}

// ApplyRebookingTiming applies rebooking delay to departure time and calculates new arrival time
// Returns new departure and arrival times (assumes 3.5 hour flight duration)
func ApplyRebookingTiming(originalDeparture time.Time) (newDeparture, newArrival time.Time) {
	delay := CalculateRebookingDelay()
	newDeparture = originalDeparture.Add(delay)
	newArrival = newDeparture.Add(3*time.Hour + 30*time.Minute)
	return newDeparture, newArrival
}

// GenerateFlightTiming generates departure and arrival times for flights
func GenerateFlightTiming(departureDateStr string, isRebooking bool) (departure, arrival time.Time, err error) {
	var departureDate time.Time

	// Parse departure date if provided
	if departureDateStr != "" {
		departureDate, err = time.Parse(time.RFC3339, departureDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else {
		// Default to tomorrow at 8 AM
		departureDate = time.Now().AddDate(0, 0, 1)
		departureDate = time.Date(
			departureDate.Year(),
			departureDate.Month(),
			departureDate.Day(),
			8, 0, 0, 0,
			time.Local,
		)
	}

	// Apply rebooking delay if needed
	if isRebooking {
		departure, arrival = ApplyRebookingTiming(departureDate)
	} else {
		departure = departureDate
		arrival = departureDate.Add(3*time.Hour + 30*time.Minute)
	}

	return departure, arrival, nil
}

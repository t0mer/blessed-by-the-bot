// Package scheduler decides which contacts are due for a blessing and sends it.
//
// Every "today" and "HH:MM" comparison happens in the timezone from settings,
// which the caller supplies by passing an already-localised time. Keeping the
// arithmetic in pure functions here is what makes the Feb-29 and year-boundary
// cases testable without a running loop.
package scheduler

import (
	"fmt"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// DefaultSendTime applies when neither the contact nor settings specify one.
const DefaultSendTime = "09:00"

// DueOn reports whether the annually recurring event stored as eventDate
// ("YYYY-MM-DD") falls on day. Only the month and day matter; the year in
// eventDate is the original occurrence, kept for age arithmetic.
//
// A February 29th event is observed on the 28th in a common year. The
// alternative — March 1st — moves the greeting out of the birth month, which
// reads as a mistake to the recipient.
func DueOn(eventDate string, day time.Time) (bool, error) {
	event, err := time.Parse(time.DateOnly, eventDate)
	if err != nil {
		return false, fmt.Errorf("parsing event date %q: %w", eventDate, err)
	}

	month, dayOfMonth := event.Month(), event.Day()
	if month == time.February && dayOfMonth == 29 && !isLeapYear(day.Year()) {
		dayOfMonth = 28
	}
	return day.Month() == month && day.Day() == dayOfMonth, nil
}

// isLeapYear implements the full Gregorian rule: 2100 is not a leap year even
// though it divides by 4, and 2000 is even though it divides by 100.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// EffectiveSendTime resolves the "HH:MM" a contact should be greeted at.
func EffectiveSendTime(c *store.Contact, general string) string {
	if c != nil && c.SendTime != nil && *c.SendTime != "" {
		return *c.SendTime
	}
	if general != "" {
		return general
	}
	return DefaultSendTime
}

// ClockPassed reports whether the wall clock of now has reached sendTime.
// The comparison is inclusive, so a contact whose send time is exactly the
// current minute goes out on this tick rather than waiting a whole day.
func ClockPassed(sendTime string, now time.Time) (bool, error) {
	hour, minute, err := parseClock(sendTime)
	if err != nil {
		return false, err
	}
	return now.Hour()*60+now.Minute() >= hour*60+minute, nil
}

func parseClock(value string) (hour, minute int, err error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing send time %q: %w", value, err)
	}
	return parsed.Hour(), parsed.Minute(), nil
}

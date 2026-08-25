package scheduler_test

import (
	"testing"
	"time"

	_ "time/tzdata"

	"github.com/t0mer/blessed-by-the-bot/internal/service/scheduler"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func day(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}
	return parsed
}

func TestDueOnMatchesMonthAndDayIgnoringYear(t *testing.T) {
	cases := []struct {
		event string
		when  string
		want  bool
	}{
		{"1990-05-17", "2026-05-17", true},
		{"1990-05-17", "2026-05-18", false},
		{"1990-05-17", "2026-06-17", false},
		{"1990-05-17", "2099-05-17", true},
		{"1990-12-31", "2026-12-31", true},
		{"1990-01-01", "2027-01-01", true},
	}
	for _, tc := range cases {
		got, err := scheduler.DueOn(tc.event, day(t, tc.when))
		if err != nil {
			t.Fatalf("DueOn(%q, %q): %v", tc.event, tc.when, err)
		}
		if got != tc.want {
			t.Errorf("DueOn(%q, %q) = %v, want %v", tc.event, tc.when, got, tc.want)
		}
	}
}

// A Feb-29 birthday has to land somewhere in a common year. Feb-28 is the spec's
// choice; March 1 would push the greeting past the month, which reads wrong.
func TestDueOnLeapDayFallsBackToFebruary28(t *testing.T) {
	cases := []struct {
		when string
		want bool
	}{
		{"2024-02-29", true},  // leap year: the real date
		{"2024-02-28", false}, // ...and only that date
		{"2026-02-28", true},  // common year: shifted back
		{"2026-03-01", false},
		{"2100-02-28", true}, // 2100 is not a leap year despite dividing by 4
		{"2000-02-29", true}, // 2000 is, dividing by 400
	}
	for _, tc := range cases {
		got, err := scheduler.DueOn("1992-02-29", day(t, tc.when))
		if err != nil {
			t.Fatalf("DueOn: %v", err)
		}
		if got != tc.want {
			t.Errorf("leap-day event on %s = %v, want %v", tc.when, got, tc.want)
		}
	}
}

// Feb-28 birthdays must NOT also fire on Feb-29 in a leap year, or someone born
// on the 28th gets two greetings every four years.
func TestDueOnFebruary28IsNotShifted(t *testing.T) {
	got, err := scheduler.DueOn("1990-02-28", day(t, "2024-02-29"))
	if err != nil {
		t.Fatalf("DueOn: %v", err)
	}
	if got {
		t.Fatal("a Feb-28 event must not fire on Feb-29")
	}
}

func TestDueOnRejectsAMalformedEventDate(t *testing.T) {
	if _, err := scheduler.DueOn("17/05/1990", day(t, "2026-05-17")); err == nil {
		t.Fatal("want an error for a malformed event date")
	}
}

func TestEffectiveSendTimePrefersTheContactOverride(t *testing.T) {
	override := "07:30"
	c := &store.Contact{SendTime: &override}
	if got := scheduler.EffectiveSendTime(c, "09:00"); got != "07:30" {
		t.Fatalf("got %q, want the contact override", got)
	}
}

func TestEffectiveSendTimeFallsBackToGeneralThenDefault(t *testing.T) {
	c := &store.Contact{}
	if got := scheduler.EffectiveSendTime(c, "10:15"); got != "10:15" {
		t.Fatalf("got %q, want the general setting", got)
	}
	if got := scheduler.EffectiveSendTime(c, ""); got != scheduler.DefaultSendTime {
		t.Fatalf("got %q, want the built-in default", got)
	}
}

func TestClockPassedComparesWallTimeInTheGivenZone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		t.Fatalf("loading zone: %v", err)
	}
	now := time.Date(2026, 5, 17, 9, 0, 0, 0, loc)

	cases := map[string]bool{
		"08:59": true,
		"09:00": true, // inclusive: a send time of exactly now is due
		"09:01": false,
		"23:59": false,
		"00:00": true,
	}
	for value, want := range cases {
		got, err := scheduler.ClockPassed(value, now)
		if err != nil {
			t.Fatalf("ClockPassed(%q): %v", value, err)
		}
		if got != want {
			t.Errorf("ClockPassed(%q, 09:00) = %v, want %v", value, got, want)
		}
	}
}

// The same instant is a different calendar day either side of the date line, so
// the zone the caller passes is what decides "today".
func TestZoneDecidesTheDay(t *testing.T) {
	jerusalemLoc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		t.Fatalf("loading zone: %v", err)
	}
	utc := time.Date(2026, 5, 17, 22, 30, 0, 0, time.UTC) // 2026-05-18 01:30 in Jerusalem

	dueUTC, err := scheduler.DueOn("1990-05-17", utc)
	if err != nil {
		t.Fatal(err)
	}
	dueLocal, err := scheduler.DueOn("1990-05-17", utc.In(jerusalemLoc))
	if err != nil {
		t.Fatal(err)
	}
	if !dueUTC || dueLocal {
		t.Fatalf("utc due = %v, jerusalem due = %v; want true and false", dueUTC, dueLocal)
	}
}

func TestClockPassedRejectsAMalformedTime(t *testing.T) {
	if _, err := scheduler.ClockPassed("9am", time.Now()); err == nil {
		t.Fatal("want an error for a malformed send time")
	}
}

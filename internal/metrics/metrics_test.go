package metrics_test

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/t0mer/blessed-by-the-bot/internal/metrics"
)

func TestOutcome(t *testing.T) {
	if got := metrics.Outcome(nil); got != metrics.OutcomeSent {
		t.Fatalf("Outcome(nil) = %q, want %q", got, metrics.OutcomeSent)
	}
	if got := metrics.Outcome(errors.New("boom")); got != metrics.OutcomeFailed {
		t.Fatalf("Outcome(err) = %q, want %q", got, metrics.OutcomeFailed)
	}
}

// A counter vector exports nothing until a label combination is first used, so
// a fresh instance would report "no data" where an alert needs an explicit zero.
// Init exists to pre-create those series; this pins the ones it promises.
func TestInitPreCreatesKnownSeriesAtZero(t *testing.T) {
	metrics.Init()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, family := range families {
		byName[family.GetName()] = family
	}

	// 2 results; 2 providers x 2 kinds x 2 outcomes; 2 providers x 4 results.
	for name, want := range map[string]int{
		"blessedbot_scheduler_ticks_total": 2,
		"blessedbot_messages_total":        8,
		"blessedbot_webhooks_total":        8,
	} {
		family, ok := byName[name]
		if !ok {
			t.Errorf("%s is not exported after Init", name)
			continue
		}
		if got := len(family.GetMetric()); got != want {
			t.Errorf("%s has %d series, want %d", name, got, want)
		}
		for _, series := range family.GetMetric() {
			if value := series.GetCounter().GetValue(); value != 0 {
				t.Errorf("%s %v starts at %v, want 0", name, series.GetLabel(), value)
			}
		}
	}

	// Chat-id-keyed series are deliberately absent: that label set is only
	// known once the user configures groups.
	for _, name := range []string{"blessedbot_wishes_matched_total", "blessedbot_group_echoes_total"} {
		if family, ok := byName[name]; ok && len(family.GetMetric()) > 0 {
			t.Errorf("%s pre-created %d series, want none", name, len(family.GetMetric()))
		}
	}
}

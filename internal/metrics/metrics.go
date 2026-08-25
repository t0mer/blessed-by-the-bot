// Package metrics defines the Prometheus collectors the application exposes on
// /metrics.
//
// Everything lives in one place so the metric names, labels and help text are
// reviewable together — a metric is an API, and renaming one breaks dashboards
// as surely as renaming an endpoint breaks clients.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Outcome label values for send counters.
const (
	OutcomeSent   = "sent"
	OutcomeFailed = "failed"
)

var (
	// MessagesSent counts outbound WhatsApp messages by provider, kind and
	// outcome. This is the metric that answers "did anyone get their birthday
	// message today?", so failures are counted rather than merely logged.
	MessagesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blessedbot_messages_total",
		Help: "Outbound WhatsApp messages, by provider, kind and outcome.",
	}, []string{"provider", "kind", "outcome"})

	// WebhooksReceived counts inbound provider callbacks by provider and result,
	// which is how a misconfigured signature secret becomes visible: a rising
	// 'unauthorized' count means the webhook is reaching us but being rejected.
	WebhooksReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blessedbot_webhooks_total",
		Help: "Inbound provider webhooks, by provider and result.",
	}, []string{"provider", "result"})

	// WishesMatched counts group messages recognised as wishes, by group.
	WishesMatched = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blessedbot_wishes_matched_total",
		Help: "Group messages recognised as congratulations, by group chat id.",
	}, []string{"chat_id"})

	// GroupEchoes counts blessings the bot posted into a group.
	GroupEchoes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blessedbot_group_echoes_total",
		Help: "Blessings posted into watched groups, by group chat id and outcome.",
	}, []string{"chat_id", "outcome"})

	// SchedulerTicks counts scheduler passes. A flat counter means the loop has
	// stopped, which no send-count alert would reveal on a quiet day.
	SchedulerTicks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blessedbot_scheduler_ticks_total",
		Help: "Scheduler passes, by result.",
	}, []string{"result"})

	// ProviderRequestDuration measures outbound provider calls.
	ProviderRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "blessedbot_provider_request_seconds",
		Help:    "Duration of outbound provider HTTP calls, by provider and outcome.",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider", "outcome"})
)

// Outcome maps an error to a counter label.
func Outcome(err error) string {
	if err != nil {
		return OutcomeFailed
	}
	return OutcomeSent
}

// Known label values, used to pre-create series at startup.
var (
	knownProviders = []string{"greenapi", "gowa"}
	knownKinds     = []string{"scheduled", "group_echo"}
	knownOutcomes  = []string{OutcomeSent, OutcomeFailed}
	webhookResults = []string{"accepted", "ignored", "unauthorized", "unparsable"}
)

// Init pre-creates every series whose labels are known in advance.
//
// A Prometheus counter vector exports nothing until a label combination is
// first used, so a freshly started instance would report "no data" rather than
// zero — and an alert on `rate(...failed...) > 0` cannot fire on a series that
// does not exist. Series keyed by chat id are deliberately left out: that label
// set grows with the user's groups and is only known at runtime.
func Init() {
	for _, result := range []string{"ok", "error"} {
		SchedulerTicks.WithLabelValues(result)
	}
	for _, provider := range knownProviders {
		for _, kind := range knownKinds {
			for _, outcome := range knownOutcomes {
				MessagesSent.WithLabelValues(provider, kind, outcome)
			}
		}
		for _, result := range webhookResults {
			WebhooksReceived.WithLabelValues(provider, result)
		}
	}
}

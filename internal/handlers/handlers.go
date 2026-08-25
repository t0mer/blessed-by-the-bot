package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Sender triggers an immediate scheduled blessing for one contact. The blessing
// engine (Phase 5) implements it; until then Deps.Sender is nil and
// POST /contacts/{id}/send-now answers 501 rather than pretending to work.
type Sender interface {
	SendNow(ctx context.Context, contactID int64, force bool) (*store.SendLogEntry, error)
}

// IncomingHandler consumes a normalized inbound message. The group-echo engine
// (Phase 6) implements it; until then webhooks still verify, parse and log,
// which is exactly what is needed to prove provider setup end to end.
type IncomingHandler interface {
	HandleIncoming(ctx context.Context, msg provider.IncomingMessage) error
}

// Deps are the collaborators the API needs. Store, Settings, Providers and
// Logger are required; the rest are optional seams.
type Deps struct {
	Store     *store.Store
	Settings  *settings.Service
	Providers *provider.Manager
	Logger    *slog.Logger
	Version   string

	// Rebuild swaps the active provider after a settings change so switching
	// providers needs no restart (spec §4). Nil disables the rebuild.
	Rebuild func(ctx context.Context, s *settings.Settings) error

	Sender   Sender
	Incoming IncomingHandler
}

// API owns the HTTP handlers.
type API struct {
	store     *store.Store
	settings  *settings.Service
	providers *provider.Manager
	log       *slog.Logger
	version   string
	rebuild   func(ctx context.Context, s *settings.Settings) error
	sender    Sender
	incoming  IncomingHandler
}

// New validates deps and returns the API.
func New(deps Deps) (*API, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("handlers: store is required")
	case deps.Settings == nil:
		return nil, errors.New("handlers: settings service is required")
	case deps.Providers == nil:
		return nil, errors.New("handlers: provider manager is required")
	case deps.Logger == nil:
		return nil, errors.New("handlers: logger is required")
	}
	version := deps.Version
	if version == "" {
		version = "dev"
	}
	return &API{
		store:     deps.Store,
		settings:  deps.Settings,
		providers: deps.Providers,
		log:       deps.Logger,
		version:   version,
		rebuild:   deps.Rebuild,
		sender:    deps.Sender,
		incoming:  deps.Incoming,
	}, nil
}

// Routes returns the router mounted at /api/v1.
//
// Routes are registered here and nowhere else, so this function is the index of
// the API surface. Middleware that applies only to the API (basic auth, per the
// spec's "structure it so auth can be added later") belongs at the top of this
// router, not in the server's global chain.
func (a *API) Routes() chi.Router {
	r := chi.NewRouter()
	r.NotFound(NotFoundJSON)
	r.MethodNotAllowed(MethodNotAllowedJSON)

	r.Get("/healthz", a.Health)

	// chi maps the "/" pattern inside Route to the bare prefix, so
	// GET /api/v1/contacts matches without a redirect. Do not add
	// middleware.StripSlashes globally — it would change webhook paths too.
	r.Route("/contacts", func(cr chi.Router) {
		cr.Get("/", a.listContacts)
		cr.Post("/", a.createContact)
		cr.Get("/{id}", a.getContact)
		cr.Put("/{id}", a.updateContact)
		cr.Delete("/{id}", a.deleteContact)
	})

	r.Route("/blessings", func(br chi.Router) {
		br.Get("/", a.listBlessings)
		br.Post("/", a.createBlessing)
		br.Get("/{id}", a.getBlessing)
		br.Put("/{id}", a.updateBlessing)
		br.Delete("/{id}", a.deleteBlessing)
	})
	return r
}

// WebhookRoutes returns the router mounted at /webhooks. It is separate from
// Routes because webhooks are provider callbacks, not part of the versioned API,
// and must never sit behind the API's future auth middleware.
func (a *API) WebhookRoutes() chi.Router {
	r := chi.NewRouter()
	r.NotFound(NotFoundJSON)
	r.MethodNotAllowed(MethodNotAllowedJSON)
	return r
}

// Health reports process and database liveness. It is exported so the server can
// also serve it at the root path for the container healthcheck.
func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	payload := map[string]string{"status": "ok", "version": a.version}
	status := http.StatusOK

	if err := a.store.Ping(r.Context()); err != nil {
		a.log.Error("health check: database unreachable", "error", err)
		payload["status"] = "error"
		payload["database"] = "error"
		status = http.StatusServiceUnavailable
	} else {
		payload["database"] = "ok"
	}
	WriteJSON(w, status, payload)
}

package api

import (
	"context"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/12go/f4/internal/api/handler"
	"github.com/12go/f4/internal/api/middleware"
	"github.com/12go/f4/internal/stage"
)

// pageOverrideRepo defines the interface for admin page overrides.
type pageOverrideRepo interface {
	GetByURL(ctx context.Context, url string) (map[string]string, error)
	Set(ctx context.Context, url, param, value string) error
}

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(logger *zap.Logger, pipeline *stage.SearchPipeline, pageOverrides pageOverrideRepo) *chi.Mux {
	r := chi.NewRouter()

	// Middleware chain
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Logging(logger))
	r.Use(middleware.Metrics(&middleware.NoopMetrics{}))
	r.Use(middleware.AgentExtraction)

	// Handlers
	searchHandler := handler.NewSearchHandler(pipeline)
	searchByStationsHandler := handler.NewSearchByStationsHandler(pipeline)
	healthHandler := handler.NewHealthHandler()
	adminHandler := handler.NewAdminSearchHandler(pageOverrides)

	// Routes
	r.Get("/health", healthHandler.ServeHTTP)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/search/{fromPlaceID}/{toPlaceID}/{date}", searchHandler.ServeHTTP)
		r.Get("/searchByStations/{fromStations}/{toStations}/{date}", searchByStationsHandler.ServeHTTP)
		r.Get("/admin/search/{fromPlaceID}/{toPlaceID}/{date}", adminHandler.ServeHTTPGet)
		r.Post("/admin/search/{fromPlaceID}/{toPlaceID}", adminHandler.ServeHTTPPost)
	})

	return r
}

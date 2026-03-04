package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/12go/f4/internal/api"
	"github.com/12go/f4/internal/cache"
	"github.com/12go/f4/internal/config"
	"github.com/12go/f4/internal/db"
	"github.com/12go/f4/internal/event"
	"github.com/12go/f4/internal/feature"
	"github.com/12go/f4/internal/refcache"
	"github.com/12go/f4/internal/repository"
	"github.com/12go/f4/internal/stage"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Initialize database connections
	connMgr, err := db.NewConnectionManager(cfg.Database)
	if err != nil {
		logger.Fatal("failed to init database", zap.Error(err))
	}
	defer connMgr.Close()

	// Initialize Redis cache
	redisCache := cache.NewRedisCache(cfg.Cache)
	defer redisCache.Close()
	_ = redisCache // available for cache-aside patterns in repositories

	// Initialize event publisher
	publisher, err := event.NewNatsPublisher(cfg.EventBus.NatsURL)
	if err != nil {
		logger.Warn("failed to init NATS, using noop publisher", zap.Error(err))
		publisher = &event.NoopPublisher{}
	}
	defer publisher.Close()

	// Feature flags
	flags := feature.New(cfg.Features)

	// Repositories
	defaultDB := connMgr.Default()
	stationRepo := repository.NewStationRepo(defaultDB)
	operatorRepo := repository.NewOperatorRepo(defaultDB)
	classRepo := repository.NewClassRepo(defaultDB)
	imageRepo := repository.NewImageRepo(defaultDB)
	dataSecRepo := repository.NewDataSecRepo(defaultDB)
	whiteLabelRepo := repository.NewWhiteLabelRepo(defaultDB)
	autopackRepo := repository.NewAutopackRepo(defaultDB)
	pageOverrideRepo := repository.NewPageOverrideRepo(defaultDB)

	// Region resolver: use DB-based resolver if default DB is available, else static fallback
	var regionResolver db.RegionResolver
	if defaultDB != nil {
		dbResolver := db.NewDBRegionResolver(defaultDB)
		if err := dbResolver.Init(context.Background()); err != nil {
			logger.Warn("failed to init DB region resolver, using static fallback", zap.Error(err))
			regionResolver = db.NewStaticRegionResolver("th")
		} else {
			regionResolver = dbResolver
		}
	} else {
		regionResolver = db.NewStaticRegionResolver("th")
	}
	tripPoolRepo := repository.NewTripPoolRepo(connMgr, regionResolver)
	tripPoolSetRepo := repository.NewTripPoolSetRepo(func(region string) *sqlx.DB {
		return connMgr.TripPool(region)
	})
	roundTripPriceRepo := repository.NewRoundTripPriceRepo(func(region string) *sqlx.DB {
		return connMgr.TripPool(region)
	})

	// Pipeline stages
	stage1 := stage.NewResolvePlacesStage(stationRepo)
	stage2 := stage.NewBuildFilterStage(dataSecRepo, whiteLabelRepo, flags)
	stage3 := stage.NewQueryTripsStage(tripPoolRepo)
	stage4 := stage.NewFilterRawTripsStage()
	stage5a := stage.NewAssembleMultiLegStage(tripPoolSetRepo, tripPoolRepo, autopackRepo, regionResolver)
	stage5b := stage.NewEnrichRoundTripsStage(roundTripPriceRepo, publisher, regionResolver)
	reasonRepo := repository.NewReasonRepo(defaultDB)
	integrationRepo := repository.NewIntegrationRepo(defaultDB)
	tranRepo := repository.NewTranRepo(defaultDB)

	// Reference data cache (each entity toggled by env var, all off by default)
	refCacheCfg := refcache.CacheConfig{
		EnableOperators:   cfg.RefCache.EnableOperators,
		EnableStations:    cfg.RefCache.EnableStations,
		EnableClasses:     cfg.RefCache.EnableClasses,
		EnableReasons:     cfg.RefCache.EnableReasons,
		EnableIntegration: cfg.RefCache.EnableIntegration,
		RefreshTTL:        cfg.RefCache.RefreshTTL,
	}
	var rCache *refcache.RefDataCache
	if refCacheCfg.EnableOperators || refCacheCfg.EnableStations || refCacheCfg.EnableClasses || refCacheCfg.EnableIntegration {
		rCache = refcache.New(refCacheCfg, defaultDB, logger)
		rCache.Start(context.Background())
		defer rCache.Stop()
		logger.Info("reference data cache enabled",
			zap.Bool("operators", refCacheCfg.EnableOperators),
			zap.Bool("stations", refCacheCfg.EnableStations),
			zap.Bool("classes", refCacheCfg.EnableClasses),
			zap.Bool("integration", refCacheCfg.EnableIntegration),
		)
	}

	stage6 := stage.NewCollectRefDataStage(stationRepo, operatorRepo, classRepo, imageRepo, reasonRepo, integrationRepo, tranRepo, rCache)
	stage7 := stage.NewHydrateResultsStage()
	stage8 := stage.NewSortAndFinalizeStage(stationRepo)
	stage9 := stage.NewSerializeResponseStage(publisher, cfg.Recheck.BaseURL)

	pipeline := stage.NewSearchPipeline(
		stage1, stage2, stage3, stage4,
		stage5a, stage5b,
		stage6, stage7, stage8, stage9,
	)

	// HTTP router
	router := api.NewRouter(logger, pipeline, pageOverrideRepo)

	// HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	// pprof debug server on separate port
	go func() {
		logger.Info("starting pprof server", zap.Int("port", 6060))
		if err := http.ListenAndServe(":6060", nil); err != nil {
			logger.Warn("pprof server error", zap.Error(err))
		}
	}()

	logger.Info("starting server", zap.Int("port", cfg.Server.Port))
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Fatal("server error", zap.Error(err))
	}
}

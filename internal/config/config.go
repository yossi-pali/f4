package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Cache    CacheConfig
	EventBus EventBusConfig
	Recheck  RecheckConfig
	Features map[string]bool
	RefCache RefCacheConfig
}

type RefCacheConfig struct {
	EnableOperators   bool          `mapstructure:"enable_operators"`
	EnableStations    bool          `mapstructure:"enable_stations"`
	EnableClasses     bool          `mapstructure:"enable_classes"`
	EnableReasons     bool          `mapstructure:"enable_reasons"`
	EnableIntegration bool          `mapstructure:"enable_integration"`
	RefreshTTL        time.Duration `mapstructure:"refresh_ttl"`
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type DatabaseConfig struct {
	Default         string            `mapstructure:"default"`
	TripPool        map[string]string `mapstructure:"trip_pool"` // region code → DSN
	MaxOpenConns    int               `mapstructure:"max_open_conns"`
	MaxIdleConns    int               `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration     `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration     `mapstructure:"conn_max_idle_time"`
}

type CacheConfig struct {
	RedisAddr     string        `mapstructure:"redis_addr"`
	RedisPassword string        `mapstructure:"redis_password"`
	RedisDB       int           `mapstructure:"redis_db"`
	DefaultTTL    time.Duration `mapstructure:"default_ttl"`
}

type EventBusConfig struct {
	NatsURL string `mapstructure:"nats_url"`
}

type RecheckConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

func Load() (*Config, error) {
	// Load .env file if present (does not override existing OS env vars)
	_ = gotenv.Load()

	v := viper.New()

	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 60*time.Second)
	v.SetDefault("cache.redis_addr", "localhost:6379")
	v.SetDefault("cache.redis_db", 0)
	v.SetDefault("cache.default_ttl", 1*time.Hour)
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 25)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)
	v.SetDefault("database.conn_max_idle_time", 5*time.Minute)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind environment variables
	v.BindEnv("server.port", "SERVER_PORT")
	v.BindEnv("database.default", "DB_DEFAULT_DSN")
	v.BindEnv("cache.redis_addr", "REDIS_ADDR")
	v.BindEnv("cache.redis_password", "REDIS_PASSWORD")
	v.BindEnv("cache.redis_db", "REDIS_DB")
	v.BindEnv("event_bus.nats_url", "NATS_URL")
	v.BindEnv("recheck.base_url", "RECHECK_BASE_URL")
	v.SetDefault("ref_cache.refresh_ttl", 5*time.Minute)

	v.BindEnv("ref_cache.enable_operators", "CACHE_OPERATORS")
	v.BindEnv("ref_cache.enable_stations", "CACHE_STATIONS")
	v.BindEnv("ref_cache.enable_classes", "CACHE_CLASSES")
	v.BindEnv("ref_cache.enable_reasons", "CACHE_REASONS")
	v.BindEnv("ref_cache.enable_integration", "CACHE_INTEGRATION")
	v.BindEnv("ref_cache.refresh_ttl", "CACHE_REFRESH_TTL")

	v.BindEnv("database.max_open_conns", "DB_MAX_OPEN_CONNS")
	v.BindEnv("database.max_idle_conns", "DB_MAX_IDLE_CONNS")
	v.BindEnv("database.conn_max_lifetime", "DB_CONN_MAX_LIFETIME")
	v.BindEnv("database.conn_max_idle_time", "DB_CONN_MAX_IDLE_TIME")

	// Regional trip pool DSNs
	for _, region := range []string{"th", "in", "eu", "asia1", "asia2"} {
		v.BindEnv("database.trip_pool."+region, "DB_TRIPPOOL_"+strings.ToUpper(region)+"_DSN")
	}

	// Try to read config file (not required)
	_ = v.ReadInConfig()

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Load feature flags from env
	cfg.Features = map[string]bool{
		"round_trips":  v.GetBool("FEATURE_ROUND_TRIPS"),
		"autopacks":    v.GetBool("FEATURE_AUTOPACKS"),
		"multiseller":  v.GetBool("FEATURE_MULTISELLER"),
		"afterfilter":  v.GetBool("FEATURE_AFTERFILTER"),
		"discounts":    v.GetBool("FEATURE_DISCOUNTS"),
	}

	return cfg, nil
}

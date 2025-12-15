package server

import (
	"context"
	"fmt"
	"github.com/doujins-org/doujins-billing/internal/manager/web/auth"
	middleware2 "github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"github.com/doujins-org/doujins-billing/internal/manager/web/request"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/pkg/cache"
)

type Dependencies struct {
	Config       *config.Config
	Cache        cache.Cache
	Runtime      *app.Runtime
	Redis        *redis.Client
	AuthVerifier auth.Verifier
}

type Server struct {
	cfg          *config.Config
	cache        cache.Cache
	runtime      *app.Runtime
	rdb          *redis.Client
	authVerifier auth.Verifier

	publicHandler *gin.Engine
	adminHandler  *gin.Engine
}

func New(deps Dependencies) (*Server, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("server config is required")
	}
	if deps.Runtime == nil {
		return nil, fmt.Errorf("server runtime is required")
	}
	if deps.Cache == nil {
		return nil, fmt.Errorf("server cache is required")
	}
	if deps.AuthVerifier == nil {
		return nil, fmt.Errorf("auth verifier is required")
	}

	s := &Server{
		cfg:          deps.Config,
		cache:        deps.Cache,
		runtime:      deps.Runtime,
		rdb:          deps.Redis,
		authVerifier: deps.AuthVerifier,
	}

	s.setupHandlers()
	s.registerPublicRoutes()
	s.registerAdminRoutes()

	log.Info("Billing service initialized successfully")
	return s, nil
}

func (s *Server) setupHandlers() {
	// Public handler with custom logging that excludes health checks
	s.publicHandler = gin.New()
	s.publicHandler.Use(gin.Recovery())
	s.publicHandler.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health/live", "/health/ready", "/healthz", "/readyz"},
	}))
	s.publicHandler.
		Use(middleware2.CORS(s.cfg.CorsOrigins)).
		Use(middleware2.RateLimit(s.cfg.RateLimits, s.rdb))

	// Admin handler (internal only, protected by API key)
	s.adminHandler = gin.New()
	s.adminHandler.Use(gin.Recovery())
	s.adminHandler.Use(middleware2.InternalOnly(s.cfg.BillingAPIKey))
}

func (s *Server) wrap(fn func(r *request.Request)) func(c *gin.Context) {
	return func(c *gin.Context) {
		fn(request.NewRequest(c, s.runtime))
	}
}

func (s *Server) Handler() http.Handler      { return s.publicHandler }
func (s *Server) AdminHandler() http.Handler { return s.adminHandler }

// Close currently does not own underlying resources; callers should close the App.
func (s *Server) Close(_ context.Context) error {
	log.Info("Billing HTTP server shut down")
	return nil
}

// StartWorkers starts River background workers within this server process.
func (s *Server) StartWorkers(ctx context.Context) {
	if s.runtime == nil {
		log.Warn("No state available; skipping worker startup")
		return
	}
	s.runtime.StartWorkers(ctx)
}

func (s *Server) Cfg() *config.Config {
	return s.cfg
}

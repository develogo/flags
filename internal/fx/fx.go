package fx

import (
	"better-feature-flag/internal/config"
	"better-feature-flag/internal/handlers"
	"better-feature-flag/internal/middleware"
	"context"
	"log/slog"
	"os"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"
)

var Module = fx.Module("server",
	fx.Provide(
		ProvideLogger,
		ProvideEcho,
	),
	fx.Invoke(RegisterRoutes),
)

func ProvideLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.App.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	return logger
}

func ProvideEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	return e
}

type RouteParams struct {
	fx.In

	Lifecycle               fx.Lifecycle
	Logger                  *slog.Logger
	Config                  *config.Config
	Echo                    *echo.Echo
	FlagsHandler            *handlers.FlagsHandler
	HealthHandler           *handlers.HealthHandler
	RateLimiter             *middleware.RateLimiter
	ClientContextMiddleware echo.MiddlewareFunc `name:"clientcontext"`
	CORSMiddleware          echo.MiddlewareFunc `name:"cors"`
	LoggerMiddleware        echo.MiddlewareFunc `name:"logger"`
	RequestIDMiddleware     echo.MiddlewareFunc `name:"requestid"`
}

func RegisterRoutes(p RouteParams) {
	// Log startup
	p.Logger.Info("Starting Better Feature Flag")
	p.Logger.Info("configuration loaded",
		slog.String("goff_endpoint", p.Config.Goff.Endpoint),
		slog.String("port", p.Config.App.Port),
	)

	MountRoutes(p)

	// Lifecycle hooks
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				p.Logger.Info("server started", slog.String("port", p.Config.App.Port))
				if err := p.Echo.Start(":" + p.Config.App.Port); err != nil {
					p.Logger.Error("server error", slog.String("error", err.Error()))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			p.Logger.Info("shutting down server")
			if err := p.Echo.Shutdown(ctx); err != nil {
				p.Logger.Error("server shutdown error", slog.String("error", err.Error()))
			}
			p.Logger.Info("server stopped")
			return nil
		},
	})
}

// MountRoutes registra middlewares globais e rotas no Echo, sem iniciar o
// servidor. Separado do lifecycle para os testes montarem a pilha HTTP real.
func MountRoutes(p RouteParams) {
	// Middlewares globais
	p.Echo.Use(p.RequestIDMiddleware)
	p.Echo.Use(p.CORSMiddleware)
	p.Echo.Use(p.LoggerMiddleware)

	// Health checks (sem rate limit)
	p.Echo.GET("/health", p.HealthHandler.Health)
	p.Echo.GET("/ready", p.HealthHandler.Ready)

	// API routes
	api := p.Echo.Group("/api/v1")
	api.Use(p.RateLimiter.Middleware())
	api.Use(p.ClientContextMiddleware)
	api.GET("/flags", p.FlagsHandler.GetFlags)
}

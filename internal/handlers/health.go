package handlers

import (
	"better-feature-flag/internal/models"
	"better-feature-flag/internal/services"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type HealthHandler struct {
	evaluator services.FeatureFlagEvaluator
	registry  services.FlagRegistry
	logger    *slog.Logger
}

func NewHealthHandler(evaluator services.FeatureFlagEvaluator, registry services.FlagRegistry, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{
		evaluator: evaluator,
		registry:  registry,
		logger:    logger,
	}
}

func (h *HealthHandler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, models.HealthResponse{
		Status: "ok",
	})
}

func (h *HealthHandler) Ready(c echo.Context) error {
	ctx := c.Request().Context()

	app, flags, err := h.registry.GetAnyApp()
	if err != nil {
		h.logger.Error("readiness check failed: no flags available", slog.String("error", err.Error()))
		return c.JSON(http.StatusServiceUnavailable, models.HealthResponse{
			Status:  "unavailable",
			Message: "No flags configured",
		})
	}

	if err := h.evaluator.HealthCheck(ctx, app, flags); err != nil {
		h.logger.Error("readiness check failed", slog.String("app", app), slog.String("error", err.Error()))
		return c.JSON(http.StatusServiceUnavailable, models.HealthResponse{
			Status:  "unavailable",
			Message: "GO Feature Flag service is not available",
		})
	}

	return c.JSON(http.StatusOK, models.HealthResponse{
		Status: "ready",
	})
}

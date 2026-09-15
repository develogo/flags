package handlers

import (
	"better-feature-flag/internal/middleware"
	"better-feature-flag/internal/models"
	"better-feature-flag/internal/services"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type FlagsHandler struct {
	evaluator services.FeatureFlagEvaluator
	registry  services.FlagRegistry
	logger    *slog.Logger
}

func NewFlagsHandler(evaluator services.FeatureFlagEvaluator, registry services.FlagRegistry, logger *slog.Logger) *FlagsHandler {
	return &FlagsHandler{
		evaluator: evaluator,
		registry:  registry,
		logger:    logger,
	}
}

// Rota legada: builds instalados do Flutter pedem sem app ou com o nome antigo
// legacyAppName, e são atendidos como legacyRouteApp.
const (
	legacyAppName  = "flutter"
	legacyRouteApp = "bettercity-flutter"
)

func resolveApp(app string) string {
	if app == "" || app == legacyAppName {
		return legacyRouteApp
	}
	return app
}

func (h *FlagsHandler) GetFlags(c echo.Context) error {
	ctx := c.Request().Context()
	clientCtx := middleware.GetClientContext(c)

	appName := resolveApp(c.QueryParam("app"))

	flagDefs, err := h.registry.GetFlagsForApp(appName)
	if err != nil {
		h.logger.Warn("unknown app requested", slog.String("app", appName))
		return c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: "Unknown application: " + appName,
		})
	}

	h.logger.Info("evaluating flags",
		slog.String("app", appName),
		slog.String("targeting_key", clientCtx.GetTargetingKey()),
		slog.String("app_version", clientCtx.AppVersion),
		slog.String("platform", clientCtx.Platform),
	)

	flags, err := h.evaluator.EvaluateFlags(ctx, appName, flagDefs, clientCtx)
	if err != nil {
		h.logger.Error("failed to evaluate flags", slog.String("app", appName), slog.String("error", err.Error()))
		return c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Failed to evaluate feature flags",
		})
	}

	return c.JSON(http.StatusOK, models.FlagsResponse{
		Flags: flags,
	})
}

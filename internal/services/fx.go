package services

import (
	"flags/internal/config"
	"log/slog"

	"go.uber.org/fx"
)

func newDefaultFlagRegistry(logger *slog.Logger) (*FlagRegistryService, error) {
	return NewFlagRegistryService(DefaultFlagsDir, ServedApps, logger)
}

func newDefaultFeatureFlagService(cfg *config.Config, logger *slog.Logger) (*FeatureFlagService, error) {
	return NewFeatureFlagService(cfg, ServedApps, logger)
}

var Module = fx.Module("services",
	fx.Provide(
		fx.Annotate(
			newDefaultFeatureFlagService,
			fx.As(new(FeatureFlagEvaluator)),
		),
		fx.Annotate(
			newDefaultFlagRegistry,
			fx.As(new(FlagRegistry)),
		),
	),
)

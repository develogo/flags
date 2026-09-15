package services

import (
	"log/slog"

	"go.uber.org/fx"
)

func newDefaultFlagRegistry(logger *slog.Logger) (*FlagRegistryService, error) {
	return NewFlagRegistryService(DefaultFlagsDir, ServedApps, logger)
}

var Module = fx.Module("services",
	fx.Provide(
		fx.Annotate(
			NewFeatureFlagService,
			fx.As(new(FeatureFlagEvaluator)),
		),
		fx.Annotate(
			newDefaultFlagRegistry,
			fx.As(new(FlagRegistry)),
		),
	),
)

package services

import (
	"context"
	"flags/internal/models"
)

type FeatureFlagEvaluator interface {
	EvaluateFlags(ctx context.Context, app string, flags []models.FlagDefinition, clientCtx *models.ClientContext) (map[string]interface{}, error)
	HealthCheck(ctx context.Context, app string, flags []models.FlagDefinition) error
}

type FlagRegistry interface {
	GetFlagsForApp(appName string) ([]models.FlagDefinition, error)
	GetAnyApp() (string, []models.FlagDefinition, error)
}

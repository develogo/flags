package services

import (
	"context"
	"flags/internal/config"
	"flags/internal/models"
	"fmt"
	"log/slog"

	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	of "github.com/open-feature/go-sdk/openfeature"
)

// FeatureFlagService avalia flags no relay com um client OpenFeature por app.
// Cada client usa um provider GOFF cuja API key é o nome do app, o que seleciona
// o flag set do app no relay.
type FeatureFlagService struct {
	clients map[string]*of.Client // só leitura depois do construtor
	logger  *slog.Logger
}

// NewFeatureFlagService cria os clients de todos os apps antes de servir: o
// OpenFeature SDK compara os providers ativos ao registrar um novo, o que
// disputa com avaliações em andamento se o registro acontecer sob demanda.
// Os providers ficam no singleton do OpenFeature, com o app como domain.
func NewFeatureFlagService(cfg *config.Config, apps []string, logger *slog.Logger) (*FeatureFlagService, error) {
	clients := make(map[string]*of.Client, len(apps))
	for _, app := range apps {
		provider, err := gofeatureflag.NewProvider(gofeatureflag.ProviderOptions{
			Endpoint:     cfg.Goff.Endpoint,
			APIKey:       app,
			DisableCache: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create provider for app %q: %w", app, err)
		}
		if err := of.SetNamedProvider(app, provider); err != nil {
			return nil, fmt.Errorf("failed to set provider for app %q: %w", app, err)
		}
		clients[app] = of.NewClient(app)
	}

	return &FeatureFlagService{
		clients: clients,
		logger:  logger,
	}, nil
}

func (s *FeatureFlagService) client(app string) (*of.Client, error) {
	c, ok := s.clients[app]
	if !ok {
		return nil, fmt.Errorf("no client for app %q", app)
	}
	return c, nil
}

func (s *FeatureFlagService) EvaluateFlags(ctx context.Context, app string, flagDefs []models.FlagDefinition, clientCtx *models.ClientContext) (map[string]interface{}, error) {
	client, err := s.client(app)
	if err != nil {
		return nil, err
	}
	logger := s.logger.With(slog.String("app", app))
	evalCtx := s.buildEvaluationContext(clientCtx)
	flags := make(map[string]interface{}, len(flagDefs))
	defaultedCount := 0

	for _, def := range flagDefs {
		switch def.Type {
		case models.FlagValueTypeBool:
			defaultValue, ok := def.Default.(bool)
			if !ok {
				defaultedCount++
				flags[def.Name] = false
				logger.Warn("invalid bool flag default; using false",
					slog.String("flag", def.Name),
				)
				continue
			}
			value, err := client.BooleanValue(ctx, def.Name, defaultValue, evalCtx)
			if err != nil {
				defaultedCount++
				flags[def.Name] = defaultValue
				logger.Warn("flag evaluation failed; using default",
					slog.String("flag", def.Name),
					slog.String("type", string(def.Type)),
					slog.String("error", err.Error()),
				)
				continue
			}
			flags[def.Name] = value
		case models.FlagValueTypeString:
			defaultValue, ok := def.Default.(string)
			if !ok {
				defaultedCount++
				flags[def.Name] = ""
				logger.Warn("invalid string flag default; using empty string",
					slog.String("flag", def.Name),
				)
				continue
			}
			value, err := client.StringValue(ctx, def.Name, defaultValue, evalCtx)
			if err != nil {
				defaultedCount++
				flags[def.Name] = defaultValue
				logger.Warn("flag evaluation failed; using default",
					slog.String("flag", def.Name),
					slog.String("type", string(def.Type)),
					slog.String("error", err.Error()),
				)
				continue
			}
			flags[def.Name] = value
		case models.FlagValueTypeInt:
			var defaultInt64 int64
			switch v := def.Default.(type) {
			case int:
				defaultInt64 = int64(v)
			case int64:
				defaultInt64 = v
			case float64:
				defaultInt64 = int64(v)
			default:
				defaultedCount++
				flags[def.Name] = int64(0)
				logger.Warn("invalid int flag default; using 0",
					slog.String("flag", def.Name),
				)
				continue
			}
			value, err := client.IntValue(ctx, def.Name, defaultInt64, evalCtx)
			if err != nil {
				defaultedCount++
				flags[def.Name] = defaultInt64
				logger.Warn("flag evaluation failed; using default",
					slog.String("flag", def.Name),
					slog.String("type", string(def.Type)),
					slog.String("error", err.Error()),
				)
				continue
			}
			flags[def.Name] = value
		case models.FlagValueTypeFloat:
			defaultValue, ok := def.Default.(float64)
			if !ok {
				defaultedCount++
				flags[def.Name] = 0.0
				logger.Warn("invalid float flag default; using 0.0",
					slog.String("flag", def.Name),
				)
				continue
			}
			value, err := client.FloatValue(ctx, def.Name, defaultValue, evalCtx)
			if err != nil {
				defaultedCount++
				flags[def.Name] = defaultValue
				logger.Warn("flag evaluation failed; using default",
					slog.String("flag", def.Name),
					slog.String("type", string(def.Type)),
					slog.String("error", err.Error()),
				)
				continue
			}
			flags[def.Name] = value
		default:
			defaultedCount++
			flags[def.Name] = def.Default
			logger.Warn("unknown flag type; using default",
				slog.String("flag", def.Name),
				slog.String("type", string(def.Type)),
			)
		}
	}

	logger.Info("all flags evaluated",
		slog.Int("count", len(flags)),
		slog.Int("defaulted", defaultedCount),
		slog.String("targeting_key", clientCtx.GetTargetingKey()),
	)

	return flags, nil
}

func (s *FeatureFlagService) buildEvaluationContext(clientCtx *models.ClientContext) of.EvaluationContext {
	attributes := map[string]interface{}{
		"app_version": clientCtx.AppVersion,
		"platform":    clientCtx.Platform,
	}

	if clientCtx.UserID != "" {
		attributes["user_id"] = clientCtx.UserID
	}
	if clientCtx.DeviceID != "" {
		attributes["device_id"] = clientCtx.DeviceID
	}

	return of.NewEvaluationContext(
		clientCtx.GetTargetingKey(),
		attributes,
	)
}

func (s *FeatureFlagService) HealthCheck(ctx context.Context, app string, flags []models.FlagDefinition) error {
	if len(flags) == 0 {
		return fmt.Errorf("no flags available for health check in app %q", app)
	}
	client, err := s.client(app)
	if err != nil {
		return err
	}
	evalCtx := of.NewEvaluationContext("health-check", map[string]interface{}{})
	// ObjectValue aceita qualquer tipo de variation: o health check só prova
	// que o relay avalia, não importa o tipo do primeiro flag.
	_, err = client.ObjectValue(ctx, flags[0].Name, flags[0].Default, evalCtx)
	return err
}

package services

import (
	"better-feature-flag/internal/models"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultFlagsDir é o diretório com os arquivos GOFF por app, relativo ao cwd
// (raiz do repo em `make run`, /app no container).
const DefaultFlagsDir = "flags/apps"

// ServedApps lista os apps que esta API serve. Cada entrada corresponde a
// <flags dir>/<app>.yaml — o mesmo arquivo que o relay carrega. Backends
// consomem o relay direto via SDK; os flags deles não passam por aqui.
var ServedApps = []string{"bettercity-flutter"}

// DiscoverApps lista os apps de dir: um app por arquivo <app>.yaml, com o nome
// do arquivo sem extensão como nome do app. É a regra única de descoberta, usada
// pelo gerador da config do relay e disponível para a API. Diretório ausente ou
// sem apps é erro.
func DiscoverApps(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read apps dir %s: %w", dir, err)
	}

	var apps []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		apps = append(apps, strings.TrimSuffix(entry.Name(), ".yaml"))
	}
	if len(apps) == 0 {
		return nil, fmt.Errorf("no apps found in %s", dir)
	}
	return apps, nil
}

type FlagRegistryService struct {
	order  []string // apps na ordem de registro
	apps   map[string][]models.FlagDefinition
	logger *slog.Logger
}

// goffFlag é o subconjunto do formato GOFF que a API precisa para montar
// um FlagDefinition: tipo (inferido das variations) e fallback (defaultRule).
type goffFlag struct {
	Variations  map[string]any `yaml:"variations"`
	DefaultRule struct {
		Variation string `yaml:"variation"`
	} `yaml:"defaultRule"`
}

// NewFlagRegistryService carrega <dir>/<app>.yaml para cada app em apps.
// Falha no startup se um arquivo estiver ausente ou um flag não tiver
// variations homogêneas de tipo escalar e um defaultRule.variation válido.
func NewFlagRegistryService(dir string, apps []string, logger *slog.Logger) (*FlagRegistryService, error) {
	registry := make(map[string][]models.FlagDefinition, len(apps))
	for _, appName := range apps {
		filePath := filepath.Join(dir, appName+".yaml")
		flags, err := loadGoffFlags(filePath)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", appName, err)
		}
		registry[appName] = flags
	}

	logger.Info("flag registry loaded", slog.Int("apps", len(registry)))
	for appName, flags := range registry {
		logger.Info("registered app flags", slog.String("app", appName), slog.Int("flags", len(flags)))
	}

	return &FlagRegistryService{order: append([]string(nil), apps...), apps: registry, logger: logger}, nil
}

func (r *FlagRegistryService) GetFlagsForApp(appName string) ([]models.FlagDefinition, error) {
	flags, ok := r.apps[appName]
	if !ok {
		return nil, fmt.Errorf("unknown app: %s", appName)
	}
	return flags, nil
}

// GetAnyApp devolve o primeiro app servido (na ordem de registro) que tem flags.
func (r *FlagRegistryService) GetAnyApp() (string, []models.FlagDefinition, error) {
	for _, appName := range r.order {
		if flags := r.apps[appName]; len(flags) > 0 {
			return appName, flags, nil
		}
	}
	return "", nil, fmt.Errorf("no flags registered")
}

func loadGoffFlags(filePath string) ([]models.FlagDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read flags file: %w", err)
	}

	var file map[string]goffFlag
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse flags file %s: %w", filePath, err)
	}

	defs := make([]models.FlagDefinition, 0, len(file))
	for name, flag := range file {
		def, err := toFlagDefinition(name, flag)
		if err != nil {
			return nil, fmt.Errorf("flag %q in %s: %w", name, filePath, err)
		}
		defs = append(defs, def)
	}

	// YAML maps não preservam ordem; ordena para resposta e logs determinísticos.
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs, nil
}

func toFlagDefinition(name string, flag goffFlag) (models.FlagDefinition, error) {
	flagType, err := inferFlagType(flag.Variations)
	if err != nil {
		return models.FlagDefinition{}, err
	}

	variation := flag.DefaultRule.Variation
	if variation == "" {
		return models.FlagDefinition{}, fmt.Errorf("defaultRule.variation is required (the API needs a single fallback value)")
	}
	defaultValue, ok := flag.Variations[variation]
	if !ok {
		return models.FlagDefinition{}, fmt.Errorf("defaultRule.variation %q is not a declared variation", variation)
	}

	return models.FlagDefinition{Name: name, Type: flagType, Default: defaultValue}, nil
}

// inferFlagType deriva o tipo do flag dos valores das variations. Todas
// precisam ter o mesmo tipo escalar; int não é promovido para float.
func inferFlagType(variations map[string]any) (models.FlagValueType, error) {
	if len(variations) == 0 {
		return "", fmt.Errorf("variations cannot be empty")
	}

	var flagType models.FlagValueType
	for variation, value := range variations {
		valueType, ok := scalarFlagType(value)
		if !ok {
			return "", fmt.Errorf("variation %q has invalid type %T (expected bool, string, int or float)", variation, value)
		}
		if flagType == "" {
			flagType = valueType
			continue
		}
		if valueType != flagType {
			return "", fmt.Errorf("variations have mixed types (%s and %s)", flagType, valueType)
		}
	}
	return flagType, nil
}

func scalarFlagType(value any) (models.FlagValueType, bool) {
	switch value.(type) {
	case bool:
		return models.FlagValueTypeBool, true
	case string:
		return models.FlagValueTypeString, true
	case int, int64:
		return models.FlagValueTypeInt, true
	case float64:
		return models.FlagValueTypeFloat, true
	}
	return "", false
}

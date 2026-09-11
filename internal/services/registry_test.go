package services_test

import (
	"better-feature-flag/internal/models"
	"better-feature-flag/internal/services"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	appsDir    = "../../testdata/apps"
	invalidDir = "../../testdata/invalid"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestFlagRegistry_LoadValid(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"flutter"}, testLogger())
	require.NoError(t, err)

	flags, err := registry.GetFlagsForApp("flutter")
	require.NoError(t, err)
	require.Len(t, flags, 2)

	// Ordenado por nome
	assert.Equal(t, models.FlagDefinition{Name: "app_version", Type: models.FlagValueTypeString, Default: "1.0.0"}, flags[0])
	assert.Equal(t, models.FlagDefinition{Name: "dark_mode", Type: models.FlagValueTypeBool, Default: false}, flags[1])
}

func TestFlagRegistry_InfersIntAndFloat(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"typed"}, testLogger())
	require.NoError(t, err)

	flags, err := registry.GetFlagsForApp("typed")
	require.NoError(t, err)
	require.Len(t, flags, 2)

	assert.Equal(t, models.FlagDefinition{Name: "max_items", Type: models.FlagValueTypeInt, Default: 500}, flags[0])
	assert.Equal(t, models.FlagDefinition{Name: "ratio", Type: models.FlagValueTypeFloat, Default: 0.25}, flags[1])
}

func TestFlagRegistry_UnknownApp(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"flutter"}, testLogger())
	require.NoError(t, err)

	_, err = registry.GetFlagsForApp("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown app")
}

func TestFlagRegistry_AppNotServedEvenIfFileExists(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"flutter"}, testLogger())
	require.NoError(t, err)

	_, err = registry.GetFlagsForApp("backend")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown app")
}

func TestFlagRegistry_FileNotFound(t *testing.T) {
	_, err := services.NewFlagRegistryService(appsDir, []string{"missing"}, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `app "missing"`)
}

func TestFlagRegistry_RejectsInvalidFlags(t *testing.T) {
	cases := []struct {
		app     string
		wantErr string
	}{
		{app: "mixed", wantErr: "mixed types"},
		{app: "object", wantErr: "invalid type"},
		{app: "percentage", wantErr: "defaultRule.variation is required"},
		{app: "badref", wantErr: "not a declared variation"},
	}
	for _, tc := range cases {
		t.Run(tc.app, func(t *testing.T) {
			_, err := services.NewFlagRegistryService(invalidDir, []string{tc.app}, testLogger())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestFlagRegistry_GetAnyFlags(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"flutter"}, testLogger())
	require.NoError(t, err)

	flags, err := registry.GetAnyFlags()
	require.NoError(t, err)
	assert.NotEmpty(t, flags)
}

func TestFlagRegistry_MultipleApps(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"flutter", "backend"}, testLogger())
	require.NoError(t, err)

	flutter, err := registry.GetFlagsForApp("flutter")
	require.NoError(t, err)
	assert.Len(t, flutter, 2)

	backend, err := registry.GetFlagsForApp("backend")
	require.NoError(t, err)
	require.Len(t, backend, 1)
	assert.Equal(t, "cache_enabled", backend[0].Name)
	assert.Equal(t, true, backend[0].Default)
}

// Garante que os arquivos reais que o relay carrega também são válidos para a API.
func TestFlagRegistry_LoadsRealServedApps(t *testing.T) {
	registry, err := services.NewFlagRegistryService("../../"+services.DefaultFlagsDir, services.ServedApps, testLogger())
	require.NoError(t, err)

	for _, app := range services.ServedApps {
		flags, err := registry.GetFlagsForApp(app)
		require.NoError(t, err)
		assert.NotEmpty(t, flags, app)
	}
}

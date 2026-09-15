package services_test

import (
	"flags/internal/models"
	"flags/internal/services"
	"log/slog"
	"os"
	"path/filepath"
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
	registry, err := services.NewFlagRegistryService(appsDir, []string{"bettercity-flutter"}, testLogger())
	require.NoError(t, err)

	flags, err := registry.GetFlagsForApp("bettercity-flutter")
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
	registry, err := services.NewFlagRegistryService(appsDir, []string{"bettercity-flutter"}, testLogger())
	require.NoError(t, err)

	_, err = registry.GetFlagsForApp("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown app")
}

func TestFlagRegistry_AppNotServedEvenIfFileExists(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"bettercity-flutter"}, testLogger())
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

func TestFlagRegistry_GetAnyApp(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"bettercity-flutter"}, testLogger())
	require.NoError(t, err)

	app, flags, err := registry.GetAnyApp()
	require.NoError(t, err)
	assert.Equal(t, "bettercity-flutter", app)
	assert.NotEmpty(t, flags)
}

func TestFlagRegistry_MultipleApps(t *testing.T) {
	registry, err := services.NewFlagRegistryService(appsDir, []string{"bettercity-flutter", "backend"}, testLogger())
	require.NoError(t, err)

	flutter, err := registry.GetFlagsForApp("bettercity-flutter")
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

func TestDiscoverApps(t *testing.T) {
	apps, err := services.DiscoverApps(appsDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"backend", "bettercity-flutter", "other", "typed"}, apps)
}

func TestDiscoverApps_IgnoresNonYAMLAndSubdirs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.yaml"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), nil, 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested.yaml"), 0o755))

	apps, err := services.DiscoverApps(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"app"}, apps)
}

func TestDiscoverApps_EmptyOrMissingDir(t *testing.T) {
	_, err := services.DiscoverApps(t.TempDir())
	assert.Error(t, err)

	_, err = services.DiscoverApps(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

// Um arquivo inválido de um app não servido não impede o boot.
func TestFlagRegistry_InvalidPrivateAppDoesNotBlockBoot(t *testing.T) {
	dir := t.TempDir()
	valid, err := os.ReadFile(filepath.Join(appsDir, "bettercity-flutter.yaml"))
	require.NoError(t, err)
	invalid, err := os.ReadFile(filepath.Join(invalidDir, "mixed.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bettercity-flutter.yaml"), valid, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "private.yaml"), invalid, 0o644))

	registry, err := services.NewFlagRegistryService(dir, []string{"bettercity-flutter"}, testLogger())
	require.NoError(t, err)
	_, err = registry.GetFlagsForApp("bettercity-flutter")
	assert.NoError(t, err)
}

// Valida todos os apps reais do repositório com as mesmas invariantes do boot,
// para que um arquivo quebrado de qualquer app reprove o CI.
func TestFlagRegistry_LoadsAllRealApps(t *testing.T) {
	dir := "../../" + services.DefaultFlagsDir
	apps, err := services.DiscoverApps(dir)
	require.NoError(t, err)

	_, err = services.NewFlagRegistryService(dir, apps, testLogger())
	require.NoError(t, err)
}

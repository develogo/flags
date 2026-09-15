package relayconfig_test

import (
	"better-feature-flag/internal/relayconfig"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const appsDir = "../../testdata/apps"

func TestGenerate_OneFlagSetPerApp(t *testing.T) {
	out, err := relayconfig.Generate(appsDir)
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	assert.NotContains(t, cfg, "retrievers", "sem retrievers no nível raiz")
	assert.Equal(t, map[string]any{"mode": "http", "port": 1031}, cfg["server"])

	flagSets, ok := cfg["flagSets"].([]any)
	require.True(t, ok, "flagSets ausente: %s", out)

	byName := map[string]map[string]any{}
	for _, raw := range flagSets {
		set := raw.(map[string]any)
		byName[set["name"].(string)] = set
	}
	require.Len(t, byName, 4)

	for _, app := range []string{"backend", "bettercity-flutter", "other", "typed"} {
		set, ok := byName[app]
		require.True(t, ok, app)
		assert.Equal(t, []any{app}, set["apiKeys"], app)
		assert.Equal(t, []any{map[string]any{
			"kind": "file",
			"path": "../../testdata/apps/" + app + ".yaml",
		}}, set["retrievers"], app)
	}
}

func TestGenerate_EmptyDir(t *testing.T) {
	_, err := relayconfig.Generate(t.TempDir())
	assert.Error(t, err)
}

func TestGenerate_MissingDir(t *testing.T) {
	_, err := relayconfig.Generate(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

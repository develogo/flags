// Package relayconfig gera a config do relay GOFF: um flag set por app.
package relayconfig

import (
	"bytes"
	"flags/internal/services"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// relayPort é a porta em que a API e o compose esperam o relay.
const relayPort = 1031

type config struct {
	Server          server    `yaml:"server"`
	PollingInterval int       `yaml:"pollingInterval"`
	FlagSets        []flagSet `yaml:"flagSets"`
}

type server struct {
	Mode string `yaml:"mode"`
	Port int    `yaml:"port"`
}

type flagSet struct {
	Name       string      `yaml:"name"`
	APIKeys    []string    `yaml:"apiKeys"`
	Retrievers []retriever `yaml:"retrievers"`
}

type retriever struct {
	Kind string `yaml:"kind"`
	Path string `yaml:"path"`
}

// Generate descobre os apps de appsDir e devolve a config do relay em YAML.
// O nome do app é o nome do flag set e sua única API key; o retriever aponta
// para <appsDir>/<app>.yaml, então appsDir deve ser o caminho visto pelo relay.
func Generate(appsDir string) ([]byte, error) {
	apps, err := services.DiscoverApps(appsDir)
	if err != nil {
		return nil, err
	}

	cfg := config{
		Server:          server{Mode: "http", Port: relayPort},
		PollingInterval: 1000,
		FlagSets:        make([]flagSet, 0, len(apps)),
	}
	dir := filepath.ToSlash(appsDir)
	for _, app := range apps {
		cfg.FlagSets = append(cfg.FlagSets, flagSet{
			Name:       app,
			APIKeys:    []string{app},
			Retrievers: []retriever{{Kind: "file", Path: path.Join(dir, app+".yaml")}},
		})
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

package engine

import _ "embed"

// DefaultConfigYAML is the embedded Plutchik-8 default configuration.
//
//go:embed plutchik8.default.yaml
var DefaultConfigYAML []byte

// DefaultConfig parses the embedded default configuration.
func DefaultConfig() (Config, error) {
	return ParseConfig(DefaultConfigYAML)
}

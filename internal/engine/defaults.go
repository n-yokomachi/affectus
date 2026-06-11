package engine

import _ "embed"

//go:embed plutchik.default.yaml
var plutchikYAML []byte

//go:embed russell.default.yaml
var russellYAML []byte

//go:embed occ.default.yaml
var occYAML []byte

// Models maps a model identifier to its embedded default configuration YAML.
var Models = map[string][]byte{
	"plutchik": plutchikYAML,
	"russell":  russellYAML,
	"occ":      occYAML,
}

// DefaultConfigYAML is the embedded Plutchik-8 default configuration. Kept as
// the package default for backward compatibility.
var DefaultConfigYAML = plutchikYAML

// DefaultConfig parses the embedded default configuration.
func DefaultConfig() (Config, error) {
	return ParseConfig(DefaultConfigYAML)
}

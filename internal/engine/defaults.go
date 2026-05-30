package engine

import _ "embed"

//go:embed plutchik8.default.yaml
var plutchikYAML []byte

//go:embed russell.default.yaml
var russellYAML []byte

// Models maps a model identifier to its embedded default configuration YAML.
var Models = map[string][]byte{
	"plutchik": plutchikYAML,
	"russell":  russellYAML,
}

// DefaultConfigYAML is the embedded Plutchik-8 default configuration. Kept as
// the package default for backward compatibility.
var DefaultConfigYAML = plutchikYAML

// DefaultConfig parses the embedded default configuration.
func DefaultConfig() (Config, error) {
	return ParseConfig(DefaultConfigYAML)
}

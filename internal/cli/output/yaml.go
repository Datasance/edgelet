package output

import "gopkg.in/yaml.v3"

// YAMLFormatter renders YAML to stdout.
type YAMLFormatter struct{}

func (YAMLFormatter) Format(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

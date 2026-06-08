package models

// YamlConfig represents the root YAML configuration structure
type YamlConfig struct {
	CurrentProfile string                    `json:"currentProfile" yaml:"currentProfile"`
	Profiles       map[string]*ProfileConfig `json:"profiles" yaml:"profiles"`
}

// NewYamlConfig creates a new YamlConfig
func NewYamlConfig() *YamlConfig {
	return &YamlConfig{
		Profiles: make(map[string]*ProfileConfig),
	}
}

// ProfileConfig represents a single profile's configuration
// Uses a map to store all properties for flexible YAML mapping
type ProfileConfig struct {
	Properties map[string]string `json:"-" yaml:",inline"`
}

// NewProfileConfig creates a new ProfileConfig
func NewProfileConfig() *ProfileConfig {
	return &ProfileConfig{
		Properties: make(map[string]string),
	}
}

// GetProperty gets a property value
func (p *ProfileConfig) GetProperty(key string) string {
	if p.Properties == nil {
		return ""
	}
	return p.Properties[key]
}

// SetProperty sets a property value
func (p *ProfileConfig) SetProperty(key, value string) {
	if p.Properties == nil {
		p.Properties = make(map[string]string)
	}
	if value == "" {
		p.Properties[key] = ""
	} else {
		p.Properties[key] = value
	}
}

// UnmarshalYAML custom unmarshaling to handle flexible YAML structure
func (p *ProfileConfig) UnmarshalYAML(unmarshal func(any) error) error {
	if p.Properties == nil {
		p.Properties = make(map[string]string)
	}

	var m map[string]any
	if err := unmarshal(&m); err != nil {
		return err
	}

	for k, v := range m {
		if str, ok := v.(string); ok {
			p.Properties[k] = str
		} else {
			// Convert non-string values to string
			p.Properties[k] = ""
		}
	}

	return nil
}

// MarshalYAML custom marshaling to output as map
func (p *ProfileConfig) MarshalYAML() (any, error) {
	if p.Properties == nil {
		return make(map[string]string), nil
	}
	return p.Properties, nil
}

// GetProfile gets a profile by name
func (y *YamlConfig) GetProfile(name string) *ProfileConfig {
	if y.Profiles == nil {
		return nil
	}
	return y.Profiles[name]
}

package models

// EnvVar represents microservice environment variables
type EnvVar struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

// NewEnvVar creates a new EnvVar
func NewEnvVar(key, value string) *EnvVar {
	return &EnvVar{
		Key:   key,
		Value: value,
	}
}

// Equals checks if two EnvVars are equal
func (e *EnvVar) Equals(other *EnvVar) bool {
	if other == nil {
		return false
	}
	return e.Key == other.Key && e.Value == other.Value
}

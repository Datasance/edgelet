package models

// EdgeResource represents an edge resource
type EdgeResource struct {
	ID                int                    `json:"id" yaml:"id"`
	Name              string                 `json:"name" yaml:"name"`
	Version           string                 `json:"version" yaml:"version"`
	Description       string                 `json:"description,omitempty" yaml:"description,omitempty"`
	InterfaceProtocol string                 `json:"interfaceProtocol,omitempty" yaml:"interfaceProtocol,omitempty"`
	Display           *Display               `json:"display,omitempty" yaml:"display,omitempty"`
	OrchestrationTags []string               `json:"orchestrationTags,omitempty" yaml:"orchestrationTags,omitempty"`
	EdgeInterface     *EdgeInterface         `json:"edgeInterface,omitempty" yaml:"edgeInterface,omitempty"`
	Custom            map[string]interface{} `json:"custom,omitempty" yaml:"custom,omitempty"`
}

// Display represents display configuration for an edge resource
type Display struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Icon        string `json:"icon,omitempty" yaml:"icon,omitempty"`
}

// EdgeInterface represents the interface configuration for an edge resource
type EdgeInterface struct {
	Type        string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Protocol    string                 `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
	Permissions []string               `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

// Equals checks if two EdgeResources are equal (based on name and version)
func (er *EdgeResource) Equals(other *EdgeResource) bool {
	if other == nil {
		return false
	}
	return er.Name == other.Name && er.Version == other.Version
}

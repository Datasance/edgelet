package models

// Registry represents Docker registry configuration
type Registry struct {
	ID        int    `json:"id" yaml:"id"`
	URL       string `json:"url" yaml:"url"`
	IsPublic  bool   `json:"isPublic" yaml:"isPublic"`
	UserName  string `json:"userName,omitempty" yaml:"userName,omitempty"`
	Password  string `json:"password,omitempty" yaml:"password,omitempty"`
	UserEmail string `json:"userEmail,omitempty" yaml:"userEmail,omitempty"`
}

// NewRegistry creates a new Registry
func NewRegistry(id int, url string, isPublic bool, userName, password, userEmail string) *Registry {
	return &Registry{
		ID:        id,
		URL:       url,
		IsPublic:  isPublic,
		UserName:  userName,
		Password:  password,
		UserEmail: userEmail,
	}
}

// Equals checks if two Registries are equal
func (r *Registry) Equals(other *Registry) bool {
	if other == nil {
		return false
	}
	return r.ID == other.ID && r.IsPublic == other.IsPublic
}

// RegistryBuilder is a builder for creating Registry instances
type RegistryBuilder struct {
	id        int
	url       string
	isPublic  bool
	userName  string
	password  string
	userEmail string
}

// NewRegistryBuilder creates a new RegistryBuilder
func NewRegistryBuilder() *RegistryBuilder {
	return &RegistryBuilder{}
}

// SetID sets the registry ID
func (b *RegistryBuilder) SetID(id int) *RegistryBuilder {
	b.id = id
	return b
}

// SetURL sets the registry URL
func (b *RegistryBuilder) SetURL(url string) *RegistryBuilder {
	b.url = url
	return b
}

// SetIsPublic sets whether the registry is public
func (b *RegistryBuilder) SetIsPublic(isPublic bool) *RegistryBuilder {
	b.isPublic = isPublic
	return b
}

// SetUserName sets the registry username
func (b *RegistryBuilder) SetUserName(userName string) *RegistryBuilder {
	b.userName = userName
	return b
}

// SetPassword sets the registry password
func (b *RegistryBuilder) SetPassword(password string) *RegistryBuilder {
	b.password = password
	return b
}

// SetUserEmail sets the registry user email
func (b *RegistryBuilder) SetUserEmail(userEmail string) *RegistryBuilder {
	b.userEmail = userEmail
	return b
}

// Build creates a Registry from the builder
func (b *RegistryBuilder) Build() *Registry {
	return NewRegistry(b.id, b.url, b.isPublic, b.userName, b.password, b.userEmail)
}

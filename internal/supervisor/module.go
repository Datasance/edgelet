package supervisor

// Module represents an ioFog module that can be managed by the Supervisor
type Module interface {
	// Start starts the module
	Start() error
	
	// Stop stops the module gracefully
	Stop() error
	
	// GetName returns the module name
	GetName() string
	
	// GetModuleIndex returns the module index (for status reporting)
	GetModuleIndex() int
}

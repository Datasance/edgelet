# Models Package

This package contains Edgelet data structures and domain models shared across the daemon, field agent, EdgeletAPI, and CLI.

## Overview

Models provide:

- JSON/YAML marshaling support
- Controller-compatible field naming for PoT wire formats
- Validation functions
- Constructor functions (New*)
- Thread-safe operations where needed

## Models

### Core Models

- **Microservice**: Main microservice/container configuration
- **MicroserviceState**: Enum for microservice states
- **PortMapping**: Port mapping configuration
- **EnvVar**: Environment variable
- **VolumeMapping**: Volume mapping configuration
- **VolumeMappingType**: Enum for volume mapping types
- **Healthcheck**: Container healthcheck configuration
- **Route**: Microservice routing configuration
- **Registry**: Docker registry configuration

### Message Models

- **Message**: ioMessage with binary and JSON serialization support
- **ExecMessage**: Exec session WebSocket message
- **LogMessage**: Log session WebSocket message

### Status Models

- **MicroserviceStatus**: Microservice status information
- **FieldAgentStatus**: Field agent status
- **ControllerStatus**: Controller connection status enum
- **StatusReporterStatus**: Status reporter status

### Configuration Models

- **YamlConfig**: Root YAML configuration structure
- **ProfileConfig**: Profile configuration with flexible property mapping

## Usage

### Creating Models

```go
// Create a microservice
ms := models.NewMicroservice("uuid-123", "image:tag")
ms.SetMemoryLimitMB(int64Ptr(512))

// Create a port mapping
pm := models.NewPortMapping(8080, 80, false)

// Create a registry using builder
reg := models.NewRegistryBuilder().
    SetID(1).
    SetURL("https://registry.example.com").
    SetIsPublic(true).
    Build()
```

### JSON Marshaling

```go
// Marshal to JSON
jsonData, err := json.Marshal(ms)
if err != nil {
    log.Fatal(err)
}

// Unmarshal from JSON
var ms2 models.Microservice
err = json.Unmarshal(jsonData, &ms2)
```

### Validation

```go
// Validate a model
if err := models.ValidateMicroservice(ms); err != nil {
    log.Fatal(err)
}

// Or use the model's Validate method
if err := ms.Validate(); err != nil {
    log.Fatal(err)
}
```

## JSON compatibility

Models use PoT-compatible JSON field names. Optional fields use pointers to distinguish nil from zero values.

## Thread Safety

- **Microservice.IsUpdating**: Thread-safe getter/setter using sync.RWMutex
- **DeleteLock**: Global mutex for microservice deletion operations

## Testing

Run tests with:
```bash
go test ./internal/models/... -v
```

## Notes

- Binary serialization for Message is not yet fully implemented (placeholder for future work)
- All optional fields use pointers to distinguish between nil and zero values
- Enum types use string-based constants for JSON compatibility

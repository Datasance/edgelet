# ioFog Agent Architecture

## Overview

The ioFog Agent is a Go-based edge computing agent that manages microservices on edge devices. It provides container orchestration, message routing, and communication with the ioFog Controller.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      Supervisor                              │
│  (Orchestrates all modules, lifecycle management)           │
└─────────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ Field Agent  │    │ Process      │    │ Message Bus  │
│              │    │ Manager      │    │              │
│ Controller   │    │              │    │ AMQP        │
│ Communication│    │ Docker       │    │ Routing      │
└──────────────┘    └──────────────┘    └──────────────┘
        │                   │                   │
        │                   │                   │
        ▼                   ▼                   ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ Local API    │    │ Resource     │    │ Status       │
│              │    │ Manager      │    │ Reporter     │
│ REST/WS      │    │              │    │              │
│ Server       │    │ Monitoring   │    │ Status Info  │
└──────────────┘    └──────────────┘    └──────────────┘
        │
        ▼
┌──────────────┐
│ GPS Manager  │
│              │
│ Location     │
│ Services     │
└──────────────┘
```

## Core Modules

### Supervisor

The Supervisor is the root module that orchestrates all other modules. It:
- Starts and stops modules in the correct order
- Monitors module health
- Handles graceful shutdown
- Manages configuration

**Dependencies**: All other modules

### Field Agent

The Field Agent manages communication with the ioFog Controller:
- Polls controller for configuration changes
- Syncs microservice definitions
- Handles provisioning
- Manages WebSocket connections for exec/log streaming

**Dependencies**: Config, Auth, StatusReporter

### Process Manager

The Process Manager manages Docker containers:
- Creates, starts, stops, and removes containers
- Monitors container health
- Manages container networking
- Handles container logs

**Dependencies**: Docker, FieldAgent (for microservice definitions)

### Message Bus

The Message Bus provides inter-microservice messaging:
- AMQP-based message routing
- Topic-based routing
- Message archiving
- Publisher/receiver management

**Dependencies**: AMQP, FieldAgent (for routing configuration)

### Local API

The Local API provides REST and WebSocket endpoints:
- REST API for agent management
- WebSocket for real-time communication
- Authentication via JWT
- Microservice management endpoints

**Dependencies**: Config, Auth, FieldAgent, ProcessManager

### Resource Manager

The Resource Manager monitors system resources:
- CPU usage
- Memory usage
- Disk usage
- Network statistics

**Dependencies**: System APIs (gopsutil)

### Status Reporter

The Status Reporter collects and reports agent status:
- Module status
- Daemon status
- Operation duration
- Warning messages

**Dependencies**: All modules (for status collection)

### GPS Manager

The GPS Manager provides GPS location services:
- GPS device management
- Location tracking
- NMEA parsing

**Dependencies**: GPS hardware, NMEA parser

## Data Flow

### Microservice Lifecycle

1. **Provisioning**: Controller sends microservice definition to Field Agent
2. **Sync**: Field Agent updates Process Manager with new microservice
3. **Deploy**: Process Manager pulls image and creates container
4. **Start**: Process Manager starts container
5. **Monitor**: Process Manager and Resource Manager monitor container
6. **Message Routing**: Message Bus routes messages to/from container
7. **Update/Delete**: Field Agent receives updates, Process Manager applies changes

### Message Flow

1. **Publish**: Microservice publishes message to Message Bus
2. **Route**: Message Bus routes message based on routing configuration
3. **Deliver**: Message Bus delivers message to target microservice
4. **Archive**: Messages are archived for replay

### Status Flow

1. **Collect**: Status Reporter collects status from all modules
2. **Aggregate**: Status Reporter aggregates status information
3. **Report**: Status Reporter reports to Controller via Field Agent
4. **Expose**: Status Reporter exposes status via Local API

## Communication Patterns

### Controller Communication

- **HTTP/HTTPS**: REST API for configuration sync
- **WebSocket**: Real-time exec/log streaming (MessagePack encoded)
- **Polling**: Periodic status updates

### Inter-Module Communication

- **Channels**: Go channels for internal communication
- **Interfaces**: Go interfaces for module abstraction
- **Context**: Context for cancellation and timeouts

### External Communication

- **Docker API**: Container management
- **AMQP**: Message bus
- **REST API**: Local API endpoints
- **WebSocket**: Real-time communication

## Configuration

Configuration is managed through:
- YAML configuration file
- Environment variables
- Controller-provided configuration

## Security

- **Authentication**: JWT tokens for API access
- **TLS**: HTTPS for secure communication
- **Certificates**: X.509 certificates for controller communication
- **Authorization**: Role-based access control

## Performance Considerations

- **Goroutines**: Concurrent module execution
- **Connection Pooling**: Reused HTTP connections
- **Caching**: Cached configuration and status
- **Resource Limits**: Container resource limits

## Error Handling

- **Graceful Degradation**: Continue operation on non-critical errors
- **Retry Logic**: Automatic retry for transient errors
- **Error Logging**: Comprehensive error logging
- **Status Reporting**: Error status reported to controller

## Deployment

The agent can be deployed as:
- **System Service**: systemd service
- **Docker Container**: Containerized deployment
- **Kubernetes Pod**: Kubernetes deployment
- **Standalone Binary**: Direct binary execution

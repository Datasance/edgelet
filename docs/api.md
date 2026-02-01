# ioFog Agent API Documentation

## Local API

The ioFog Agent exposes a Local API for managing the agent and its microservices.

### Base URL

By default, the Local API is available at:
- HTTP: `http://localhost:54321`
- WebSocket: `ws://localhost:54321`

### Authentication

The Local API uses JWT tokens for authentication. Tokens can be obtained through the provisioning process or generated locally.

### Endpoints

#### Status Endpoints

##### GET /status
Get agent status information.

**Response:**
```json
{
  "status": "running",
  "timestamp": 1234567890,
  "uptime": 3600,
  "modules": {
    "supervisor": "running",
    "fieldAgent": "running",
    "processManager": "running",
    "messageBus": "running",
    "localApi": "running"
  }
}
```

##### GET /status/supervisor
Get supervisor status.

##### GET /status/fieldagent
Get Field Agent status.

##### GET /status/processmanager
Get Process Manager status.

##### GET /status/messagebus
Get Message Bus status.

#### Microservice Endpoints

##### GET /microservices
List all microservices.

**Response:**
```json
{
  "microservices": [
    {
      "uuid": "microservice-uuid",
      "name": "microservice-name",
      "status": "running",
      "containerId": "container-id"
    }
  ]
}
```

##### GET /microservices/:uuid
Get microservice details.

##### POST /microservices/:uuid/restart
Restart a microservice.

##### DELETE /microservices/:uuid
Stop and remove a microservice.

#### Message Bus Endpoints

##### POST /messages
Publish a message to the message bus.

**Request:**
```json
{
  "publisher": "publisher-id",
  "message": "base64-encoded-message"
}
```

##### GET /messages/:publisher
Get messages for a publisher.

#### Configuration Endpoints

##### GET /config
Get current configuration.

##### POST /config
Update configuration (requires restart).

#### WebSocket Endpoints

##### /exec/:uuid
WebSocket endpoint for executing commands in a microservice container.

##### /logs/:uuid
WebSocket endpoint for streaming logs from a microservice container.

### Error Responses

All endpoints return standard HTTP status codes:

- `200 OK`: Success
- `400 Bad Request`: Invalid request
- `401 Unauthorized`: Authentication required
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error

Error response format:
```json
{
  "error": "Error message",
  "code": "ERROR_CODE"
}
```

## Field Agent API

The Field Agent communicates with the ioFog Controller using the Controller API.

### Controller Communication

- **Protocol**: HTTP/HTTPS
- **WebSocket**: For exec and log streaming (MessagePack encoded)
- **Authentication**: JWT tokens

### Endpoints

The Field Agent communicates with the controller at the configured controller URL.

## Message Bus

The Message Bus uses AMQP for inter-microservice communication.

### AMQP Configuration

- **Protocol**: AMQP 0.9.1
- **Exchange**: `iofog`
- **Routing**: Topic-based routing

### Message Format

Messages are encoded in the ioMessage format as specified in the ioFog Message Specification.

## Process Manager

The Process Manager manages Docker containers for microservices.

### Docker Integration

- Uses Docker API for container lifecycle management
- Supports Docker, Podman, and Kubernetes (via Docker API)

## GPS Manager

The GPS Manager provides GPS location information.

### Endpoints

##### GET /gps/status
Get GPS status and current location.

**Response:**
```json
{
  "enabled": true,
  "latitude": 40.7128,
  "longitude": -74.0060,
  "altitude": 10.5,
  "timestamp": 1234567890
}
```

## Resource Manager

The Resource Manager monitors system resources.

### Endpoints

##### GET /resources
Get current resource usage.

**Response:**
```json
{
  "cpu": {
    "usage": 25.5,
    "cores": 4
  },
  "memory": {
    "used": 2048,
    "total": 8192,
    "usage": 25.0
  },
  "disk": {
    "used": 10240,
    "total": 50000,
    "usage": 20.5
  }
}
```

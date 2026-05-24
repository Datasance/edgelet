# ioFog Agent API Documentation

## Local API

The ioFog Agent exposes a Local API for managing the agent and its microservices.

> Note: current implementation is LocalAPI **v3** (`/v1/...`). Legacy path examples in this file are historical and should be treated as non-authoritative.

### Base URL

By default, the Local API is available at:
- HTTP: `http://localhost:54321`
- WebSocket: `ws://localhost:54321`

### Authentication

The Local API uses JWT tokens for authentication. Tokens can be obtained through the provisioning process or generated locally.

### Endpoints

#### RuntimeClass endpoints (v3)

- `POST /v1/deploy/runtimeclasses:validate`
- `POST /v1/deploy/runtimeclasses:apply`
- `GET /v1/deploy/runtimeclasses:apply/{operationId}`
- `GET /v1/deploy/runtimeclasses`
- `GET /v1/deploy/runtimeclasses/{name}`
- `DELETE /v1/deploy/runtimeclasses/{name}`
- `GET /v1/deploy/runtimeclasses:delete/{operationId}`

RuntimeClass support is gated to `full` flavor with `containerEngine=iofog`.
Unsupported modes return:
`400 INVALID_ARGUMENT` + `runtimeclass is supported only when containerEngine=iofog on full flavor builds`.

##### RuntimeClass apply contract

Request transport is `multipart/form-data` with fields:
- `manifest` (required)
- `dryRun` (optional boolean, default `false`)
- `async` (optional boolean, default `false`)

Semantics:
- `dryRun=true`: synchronous validation only (`200`), no persistence.
- `dryRun=false, async=true`: returns `202` with `operationId`; execution continues in background.
- `dryRun=false, async=false`: bounded synchronous wait:
  - completes in-window -> `200`
  - still running -> `202` with `operationId`
- Poll endpoint (`GET ...:apply/{operationId}`):
  - known operation -> `200` with `data.status` and optional `data.error`
  - unknown operation -> `404 NOT_FOUND`

##### RuntimeClass delete contract

Request transport:
- `DELETE /v1/deploy/runtimeclasses/{name}`
- optional query: `async=true|false` (default `false`)

Semantics:
- `async=true`: returns `202` with `operationId`.
- `async=false`: bounded synchronous wait (`200` on completion, `202` fallback).
- Poll endpoint (`GET ...:delete/{operationId}`):
  - known operation -> `200` with `data.status` and optional `data.error`
  - unknown operation -> `404 NOT_FOUND`

Guards:
- reserved runtime names (for example `crun`) -> `400 INVALID_ARGUMENT`
- in-use runtime class (microservice currently using canonical `name`) -> `400 INVALID_ARGUMENT` with blocking UUID details

##### RuntimeClass examples

Logical request (docs clarity):
```json
{
  "manifest": "apiVersion: iofog.org/v3\nkind: RuntimeClass\nmetadata:\n  name: spin\nhandler: spin\n",
  "dryRun": false,
  "async": true
}
```

Actual multipart form:
```json
{
  "contentType": "multipart/form-data",
  "fields": {
    "manifest": "<runtimeclass yaml>",
    "dryRun": "false",
    "async": "true"
  }
}
```

Apply dry-run success (`200`):
```json
{
  "success": true,
  "data": {
    "operationId": "5e8a0f53-c036-4f79-8f5f-4a8d14978258",
    "status": "succeeded",
    "stage": "done",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": true,
    "startedAt": "2026-05-21T12:40:01.123456Z",
    "endedAt": "2026-05-21T12:40:01.127000Z",
    "runtimeClass": {
      "name": "spin",
      "handler": "spin",
      "runtimeName": "spin"
    }
  }
}
```

Apply async accepted (`202`):
```json
{
  "success": true,
  "data": {
    "operationId": "91efc3cf-63f0-4587-a646-0a6f336d92e3",
    "status": "queued",
    "stage": "write_config",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": false,
    "startedAt": "2026-05-21T12:40:01.123456Z"
  }
}
```

Apply sync immediate success (`200`):
```json
{
  "success": true,
  "data": {
    "operationId": "2a9a1e69-35d7-4f16-985c-f1ac7d588f15",
    "status": "succeeded",
    "stage": "done",
    "kind": "RuntimeClass",
    "name": "edgelet",
    "dryRun": false,
    "startedAt": "2026-05-21T12:41:10.001Z",
    "endedAt": "2026-05-21T12:41:11.438Z",
    "runtimeClass": {
      "name": "edgelet",
      "handler": "edgelet-wasm",
      "runtimeName": "edgelet"
    }
  }
}
```

Apply sync timeout fallback (`202`):
```json
{
  "success": true,
  "data": {
    "operationId": "e522f786-9fd3-4d2a-b2d0-3ddc12f2d2d4",
    "status": "running",
    "stage": "write_config",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": false,
    "startedAt": "2026-05-21T12:42:05.882Z"
  }
}
```

Apply poll running (`200`):
```json
{
  "success": true,
  "data": {
    "operationId": "e522f786-9fd3-4d2a-b2d0-3ddc12f2d2d4",
    "status": "running",
    "stage": "write_config",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": false,
    "startedAt": "2026-05-21T12:42:05.882Z"
  }
}
```

Apply poll succeeded (`200`):
```json
{
  "success": true,
  "data": {
    "operationId": "e522f786-9fd3-4d2a-b2d0-3ddc12f2d2d4",
    "status": "succeeded",
    "stage": "done",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": false,
    "startedAt": "2026-05-21T12:42:05.882Z",
    "endedAt": "2026-05-21T12:42:09.217Z",
    "runtimeClass": {
      "name": "spin",
      "handler": "spin",
      "runtimeName": "spin"
    }
  }
}
```

Apply poll failed (`200`, operation resource style):
```json
{
  "success": true,
  "data": {
    "operationId": "e522f786-9fd3-4d2a-b2d0-3ddc12f2d2d4",
    "status": "failed",
    "stage": "write_config",
    "kind": "RuntimeClass",
    "name": "spin",
    "dryRun": false,
    "startedAt": "2026-05-21T12:42:05.882Z",
    "endedAt": "2026-05-21T12:42:50.901Z",
    "error": {
      "code": "INTERNAL",
      "message": "failed to upsert local runtimeclass: database is locked"
    }
  }
}
```

Malformed manifest (`400 INVALID_ARGUMENT`):
```json
{
  "success": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "invalid runtimeclass manifest YAML: yaml: unmarshal errors: field handlr not found in type models.LocalRuntimeClassManifest"
  }
}
```

Unsupported mode (`400 INVALID_ARGUMENT`):
```json
{
  "success": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "runtimeclass is supported only when containerEngine=iofog on full flavor builds"
  }
}
```

Unknown apply operation (`404 NOT_FOUND`):
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "runtimeclass apply operation not found"
  }
}
```

Delete async accepted (`202`):
```json
{
  "success": true,
  "data": {
    "operationId": "7f8fd1dd-2ac6-4c5d-a4fb-e3f6fbb5ec74",
    "status": "queued",
    "stage": "write_config",
    "kind": "RuntimeClassDelete",
    "name": "spin",
    "startedAt": "2026-05-21T12:45:03.100Z",
    "runtimeClass": {
      "name": "spin",
      "handler": "spin",
      "runtimeName": "spin"
    }
  }
}
```

Delete reserved runtime (`400 INVALID_ARGUMENT`):
```json
{
  "success": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "runtimeclass delete is not allowed for reserved runtime name: crun",
    "details": {
      "runtimeClassName": "crun"
    }
  }
}
```

Delete in-use runtime (`400 INVALID_ARGUMENT`):
```json
{
  "success": false,
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "cannot delete runtimeclass 'spin': microservice uuid=4b501939-43b5-4523-a417-577518409df0 is still using runtime 'spin'; delete dependent microservices first",
    "details": {
      "runtimeClassName": "spin",
      "runtimeNames": [
        "spin"
      ],
      "blockingMicroserviceUuids": [
        "4b501939-43b5-4523-a417-577518409df0"
      ]
    }
  }
}
```

Unknown delete operation (`404 NOT_FOUND`):
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "runtimeclass delete operation not found"
  }
}
```

#### Status Endpoints

##### GET /status
Get agent status information.

For v3 (`GET /v1/system/status`), response includes `availableRuntimes`.

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
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Error message",
    "details": {}
  }
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

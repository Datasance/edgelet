# LocalAPI v1 Error Codes

Stable LocalAPI v1 error taxonomy for CLI and API consumers.

## Codes

- `INVALID_ARGUMENT`: malformed payload, unsupported field/value, validation error
- `UNAUTHORIZED`: missing/invalid authentication token
- `FORBIDDEN`: authenticated but not allowed
- `NOT_FOUND`: requested resource does not exist
- `CONFLICT`: state conflict prevents the operation
- `NOT_IMPLEMENTED`: endpoint or operation not implemented
- `METHOD_NOT_ALLOWED`: HTTP method is not supported for endpoint
- `INTERNAL`: unexpected server-side failure

## CLI Exit Mapping

- `INVALID_ARGUMENT` -> exit code `2`
- `UNAUTHORIZED` / `FORBIDDEN` -> exit code `3`
- `NOT_FOUND` -> exit code `4`
- `CONFLICT` -> exit code `5`
- `NOT_IMPLEMENTED` -> exit code `6`
- all others -> exit code `1`

## RuntimeClass-specific deterministic reject

When RuntimeClass endpoints are called outside supported mode (`full` + `containerEngine=iofog`), response is:

- HTTP: `400`
- code: `INVALID_ARGUMENT`
- message: `runtimeclass is supported only when containerEngine=iofog on full flavor builds`

## RuntimeClass delete deterministic rejects

- Reserved runtime class delete (for example `crun`):
  - HTTP: `400`
  - code: `INVALID_ARGUMENT`
  - message: `runtimeclass delete is not allowed for reserved runtime name: <name>`
  - details include `runtimeClassName`

- Runtime class in use by running microservice(s):
  - HTTP: `400`
  - code: `INVALID_ARGUMENT`
  - message includes blocking microservice UUID and runtime name
  - details include:
    - `runtimeClassName`
    - `runtimeNames`
    - `blockingMicroserviceUuids`

## RuntimeClass operation polling semantics

- `GET /v1/deploy/runtimeclasses:apply/{operationId}`
- `GET /v1/deploy/runtimeclasses:delete/{operationId}`

For known operations, polling always returns HTTP `200` with `success=true`.
Terminal operation failure is represented as:

- `data.status=failed`
- `data.error.code`
- `data.error.message`
- optional `data.error.details`

Unknown operation IDs return:

- HTTP: `404`
- code: `NOT_FOUND`

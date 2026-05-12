# LocalAPI v3 Error Codes

Stable LocalAPI v3 error taxonomy for CLI and API consumers.

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

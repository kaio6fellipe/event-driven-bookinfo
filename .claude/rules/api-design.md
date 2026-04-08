---
globs:
  - "**/adapter/inbound/http/**"
---

# HTTP API Design Rules

## Status Codes
- `201 Created` for successful POST (resource created). Include the created resource in the response body.
- `200 OK` for successful GET.
- `204 No Content` for successful DELETE.
- `400 Bad Request` for validation errors or malformed input.
- `404 Not Found` when a requested resource does not exist.
- `500 Internal Server Error` for unexpected failures (should be rare).

## Error Responses
Always return JSON errors in this format:
```json
{"error": "human-readable description of the problem"}
```

## Request/Response Types (DTOs)
- DTOs live in `dto.go`, separate from domain types.
- Request DTOs: `CreateUserRequest`, `UpdateOrderRequest`.
- Response DTOs: `UserResponse`, `OrderResponse`.
- Conversion functions: `toResponse(domain) Response`, not methods on domain types.

## Handler Patterns
- Parse input (path params, query params, JSON body).
- Validate input at the handler level before calling the service.
- Call service via the inbound port interface.
- Convert domain result to response DTO.
- Write JSON response with appropriate status code.

## JSON Conventions
- Use `json:"snake_case"` struct tags.
- Use `omitempty` for optional fields.
- Set `Content-Type: application/json` header on all JSON responses.

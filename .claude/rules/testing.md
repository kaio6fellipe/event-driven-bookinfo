# Testing Standards

## Table-Driven Tests
Use table-driven tests for any function with multiple input/output combinations:

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {name: "valid input", input: "hello", want: "HELLO"},
        {name: "empty input", input: "", wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

## Coverage
- Target 80%+ coverage on service and domain packages.
- Run with: `go test -coverprofile=coverage.out ./services/<name>/...`

## Mocking
- Mock interfaces only, never concrete types.
- For service-layer tests: use the in-memory adapter directly (it implements the outbound port).
- For handler tests: use `httptest.NewServer` to mock downstream services.

## Test Patterns
- Use `t.Setenv("KEY", "value")` for environment variables (auto-cleaned up).
- Use `httptest.NewRequest` + `httptest.NewRecorder` for HTTP handler tests.
- Use `t.Helper()` in test helper functions.
- Use `t.Parallel()` for independent tests when safe.

## What to Test
- Domain validation rules (unit tests).
- Service business logic with in-memory adapters (integration tests).
- HTTP handlers with httptest (API contract tests).
- Error paths — not just the happy path.

---
description: Run the full test suite for a service with race detection and coverage
arguments:
  - name: service
    description: Service name (ratings, details, reviews, notification, productpage)
    required: true
---

# /test-service

Run the complete test suite for a service including race detection and coverage report.

## Steps

1. Run tests with race detector and coverage:
```bash
go test -race -count=1 -coverprofile=coverage.out ./services/{{service}}/...
```

2. Show coverage summary:
```bash
go tool cover -func=coverage.out
```

3. Report results: number of tests, pass/fail, coverage percentage.

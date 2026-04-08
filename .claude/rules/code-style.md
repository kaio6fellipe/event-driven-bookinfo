# Go Code Style

## Formatting
- All code must pass `gofmt` and `goimports`.
- No trailing whitespace, no unused imports, no unused variables.

## Naming
- Receiver names: 1-2 characters (`s` for service, `r` for repository, `h` for handler).
- Interface names: describe the behavior, no "I" prefix (`UserService`, not `IUserService`).
- Exported types get doc comments. Unexported helpers do not need them.
- Acronyms are all-caps in names: `ID`, `HTTP`, `URL`, `JSON`, `UUID`.

## File Organization
- One primary type per file. File name matches the type: `user_service.go` for `UserService`.
- Group related functions together. Keep files under 300 lines when practical.
- Test files live next to the code they test: `user_service_test.go`.

## Error Handling
- Always wrap errors with context: `fmt.Errorf("getting user %s: %w", id, err)`.
- Never assign errors to `_`. Handle or return them.
- Use sentinel errors (`var ErrNotFound = errors.New(...)`) for domain errors that callers need to check.
- Check errors immediately after the call that produces them.

## Functions
- Prefer early returns over deep nesting.
- `context.Context` is always the first parameter.
- Keep functions focused — if it does two things, split it.

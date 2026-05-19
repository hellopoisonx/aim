# Lint Maintenance Notes

## golangci-lint

- Repository lint entrypoint: `golangci-lint run ./...`.
- The project uses go-zero config tags such as `json:",default=30"` and `json:",optional"`; these tags are framework contracts and must not be rewritten to satisfy `staticcheck` SA5008.
- Keep the SA5008 exclusion in `.golangci.yaml` scoped to `app/.*/internal/config/config.go` so ordinary malformed struct tags remain linted.

## Common Fix Patterns

- Check all close errors in tests and runtime code; log cleanup errors with `t.Logf` in tests or `logx.Errorf` in services.
- For integer narrowing, validate bounds before conversion and keep a local `//nolint:gosec` justification on the conversion line if gosec cannot infer the guard.
- Use `context.WithoutCancel(ctx)` when a goroutine needs request values but must outlive a short dial/setup timeout.
- Desktop device config files should use private permissions: directories `0750`, files `0600`.

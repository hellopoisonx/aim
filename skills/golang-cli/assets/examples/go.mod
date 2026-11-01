// Standalone module for the snippets referenced by skills/golang-cli/SKILL.md.
//
// It exists so the illustrative Go files under this directory are excluded from
// the repository's root module: they reference packages that only exist inside a
// generated project (`github.com/you/myapp/...`), and without this go.mod they
// broke `go build ./...` and `go mod tidy` for the whole repository.
//
// The files are documentation, not a buildable program.
module example.com/golang-cli-examples

go 1.26

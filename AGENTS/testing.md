# Running the tests

`TestMain` points `HOME` and `GIT_CONFIG_GLOBAL` at a config holding only an
identity, so the machine's own git config — which may sign commits, and
go-git cannot sign — never reaches a test.

`helpers_test.go` serves the GitHub API from an `httptest` server and keeps
git remotes as bare repositories in a temporary directory, through
`Service.gitURL`. A test that reaches github.com is a bug.

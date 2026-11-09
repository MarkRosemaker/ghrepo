package ghrepo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/google/go-github/v80/github"
)

// TestMain points git at a config holding only an identity, so no machine's global config (which may sign commits) reaches the tests.
func TestMain(m *testing.M) { os.Exit(runTests(m)) }

func runTests(m *testing.M) int {
	home, err := os.MkdirTemp("", "ghrepo-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home) //nolint:errcheck

	cfg := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Test\n\temail = test@example.com\n[commit]\n\tgpgSign = false\n"), 0o600); err != nil {
		panic(err)
	}

	for k, v := range map[string]string{
		"HOME":                home,
		"XDG_CONFIG_HOME":     home,
		"GIT_CONFIG_GLOBAL":   cfg,
		"GIT_CONFIG_NOSYSTEM": "1",
	} {
		if err := os.Setenv(k, v); err != nil {
			panic(err)
		}
	}

	return m.Run()
}

type request struct {
	Method, Path, Query, Auth, ContentType string
	ContentLength                          int64
	Body                                   []byte
}

type recorder struct {
	mu   sync.Mutex
	reqs []request
}

func (rec *recorder) requests() []request {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return append([]request(nil), rec.reqs...)
}

// newTestService returns a Service whose GitHub API is an httptest server serving routes, and whose git remotes live in a temporary directory.
func newTestService(t *testing.T, routes map[string]http.HandlerFunc, opts ...Option) (*Service, *recorder) {
	t.Helper()

	mux := http.NewServeMux()
	for pattern, h := range routes {
		mux.HandleFunc(pattern, h)
	}

	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))

		rec.mu.Lock()
		rec.reqs = append(rec.reqs, request{
			Method:        r.Method,
			Path:          r.URL.Path,
			Query:         r.URL.RawQuery,
			Auth:          r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
			ContentLength: r.ContentLength,
			Body:          body,
		})
		rec.mu.Unlock()

		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	s := NewService(t.Context(), "test-token", opts...)

	u, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}

	s.github.BaseURL = u
	s.github.UploadURL = u
	s.gitURL = t.TempDir() + "/"

	return s, rec
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respond(status int, v any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, status, v) }
}

// newOrigin creates the bare repository s clones from and pushes to, holding one commit on main.
func newOrigin(t *testing.T, s *Service, owner, name string) string {
	t.Helper()

	work := t.TempDir()

	r, err := git.PlainInit(work, false, git.WithDefaultBranch(plumbing.Main))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := makeCommit(work, r); err != nil {
		t.Fatal(err)
	}

	bare := filepath.Join(s.gitURL, owner, name+".git")
	if _, err := git.PlainClone(bare, &git.CloneOptions{URL: work, Bare: true}); err != nil {
		t.Fatal(err)
	}

	return bare
}

// cloneRepo returns a Repository "o/n" cloned from a fresh origin.
func cloneRepo(t *testing.T, s *Service) *Repository {
	t.Helper()

	newOrigin(t, s, "o", "n")

	r, err := s.NewRepository(t.Context(), "o", "n",
		WithBaseDir(t.TempDir()), MakeDirAll, CloneGit,
		WithGithubRepo(&github.Repository{Name: new("n")}))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	return r
}

// pushFromElsewhere commits file to origin's main from a second clone.
func pushFromElsewhere(t *testing.T, origin, file string) plumbing.Hash {
	t.Helper()

	dir := t.TempDir()

	other, err := git.PlainClone(dir, &git.CloneOptions{URL: origin})
	if err != nil {
		t.Fatal(err)
	}

	h := commitFile(t, other, dir, file)
	if err := other.Push(&git.PushOptions{}); err != nil {
		t.Fatal(err)
	}

	return h
}

func commitFile(t *testing.T, r *git.Repository, dir, file string) plumbing.Hash {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.Add(file); err != nil {
		t.Fatal(err)
	}

	h, err := w.Commit("add "+file, &git.CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}

	return h
}

func head(t *testing.T, r *Repository) plumbing.Hash {
	t.Helper()

	h, err := r.gitrepo.Head()
	if err != nil {
		t.Fatal(err)
	}

	return h.Hash()
}

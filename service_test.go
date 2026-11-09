package ghrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	s := NewService(t.Context(), "test-token")
	if s == nil {
		t.Fatal("NewService returned nil")
	}

	if s.githubToken != "test-token" {
		t.Fatalf("githubToken = %q, want %q", s.githubToken, "test-token")
	}

	if s.github == nil {
		t.Fatal("github client is nil")
	}

	if len(s.gitOpts) == 0 {
		t.Fatal("gitOpts is empty")
	}

	if s.repos == nil {
		t.Fatal("repos map is nil")
	}
}

func TestAddRepos_GetRepo(t *testing.T) {
	t.Parallel()

	t.Run("adds_and_retrieves", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		s.addRepos("alice", []*github.Repository{
			{Name: new("alpha")},
			{Name: new("beta")},
		})

		if got := s.getRepo("alice", "alpha"); got == nil || got.GetName() != "alpha" {
			t.Fatalf("getRepo(alice, alpha) = %v, want name=alpha", got)
		}

		if got := s.getRepo("alice", "beta"); got == nil || got.GetName() != "beta" {
			t.Fatalf("getRepo(alice, beta) = %v, want name=beta", got)
		}
	})

	t.Run("empty_list_noop", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		s.addRepos("alice", nil)

		if got := s.getRepo("alice", "anything"); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("unknown_owner_returns_nil", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		if got := s.getRepo("nobody", "nope"); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("overwrites_existing", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		first := &github.Repository{Name: new("x"), Description: new("first")}
		second := &github.Repository{Name: new("x"), Description: new("second")}

		s.addRepos("alice", []*github.Repository{first})
		s.addRepos("alice", []*github.Repository{second})

		got := s.getRepo("alice", "x")
		if got == nil {
			t.Fatal("expected non-nil repo")
		}

		if got.GetDescription() != "second" {
			t.Fatalf("description = %q, want %q", got.GetDescription(), "second")
		}
	})
}

func TestNewService_SendsToken(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"GET /repos/o/n/releases/latest": respond(http.StatusOK, map[string]any{"tag_name": "v1.0.0"}),
	})

	r := &Repository{owner: "o", name: "n", s: s}
	if _, err := r.LatestRelease(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := rec.requests()[0].Auth; got != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer test-token")
	}
}

func TestPrefetchRepositories(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		path     string
		prefetch func(*Service, context.Context, string) error
	}{
		{"user", "/users/alice/repos", (*Service).PrefetchUserRepositories},
		{"org", "/orgs/alice/repos", (*Service).PrefetchOrgRepositories},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var s *Service

			s, rec := newTestService(t, map[string]http.HandlerFunc{
				"GET " + tc.path: func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Get("page") == "1" {
						w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=2>; rel="next"`, s.github.BaseURL, tc.path[1:]))
						writeJSON(w, http.StatusOK, []map[string]any{{"name": "alpha"}})

						return
					}

					writeJSON(w, http.StatusOK, []map[string]any{{"name": "beta", "default_branch": "trunk"}})
				},
			})

			if err := tc.prefetch(s, t.Context(), "alice"); err != nil {
				t.Fatal(err)
			}

			reqs := rec.requests()
			if len(reqs) != 2 {
				t.Fatalf("got %d requests, want 2", len(reqs))
			}

			for i, req := range reqs {
				q, _ := url.ParseQuery(req.Query)
				if q.Get("page") != fmt.Sprint(i+1) || q.Get("per_page") != "100" {
					t.Fatalf("request %d query = %q, want page=%d&per_page=100", i, req.Query, i+1)
				}
			}

			if s.getRepo("alice", "alpha") == nil {
				t.Fatal("alpha from page 1 not cached")
			}

			if got := s.getRepo("alice", "beta").GetDefaultBranch(); got != "trunk" {
				t.Fatalf("beta default branch = %q, want trunk", got)
			}
		})

		t.Run(tc.name+"_error", func(t *testing.T) {
			t.Parallel()

			s, _ := newTestService(t, map[string]http.HandlerFunc{
				"GET " + tc.path: respond(http.StatusInternalServerError, map[string]any{"message": "boom"}),
			})

			if err := tc.prefetch(s, t.Context(), "alice"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewRepository_InitFromCache(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"GET /users/o/repos": respond(http.StatusOK, []map[string]any{{"name": "n", "default_branch": "trunk", "description": "cached"}}),
	})
	if err := s.PrefetchUserRepositories(t.Context(), "o"); err != nil {
		t.Fatal(err)
	}

	base := t.TempDir()

	r, err := s.NewRepository(t.Context(), "o", "n", WithBaseDir(base), MakeDirAll, InitGit, CreateRemote)
	if err != nil {
		t.Fatal(err)
	}

	if n := len(rec.requests()); n != 1 {
		t.Fatalf("got %d API requests, want only the prefetch", n)
	}

	if r.path != filepath.Join(base, "o", "n") {
		t.Fatalf("path = %q", r.path)
	}

	ref, err := r.gitrepo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		t.Fatal(err)
	}

	if ref.Target() != plumbing.NewBranchReferenceName("trunk") {
		t.Fatalf("HEAD -> %q, want GitHub's default branch trunk", ref.Target())
	}

	origin, err := r.gitrepo.Remote("origin")
	if err != nil {
		t.Fatal(err)
	}

	if got := origin.Config().URLs; len(got) != 1 || got[0] != s.remoteURL("o", "n") {
		t.Fatalf("origin URLs = %v", got)
	}

	if s.remoteURL("o", "n") != s.gitURL+"o/n.git" {
		t.Fatalf("remoteURL = %q", s.remoteURL("o", "n"))
	}

	if r.Description() != "cached" || r.Owner() != "o" || r.Name() != "n" || r.String() != "o/n" {
		t.Fatalf("got %q %q %q %q", r.Description(), r.Owner(), r.Name(), r.String())
	}
}

func TestNewRepository_InitDefaultsToMain(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, map[string]http.HandlerFunc{
		"GET /repos/o/n": respond(http.StatusOK, map[string]any{"name": "n", "description": "fetched"}),
	})

	r, err := s.NewRepository(t.Context(), "o", "n", WithBaseDir(t.TempDir()), MakeDirAll, InitGit, CreateRemote)
	if err != nil {
		t.Fatal(err)
	}

	ref, err := r.gitrepo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		t.Fatal(err)
	}

	if ref.Target() != plumbing.Main {
		t.Fatalf("HEAD -> %q, want main", ref.Target())
	}

	if len(rec.requests()) != 1 || r.Description() != "fetched" {
		t.Fatalf("requests = %v, description = %q", rec.requests(), r.Description())
	}
}

func TestNewRepository_ServiceOptions(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	s, _ := newTestService(t, nil, WithBaseDir(base), MakeDirAll, InitGit, CreateRemote)

	r, err := s.NewRepository(t.Context(), "o", "n", WithGithubRepo(&github.Repository{}))
	if err != nil {
		t.Fatal(err)
	}

	if r.path != filepath.Join(base, "o", "n") {
		t.Fatalf("path = %q, want under the service's base dir", r.path)
	}
}

func TestNewRepository_CreateOnGitHub(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts []Option
		path string
	}{
		{"user", nil, "/user/repos"},
		{"org", []Option{OwnerIsOrg}, "/orgs/o/repos"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, rec := newTestService(t, map[string]http.HandlerFunc{
				"POST " + tc.path: respond(http.StatusCreated, map[string]any{"name": "n", "private": true}),
			})

			r, err := s.NewRepository(t.Context(), "o", "n", append(tc.opts,
				WithBaseDir(t.TempDir()), MakeDirAll, InitGit, CreateRemote, CreateOnGitHub)...)
			if err != nil {
				t.Fatal(err)
			}

			reqs := rec.requests()
			if len(reqs) != 2 || reqs[0].Path != "/repos/o/n" || reqs[1].Path != tc.path {
				t.Fatalf("requests = %v", reqs)
			}

			var body map[string]any
			if err := json.Unmarshal(reqs[1].Body, &body); err != nil {
				t.Fatal(err)
			}

			if body["name"] != "n" || body["visibility"] != "private" {
				t.Fatalf("create body = %v, want a private repository named n", body)
			}

			if !r.Private() {
				t.Fatal("created repository should be private")
			}
		})
	}
}

func TestNewRepository_Errors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		routes  map[string]http.HandlerFunc
		prepare func(t *testing.T, dir string)
		opts    []Option
		want    string
		errIs   error
		posts   int
	}{
		{name: "missing directory", errIs: fs.ErrNotExist},
		{
			name:    "not a git repository",
			prepare: func(t *testing.T, dir string) { t.Helper(); mkdir(t, dir) },
			want:    "failed to open git repo",
		},
		{
			name: "no remote",
			prepare: func(t *testing.T, dir string) {
				t.Helper()

				if _, err := git.PlainInit(dir, false); err != nil {
					t.Fatal(err)
				}
			},
			want: "failed to get remote",
		},
		{
			name: "nothing to clone",
			opts: []Option{MakeDirAll, CloneGit},
			want: "not found",
		},
		{
			name: "not on GitHub",
			opts: []Option{MakeDirAll, InitGit, CreateRemote},
			want: "getting GitHub repository",
		},
		{
			name:   "GitHub fails",
			routes: map[string]http.HandlerFunc{"GET /repos/o/n": respond(http.StatusInternalServerError, map[string]any{"message": "boom"})},
			opts:   []Option{MakeDirAll, InitGit, CreateRemote, CreateOnGitHub},
			want:   "getting GitHub repository",
		},
		{
			name:   "creation fails",
			routes: map[string]http.HandlerFunc{"POST /user/repos": respond(http.StatusUnprocessableEntity, map[string]any{"message": "nope"})},
			opts:   []Option{MakeDirAll, InitGit, CreateRemote, CreateOnGitHub},
			want:   "creating GitHub repository",
			posts:  1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, rec := newTestService(t, tc.routes)
			base := t.TempDir()

			if tc.prepare != nil {
				tc.prepare(t, filepath.Join(base, "o", "n"))
			}

			_, err := s.NewRepository(t.Context(), "o", "n", append(tc.opts, WithBaseDir(base))...)
			if err == nil || !strings.Contains(err.Error(), tc.want) || tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}

			posts := 0
			for _, req := range rec.requests() {
				if req.Method == http.MethodPost {
					posts++
				}
			}

			if posts != tc.posts {
				t.Fatalf("got %d create requests, want %d", posts, tc.posts)
			}
		})
	}
}

func TestNewRepository_Clone(t *testing.T) {
	t.Parallel()

	s, rec := newTestService(t, nil)
	r := cloneRepo(t, s)

	if len(rec.requests()) != 0 {
		t.Fatalf("WithGithubRepo should spare the API, got %v", rec.requests())
	}

	b, err := afero.ReadFile(r, "README.md")
	if err != nil || string(b) != "# Test Repo" {
		t.Fatalf("README.md = %q, %v; want the cloned file through the repository's Fs", b, err)
	}

	if r.defaultBranch != plumbing.Main {
		t.Fatalf("default branch = %q", r.defaultBranch)
	}

	if got := r.remote.Config().URLs[0]; got != s.remoteURL("o", "n") {
		t.Fatalf("origin = %q", got)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

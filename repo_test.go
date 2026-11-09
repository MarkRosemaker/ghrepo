package ghrepo

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

var (
	remoteMain = plumbing.NewRemoteReferenceName("origin", "main")
	remoteHead = plumbing.NewRemoteReferenceName("origin", "HEAD")
)

func makeCommit(dir string, r *git.Repository) (plumbing.Hash, error) {
	if err := os.WriteFile(dir+"/README.md", []byte("# Test Repo"), 0o644); err != nil {
		return plumbing.ZeroHash, err
	}

	w, err := r.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}

	if _, err := w.Add("README.md"); err != nil {
		return plumbing.ZeroHash, err
	}

	return w.Commit("initial commit", &git.CommitOptions{All: true})
}

func makeCommit2(dir string, r *git.Repository) error {
	if err := os.WriteFile(dir+"/TODO.md", []byte("# What needs to be done"), 0o644); err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	if _, err := w.Add("TODO.md"); err != nil {
		return err
	}

	if _, err := w.Commit("second commit", &git.CommitOptions{All: true}); err != nil {
		return err
	}

	return nil
}

func TestGetDefaultBranch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		init func(dir string) (*git.Repository, error)
		want plumbing.ReferenceName
	}{
		{"main", func(dir string) (*git.Repository, error) {
			return git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Main))
		}, plumbing.Main},
		{"master but no commit", func(dir string) (*git.Repository, error) {
			return git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Master))
		}, plumbing.Main},
		{"main with commit", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Main))
			if err != nil {
				return nil, err
			}

			_, err = makeCommit(dir, r)

			return r, err
		}, plumbing.Main},
		{"master with commit", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Master))
			if err != nil {
				return nil, err
			}

			_, err = makeCommit(dir, r)

			return r, err
		}, plumbing.Master},
		{"remote default main overrides local", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false)
			if err != nil {
				return nil, err
			}

			if _, err := makeCommit(dir, r); err != nil {
				return nil, err
			}

			h, err := r.Head()
			if err != nil {
				return nil, err
			}

			// Mock remote tracking branch "main"
			if err := r.Storer.SetReference(plumbing.NewHashReference(remoteMain, h.Hash())); err != nil {
				return nil, err
			}

			// Mock remote default HEAD pointing to main
			if err := r.Storer.SetReference(plumbing.NewSymbolicReference(remoteHead, remoteMain)); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Main},
		{"remote default master", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false)
			if err != nil {
				return nil, err
			}

			if _, err := makeCommit(dir, r); err != nil {
				return nil, err
			}

			h, err := r.Head()
			if err != nil {
				return nil, err
			}

			// Mock remote tracking "master"
			if err := r.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/master", h.Hash())); err != nil {
				return nil, err
			}

			if err := r.Storer.SetReference(plumbing.NewSymbolicReference(remoteHead, "refs/remotes/origin/master")); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Master},
		{"current branch not default, with remote main", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false)
			if err != nil {
				return nil, err
			}

			if _, err := makeCommit(dir, r); err != nil {
				return nil, err
			}

			h, err := r.Head()
			if err != nil {
				return nil, err
			}

			if err := r.Storer.SetReference(plumbing.NewHashReference(remoteMain, h.Hash())); err != nil {
				return nil, err
			}

			if err := r.Storer.SetReference(plumbing.NewSymbolicReference(remoteHead, remoteMain)); err != nil {
				return nil, err
			}

			// Checkout to a different branch "feature"
			w, err := r.Worktree()
			if err != nil {
				return nil, err
			}

			if err := w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName("feature"),
				Create: true,
			}); err != nil {
				return nil, err
			}

			if err := makeCommit2(dir, r); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Main}, // Should ignore current "feature" and use remote default
		{"detached HEAD falls back to master", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Master))
			if err != nil {
				return nil, err
			}

			commit, err := makeCommit(dir, r)
			if err != nil {
				return nil, err
			}

			w, err := r.Worktree()
			if err != nil {
				return nil, err
			}

			if err := w.Checkout(&git.CheckoutOptions{
				Hash: commit,
			}); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Master},
		{"detached HEAD falls back to main", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false, git.WithDefaultBranch(plumbing.Main))
			if err != nil {
				return nil, err
			}

			commit, err := makeCommit(dir, r)
			if err != nil {
				return nil, err
			}

			w, err := r.Worktree()
			if err != nil {
				return nil, err
			}

			if err := w.Checkout(&git.CheckoutOptions{
				Hash: commit,
			}); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Main},
		{"unborn non-standard branch prefers main", func(dir string) (*git.Repository, error) {
			r, err := git.PlainInit(dir, false)
			if err != nil {
				return nil, err
			}

			// Override symbolic HEAD to a custom unborn branch
			if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("trunk"))); err != nil {
				return nil, err
			}

			return r, nil
		}, plumbing.Main}, // Matches pattern of preferring main in empty/unborn cases
		{"bare repo assumes main", func(dir string) (*git.Repository, error) {
			return git.PlainInit(dir, true)
		}, plumbing.Main},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			r, err := tc.init(dir)
			if err != nil {
				t.Fatalf("init git repo: %v", err)
			}

			b, err := getDefaultBranch(r)
			if err != nil {
				stat := exec.CommandContext(t.Context(), "git", "status")
				stat.Dir = dir
				out, _ := stat.CombinedOutput()
				t.Logf("Git status output: %s", out)

				t.Fatalf("getting default branch: %v", err)
			}

			if b != tc.want {
				t.Fatalf("expected default branch to be %q, got %q", tc.want, b)
			}
		})
	}
}

func TestPrivate(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo *github.Repository
		want bool
	}{
		{"private", &github.Repository{Private: new(true)}, true},
		{"public", &github.Repository{Private: new(false)}, false},
		// Unknown means private: guessing public is the guess that leaks.
		{"field unset", &github.Repository{}, true},
		{"no metadata", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Repository{github: tc.repo}
			if got := r.Private(); got != tc.want {
				t.Errorf("Private() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHasChanges: GitHub reports a repository with no description as null,
// and setting it to "" sent a PATCH that changed nothing on every run.
func TestHasChanges(t *testing.T) {
	for _, tc := range []struct {
		name            string
		initial, update *github.Repository
		want            bool
	}{
		{"same description", &github.Repository{Description: new("A thing.")}, &github.Repository{Description: new("A thing.")}, false},
		{"new description", &github.Repository{Description: new("A thing.")}, &github.Repository{Description: new("Another.")}, true},
		{"null set to empty", &github.Repository{}, &github.Repository{Description: new("")}, false},
		{"null set to something", &github.Repository{}, &github.Repository{Description: new("A thing.")}, true},
		{"something set to empty", &github.Repository{Description: new("A thing.")}, &github.Repository{Description: new("")}, true},
		{"null set to no topics", &github.Repository{}, &github.Repository{Topics: []string{}}, false},
		{"null set to topics", &github.Repository{}, &github.Repository{Topics: []string{"go"}}, true},
		{"nothing asked", &github.Repository{Description: new("A thing.")}, &github.Repository{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasChanges(tc.initial, tc.update); got != tc.want {
				t.Errorf("hasChanges() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStatusAndCommit(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, nil)
	r := cloneRepo(t, s)
	start := head(t, r)

	if changed, err := r.HasChanges(); err != nil || changed {
		t.Fatalf("fresh clone HasChanges() = %v, %v; want false", changed, err)
	}

	for _, f := range []string{"a.txt", "b.txt"} {
		if err := afero.WriteFile(r, f, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if changed, err := r.HasChanges(); err != nil || !changed {
		t.Fatalf("HasChanges() = %v, %v; want true", changed, err)
	}

	if files, err := r.GetChangedFiles(); err != nil || !slices.Equal(sorted(files), []string{"a.txt", "b.txt"}) {
		t.Fatalf("GetChangedFiles() = %v, %v", files, err)
	}

	if err := r.Commit([]string{"a.txt"}, "add a"); err != nil {
		t.Fatal(err)
	}

	c, err := r.gitrepo.CommitObject(head(t, r))
	if err != nil {
		t.Fatal(err)
	}

	if c.Message != "add a" || c.ParentHashes[0] != start {
		t.Fatalf("commit %q with parents %v", c.Message, c.ParentHashes)
	}

	if _, err := c.File("b.txt"); err == nil {
		t.Fatal("b.txt was committed though only a.txt was asked for")
	}

	st, err := r.GitStatus()
	if err != nil {
		t.Fatal(err)
	}

	if len(st) != 1 || st.File("b.txt").Worktree != git.Untracked {
		t.Fatalf("status = %v, want only b.txt untracked", st)
	}

	if changed, err := r.HasChanges(); err != nil || !changed {
		t.Fatalf("HasChanges() = %v, %v with one untracked file", changed, err)
	}

	if err := afero.WriteFile(r, "README.md", []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	if files, err := r.GetChangedFiles(); err != nil || !slices.Equal(sorted(files), []string{"README.md", "b.txt"}) {
		t.Fatalf("GetChangedFiles() = %v, %v; want the unstaged edit too", files, err)
	}

	if err := r.CommitAll("add the rest"); err != nil {
		t.Fatal(err)
	}

	if changed, _ := r.HasChanges(); changed {
		t.Fatal("CommitAll left changes behind")
	}

	before := head(t, r)
	if err := r.CommitAll("nothing"); err != nil {
		t.Fatalf("an empty commit should be no error: %v", err)
	}

	if head(t, r) != before {
		t.Fatal("an empty commit should create nothing")
	}

	if err := r.Commit([]string{"missing.txt"}, "msg"); err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("err = %v, want it to name missing.txt", err)
	}
}

func TestGitResetAndClean(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, nil)
	r := cloneRepo(t, s)

	if err := afero.WriteFile(r, "README.md", []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.MkdirAll("junk", 0o755); err != nil {
		t.Fatal(err)
	}

	if err := afero.WriteFile(r, "junk/file.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.GitReset(git.HardReset); err != nil {
		t.Fatal(err)
	}

	if b, _ := afero.ReadFile(r, "README.md"); string(b) != "# Test Repo" {
		t.Fatalf("README.md = %q after hard reset", b)
	}

	if err := r.GitClean(); err != nil {
		t.Fatal(err)
	}

	if ok, _ := afero.Exists(r, "junk"); ok {
		t.Fatal("GitClean left the untracked directory")
	}

	if changed, _ := r.HasChanges(); changed {
		t.Fatal("still changes after reset and clean")
	}
}

func TestCheckoutDefault(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, nil)
	r := cloneRepo(t, s)

	if ok, err := r.IsDefaultBranch(); err != nil || !ok {
		t.Fatalf("IsDefaultBranch() = %v, %v on a fresh clone", ok, err)
	}

	if err := r.worktree.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatal(err)
	}

	if ok, err := r.IsDefaultBranch(); err != nil || ok {
		t.Fatalf("IsDefaultBranch() = %v, %v on feature", ok, err)
	}

	if err := r.CheckoutDefault(); err != nil {
		t.Fatal(err)
	}

	if ok, err := r.IsDefaultBranch(); err != nil || !ok {
		t.Fatalf("IsDefaultBranch() = %v, %v after CheckoutDefault", ok, err)
	}
}

func TestIsDefaultBranch_Unborn(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, nil)

	r, err := s.NewRepository(t.Context(), "o", "n", WithBaseDir(t.TempDir()), MakeDirAll, InitGit, CreateRemote,
		WithGithubRepo(&github.Repository{}))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.IsDefaultBranch(); err == nil {
		t.Fatal("expected an error without any commit")
	}
}

func TestPush(t *testing.T) {
	t.Parallel()

	s, _ := newTestService(t, nil)
	r := cloneRepo(t, s)

	if err := r.Push(t.Context()); err != nil {
		t.Fatalf("nothing to push should be no error: %v", err)
	}

	if err := afero.WriteFile(r, "new.txt", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := r.CommitAll("add new"); err != nil {
		t.Fatal(err)
	}

	if err := r.Push(t.Context()); err != nil {
		t.Fatal(err)
	}

	origin, err := git.PlainOpen(filepath.Join(s.gitURL, "o", "n.git"))
	if err != nil {
		t.Fatal(err)
	}

	ref, err := origin.Reference(plumbing.Main, true)
	if err != nil {
		t.Fatal(err)
	}

	if ref.Hash() != head(t, r) {
		t.Fatalf("origin main = %s, want the pushed %s", ref.Hash(), head(t, r))
	}

	s.gitURL = t.TempDir() + "/"
	if err := r.Push(t.Context()); err == nil || !strings.Contains(err.Error(), "push failed") {
		t.Fatalf("err = %v, want push failed for a missing remote", err)
	}
}

func TestPull(t *testing.T) {
	t.Parallel()

	t.Run("up to date", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := cloneRepo(t, s)

		before := head(t, r)
		if err := r.Pull(t.Context()); err != nil || head(t, r) != before {
			t.Fatalf("Pull() = %v, head moved: %v", err, head(t, r) != before)
		}
	})

	t.Run("fast forward", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := cloneRepo(t, s)
		want := pushFromElsewhere(t, r.remote.Config().URLs[0], "remote.txt")

		if err := r.Pull(t.Context()); err != nil {
			t.Fatal(err)
		}

		if head(t, r) != want {
			t.Fatalf("head = %s, want %s", head(t, r), want)
		}

		if ok, _ := afero.Exists(r, "remote.txt"); !ok {
			t.Fatal("remote.txt not pulled")
		}
	})

	t.Run("diverged resets to origin", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := cloneRepo(t, s)
		want := pushFromElsewhere(t, r.remote.Config().URLs[0], "remote.txt")
		commitFile(t, r.gitrepo, r.path, "local.txt")

		if err := r.Pull(t.Context()); err != nil {
			t.Fatal(err)
		}

		if head(t, r) != want {
			t.Fatalf("head = %s, want origin's %s", head(t, r), want)
		}

		if ok, _ := afero.Exists(r, "local.txt"); ok {
			t.Fatal("the local commit should be discarded")
		}
	})

	t.Run("remote gone", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := cloneRepo(t, s)

		if err := os.RemoveAll(r.remote.Config().URLs[0]); err != nil {
			t.Fatal(err)
		}

		if err := r.Pull(t.Context()); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestSetTopics(t *testing.T) {
	t.Parallel()

	t.Run("unchanged", func(t *testing.T) {
		t.Parallel()

		s, rec := newTestService(t, nil)
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{Topics: []string{"go"}}}

		if err := r.SetTopics(t.Context(), []string{"go"}); err != nil || len(rec.requests()) != 0 {
			t.Fatalf("SetTopics() = %v with %d requests, want no request", err, len(rec.requests()))
		}
	})

	t.Run("replaced", func(t *testing.T) {
		t.Parallel()

		s, rec := newTestService(t, map[string]http.HandlerFunc{
			"PUT /repos/o/n/topics": respond(http.StatusOK, map[string]any{"names": []string{"cli", "go"}}),
		})
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{Topics: []string{"go"}}}

		if err := r.SetTopics(t.Context(), []string{"go", "cli"}); err != nil {
			t.Fatal(err)
		}

		if got := string(rec.requests()[0].Body); got != `{"names":["go","cli"]}`+"\n" {
			t.Fatalf("body = %q", got)
		}

		if got := r.Topics(); !slices.Equal(got, []string{"cli", "go"}) {
			t.Fatalf("Topics() = %v, want what GitHub returned", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{Topics: []string{"go"}}}

		if err := r.SetTopics(t.Context(), nil); err == nil {
			t.Fatal("expected error")
		}

		if got := r.Topics(); !slices.Equal(got, []string{"go"}) {
			t.Fatalf("Topics() = %v, want unchanged after a failure", got)
		}
	})
}

func TestEdit(t *testing.T) {
	t.Parallel()

	t.Run("unchanged", func(t *testing.T) {
		t.Parallel()

		s, rec := newTestService(t, nil)
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{Description: new("same")}}

		if err := r.SetDescription(t.Context(), "same"); err != nil || len(rec.requests()) != 0 {
			t.Fatalf("SetDescription() = %v with %d requests, want no request", err, len(rec.requests()))
		}
	})

	t.Run("changed", func(t *testing.T) {
		t.Parallel()

		s, rec := newTestService(t, map[string]http.HandlerFunc{
			"PATCH /repos/o/n": respond(http.StatusOK, map[string]any{"description": "new", "archived": true}),
		})
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{Description: new("old"), Archived: new(false)}}

		if err := r.SetDescription(t.Context(), "new"); err != nil {
			t.Fatal(err)
		}

		if got := string(rec.requests()[0].Body); got != `{"description":"new"}`+"\n" {
			t.Fatalf("body = %q", got)
		}

		if r.Description() != "new" || !r.Archived() {
			t.Fatalf("Description() = %q, Archived() = %v; want GitHub's response", r.Description(), r.Archived())
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		s, _ := newTestService(t, nil)
		r := &Repository{owner: "o", name: "n", s: s, github: &github.Repository{}}

		if err := r.Edit(t.Context(), &github.Repository{Homepage: new("https://example.com")}); err == nil {
			t.Fatal("expected error")
		}

		if r.Description() != "" {
			t.Fatalf("Description() = %q, want empty for an unset description", r.Description())
		}
	})
}

func sorted(s []string) []string {
	slices.Sort(s)
	return s
}

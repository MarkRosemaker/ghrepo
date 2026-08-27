package ghrepo

import (
	"os"
	"os/exec"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/google/go-github/v80/github"
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
				stat := exec.Command("git", "status")
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
		{"private", &github.Repository{Private: github.Ptr(true)}, true},
		{"public", &github.Repository{Private: github.Ptr(false)}, false},
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

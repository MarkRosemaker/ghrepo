package ghrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

const remoteName = "origin"

// Repository represents a local Git repository linked to a GitHub remote.
type Repository struct {
	// Use the repository folder as its own file system.
	afero.Fs
	owner       string
	name        string
	path        string // Local filesystem path
	gitrepo     *git.Repository
	worktree    *git.Worktree
	remote      *git.Remote
	github      *github.Repository
	githubToken string
}

// NewRepository opens or initializes a repository at the given path.
func (s *Service) NewRepository(ctx context.Context, owner, name string, opts ...Option) (*Repository, error) {
	// Apply the options
	cfg := &repoConfig{}
	for _, opt := range append(s.opts, opts...) {
		opt(cfg)
	}

	path := filepath.Join(cfg.baseDir, owner, name)

	// Make sure it exists on local
	if cfg.mkdirAll {
		if err := os.MkdirAll(path, fs.ModePerm); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	// Make sure we have a git repo
	gitrepo, err := git.PlainOpen(path)
	if err != nil {
		if !cfg.initGit || !errors.Is(err, git.ErrRepositoryNotExists) {
			return nil, fmt.Errorf("failed to open git repo at %s: %w", path, err)
		}

		gitrepo, err = git.PlainInit(path, false, initOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to init git repo at %s: %w", path, err)
		}
	}

	// Make sure we have a remote
	remote, err := gitrepo.Remote(remoteName)
	if err != nil {
		if !cfg.createRemote || !errors.Is(err, git.ErrRemoteNotFound) {
			return nil, fmt.Errorf("failed to get remote: %w", err)
		}

		// Add correct HTTPS remote
		if remote, err = gitrepo.CreateRemote(&config.RemoteConfig{
			Name: remoteName,
			URLs: []string{fmt.Sprintf("https://github.com/%s/%s.git", owner, name)},
		}); err != nil {
			return nil, fmt.Errorf("failed to create origin remote: %w", err)
		}
	}

	// Make sure we have the repo on GitHub
	ghrepo, rsp, err := s.github.Repositories.Get(ctx, owner, name)
	if err != nil {
		if !cfg.createOnGitHub || rsp.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("getting GitHub repository: %w", err)
		}

		org := ""
		if cfg.ownerIsOrg {
			org = owner
		}

		ghrepo, _, err = s.github.Repositories.Create(ctx, org, &github.Repository{
			Name: newGo126(name),
			// We start out with a private repository until the repository is ready to be published.
			Visibility: newGo126("private"),
		})
		if err != nil {
			return nil, fmt.Errorf("creating GitHub repository: %w", err)
		}
	}

	// Get the worktree
	wt, err := gitrepo.Worktree()
	if err != nil {
		return nil, err
	}

	// TODO
	//   - Creating the GitHub repo if requested
	//   - Setting up origin remote
	//   - Making an initial commit if provided

	return &Repository{
		Fs:          afero.NewBasePathFs(afero.NewOsFs(), path),
		owner:       owner,
		name:        name,
		path:        path,
		gitrepo:     gitrepo,
		worktree:    wt,
		remote:      remote,
		github:      ghrepo,
		githubToken: s.githubToken,
	}, nil
}

// HasChanges returns true if there are any unstaged, staged, or untracked changes.
// It returns false only if the working tree is completely clean.
func (r *Repository) HasChanges() (bool, error) {
	status, err := r.worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	return !status.IsClean(), nil
}

// ExecCommand runs a command in the repository's root directory.
// It returns the combined stdout + stderr as a string.
// The command name and arguments are passed separately (like exec.Command).
func (r *Repository) ExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.path

	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, nil
	}

	// Include command for better debugging
	cmdStr := strings.Join(append([]string{name}, args...), " ")
	if out = bytes.TrimSpace(out); len(out) > 0 {
		return nil, fmt.Errorf("command %q failed:\n%s\n%w", cmdStr, string(out), err)
	}

	return nil, fmt.Errorf("command %q failed: %w", cmdStr, err)
}

// Commit adds all changes, commits with the given message.
func (r *Repository) CommitAll(message string) error {
	// Stage all changes
	if _, err := r.worktree.Add("."); err != nil {
		return fmt.Errorf("failed to add all files to worktree: %w", err)
	}

	// Commit
	if _, err := r.worktree.Commit(message, &git.CommitOptions{}); err != nil &&
		!errors.Is(err, git.ErrEmptyCommit) {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// Push pushes to the default remote.
func (r *Repository) Push(ctx context.Context) error {
	if err := r.gitrepo.PushContext(ctx, &git.PushOptions{
		RemoteURL: fmt.Sprintf("https://github.com/%s/%s.git", r.owner, r.name),
		Auth: &githttp.BasicAuth{
			// Can be anything non-empty for token auth
			Username: "git",
			// Recommended: GitHub PAT (not raw password)
			Password: r.githubToken,
		},
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push failed: %w", err)
	}

	return nil

	// git push -u origin main
	// if err := r.Git.Push(&git.PushOptions{
	// 	RemoteName: remoteName,
	// 	RemoteURL:  fmt.Sprintf("https://github.com/%s/%s.git", r.Owner, r.Name),

	// 	// RefSpecs: []plumbing.RefSpec{plumbing.RefSpec(branchName + ":" + branchName)},
	// 	Auth: &http.BasicAuth{
	// 		Username: "MarkRosemaker",
	// 		Password: os.Getenv("GITHUB_TOKEN"),
	// 	},
	// 	// Auth: &http.TokenAuth{Token: os.Getenv("GITHUB_TOKEN")},
	// }); err != nil {
	// 	// if err := execute(r.Path, "git", "push", "-u", "origin", "main"); err != nil {
	// 	return fmt.Errorf("pushing changes: %w", err)
	// }

	return nil
}

// // UpdateDescription changes the repository description on GitHub.
// func (r *Repository) UpdateDescription(newDesc string) error {
// 	repoEdit := &github.Repository{
// 		Description: github.String(newDesc),
// 	}

// 	_, _, err := r.GitHubClient.Repositories.Edit(r.ctx, r.GitHubOwner, r.GitHubName, repoEdit)
// 	if err != nil {
// 		return fmt.Errorf("failed to update description on GitHub: %w", err)
// 	}
// 	return nil
// }

// TODO: replace with new once go1.26 is out
func newGo126[T any](v T) *T { return &v }

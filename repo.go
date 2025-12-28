package ghrepo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/google/go-github/v80/github"
)

const remoteName = "origin"

// Repository represents a local Git repository linked to a GitHub remote.
type Repository struct {
	owner    string
	name     string
	path     string // Local filesystem path
	gitrepo  *git.Repository
	worktree *git.Worktree
	remote   *git.Remote
	ghClient *github.Client
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
		owner:    owner,
		name:     name,
		path:     path,
		gitrepo:  gitrepo,
		worktree: wt,
		remote:   remote,
	}, nil
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
func (r *Repository) Push() error {
	if err := r.gitrepo.Push(&git.PushOptions{
		RemoteName: remoteName,
		// RemoteURL:  deref(r.Github.GitCommitsURL),
		// RemoteURL:  fmt.Sprintf("https://github.com/%s/%s.git", r.Owner, r.Name),

		// RemoteName: "origin",
		// RefSpecs:   []config.RefSpec{"refs/heads/*:refs/heads/*"}, // Push all branches (or specify "refs/heads/main:refs/heads/main")
		Auth: &http.BasicAuth{
			Username: "git",                     // Can be anything non-empty for token auth
			Password: os.Getenv("GITHUB_TOKEN"), // Recommended: GitHub PAT (not raw password)
		},
		Progress: os.Stdout, // Optional: show progress
	}); err != nil { // && err != git.NoErrAlreadyUpToDate
		return fmt.Errorf("push failed: %w", err)
	}

	// RemoteURL: fmt.Sprintf("https://%s@github.com/%s/%s.git",
	// 	os.Getenv("GITHUB_TOKEN"), r.owner, r.name),
	// // RemoteURL: fmt.Sprintf("git@github.com:%s/%s.git", r.owner, r.name),
	// Auth: &http.TokenAuth{Token: os.Getenv("GITHUB_TOKEN")},

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

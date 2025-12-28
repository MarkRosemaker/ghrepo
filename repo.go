package ghrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
)

// Repository represents a local Git repository linked to a GitHub remote.
type Repository struct {
	owner string
	name  string
	path  string // Local filesystem path
	// Repo         *git.Repository
	worktree *git.Worktree
	// GitHubClient *github.Client
}

// New opens or initializes a repository at the given path.
func New(owner, name string, opts ...Option) (*Repository, error) {
	// Apply the options
	cfg := &repoConfig{}
	for _, opt := range opts {
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
		worktree: wt,
	}, nil
}

// Commit adds all changes, commits with the given message.
func (r *Repository) CommitAll(message string) error {
	// Stage all changes
	if _, err := r.worktree.Add("."); err != nil {
		return fmt.Errorf("failed to add all files to worktree: %w", err)
	}

	// Commit
	if _, err := r.worktree.Commit(message, &git.CommitOptions{}); err != nil {
		fmt.Printf("err: %#v\n", err)
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// // Push pushes to the default remote (usually "origin").
// func (r *Repository) Push() error {
// 	err := r.Repo.Push(&git.PushOptions{
// 		// Auth will be automatically handled if SSH key or HTTPS with token in remote URL
// 		// For HTTPS with token, make sure remote URL is like:
// 		// https://<token>@github.com/owner/name.git
// 	})
// 	if err != nil && err != git.NoErrAlreadyUpToDate {
// 		return fmt.Errorf("push failed: %w", err)
// 	}
// 	return nil
// }

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

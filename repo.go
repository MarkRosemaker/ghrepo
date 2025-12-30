package ghrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
)

const remoteName = "origin"

// Repository represents a local Git repository linked to a GitHub remote.
type Repository struct {
	// Use the repository folder as its own file system.
	afero.Fs
	owner    string
	name     string
	path     string // Local filesystem path
	gitrepo  *git.Repository
	worktree *git.Worktree
	remote   *git.Remote
	github   *github.Repository
	s        *Service
}

func (r Repository) String() string { return fmt.Sprintf("%s/%s", r.owner, r.name) }

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
		Auth:      r.s.gitAuth,
	}); err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push failed: %w", err)
	}

	return nil
}

// func (r *Repository) SetTeamPermission(ctx context.Context, teamSlug, permission string) error {
// 	_, err := r.s.github.Teams.AddTeamRepoBySlug(ctx, r.owner, teamSlug, r.owner, r.name,
// 		&github.TeamAddTeamRepoOptions{Permission: permission})
// 	return err
// }

func (r *Repository) SetTopics(ctx context.Context, topics []string) error {
	_, _, err := r.s.github.Repositories.ReplaceAllTopics(ctx, r.owner, r.name, topics)
	return err
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

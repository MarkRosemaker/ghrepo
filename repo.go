package ghrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"slices"
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

// GetChangedFiles returns a list of files that were changed.
func (r *Repository) GetChangedFiles() ([]string, error) {
	s, err := r.worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	changes := []string{}
	for file, status := range s {
		if status.Worktree != git.Unmodified || status.Staging != git.Unmodified {
			changes = append(changes, file)
		}
	}

	return changes, nil
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

// SetTopics sets the repository topics on GitHub.
func (r *Repository) SetTopics(ctx context.Context, topics []string) error {
	if slices.Equal(r.github.Topics, topics) {
		return nil
	}

	updated, _, err := r.s.github.Repositories.ReplaceAllTopics(ctx, r.owner, r.name, topics)
	if err != nil {
		return err
	}

	r.github.Topics = updated

	return nil
}

// Edit edits the repository on GitHub.
func (r *Repository) Edit(ctx context.Context, update *github.Repository) error {
	if !hasChanges(r.github, update) {
		return nil
	}

	repo, _, err := r.s.github.Repositories.Edit(ctx, r.owner, r.name, update)
	if err != nil {
		return err
	}

	r.github = repo

	return nil
}

func hasChanges(initial, update *github.Repository) bool {
	uv := reflect.ValueOf(update)
	iv := reflect.ValueOf(initial)

	for i := 0; i < uv.NumField(); i++ {
		uField := uv.Field(i)
		if uField.IsNil() {
			continue // skip nil fields in update
		}

		iField := iv.Field(i)
		if iField.IsNil() {
			return true // update sets a value where initial is nil
		}

		uVal := reflect.Indirect(uField)
		iVal := reflect.Indirect(iField)
		if !reflect.DeepEqual(uVal.Interface(), iVal.Interface()) {
			return true
		}
	}

	return false
}

// UpdateDescription changes the repository description on GitHub.
func (r *Repository) UpdateDescription(ctx context.Context, descr string) error {
	return r.Edit(ctx, &github.Repository{Description: github.Ptr(descr)})
}

func (r *Repository) GetDescription() string {
	if r.github.Description == nil {
		return ""
	}

	return *r.github.Description
}

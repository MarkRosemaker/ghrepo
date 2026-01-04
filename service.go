package ghrepo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/google/go-github/v80/github"
	"github.com/spf13/afero"
	"golang.org/x/oauth2"
)

type Service struct {
	githubToken string
	github      *github.Client
	gitAuth     *githttp.BasicAuth
	opts        []Option
}

func NewService(ctx context.Context, githubToken string, opts ...Option) *Service {
	return &Service{
		githubToken: githubToken,
		github: github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken}))),
		gitAuth: &githttp.BasicAuth{
			// Can be anything non-empty for token auth
			Username: "git",
			// Recommended: GitHub PAT (not raw password)
			Password: githubToken,
		},
		opts: opts,
	}
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

	defaultBranch, err := getDefaultBranch(gitrepo)
	if err != nil {
		return nil, err
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
			Name: github.Ptr(name),
			// We start out with a private repository until the repository is ready to be published.
			Visibility: github.Ptr("private"),
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

	return &Repository{
		Fs:            afero.NewBasePathFs(afero.NewOsFs(), path),
		owner:         owner,
		name:          name,
		path:          path,
		gitrepo:       gitrepo,
		defaultBranch: defaultBranch,
		worktree:      wt,
		remote:        remote,
		github:        ghrepo,
		s:             s,
	}, nil
}

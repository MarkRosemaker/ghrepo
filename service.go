package ghrepo

import (
	"context"

	"github.com/google/go-github/v80/github"
	"golang.org/x/oauth2"
)

type Service struct {
	githubToken string
	github      *github.Client
	opts        []Option
}

func NewService(ctx context.Context, githubToken string, opts ...Option) *Service {
	return &Service{
		githubToken: githubToken,
		github: github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken}))),
		opts: opts,
	}
}

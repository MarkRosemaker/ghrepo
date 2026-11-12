package ghrepo

import (
	"context"
	"strings"

	"github.com/google/go-github/v80/github"
)

// FindOpenIssue returns the first open issue whose body contains text, or nil
// where there is none. Pull requests, which GitHub lists as issues too, are
// passed over.
func (r *Repository) FindOpenIssue(ctx context.Context, text string) (*github.Issue, error) {
	opts := &github.IssueListByRepoOptions{State: "open", ListOptions: github.ListOptions{PerPage: 100}}

	for {
		issues, resp, err := r.s.github.Issues.ListByRepo(ctx, r.owner, r.name, opts)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues {
			if !issue.IsPullRequest() && strings.Contains(issue.GetBody(), text) {
				return issue, nil
			}
		}

		if resp.NextPage == 0 {
			return nil, nil
		}

		opts.ListOptions.Page = resp.NextPage
	}
}

// CreateIssue opens an issue.
func (r *Repository) CreateIssue(ctx context.Context, title, body string) (*github.Issue, error) {
	issue, _, err := r.s.github.Issues.Create(ctx, r.owner, r.name,
		&github.IssueRequest{Title: &title, Body: &body})

	return issue, err
}

// EditIssue replaces an issue's title and body.
func (r *Repository) EditIssue(ctx context.Context, number int, title, body string) (*github.Issue, error) {
	issue, _, err := r.s.github.Issues.Edit(ctx, r.owner, r.name, number,
		&github.IssueRequest{Title: &title, Body: &body})

	return issue, err
}

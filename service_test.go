package ghrepo

import (
	"context"
	"testing"

	"github.com/google/go-github/v80/github"
)

func TestNewService(t *testing.T) {
	t.Parallel()

	s := NewService(context.Background(), "test-token")
	if s == nil {
		t.Fatal("NewService returned nil")
	}
	if s.githubToken != "test-token" {
		t.Fatalf("githubToken = %q, want %q", s.githubToken, "test-token")
	}
	if s.github == nil {
		t.Fatal("github client is nil")
	}
	if len(s.gitOpts) == 0 {
		t.Fatal("gitOpts is empty")
	}
	if s.repos == nil {
		t.Fatal("repos map is nil")
	}
}

func TestAddRepos_GetRepo(t *testing.T) {
	t.Parallel()

	t.Run("adds_and_retrieves", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		s.addRepos("alice", []*github.Repository{
			{Name: new("alpha")},
			{Name: new("beta")},
		})

		if got := s.getRepo("alice", "alpha"); got == nil || got.GetName() != "alpha" {
			t.Fatalf("getRepo(alice, alpha) = %v, want name=alpha", got)
		}
		if got := s.getRepo("alice", "beta"); got == nil || got.GetName() != "beta" {
			t.Fatalf("getRepo(alice, beta) = %v, want name=beta", got)
		}
	})

	t.Run("empty_list_noop", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		s.addRepos("alice", nil)
		if got := s.getRepo("alice", "anything"); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("unknown_owner_returns_nil", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		if got := s.getRepo("nobody", "nope"); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("overwrites_existing", func(t *testing.T) {
		t.Parallel()

		s := &Service{repos: map[string]map[string]*github.Repository{}}
		first := &github.Repository{Name: new("x"), Description: new("first")}
		second := &github.Repository{Name: new("x"), Description: new("second")}

		s.addRepos("alice", []*github.Repository{first})
		s.addRepos("alice", []*github.Repository{second})

		got := s.getRepo("alice", "x")
		if got == nil {
			t.Fatal("expected non-nil repo")
		}
		if got.GetDescription() != "second" {
			t.Fatalf("description = %q, want %q", got.GetDescription(), "second")
		}
	})
}

package ghrepo

import (
	"testing"

	"github.com/google/go-github/v80/github"
)

func TestOptions(t *testing.T) {
	t.Parallel()

	ghRepo := &github.Repository{}

	for _, tc := range []struct {
		name  string
		apply func(*repoConfig)
		check func(t *testing.T, cfg *repoConfig)
	}{
		{
			name:  "WithBaseDir",
			apply: WithBaseDir("/tmp/x"),
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if cfg.baseDir != "/tmp/x" {
					t.Fatalf("baseDir = %q, want %q", cfg.baseDir, "/tmp/x")
				}
			},
		},
		{
			name:  "WithGithubRepo",
			apply: WithGithubRepo(ghRepo),
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if cfg.onGithub != ghRepo {
					t.Fatal("onGithub not set to provided repo")
				}
			},
		},
		{
			name:  "MakeDirAll",
			apply: MakeDirAll,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.mkdirAll {
					t.Fatal("mkdirAll not set")
				}
			},
		},
		{
			name:  "CloneGit",
			apply: CloneGit,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.cloneGit {
					t.Fatal("cloneGit not set")
				}
			},
		},
		{
			name:  "InitGit",
			apply: InitGit,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.initGit {
					t.Fatal("initGit not set")
				}
			},
		},
		{
			name:  "CreateRemote",
			apply: CreateRemote,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.createRemote {
					t.Fatal("createRemote not set")
				}
			},
		},
		{
			name:  "CreateOnGitHub",
			apply: CreateOnGitHub,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.createOnGitHub {
					t.Fatal("createOnGitHub not set")
				}
			},
		},
		{
			name:  "OwnerIsOrg",
			apply: OwnerIsOrg,
			check: func(t *testing.T, cfg *repoConfig) {
				t.Helper()

				if !cfg.ownerIsOrg {
					t.Fatal("ownerIsOrg not set")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &repoConfig{}
			tc.apply(cfg)
			tc.check(t, cfg)
		})
	}
}

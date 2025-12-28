package ghrepo

import (
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
)

var globalConfig = func() *config.Config {
	c, err := config.LoadConfig(config.GlobalScope)
	if err != nil {
		panic(fmt.Errorf("loading global git config: %w", err))
	}

	return c
}()

var initOpts = func() []git.InitOption {
	opts := []git.InitOption{}

	if globalConfig.Init.DefaultBranch != "" {
		opts = append(opts, git.WithDefaultBranch(
			plumbing.NewBranchReferenceName(globalConfig.Init.DefaultBranch),
		))
	}

	return opts
}()

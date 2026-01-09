package main

import (
	"context"
	"log"
	"os"

	"github.com/MarkRosemaker/ghrepo"
)

func main() {
	if err := do(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func do(ctx context.Context) error {
	s := ghrepo.NewService(ctx, os.Getenv("GITHUB_TOKEN"),
		ghrepo.WithBaseDir("/Users/mark/go/src/github.com/"),
		// ghrepo.MakeDirAll,
		// ghrepo.InitGit,
		// ghrepo.CreateRemote,
		// ghrepo.CreateOnGitHub,
	)

	if err := s.PrefetchUserRepositories(ctx, "MarkRosemaker"); err != nil {
		return err
	}

	// r, err := s.NewRepository(ctx, "faetools", "devtool")
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(r.LatestReleaseVersion(ctx))

	// fmt.Println(r.IsDefaultBranch())

	// if err := r.CheckoutDefault(true); err != nil {
	// 	return fmt.Errorf("checking out default: %w", err)
	// }

	// if err := r.CommitAll("auto commit"); err != nil {
	// 	return err
	// }

	// if err := r.Push(ctx); err != nil {
	// 	return err
	// }

	return nil
}

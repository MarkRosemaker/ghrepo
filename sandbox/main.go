package main

import (
	"context"
	"fmt"
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
		ghrepo.MakeDirAll,
		ghrepo.InitGit,
		ghrepo.CreateRemote,
		ghrepo.CreateOnGitHub,
	)

	r, err := s.NewRepository(ctx, "MarkRosemaker", "ghrepo")
	if err != nil {
		return err
	}

	fmt.Println(r.IsDefaultBranch())

	// if err := r.CheckoutDefault(true); err != nil {
	// 	return fmt.Errorf("checking out default: %w", err)
	// }

	fmt.Printf("r.Pull(ctx): %v\n", r.Pull(ctx))

	// if err := r.CommitAll("auto commit"); err != nil {
	// 	return err
	// }

	// if err := r.Push(ctx); err != nil {
	// 	return err
	// }

	return nil
}

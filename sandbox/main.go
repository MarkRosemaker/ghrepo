package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/MarkRosemaker/ghrepo"
)

var opts = []ghrepo.Option{
	ghrepo.WithBaseDir("/Users/mark/go/src/github.com/"),
	ghrepo.MakeDirAll,
	ghrepo.InitGit,
	ghrepo.CreateRemote,
}

func main() {
	if err := do(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func do(ctx context.Context) error {
	s := ghrepo.NewService(ctx, os.Getenv("GITHUB_TOKEN"), opts...)

	r, err := s.NewRepository(ctx, "MarkRosemaker", "ghrepo")
	if err != nil {
		return err
	}

	return nil
	// if err := r.CommitAll("auto commit"); err != nil {
	// 	return err
	// }

	if err := r.Push(); err != nil {
		return err
	}

	return nil

	r, err = s.NewRepository(ctx, "MarkRosemaker", "oauth2local", opts...)
	if err != nil {
		return err
	}

	fmt.Printf("r: %#v\n", r)

	r, err = s.NewRepository(ctx, "MarkRosemaker", "foo", opts...)
	if err != nil {
		return err
	}

	fmt.Printf("r: %#v\n", r)
	return nil
}

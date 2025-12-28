package main

import (
	"fmt"
	"log"

	"github.com/MarkRosemaker/ghrepo"
)

var opts = []ghrepo.Option{
	ghrepo.WithBaseDir("/Users/mark/go/src/github.com/"),
	ghrepo.MakeDirAll,
	ghrepo.InitGit,
}

func main() {
	if err := do(); err != nil {
		log.Fatal(err)
	}
}

func do() error {
	r, err := ghrepo.New("MarkRosemaker", "ghrepo", opts...)
	if err != nil {
		return err
	}

	if err := r.CommitAll("auto commit"); err != nil {
		return err
	}

	if err := r.Push(); err != nil {
		return err
	}

	return nil

	r, err = ghrepo.New("MarkRosemaker", "oauth2local", opts...)
	if err != nil {
		return err
	}

	fmt.Printf("r: %#v\n", r)

	r, err = ghrepo.New("MarkRosemaker", "foo", opts...)
	if err != nil {
		return err
	}

	fmt.Printf("r: %#v\n", r)
	return nil
}

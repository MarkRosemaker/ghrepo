# Roadmap

- `CloneGit` fails with "remote repository is empty" on a GitHub repository
  with no commits, so the branch in `NewRepository` that creates `main` after
  a clone without a default branch never runs. Decide whether an empty
  remote should be initialised locally instead.
- `zipBinary` leaves its temporary zip behind when it fails after creating
  it; only the success path removes it.

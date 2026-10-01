# Running the tests

`TestGetDefaultBranch` makes real commits through go-git, which reads the
machine's global git config. Two settings there decide whether it can:

- `user.name` and `user.email` must be set, or every commit fails with
  "author field is required".
- `commit.gpgSign` must be off. go-git cannot sign, and fails every commit
  with "cannot auto-sign commit" where it is on — as it is in some sandboxes.

Where either is wrong, point git at a config that holds only an identity:

    printf '[user]\n\tname = T\n\temail = t@example.com\n' > /tmp/gitconfig
    GIT_CONFIG_GLOBAL=/tmp/gitconfig make ready

Seven failing subtests that all say one of those two things are the machine,
not the change.

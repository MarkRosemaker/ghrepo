---
tagline: A GitHub repository as one Go value, its clone and its settings on GitHub.
---

`NewRepository` opens a GitHub repository as a `Repository`: a clone on this
machine, made or reused, with the git operations a tool needs on it (pull,
commit, push, status, reset and clean), and the repository on GitHub: its
description and topics, its releases and their binaries, and its issues, found
by their text, opened and edited. `ExecCommand` runs a command in the clone.

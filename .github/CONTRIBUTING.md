# Contributing

:wave: Hi there!

We're thrilled that you'd like to contribute to this project. Your help is
essential for keeping it great.

## Submitting a pull request

We use [pull requests][open-prs] to contribute new features, fixes, or
documentation.

[open-prs]: https://github.com/notorious-go/sync/pulls

Here are a few things you can do that will increase the likelihood of your pull
request being accepted:

- Keep your change as focused as possible. If there are multiple changes you
  would like to make that are not dependent upon each other, submit them as
  separate pull requests.
- Write [descriptive commit messages][commit-format]. Review the Git history to
  get a feel for it.

[commit-format]: https://go.dev/wiki/CommitMessage

Draft pull requests are also welcome to get feedback early on, or if there is
something blocking you.

## Running super-linter locally

All pull requests are validated with [super-linter][]. Since super-linter runs
via Docker, you'll need Docker installed to run it locally.

Before submitting your PR, validate your changes:

```bash
docker run --rm --platform=linux/amd64 \
  -e RUN_LOCAL=true \
  -e DEFAULT_BRANCH=main \
  -e VALIDATE_GITHUB_ACTIONS_ZIZMOR=false \
  -v "$(git rev-parse --show-toplevel)":/tmp/lint \
  -v "$(git rev-parse --git-common-dir)":"$(git rev-parse --git-common-dir)" \
  ghcr.io/super-linter/super-linter:latest
```

The `--platform=linux/amd64` flag provides compatibility on Apple Silicon Macs, until super-linter supports ARM natively.

When working with worktrees, you'll need to mount the worktree directory as a volume in the Docker container.

For automatic fixes to common issues, see super-linter's [automatic fixing documentation][autofix].

This "ZIZMOR" pedantic GHA analyzer is disabled because it is overly strict.

[super-linter]: https://github.com/super-linter/super-linter
[autofix]: https://github.com/super-linter/super-linter#how-to-use-fix-mode

## Resources

- [Go `sync` package][]
- [Go additional `x/sync` primitives][]
- [How to Contribute to Open Source][]
- [Using Pull Requests][]
- [GitHub Help][]

[Go `sync` package]: https://pkg.go.dev/sync
[Go additional `x/sync` primitives]: https://pkg.go.dev/golang.org/x/sync
[How to Contribute to Open Source]: https://opensource.guide/how-to-contribute/
[Using Pull Requests]: https://docs.github.com/en/github/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/about-pull-requests
[GitHub Help]: https://docs.github.com/en

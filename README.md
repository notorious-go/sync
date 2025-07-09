# sync

[![Go Reference](https://pkg.go.dev/badge/github.com/notorious-go/sync.svg)](https://pkg.go.dev/github.com/notorious-go/sync)

This repository provides supplementary Go concurrency primitives in addition to the ones provided
by the language, its standard library, and extension packages.

## Overview

The `sync` collection offers advanced concurrency constructs for experienced Go developers who need
specialized patterns beyond the standard library. Each primitive is designed to complement, not
replace, Go's built-in concurrency tools like channels, mutexes, and the `sync` package.

Similar to how `golang.org/x/sync` provides useful constructs (e.g. `errgroup`) while remaining
outside the standard library, this collection focuses on bespoke constructs that solve
general-purpose but specialized concurrency challenges.

## Project Structure

Each concurrency primitive is implemented as its own Go module to prevent dependency bloat. You can
import only the specific constructs you need:

```
go get github.com/danielorbach/notorious-sync/[module-name]
```

Each module follows standard Go package conventions:

- Comprehensive documentation with examples
- Full test coverage
- Benchmarks where applicable
- Clear API design following Go idioms

## Modules

_Modules will be added as they are developed. Each will be documented here with usage examples and
installation instructions._

## Contributing

Contributions should follow standard Go practices:

- Code style follows the [Google Go Style Guide](https://google.github.io/styleguide/go/)
- Commit messages follow conventional format
- All code must include tests and documentation
- Run `go fmt`, `go vet`, and `go test` before submitting

## License

[License will be added]

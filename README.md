# Notorious Concurrency Constructs

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

```shell
go get github.com/notorious-go/sync/[module-name]
```

Each module follows standard Go package conventions:

- Comprehensive documentation with examples
- Full test coverage
- Benchmarks where applicable
- Clear API design following Go idioms

## Modules

This repository does not contain a Go module at its root by design. Instead, each concurrency
primitive is packaged as an independent module in its own subdirectory. This approach prevents
dependency bloat and allows you to import only the specific constructs you need.

| Module | Docs | Description |
|--------|-----------|-------------|
| **[semaphore](./semaphore)** | [![Go Reference](https://pkg.go.dev/badge/github.com/notorious-go/sync/semaphore.svg)](https://pkg.go.dev/github.com/notorious-go/sync/semaphore) | A counting semaphore implementation for managing access to a limited number of resources. |

### Versioning with Git Tags

Each module in this repository is versioned independently using Git tags with the module path prefix.
This follows Go's multi-module repository conventions:

- Root-level tags like `v1.0.0` would apply to a root module (which doesn't exist here).
- Module-specific tags like `modulename/v1.0.0` version individual modules.
- Each module can evolve at its own pace without affecting others.

For example:

```shell
git tag workerpool/v1.0.0    # versions the workerpool module at v1.0.0
git tag semaphore/v3.2.1     # versions the semaphore module at v3.2.1
```

When importing, Go will resolve the correct version for each module:

```go
module example.com

go 1.24

require (
    github.com/notorious-go/sync/workerpool v1.0.0  // uses workerpool/v1.0.0 tag
    github.com/notorious-go/sync/semaphore v3.2.1   // uses semaphore/v3.2.1 tag
)
```

## Contributing

Contributions should follow standard Go practices:

- Code style follows the [Google Go Style Guide](https://google.github.io/styleguide/go/)
- Commit messages follow conventional format
- All code must include tests and documentation
- Run `go fmt`, `go vet`, and `go test` before submitting

## License

[License will be added]

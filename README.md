# BootLab Tester Utils

A shared framework module for BootLab course testing tools.

**Based on:** [codecrafters-io/tester-utils](https://github.com/codecrafters-io/tester-utils)

**Key improvements:**

1. **Flexible run modes** - Three execution modes:
   - Full JSON format (platform-dispatched)
   - Single stage test (development/debugging)
   - Run all tests (local self-test, default)

2. **CLI argument support** - Command-line argument parsing:
   - Positional: `./tester hello`
   - Flags: `./tester -s hello -d ~/work`
   - Help/version: `--help`, `--version`

3. **Optional config file** - `bootlab.yml` is optional (with sensible defaults), not mandatory

4. **Default working directory** - `BOOTLAB_REPOSITORY_DIR` defaults to current directory `.`, no explicit setup needed

5. **New SubmissionDir** - `TestCaseHarness` exposes the student submission directory for easy relative path access

6. **Improved Runner API** - Enhanced program testing API with automatic local executable detection

---

## Quick Start

```go
package main

import (
    "os"
    tester_utils "github.com/bootlab-dev/tester-utils"
)

func main() {
    definition := GetDefinition() // your test definition
    os.Exit(tester_utils.Run(os.Args[1:], definition))
}
```

## CLI Usage

```bash
# Run all tests
./tester

# Run a specific stage
./tester hello
./tester -s hello
./tester --stage hello

# Specify working directory
./tester -d ./my-solution hello

# Show help
./tester --help
```

## Runner Package

Fluent API for testing programs (similar to check50):

```go
import "github.com/bootlab-dev/tester-utils/runner"

// Basic usage
err := runner.Run("./hello").
    Stdin("Alice").
    Stdout("hello, Alice").
    Exit(0)

// PTY support
err := runner.Run("./mario").
    WithPty().
    Stdin("5").
    Stdout("#####").
    Exit(0)

// Test input rejection
err := runner.Run("./mario").
    Stdin("-1").
    Reject()
```

## Environment Variables

**Streaming log support** (Worker integration):

- `BOOTLAB_STREAM_LOGS=1` - Disables colors and redirects stdout to stderr, enabling the Worker to capture real-time log streams

## Documentation

For detailed API documentation, see [GoDoc](https://pkg.go.dev/github.com/bootlab-dev/tester-utils).

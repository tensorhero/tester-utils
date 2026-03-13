# TensorHero Tester Utils

A shared framework module for TensorHero course testing tools.

**Based on:** [codecrafters-io/tester-utils](https://github.com/codecrafters-io/tester-utils)

## Features

- **4-phase test pipeline** — declarative file checks, compilation, and pre-test hooks before `TestFunc`
- **Fluent Runner API** — check50-style program testing with blocking, interactive, and PTY modes
- **Flexible run modes** — platform-dispatched (JSON), single stage, or run-all (default)
- **CLI support** — `./tester hello`, `./tester -s hello -d ~/work`, `--help`
- **Sensible defaults** — `tensorhero.yml` optional, working directory defaults to `.`

## Quick Start

```go
package main

import (
    "os"
    tester_utils "github.com/tensorhero-dev/tensorhero-tester-utils"
    "github.com/tensorhero-dev/tensorhero-tester-utils/tester_definition"
)

func main() {
    definition := tester_definition.TesterDefinition{
        TestCases: []tester_definition.TestCase{
            {
                Slug:          "hello",
                RequiredFiles: []string{"hello.c"},
                CompileStep: &tester_definition.CompileStep{
                    Language: "c", Source: "hello.c", Output: "hello",
                },
                TestFunc: testHello,
            },
        },
    }
    os.Exit(tester_utils.Run(os.Args[1:], definition))
}
```

## 4-Phase Test Pipeline

Each `TestCase` executes through a 4-phase pipeline. Any phase failure skips remaining phases, runs `TeardownFuncs`, and reports the error.

```
Phase 1: RequiredFiles  →  Phase 2: CompileStep  →  Phase 3: BeforeFunc  →  Phase 4: TestFunc
(file existence check)     (compile with 30s timeout) (custom hook + panic recovery) (actual test)
```

All phases are opt-in via zero-value semantics (`nil`/empty = skip).

```go
tester_definition.TestCase{
    Slug:          "hello",
    RequiredFiles: []string{"hello.c"},                        // Phase 1
    CompileStep: &tester_definition.CompileStep{               // Phase 2
        Language: "c", Source: "hello.c", Output: "hello",
        IncludeParentDir: true,  // adds -I.. for shared headers
    },
    BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {  // Phase 3
        // custom setup (e.g. start a server, assemble test files)
        return nil
    },
    TestFunc: testHello,                                       // Phase 4
}
```

### CompileStep

| Language | Behavior                                                 | Example            |
| -------- | -------------------------------------------------------- | ------------------ |
| `"c"`    | `clang -o {Output} {Source} -lm -Wall -Werror` + `Flags` | C stages           |
| `"make"` | `make {Output}`                                          | speller (Makefile) |

Default C flags (`-lm -Wall -Werror`) are always applied; `Flags` appends extra flags.

## Runner Package

Fluent API for testing programs:

```go
import "github.com/tensorhero-dev/tensorhero-tester-utils/runner"

// Blocking mode — send stdin, check stdout + exit code
err := runner.Run(workDir, "hello").
    Stdin("Alice").
    Stdout("hello, Alice").
    Exit(0).
    Error()

// Interactive mode — test input rejection
err := runner.Run(workDir, "mario").
    Start().
    SendLine("-1").Reject().        // expect program to re-prompt
    SendLine("4").Stdout("#####").
    Exit(0).
    Error()

// PTY mode
err := runner.Run(workDir, "mario").
    WithPty().
    Stdin("5").Stdout("#####").
    Exit(0).
    Error()

// Compile C source
err := runner.CompileC(workDir, "hello.c", "hello", "-I..")
```

## CLI Usage

```bash
./tester              # run all tests
./tester hello        # run specific stage
./tester -s hello     # same, with flag
./tester -d ./work    # specify working directory
./tester --help       # show help
```

## Environment Variables

| Variable                     | Description                                          |
| ---------------------------- | ---------------------------------------------------- |
| `TENSORHERO_REPOSITORY_DIR`  | Working directory (default: `.`)                     |
| `TENSORHERO_STAGE`           | Run a single stage by slug (debug use)               |
| `TENSORHERO_TEST_CASES_JSON` | Full JSON test case list (used by Worker dispatch)   |
| `TENSORHERO_RANDOM_SEED`     | Fixed seed for deterministic random numbers          |
| `TENSORHERO_SKIP_ANTI_CHEAT` | Set `true` to skip anti-cheat test cases             |
| `TENSORHERO_STREAM_LOGS`     | Set `1` to disable colors and redirect stdout→stderr |
| `TENSORHERO_RECORD_FIXTURES` | Set `true` to record/update test fixtures            |

## Documentation

- [GoDoc](https://pkg.go.dev/github.com/tensorhero-dev/tensorhero-tester-utils)
- [4-Phase Pipeline Design](docs/4-phase-pipeline/design.md)

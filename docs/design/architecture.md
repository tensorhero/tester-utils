# BootCraft Tester Utils — Architecture Design

## 1. Overview

**tensorhero-tester-utils** is a Go framework for building course auto-grading testers. Each course ships its own tester binary that imports this library. The framework manages submission discovery, process execution, output assertion, and structured result reporting — allowing course authors to focus on writing test logic.

```
Course Tester Binary
  └── tensorhero-tester-utils (this library)
        ├── CLI entry point & environment parsing
        ├── 4-phase test pipeline
        ├── Process execution (pipe / PTY)
        └── Fluent assertion API (Runner)
```

## 2. Module Map

| Module                  | Responsibility                                                                                |
| ----------------------- | --------------------------------------------------------------------------------------------- |
| `tester.go`             | Entry point. Parses CLI args, merges environment, orchestrates stage execution.               |
| `tester_definition`     | Data model: `TesterDefinition`, `TestCase`, `CompileStep`.                                    |
| `tester_context`        | Resolves runtime context: run mode (JSON/STAGE/ALL), submission dir, executable path.         |
| `test_runner`           | 4-phase pipeline executor with timeout enforcement.                                           |
| `test_case_harness`     | Sandbox for `TestFunc`: provides logger, executable builder, file helpers, teardown registry. |
| `executable`            | Process lifecycle: spawn, I/O relay, memory limit (Linux), kill.                              |
| `runner`                | Fluent API for output assertions: `Run().Stdin().Stdout().Exit()`.                            |
| `logger`                | Color-coded leveled logging with global mutex serialization and quiet mode.                   |
| `linewriter`            | Channel-based line buffer; flushes on newline or 500 ms timeout.                              |
| `random`                | Deterministic RNG seeded via `BOOTCRAFT_RANDOM_SEED`.                                        |
| `bytes_diff_visualizer` | Hex + ASCII diff rendering with ANSI color at first differing byte.                           |
| `stdio_mocker`          | Test utility: replaces `os.Stdout/Stdin/Stderr` with temp files.                              |
| `testing`               | `ValidateTesterDefinitionAgainstYAML` — validates tester definitions against course YAML.     |

## 3. Execution Flow

```
main() → tester.Run(definition)
           │
           ├─ ParseArgs(os.Args)
           ├─ MergeArgsIntoEnv()
           ├─ RunCLI()
           │    ├─ newTester(definition)
           │    │    └─ NewTesterContext(env)  // resolve mode, dir, executable
           │    │
           │    ├─ runStages(testCases)
           │    │    └─ for each TestCase:
           │    │         TestRunner.RunTest()  // 4-phase pipeline
           │    │
           │    └─ runAntiCheatStages(antiCheatTestCases)  // quiet mode
           │
           └─ exit(0 | 1)
```

### Run Modes

| Mode      | Trigger                          | Behavior                                                         |
| --------- | -------------------------------- | ---------------------------------------------------------------- |
| **JSON**  | `BOOTCRAFT_TEST_CASES_JSON` set | Run specific test cases listed in JSON.                          |
| **STAGE** | `BOOTCRAFT_STAGE` set           | Run all test cases up to and including the specified stage slug. |
| **ALL**   | Neither set                      | Run every test case sequentially.                                |

## 4. Four-Phase Test Pipeline

Each `TestCase` executes through four sequential phases inside `TestRunner.RunTest()`:

```
Phase 1: RequiredFiles
  └─ Verify expected files exist in submission directory.

Phase 2: CompileStep
  └─ Compile the submission (language, source, flags, output path).
     Supports IncludeParentDir for multi-file compilation.

Phase 3: BeforeFunc
  └─ Optional setup callback (e.g., seed database, create fixtures).

Phase 4: TestFunc                          ← with timeout
  └─ Core test logic runs inside TestCaseHarness sandbox.
     Has access to: Logger, Executable builder, SubmissionDir, file helpers.
     Uses time.NewTimer for clean timeout cancellation.
```

If any phase fails, later phases are skipped and the test case reports failure.

## 5. Executable Subsystem

The `executable` package manages child process lifecycle:

```
Executable
  ├── WorkingDir, Args, Env
  ├── StdioHandler (interface)
  │     ├── PipeStdioHandler   — line-buffered pipe I/O via LineWriter
  │     └── PTYStdioHandler    — pseudo-terminal for interactive programs
  ├── MemoryLimit (default 2 GB)
  └── LoggerFunc
```

### I/O Relay

`setupIORelay` copies child stdout/stderr to the logger, enforcing a **30 KB** output cap per stream. Output is relayed through `LineWriter` which flushes complete lines or after a 500 ms idle timeout.

### Memory Monitoring (Linux)

```
memoryMonitor
  ├── Polls /proc/<pid>/status (VmRSS) every 100 ms
  ├── Sets RLIMIT_AS = 3× limit (virtual memory safety net)
  └── Marks oomKilled (atomic) when RSS exceeds limit
```

On non-Linux platforms, `memoryMonitor` is a no-op (build tags).

### Security

`getSafeEnvironmentVariables()` filters out any env var matching `TENSORHERO_SECRET*` before passing the environment to child processes. This prevents secret leakage from the grading environment into student code.

## 6. Runner — Fluent Assertion API

The `Runner` provides a check50-style chain API for testing executables:

```go
// Blocking mode — full lifecycle
testCase.Run("./server").
    Stdin("GET /").
    Stdout("200 OK").
    Exit(0)

// Interactive mode — step-by-step
r := testCase.Start("./repl")
r.SendLine("help")
r.Stdout("Available commands:")
r.SendLine("quit")
r.WaitForExit()
```

### Output Matchers

| Method           | Match Type               |
| ---------------- | ------------------------ |
| `Stdout(s)`      | Contains substring       |
| `StdoutExact(s)` | Exact match              |
| `StdoutRegex(p)` | Regex pattern            |
| `Exit(code)`     | Exit code equals         |
| `Reject(code)`   | Exit code does NOT equal |

On mismatch, the runner logs expected vs. actual output. For byte-level differences, `BytesDiffVisualizer` renders a hex + ASCII comparison.

## 7. Logger Architecture

```
Logger
  ├── Levels: Debug, Info, Success, Error
  ├── Color: yellow prefix, green success, red error, cyan debug
  ├── syncWriter → global mutex (serializes all logger instances)
  ├── Quiet mode: suppresses non-critical output (anti-cheat stages)
  └── Secondary prefixes: nested context (e.g., stage → phase → detail)
```

## 8. Configuration Contract

### Environment Variables

| Variable                     | Required | Description                                |
| ---------------------------- | -------- | ------------------------------------------ |
| `BOOTCRAFT_REPOSITORY_DIR`  | Yes      | Path to submission directory.              |
| `BOOTCRAFT_STAGE`           | No       | Target stage slug (STAGE mode).            |
| `BOOTCRAFT_TEST_CASES_JSON` | No       | JSON array of test case slugs (JSON mode). |
| `BOOTCRAFT_RANDOM_SEED`     | No       | Deterministic seed for reproducible tests. |
| `BOOTCRAFT_SKIP_ANTI_CHEAT` | No       | Skip anti-cheat stages when set.           |
| `BOOTCRAFT_STREAM_LOGS`     | No       | Stream logs in real time.                  |
| `BOOTCRAFT_RECORD_FIXTURES` | No       | Record test fixtures for replay.           |
| `TENSORHERO_SECRET`          | No       | Secret(s) filtered from child env.         |

### Submission Config (`bootcraft.yml`)

Optional YAML file in the submission directory:

```yaml
debug: true # Enables debug-level logging
```

## 9. Data Flow Diagram

```
┌──────────────┐    env vars    ┌────────────────┐
│  CI / Runner │ ──────────────→│  Tester Binary  │
└──────────────┘                └───────┬────────┘
                                        │
                        ┌───────────────┼───────────────┐
                        ▼               ▼               ▼
                  TesterContext    TestRunner ×N    AntiCheat ×M
                  (mode, dir,     (4-phase each)   (quiet mode)
                   executable)          │
                                        ▼
                                  TestCaseHarness
                                   │         │
                              Executable    Runner
                              (spawn,       (assert
                               I/O relay,    output,
                               mem limit)    exit code)
                                   │
                                   ▼
                            Student Process
```

## 10. Key Design Decisions

| Decision                      | Rationale                                                                           |
| ----------------------------- | ----------------------------------------------------------------------------------- |
| Build-tag memory monitoring   | Only Linux exposes `/proc` RSS; other platforms get a no-op.                        |
| 30 KB output cap              | Prevents student programs from exhausting grader memory.                            |
| Global logger mutex           | Simple serialization avoids interleaved output from concurrent phases.              |
| LineWriter channel flush      | Decouples write granularity from display; ensures complete lines in output.         |
| Fluent Runner API             | Mirrors check50 UX — course authors write readable, sequential assertions.          |
| Anti-cheat quiet mode         | Runs duplicate tests silently so students cannot reverse-engineer expected outputs. |
| `TENSORHERO_SECRET` filtering | Defense-in-depth: secrets never leak into child process environment.                |
| Deterministic RNG             | `BOOTCRAFT_RANDOM_SEED` enables reproducible test runs for debugging.              |

## 11. Extension Points

- **New `StdioHandler`**: Implement the interface to add custom I/O strategies beyond pipe and PTY.
- **New `CompileStep` languages**: Add language-specific compilation logic in the compile phase.
- **Custom `BeforeFunc`**: Arbitrary setup logic per test case (DB seeding, fixture generation, etc.).
- **Additional output matchers**: Extend `Runner` with new assertion methods as needed.

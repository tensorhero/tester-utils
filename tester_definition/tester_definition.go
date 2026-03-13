package tester_definition

import (
	"time"

	"github.com/tensorhero-dev/tensorhero-tester-utils/test_case_harness"
)

// CompileStep declares a compilation step to be executed by the framework
// before TestFunc. Supports C (clang) and Make.
type CompileStep struct {
	// Language is the compile language: "c" or "make".
	//   "c"    → invokes clang with default flags (-lm -Wall -Werror) + Flags
	//   "make" → invokes make <Target>
	Language string

	// Source is the source file to compile (required when Language="c").
	Source string

	// Output is the compilation target.
	//   Language="c":    output binary name (e.g. "hello"), written to {SubmissionDir}/{Output}.
	//   Language="make": make target name (e.g. "speller"), equivalent to `make {Output}`.
	Output string

	// Flags are extra compiler flags appended (not replacing) after the default flags.
	Flags []string

	// IncludeParentDir adds -I.. to include the parent directory (e.g. for tensorhero.h).
	IncludeParentDir bool
}

// TestCase represents a test case that'll be run against the user's code.
//
// For now, we only support one test case per stage. This may change in the future.
//
// We enforce the one-test-case-per-stage rule by requiring that the test case's slug matches the stage's slug (from the YAML definition).
//
// Execution pipeline (any phase failure skips remaining phases, then runs TeardownFuncs):
//
//	Phase 1: RequiredFiles  → check files exist
//	Phase 2: CompileStep    → compile source (with 30s timeout)
//	Phase 3: BeforeFunc     → custom pre-test hook (with panic recovery)
//	Phase 4: TestFunc       → actual test (with Timeout)
type TestCase struct {
	// Slug is the unique identifier for this test case. For now, it must match the slug of the stage from the course's YAML definition.
	Slug string

	// TestFunc is the function that'll be run against the user's code.
	TestFunc func(testCaseHarness *test_case_harness.TestCaseHarness) error

	// Timeout is the maximum amount of time that the test case can run for.
	Timeout time.Duration

	// RequiredFiles declares files that must exist in the submission directory.
	// The framework checks these before TestFunc; any missing file aborts the test.
	// nil or empty = skip check.
	RequiredFiles []string

	// CompileStep declares a compilation step executed after RequiredFiles check
	// and before TestFunc. nil = skip compilation.
	// The compiled output is written to {SubmissionDir}/{CompileStep.Output}.
	CompileStep *CompileStep

	// BeforeFunc is a custom pre-test hook executed after declarative checks
	// (RequiredFiles, CompileStep) and before TestFunc.
	// nil = skip. Returning an error skips TestFunc. Panics are recovered.
	// Shares the same TestCaseHarness instance as TestFunc.
	BeforeFunc func(testCaseHarness *test_case_harness.TestCaseHarness) error
}

func (t TestCase) CustomOrDefaultTimeout() time.Duration {
	if (t.Timeout == 0) || (t.Timeout == time.Duration(0)) {
		return 10 * time.Second
	} else {
		return t.Timeout
	}
}

type TesterDefinition struct {
	// Example: spawn_redis_server.sh
	ExecutableFileName       string
	LegacyExecutableFileName string

	TestCases          []TestCase
	AntiCheatTestCases []TestCase
}

func (t TesterDefinition) TestCaseBySlug(slug string) (TestCase, bool) {
	for _, testCase := range t.TestCases {
		if testCase.Slug == slug {
			return testCase, true
		}
	}

	return TestCase{}, false
}

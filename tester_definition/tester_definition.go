package tester_definition

import (
	"time"

	"github.com/tensorhero-cn/tester-utils/test_case_harness"
)

// LanguageRule defines a language detection rule for CompileStep.Language="auto" mode.
// The framework checks AutoDetect rules in order; the first matching DetectFile wins.
type LanguageRule struct {
	// DetectFile is the file whose existence signals this language (relative to submission dir).
	DetectFile string

	// Language is the compile language to use when matched ("java", "python", "c", "make").
	Language string

	// Source is the main source file for compilation. Defaults to DetectFile if empty.
	Source string

	// Flags are extra compiler flags (e.g. additional .java files for javac).
	Flags []string

	// RunCmd is the run command (e.g. "java", "python3") exposed via harness.DetectedLang.
	RunCmd string

	// RunArgs are the run arguments (e.g. ["-cp", ".", "TestE01"]) exposed via harness.DetectedLang.
	RunArgs []string
}

// CompileStep declares a compilation step to be executed by the framework
// before TestFunc. Supports C (clang), Make, Java (javac), Python (py_compile), and auto-detection.
type CompileStep struct {
	// Language is the compile language: "c", "make", "java", "python", or "auto".
	//   "c"      → invokes clang with default flags (-lm -Wall -Werror) + Flags
	//   "make"   → invokes make <Target>
	//   "java"   → invokes javac
	//   "python" → invokes python3 -m py_compile (syntax check)
	//   "auto"   → detects language via AutoDetect rules, then dispatches
	Language string

	// Source is the source file to compile (required when Language="c" or "java").
	Source string

	// Output is the compilation target (used by "c" and "make" only; ignored for java/python/auto).
	Output string

	// Flags are extra compiler flags appended (not replacing) after the default flags.
	Flags []string

	// IncludeParentDir adds -I.. to include the parent directory (e.g. for tensorhero.h).
	IncludeParentDir bool

	// AutoDetect is the ordered list of language detection rules (used only when Language="auto").
	// The first rule whose DetectFile exists in the submission directory wins.
	AutoDetect []LanguageRule
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

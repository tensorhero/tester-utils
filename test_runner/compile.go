package test_runner

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/bootlab-dev/tester-utils/test_case_harness"
	"github.com/bootlab-dev/tester-utils/tester_definition"
)

// defaultCompileTimeout is the hard timeout for compilation steps.
const defaultCompileTimeout = 30 * time.Second

// runCompileStep dispatches compilation based on CompileStep.Language.
func (r TestRunner) runCompileStep(harness *test_case_harness.TestCaseHarness, cs *tester_definition.CompileStep) error {
	logger := harness.Logger
	workDir := harness.SubmissionDir

	switch cs.Language {
	case "c":
		logger.Infof("Compiling %s...", cs.Source)
		if err := compileC(workDir, cs); err != nil {
			return fmt.Errorf("%s does not compile: %v", cs.Source, err)
		}
		logger.Successf("%s compiles", cs.Source)
		return nil

	case "make":
		logger.Infof("Running make %s...", cs.Output)
		cmd := exec.Command("make", cs.Output)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("make %s failed: %s\n%s", cs.Output, err, string(out))
		}
		logger.Successf("make %s succeeds", cs.Output)
		return nil

	default:
		return fmt.Errorf("unsupported compile language: %s", cs.Language)
	}
}

// runCompileStepWithTimeout wraps runCompileStep with a hard timeout to prevent
// compilation from hanging indefinitely (e.g. due to #include cycles or disk issues).
func (r TestRunner) runCompileStepWithTimeout(harness *test_case_harness.TestCaseHarness, cs *tester_definition.CompileStep, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- r.runCompileStep(harness, cs)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("compilation timed out after %v", timeout)
	}
}

// compileC compiles a C source file using clang with default flags (-lm -Wall -Werror).
// CompileStep.Flags are appended (not replacing) the defaults.
//
// Note: runner.CompileC() in the runner package exists without default flags.
// This function is used by the framework's automatic CompileStep pipeline.
// For complex stages, use runner.CompileC() directly in BeforeFunc.
func compileC(workDir string, cs *tester_definition.CompileStep) error {
	args := []string{"-o", cs.Output, cs.Source, "-lm", "-Wall", "-Werror"}
	args = append(args, cs.Flags...)
	if cs.IncludeParentDir {
		args = append(args, "-I..")
	}

	cmd := exec.Command("clang", args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s\nOutput:\n%s", err, string(out))
	}
	return nil
}

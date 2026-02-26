package test_runner

import (
	"fmt"
	"time"

	"github.com/hellobyte-dev/tester-utils/executable"
	"github.com/hellobyte-dev/tester-utils/logger"
	"github.com/hellobyte-dev/tester-utils/test_case_harness"
	"github.com/hellobyte-dev/tester-utils/tester_definition"
)

type TestRunnerStep struct {
	// TestCase is the test case that'll be run against the user's code.
	TestCase tester_definition.TestCase

	// TesterLogPrefix is the prefix that'll be used for all logs emitted by the tester. Example: "stage-1"
	TesterLogPrefix string

	// Title is the title of the test case. Example: "Stage #1: Bind to a port"
	Title string
}

// testRunner is used to run multiple tests
type TestRunner struct {
	isQuiet       bool   // Used for anti-cheat tests, where we only want Critical logs to be emitted
	submissionDir string // The directory containing the student's submission
	steps         []TestRunnerStep
}

func NewTestRunner(steps []TestRunnerStep, submissionDir string) TestRunner {
	return TestRunner{
		steps:         steps,
		submissionDir: submissionDir,
	}
}

func NewQuietTestRunner(steps []TestRunnerStep, submissionDir string) TestRunner {
	return TestRunner{isQuiet: true, steps: steps, submissionDir: submissionDir}
}

// Run runs all tests in a stageRunner
func (r TestRunner) Run(isDebug bool, executable *executable.Executable) bool {
	for index, step := range r.steps {
		if index != 0 {
			fmt.Println("")
		}

		testCaseHarness := test_case_harness.TestCaseHarness{
			Logger:        r.getLoggerForStep(isDebug, step),
			SubmissionDir: r.submissionDir,
			Executable:    executable.Clone(),
		}

		logger := testCaseHarness.Logger
		logger.Infof("Running tests for %s", step.Title)

		// ========== Phase 1: RequiredFiles ==========
		if len(step.TestCase.RequiredFiles) > 0 {
			if err := r.checkRequiredFiles(&testCaseHarness, step.TestCase.RequiredFiles); err != nil {
				r.reportTestError(err, isDebug, logger)
				testCaseHarness.RunTeardownFuncs()
				return false
			}
		}

		// ========== Phase 2: CompileStep (with 30s timeout) ==========
		if step.TestCase.CompileStep != nil {
			if err := r.runCompileStepWithTimeout(&testCaseHarness, step.TestCase.CompileStep, defaultCompileTimeout); err != nil {
				r.reportTestError(err, isDebug, logger)
				testCaseHarness.RunTeardownFuncs()
				return false
			}
		}

		// ========== Phase 3: BeforeFunc (with panic recovery) ==========
		if step.TestCase.BeforeFunc != nil {
			if err := r.safeRunBeforeFunc(&testCaseHarness, step.TestCase.BeforeFunc); err != nil {
				r.reportTestError(err, isDebug, logger)
				testCaseHarness.RunTeardownFuncs()
				return false
			}
		}

		// ========== Phase 4: TestFunc (original logic) ==========
		stepResultChannel := make(chan error, 1)
		go func() {
			err := step.TestCase.TestFunc(&testCaseHarness)
			stepResultChannel <- err
		}()

		timeout := step.TestCase.CustomOrDefaultTimeout()

		var err error
		select {
		case stageErr := <-stepResultChannel:
			err = stageErr
		case <-time.After(timeout):
			err = fmt.Errorf("timed out, test exceeded %d seconds", int64(timeout.Seconds()))
		}

		if err != nil {
			r.reportTestError(err, isDebug, logger)
		} else {
			logger.Successf("Test passed.")
		}

		testCaseHarness.RunTeardownFuncs()

		if err != nil {
			return false
		}
	}

	return true
}

func (r TestRunner) getLoggerForStep(isDebug bool, step TestRunnerStep) *logger.Logger {
	if r.isQuiet {
		return logger.GetQuietLogger("")
	} else {
		return logger.GetLogger(isDebug, fmt.Sprintf("[%s] ", step.TesterLogPrefix))
	}
}

func (r TestRunner) reportTestError(err error, isDebug bool, logger *logger.Logger) {
	logger.Errorf("%s", err)
	logger.Errorf("Test failed")
}

// Fuck you, go
func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

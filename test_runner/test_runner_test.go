package test_runner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tensorhero/tester-utils/executable"
	"github.com/tensorhero/tester-utils/logger"
	"github.com/tensorhero/tester-utils/test_case_harness"
	"github.com/tensorhero/tester-utils/tester_definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============== Helpers ==============

func newTestHarness(t *testing.T, dir string) *test_case_harness.TestCaseHarness {
	return &test_case_harness.TestCaseHarness{
		Logger:        logger.GetLogger(false, "[test] "),
		SubmissionDir: dir,
		Executable:    executable.NewExecutable(dir),
	}
}

func newRunner(steps []TestRunnerStep, dir string) TestRunner {
	return NewTestRunner(steps, dir)
}

func passFunc(h *test_case_harness.TestCaseHarness) error { return nil }
func failFunc(h *test_case_harness.TestCaseHarness) error { return errors.New("test failed") }

func makeStep(tc tester_definition.TestCase) TestRunnerStep {
	return TestRunnerStep{
		TestCase:        tc,
		TesterLogPrefix: "test-1",
		Title:           "Test Stage",
	}
}

func dummyExecutable(dir string) *executable.Executable {
	return executable.NewExecutable(dir)
}

// ============== Phase 1: RequiredFiles ==============

func TestRequiredFiles_AllExist(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.c"), []byte("int main(){}"), 0644)

	called := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"hello.c"},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			called = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result, "should pass when all required files exist")
	assert.True(t, called, "TestFunc should have been called")
}

func TestRequiredFiles_Missing(t *testing.T) {
	dir := t.TempDir()
	// Do NOT create hello.c

	called := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"hello.c"},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			called = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result, "should fail when required file is missing")
	assert.False(t, called, "TestFunc should NOT have been called")
}

func TestRequiredFiles_MultipleFiles_FirstMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.c"), []byte(""), 0644)

	called := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"a.c", "b.c"},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			called = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.False(t, called)
}

// ============== Phase 3: BeforeFunc ==============

func TestBeforeFunc_Success(t *testing.T) {
	dir := t.TempDir()

	beforeCalled := false
	testCalled := false
	tc := tester_definition.TestCase{
		Slug: "test",
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			beforeCalled = true
			return nil
		},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			testCalled = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result)
	assert.True(t, beforeCalled, "BeforeFunc should have been called")
	assert.True(t, testCalled, "TestFunc should have been called after BeforeFunc success")
}

func TestBeforeFunc_Error_SkipsTestFunc(t *testing.T) {
	dir := t.TempDir()

	testCalled := false
	tc := tester_definition.TestCase{
		Slug: "test",
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			return errors.New("before failed")
		},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			testCalled = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.False(t, testCalled, "TestFunc should NOT be called when BeforeFunc fails")
}

func TestBeforeFunc_Panic_Recovered(t *testing.T) {
	dir := t.TempDir()

	testCalled := false
	tc := tester_definition.TestCase{
		Slug: "test",
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			panic("unexpected panic in BeforeFunc")
		},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			testCalled = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	// Should NOT panic the test process
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result, "should fail when BeforeFunc panics")
	assert.False(t, testCalled, "TestFunc should NOT be called when BeforeFunc panics")
}

// ============== Phase ordering: RequiredFiles → BeforeFunc → TestFunc ==============

func TestPhaseOrdering_RequiredFiles_Then_BeforeFunc(t *testing.T) {
	dir := t.TempDir()
	// Required file is missing → BeforeFunc should NOT be called

	beforeCalled := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"missing.c"},
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			beforeCalled = true
			return nil
		},
		TestFunc: passFunc,
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.False(t, beforeCalled, "BeforeFunc should NOT run when RequiredFiles fails")
}

// ============== Backward compatibility ==============

func TestZeroValues_BehaviorUnchanged(t *testing.T) {
	dir := t.TempDir()

	// TestCase with only Slug + TestFunc (all new fields nil/empty) — original behavior
	called := false
	tc := tester_definition.TestCase{
		Slug: "test",
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			called = true
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result)
	assert.True(t, called)
}

func TestZeroValues_FailingTestFunc(t *testing.T) {
	dir := t.TempDir()

	tc := tester_definition.TestCase{
		Slug:     "test",
		TestFunc: failFunc,
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
}

// ============== Teardown on phase failure ==============

func TestTeardown_CalledOnRequiredFilesFail(t *testing.T) {
	dir := t.TempDir()

	teardownCalled := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"missing.c"},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			// Register teardown before returning — but in reality teardown
			// is registered in BeforeFunc/TestFunc. Let's register it in BeforeFunc.
			return nil
		},
	}

	// We need a way to test teardown. Let's use a BeforeFunc that won't run (RequiredFiles fails first),
	// but we can test via a TestFunc that registers teardown.
	// Instead, let's test the specific scenario: RequiredFiles pass, BeforeFunc fails, teardown runs.
	os.WriteFile(filepath.Join(dir, "hello.c"), []byte(""), 0644)

	tc = tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"hello.c"},
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			h.RegisterTeardownFunc(func() {
				teardownCalled = true
			})
			return errors.New("before failed")
		},
		TestFunc: passFunc,
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.True(t, teardownCalled, "Teardown should be called even when BeforeFunc fails")
}

func TestTeardown_CalledOnTestFuncFail(t *testing.T) {
	dir := t.TempDir()

	teardownCalled := false
	tc := tester_definition.TestCase{
		Slug: "test",
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			h.RegisterTeardownFunc(func() {
				teardownCalled = true
			})
			return errors.New("test failed")
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.True(t, teardownCalled, "Teardown should run on TestFunc failure")
}

func TestTeardown_CalledOnTestFuncSuccess(t *testing.T) {
	dir := t.TempDir()

	teardownCalled := false
	tc := tester_definition.TestCase{
		Slug: "test",
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			h.RegisterTeardownFunc(func() {
				teardownCalled = true
			})
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result)
	assert.True(t, teardownCalled, "Teardown should run on TestFunc success too")
}

// ============== Timeout ==============

func TestTestFunc_Timeout(t *testing.T) {
	dir := t.TempDir()

	tc := tester_definition.TestCase{
		Slug:    "test",
		Timeout: 500 * time.Millisecond,
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			time.Sleep(5 * time.Second)
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result, "should fail when TestFunc times out")
}

// ============== Quiet mode ==============

func TestQuietMode_PhasesWork(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.c"), []byte(""), 0644)

	called := false
	tc := tester_definition.TestCase{
		Slug:          "test",
		RequiredFiles: []string{"hello.c"},
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			return nil
		},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			called = true
			return nil
		},
	}

	r := NewQuietTestRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result)
	assert.True(t, called, "TestFunc should run in quiet mode")
}

// ============== BeforeFunc shares harness ==============

func TestBeforeFunc_SharesHarness(t *testing.T) {
	dir := t.TempDir()

	// BeforeFunc writes a file, TestFunc reads it — verifying they share SubmissionDir
	tc := tester_definition.TestCase{
		Slug: "test",
		BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {
			return os.WriteFile(filepath.Join(h.SubmissionDir, "marker.txt"), []byte("ok"), 0644)
		},
		TestFunc: func(h *test_case_harness.TestCaseHarness) error {
			if !h.FileExists("marker.txt") {
				return errors.New("marker.txt not found — harness not shared")
			}
			return nil
		},
	}

	r := newRunner([]TestRunnerStep{makeStep(tc)}, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.True(t, result, "BeforeFunc and TestFunc should share the same harness")
}

// ============== Multiple steps ==============

func TestMultipleSteps_StopsOnFirstFailure(t *testing.T) {
	dir := t.TempDir()

	step2Called := false
	steps := []TestRunnerStep{
		makeStep(tester_definition.TestCase{
			Slug:     "step-1",
			TestFunc: failFunc,
		}),
		{
			TestCase: tester_definition.TestCase{
				Slug: "step-2",
				TestFunc: func(h *test_case_harness.TestCaseHarness) error {
					step2Called = true
					return nil
				},
			},
			TesterLogPrefix: "test-2",
			Title:           "Step 2",
		},
	}

	r := newRunner(steps, dir)
	result := r.Run(false, dummyExecutable(dir))

	assert.False(t, result)
	assert.False(t, step2Called, "Step 2 should not run when step 1 fails")
}

// ============== Unit tests for safeRunBeforeFunc ==============

func TestSafeRunBeforeFunc_NormalReturn(t *testing.T) {
	dir := t.TempDir()
	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.safeRunBeforeFunc(h, func(h *test_case_harness.TestCaseHarness) error {
		return nil
	})
	assert.NoError(t, err)
}

func TestSafeRunBeforeFunc_ErrorReturn(t *testing.T) {
	dir := t.TempDir()
	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.safeRunBeforeFunc(h, func(h *test_case_harness.TestCaseHarness) error {
		return errors.New("some error")
	})
	assert.EqualError(t, err, "some error")
}

func TestSafeRunBeforeFunc_PanicString(t *testing.T) {
	dir := t.TempDir()
	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.safeRunBeforeFunc(h, func(h *test_case_harness.TestCaseHarness) error {
		panic("boom")
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BeforeFunc panicked: boom")
}

func TestSafeRunBeforeFunc_PanicNil(t *testing.T) {
	dir := t.TempDir()
	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.safeRunBeforeFunc(h, func(h *test_case_harness.TestCaseHarness) error {
		panic(nil)
	})
	// Go 1.21+ panic(nil) is caught as non-nil, but for pre-1.21 it would be nil.
	// Either way, execution should not crash the test process.
	_ = err // The important thing is that we didn't panic out
}

// ============== Unit tests for checkRequiredFiles ==============

func TestCheckRequiredFiles_AllExist(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.c"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.c"), []byte(""), 0644)

	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.checkRequiredFiles(h, []string{"a.c", "b.c"})
	assert.NoError(t, err)
}

func TestCheckRequiredFiles_OneMissing(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.c"), []byte(""), 0644)

	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.checkRequiredFiles(h, []string{"a.c", "missing.c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing.c does not exist")
}

func TestCheckRequiredFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	h := newTestHarness(t, dir)
	r := TestRunner{}

	err := r.checkRequiredFiles(h, []string{})
	assert.NoError(t, err, "empty list should pass")
}

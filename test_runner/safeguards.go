package test_runner

import (
	"fmt"

	"github.com/tensorhero-cn/tester-utils/test_case_harness"
)

// safeRunBeforeFunc executes a BeforeFunc with panic recovery.
// If the function panics, the panic is caught and converted to an error
// instead of crashing the entire process.
//
// This is necessary because Phase 3 (BeforeFunc) runs in the main goroutine,
// not inside the timeout goroutine used for TestFunc (Phase 4).
func (r TestRunner) safeRunBeforeFunc(harness *test_case_harness.TestCaseHarness, fn func(*test_case_harness.TestCaseHarness) error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("BeforeFunc panicked: %v", rec)
		}
	}()
	return fn(harness)
}

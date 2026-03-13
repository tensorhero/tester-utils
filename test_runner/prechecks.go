package test_runner

import (
	"fmt"

	"github.com/tensorhero/tester-utils/test_case_harness"
)

// checkRequiredFiles verifies that all required files exist in the submission directory.
// Returns nil if all files exist, or an error for the first missing file.
func (r TestRunner) checkRequiredFiles(harness *test_case_harness.TestCaseHarness, files []string) error {
	logger := harness.Logger
	for _, f := range files {
		logger.Infof("Checking %s exists...", f)
		if !harness.FileExists(f) {
			return fmt.Errorf("%s does not exist", f)
		}
		logger.Successf("%s exists", f)
	}
	return nil
}

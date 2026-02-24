package test_case_harness

import (
	"os"
	"path/filepath"

	"github.com/bootlab-dev/bootlab-tester-utils/executable"
	"github.com/bootlab-dev/bootlab-tester-utils/logger"
)

// TestCaseHarness is passed to your TestCase's TestFunc.
//
// For BootLab courses that don't use your_program.sh, use SubmissionDir directly:
//
//	if !harness.FileExists("hello.c") {
//	    return fmt.Errorf("hello.c does not exist")
//	}
//
// For long-lived programs (like a Redis server), use Executable:
//
//	if err := harness.Executable.Start(); err != nil {
//	   return err
//	}
//	harness.RegisterTeardownFunc(func() { harness.Executable.Kill() })
//
// For scripts that run and exit (like a Git command):
//
//	result, err := harness.Executable.Run("cat-file", "-p", "sha")
//	if err != nil {
//	    return err
//	}
type TestCaseHarness struct {
	// Logger is to be used for all logs generated from the test function.
	Logger *logger.Logger

	// SubmissionDir is the directory containing the student's submission.
	// Use this for direct file access without needing your_program.sh.
	SubmissionDir string

	// Executable is the program to be tested (may point to SubmissionDir if no ExecutableFileName).
	Executable *executable.Executable

	// teardownFuncs are run once the error has been reported to the user
	teardownFuncs []func()
}

func (s *TestCaseHarness) RegisterTeardownFunc(teardownFunc func()) {
	s.teardownFuncs = append(s.teardownFuncs, teardownFunc)
}

func (s *TestCaseHarness) RunTeardownFuncs() {
	for _, teardownFunc := range s.teardownFuncs {
		teardownFunc()
	}
}

func (s *TestCaseHarness) NewExecutable() *executable.Executable {
	return s.Executable.Clone()
}

// FilePath returns the absolute path to a file within the submission directory.
func (s *TestCaseHarness) FilePath(relativePath string) string {
	return filepath.Join(s.SubmissionDir, relativePath)
}

// FileExists checks if a file exists within the submission directory.
func (s *TestCaseHarness) FileExists(relativePath string) bool {
	_, err := os.Stat(s.FilePath(relativePath))
	return err == nil
}

// ReadFile reads the contents of a file within the submission directory.
func (s *TestCaseHarness) ReadFile(relativePath string) ([]byte, error) {
	return os.ReadFile(s.FilePath(relativePath))
}

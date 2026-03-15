package test_runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorhero/tester-utils/tester_definition"
)

// detectLanguage checks rules in order, returning the first rule whose DetectFile exists.
// Returns an error listing all expected files when nothing matches.
func detectLanguage(workDir string, rules []tester_definition.LanguageRule) (*tester_definition.LanguageRule, error) {
	for i := range rules {
		path := filepath.Join(workDir, rules[i].DetectFile)
		if _, err := os.Stat(path); err == nil {
			return &rules[i], nil
		}
	}

	expected := make([]string, len(rules))
	for i, r := range rules {
		expected[i] = fmt.Sprintf("%s (%s)", r.DetectFile, r.Language)
	}
	return nil, fmt.Errorf("cannot detect language: none of [%s] found in submission",
		strings.Join(expected, ", "))
}

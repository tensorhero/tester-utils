package test_runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensorhero-cn/tester-utils/tester_definition"
)

var twoRules = []tester_definition.LanguageRule{
	{DetectFile: "NDArray.java", Language: "java", RunCmd: "java", RunArgs: []string{"-cp", ".", "TestE01"}},
	{DetectFile: "num4py/ndarray.py", Language: "python", RunCmd: "python3", RunArgs: []string{"tests/test_e01.py"}},
}

func TestDetectLanguage_FirstRuleMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "NDArray.java"), []byte(""), 0644)

	rule, err := detectLanguage(dir, twoRules)
	require.NoError(t, err)
	assert.Equal(t, "java", rule.Language)
	assert.Equal(t, "NDArray.java", rule.DetectFile)
}

func TestDetectLanguage_SecondRuleMatches(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "num4py"), 0755)
	os.WriteFile(filepath.Join(dir, "num4py/ndarray.py"), []byte(""), 0644)

	rule, err := detectLanguage(dir, twoRules)
	require.NoError(t, err)
	assert.Equal(t, "python", rule.Language)
}

func TestDetectLanguage_BothExist_FirstWins(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "NDArray.java"), []byte(""), 0644)
	os.MkdirAll(filepath.Join(dir, "num4py"), 0755)
	os.WriteFile(filepath.Join(dir, "num4py/ndarray.py"), []byte(""), 0644)

	rule, err := detectLanguage(dir, twoRules)
	require.NoError(t, err)
	assert.Equal(t, "java", rule.Language, "first rule should win when both files exist")
}

func TestDetectLanguage_NoMatch(t *testing.T) {
	dir := t.TempDir()

	rule, err := detectLanguage(dir, twoRules)
	assert.Nil(t, rule)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot detect language")
	assert.Contains(t, err.Error(), "NDArray.java (java)")
	assert.Contains(t, err.Error(), "num4py/ndarray.py (python)")
}

func TestDetectLanguage_EmptyRules(t *testing.T) {
	dir := t.TempDir()

	rule, err := detectLanguage(dir, nil)
	assert.Nil(t, rule)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot detect language")
}

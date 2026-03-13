package tester_context

import (
	"fmt"
	"testing"
	"time"

	"github.com/tensorhero/tester-utils/test_case_harness"
	"github.com/tensorhero/tester-utils/tester_definition"
	"github.com/stretchr/testify/assert"
)

// TestDefaultRepositoryDir 测试默认目录为当前目录
func TestDefaultRepositoryDir(t *testing.T) {
	definition := tester_definition.TesterDefinition{
		TestCases: []tester_definition.TestCase{
			{Slug: "hello", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
		},
	}

	// 不设置 TENSORHERO_REPOSITORY_DIR，应该默认为 "."
	context, err := GetTesterContext(map[string]string{}, definition)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, ".", context.SubmissionDir)
}

// TestDefaultRunAllStages 测试默认运行所有测试的模式
func TestDefaultRunAllStages(t *testing.T) {
	definition := tester_definition.TesterDefinition{
		TestCases: []tester_definition.TestCase{
			{Slug: "hello", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
			{Slug: "mario-less", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
		},
	}

	context, err := GetTesterContext(map[string]string{
		"TENSORHERO_REPOSITORY_DIR": "./test_helpers/valid_app_dir",
	}, definition)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	// 应该运行所有测试
	assert.Equal(t, 2, len(context.TestCases))
	assert.Equal(t, "hello", context.TestCases[0].Slug)
	assert.Equal(t, "stage-1", context.TestCases[0].TesterLogPrefix)
	assert.Equal(t, "Hello", context.TestCases[0].Title)
	assert.Equal(t, "mario-less", context.TestCases[1].Slug)
	assert.Equal(t, "stage-2", context.TestCases[1].TesterLogPrefix)
	assert.Equal(t, "Mario Less", context.TestCases[1].Title)
}

// TestSingleStageMode 测试指定单个 stage 的模式
func TestSingleStageMode(t *testing.T) {
	definition := tester_definition.TesterDefinition{
		TestCases: []tester_definition.TestCase{
			{Slug: "hello", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
			{Slug: "mario-less", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
		},
	}

	context, err := GetTesterContext(map[string]string{
		"TENSORHERO_REPOSITORY_DIR": "./test_helpers/valid_app_dir",
		"TENSORHERO_STAGE":          "mario-less",
	}, definition)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	// 只运行指定的 stage
	assert.Equal(t, 1, len(context.TestCases))
	assert.Equal(t, "mario-less", context.TestCases[0].Slug)
	assert.Equal(t, "stage-2", context.TestCases[0].TesterLogPrefix)
}

// TestSingleStageMode_NotFound 测试指定不存在的 stage
func TestSingleStageMode_NotFound(t *testing.T) {
	definition := tester_definition.TesterDefinition{
		TestCases: []tester_definition.TestCase{
			{Slug: "hello", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
		},
	}

	_, err := GetTesterContext(map[string]string{
		"TENSORHERO_REPOSITORY_DIR": "./test_helpers/valid_app_dir",
		"TENSORHERO_STAGE":          "nonexistent",
	}, definition)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSuccessParsingTestCases(t *testing.T) {
	context, err := GetTesterContext(map[string]string{
		"TENSORHERO_TEST_CASES_JSON": `[{ "slug": "test", "tester_log_prefix": "test", "title": "Test"}]`,
		"TENSORHERO_REPOSITORY_DIR":  "./test_helpers/valid_app_dir",
	}, tester_definition.TesterDefinition{})
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, len(context.TestCases), 1)
	assert.Equal(t, context.TestCases[0].Slug, "test")
	assert.Equal(t, context.TestCases[0].TesterLogPrefix, "test")
	assert.Equal(t, context.TestCases[0].Title, "Test")
}

// TestJSONModeTakesPrecedence 测试 JSON 模式优先级最高
func TestJSONModeTakesPrecedence(t *testing.T) {
	definition := tester_definition.TesterDefinition{
		TestCases: []tester_definition.TestCase{
			{Slug: "hello", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
			{Slug: "mario-less", Timeout: 10 * time.Second, TestFunc: func(h *test_case_harness.TestCaseHarness) error { return nil }},
		},
	}

	// 同时设置 JSON 和 STAGE，JSON 应该优先
	context, err := GetTesterContext(map[string]string{
		"TENSORHERO_TEST_CASES_JSON": `[{ "slug": "custom", "tester_log_prefix": "custom", "title": "Custom"}]`,
		"TENSORHERO_REPOSITORY_DIR":  "./test_helpers/valid_app_dir",
		"TENSORHERO_STAGE":           "hello",
	}, definition)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	assert.Equal(t, 1, len(context.TestCases))
	assert.Equal(t, "custom", context.TestCases[0].Slug)
}

func TestFormatTitle(t *testing.T) {
	tests := []struct {
		slug     string
		expected string
	}{
		{"hello", "Hello"},
		{"mario-less", "Mario Less"},
		{"mario-more", "Mario More"},
		{"build-your-own-redis", "Build Your Own Redis"},
	}

	for _, tc := range tests {
		result := formatTitle(tc.slug)
		assert.Equal(t, tc.expected, result, "slug=%s", tc.slug)
	}
}

func TestCorrectExecutable(t *testing.T) {
	tests := []struct {
		submissionDir      string
		expectedExecutable string
	}{
		{"valid_app_dir", "your_program.sh"}, // neither executables present
		{"valid_app_dir_legacy_only", "spawn_redis_server.sh"},
		{"valid_app_dir_both", "your_program.sh"},
	}

	for _, tt := range tests {
		context, err := GetTesterContext(map[string]string{
			"TENSORHERO_TEST_CASES_JSON": `[{ "slug": "test", "tester_log_prefix": "test", "title": "Test"}]`,
			"TENSORHERO_REPOSITORY_DIR":  fmt.Sprintf("./test_helpers/%s", tt.submissionDir),
		}, tester_definition.TesterDefinition{
			ExecutableFileName:       "your_program.sh",
			LegacyExecutableFileName: "spawn_redis_server.sh",
		})

		if !assert.NoError(t, err) {
			t.FailNow()
		}

		assert.Equal(t, context.ExecutablePath, fmt.Sprintf("test_helpers/%s/%s", tt.submissionDir, tt.expectedExecutable))
	}
}

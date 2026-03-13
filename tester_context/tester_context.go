package tester_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/tensorhero/tester-utils/internal"
	"github.com/tensorhero/tester-utils/tester_definition"
	"gopkg.in/yaml.v2"
)

// TesterContextTestCase represents one element in the TENSORHERO_TEST_CASES environment variable
type TesterContextTestCase struct {
	// Slug is the slug of the test case. Example: "bind-to-port"
	Slug string `json:"slug"`

	// TesterLogPrefix is the prefix that'll be used for all logs emitted by the tester. Example: "stage-1"
	TesterLogPrefix string `json:"tester_log_prefix"`

	// Title is the title of the test case. Example: "Stage #1: Bind to a port"
	Title string `json:"title"`
}

// TesterContext holds all flags passed in via environment variables, or from the tensorhero.yml file
type TesterContext struct {
	// SubmissionDir is the directory containing the student's submission
	SubmissionDir string

	// ExecutablePath is the path to the executable (may be empty if ExecutableFileName is not set)
	ExecutablePath string

	IsDebug                      bool
	TestCases                    []TesterContextTestCase
	ShouldSkipAntiCheatTestCases bool
}

type yamlConfig struct {
	Debug bool `yaml:"debug"`
}

func (c TesterContext) Print() {
	fmt.Println("Debug =", c.IsDebug)
}

// GetTesterContext parses flags and returns a Context object
// 支持三种模式：
// 1. TENSORHERO_TEST_CASES_JSON - 完整 JSON 格式（兼容 worker 调度）
// 2. TENSORHERO_STAGE - 指定单个 stage slug（调试用）
// 3. 无环境变量 - 运行所有测试（默认行为）
//
// TENSORHERO_REPOSITORY_DIR 默认为当前目录 "."
func GetTesterContext(env map[string]string, definition tester_definition.TesterDefinition) (TesterContext, error) {
	submissionDir, ok := env["TENSORHERO_REPOSITORY_DIR"]
	if !ok {
		// 默认为当前目录
		submissionDir = "."
	}

	var testCases []TesterContextTestCase
	var err error

	// 优先级：JSON > STAGE > 全部运行
	if testCasesJson, ok := env["TENSORHERO_TEST_CASES_JSON"]; ok {
		// 模式1：完整 JSON 格式（兼容 worker）
		testCases, err = parseTestCasesFromJSON(testCasesJson)
		if err != nil {
			return TesterContext{}, err
		}
	} else if stageSlug, ok := env["TENSORHERO_STAGE"]; ok {
		// 模式2：单个 stage（调试用）
		testCases, err = buildTestCasesForStage(stageSlug, definition)
		if err != nil {
			return TesterContext{}, err
		}
	} else {
		// 模式3：运行所有测试（默认）
		testCases = buildTestCasesForAll(definition)
	}

	if len(testCases) == 0 {
		return TesterContext{}, fmt.Errorf("no test cases to run")
	}

	var shouldSkipAntiCheatTestCases = false
	skipAntiCheatValue, ok := env["TENSORHERO_SKIP_ANTI_CHEAT"]
	if ok && skipAntiCheatValue == "true" {
		shouldSkipAntiCheatTestCases = true
	}

	newExecutablePath := path.Join(submissionDir, definition.ExecutableFileName)
	executablePath := newExecutablePath

	if definition.LegacyExecutableFileName != "" {
		_, newExecutablePathErr := os.Stat(newExecutablePath)
		legacyExecutablePath := path.Join(submissionDir, definition.LegacyExecutableFileName)

		_, legacyExecutablePathErr := os.Stat(legacyExecutablePath)

		// Only use legacyExecutablePath if the legacy file is present AND new file isn't
		if legacyExecutablePathErr == nil && errors.Is(newExecutablePathErr, os.ErrNotExist) {
			executablePath = legacyExecutablePath
		}
	}

	configPath := path.Join(submissionDir, "tensorhero.yml")

	yamlConfig, err := readFromYAML(configPath)
	if err != nil {
		return TesterContext{}, err
	}

	return TesterContext{
		SubmissionDir:                submissionDir,
		ExecutablePath:               executablePath,
		IsDebug:                      yamlConfig.Debug,
		TestCases:                    testCases,
		ShouldSkipAntiCheatTestCases: shouldSkipAntiCheatTestCases,
	}, nil
}

// parseTestCasesFromJSON 从 JSON 字符串解析测试用例
func parseTestCasesFromJSON(jsonStr string) ([]TesterContextTestCase, error) {
	testCases := []TesterContextTestCase{}
	if err := json.Unmarshal([]byte(jsonStr), &testCases); err != nil {
		return nil, fmt.Errorf("failed to parse TENSORHERO_TEST_CASES_JSON: %s", err)
	}

	for _, tc := range testCases {
		if tc.Slug == "" {
			return nil, fmt.Errorf("TENSORHERO_TEST_CASES_JSON contains a test case with an empty slug")
		}
		if tc.TesterLogPrefix == "" {
			return nil, fmt.Errorf("TENSORHERO_TEST_CASES_JSON contains a test case with an empty tester_log_prefix")
		}
		if tc.Title == "" {
			return nil, fmt.Errorf("TENSORHERO_TEST_CASES_JSON contains a test case with an empty title")
		}
	}

	return testCases, nil
}

// buildTestCasesForStage 为单个 stage 构建测试用例
func buildTestCasesForStage(stageSlug string, definition tester_definition.TesterDefinition) ([]TesterContextTestCase, error) {
	// 找到对应的测试用例
	for i, tc := range definition.TestCases {
		if tc.Slug == stageSlug {
			return []TesterContextTestCase{
				{
					Slug:            tc.Slug,
					TesterLogPrefix: fmt.Sprintf("stage-%d", i+1),
					Title:           formatTitle(tc.Slug),
				},
			}, nil
		}
	}
	return nil, fmt.Errorf("stage %q not found in tester definition", stageSlug)
}

// buildTestCasesForAll 为所有 stage 构建测试用例
func buildTestCasesForAll(definition tester_definition.TesterDefinition) []TesterContextTestCase {
	testCases := make([]TesterContextTestCase, 0, len(definition.TestCases))
	for i, tc := range definition.TestCases {
		testCases = append(testCases, TesterContextTestCase{
			Slug:            tc.Slug,
			TesterLogPrefix: fmt.Sprintf("stage-%d", i+1),
			Title:           formatTitle(tc.Slug),
		})
	}
	return testCases
}

// formatTitle 将 slug 转换为可读标题
// "mario-less" -> "Mario Less"
func formatTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func readFromYAML(configPath string) (yamlConfig, error) {
	c := &yamlConfig{}

	fileContents, err := os.ReadFile(configPath)
	if err != nil {
		// tensorhero.yml is optional - return default config if not found
		if os.IsNotExist(err) {
			return yamlConfig{Debug: false}, nil
		}
		return yamlConfig{}, &internal.UserError{
			Message: fmt.Sprintf("Can't read tensorhero.yml file: %v", err),
		}
	}

	if err := yaml.Unmarshal(fileContents, c); err != nil {
		return yamlConfig{}, &internal.UserError{
			Message: fmt.Sprintf("error parsing tensorhero.yml: %s", err),
		}
	}

	return *c, nil
}

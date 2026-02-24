package tester_utils

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bootlab-dev/tester-utils/executable"
	"github.com/bootlab-dev/tester-utils/internal"
	"github.com/bootlab-dev/tester-utils/logger"
	"github.com/bootlab-dev/tester-utils/random"
	"github.com/bootlab-dev/tester-utils/test_runner"
	"github.com/bootlab-dev/tester-utils/tester_context"
	"github.com/bootlab-dev/tester-utils/tester_definition"
	"github.com/fatih/color"
)

type Tester struct {
	context    tester_context.TesterContext
	definition tester_definition.TesterDefinition
}

// newTester creates a Tester based on the TesterDefinition provided
func newTester(env map[string]string, definition tester_definition.TesterDefinition) (Tester, error) {
	context, err := tester_context.GetTesterContext(env, definition)
	if err != nil {
		if userError, ok := err.(*internal.UserError); ok {
			return Tester{}, fmt.Errorf("%s", userError.Message)
		}

		return Tester{}, fmt.Errorf("BootLab internal error. Error fetching tester context: %v", err)
	}

	tester := Tester{
		context:    context,
		definition: definition,
	}

	if err := tester.validateContext(); err != nil {
		return Tester{}, fmt.Errorf("BootLab internal error. Error validating tester context: %v", err)
	}

	return tester, nil
}

// CLIArgs holds parsed command-line arguments
type CLIArgs struct {
	Stage   string // Stage slug to run (empty = run all)
	Dir     string // Working directory (empty = current dir)
	Help    bool   // Show help
	Version bool   // Show version
}

// ParseArgs parses command-line arguments
// Supports:
//   - ./tester [stage]           # positional argument
//   - ./tester --stage <slug>    # flag
//   - ./tester -d <dir>          # specify directory
func ParseArgs(args []string) CLIArgs {
	result := CLIArgs{}

	// Create a new FlagSet to avoid global state
	fs := flag.NewFlagSet("tester", flag.ContinueOnError)
	fs.StringVar(&result.Stage, "stage", "", "Stage slug to run")
	fs.StringVar(&result.Stage, "s", "", "Stage slug to run (shorthand)")
	fs.StringVar(&result.Dir, "dir", "", "Working directory")
	fs.StringVar(&result.Dir, "d", "", "Working directory (shorthand)")
	fs.BoolVar(&result.Help, "help", false, "Show help")
	fs.BoolVar(&result.Help, "h", false, "Show help (shorthand)")
	fs.BoolVar(&result.Version, "version", false, "Show version")
	fs.BoolVar(&result.Version, "v", false, "Show version (shorthand)")

	// Parse flags (ignore errors for unknown flags)
	fs.Parse(args)

	// If no --stage flag but there's a positional argument, use it as stage
	if result.Stage == "" && fs.NArg() > 0 {
		result.Stage = fs.Arg(0)
	}

	return result
}

// MergeArgsIntoEnv merges CLI args into env map (CLI args take precedence)
func MergeArgsIntoEnv(args CLIArgs, env map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range env {
		result[k] = v
	}

	if args.Stage != "" {
		result["BOOTLAB_STAGE"] = args.Stage
	}
	if args.Dir != "" {
		result["BOOTLAB_REPOSITORY_DIR"] = args.Dir
	}

	return result
}

// Run executes the tester with command-line arguments and environment
// This is the recommended entry point for tester main functions
//
// Usage:
//
//	os.Exit(tester_utils.Run(os.Args[1:], definition))
func Run(args []string, definition tester_definition.TesterDefinition) int {
	// Configure streaming logs if enabled by Worker
	// When BOOTLAB_STREAM_LOGS=1, redirect stdout to stderr and disable colors
	// This allows Worker to capture all logs through stderr for real-time streaming
	if os.Getenv("BOOTLAB_STREAM_LOGS") == "1" {
		os.Stdout = os.Stderr // Redirect stdout to stderr
		color.NoColor = true  // Disable ANSI color codes
	}

	cliArgs := ParseArgs(args)

	if cliArgs.Help {
		printUsage(definition)
		return 0
	}

	if cliArgs.Version {
		fmt.Println("bcs100x-tester v0.1.0")
		return 0
	}

	// Merge CLI args into environment (CLI takes precedence)
	env := getEnvMap()
	env = MergeArgsIntoEnv(cliArgs, env)

	return RunCLI(env, definition)
}

// getEnvMap converts os.Environ() to a map
func getEnvMap() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			env[pair[0]] = pair[1]
		}
	}
	return env
}

// printUsage prints help message
func printUsage(definition tester_definition.TesterDefinition) {
	fmt.Println("Usage: tester [options] [stage]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -s, --stage <slug>  Run a specific stage")
	fmt.Println("  -d, --dir <path>    Set working directory (default: current dir)")
	fmt.Println("  -h, --help          Show this help message")
	fmt.Println("  -v, --version       Show version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  tester              # Run all stages")
	fmt.Println("  tester hello        # Run 'hello' stage")
	fmt.Println("  tester -s hello     # Same as above")
	fmt.Println()
	fmt.Println("Available stages:")
	for _, tc := range definition.TestCases {
		fmt.Printf("  %s\n", tc.Slug)
	}
}

// RunCLI executes the tester based on user-provided env vars
// Deprecated: Use Run() instead for command-line argument support
func RunCLI(env map[string]string, definition tester_definition.TesterDefinition) int {
	random.Init()

	tester, err := newTester(env, definition)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}

	tester.printDebugContext()

	// TODO: Validate context here instead of in NewTester?

	if !tester.runStages() {
		return 1
	}

	if !tester.context.ShouldSkipAntiCheatTestCases && !tester.runAntiCheatStages() {
		return 1
	}

	return 0
}

// PrintDebugContext is to be run as early as possible after creating a Tester
func (tester Tester) printDebugContext() {
	if !tester.context.IsDebug {
		return
	}

	tester.context.Print()
	fmt.Println("")
}

// runAntiCheatStages runs any anti-cheat stages specified in the TesterDefinition. Only critical logs are emitted. If
// the stages pass, the user won't see any visible output.
func (tester Tester) runAntiCheatStages() bool {
	return tester.getAntiCheatRunner().Run(false, tester.getQuietExecutable())
}

// runStages runs all the stages upto the current stage the user is attempting. Returns true if all stages pass.
func (tester Tester) runStages() bool {
	return tester.getRunner().Run(tester.context.IsDebug, tester.getExecutable())
}

func (tester Tester) getRunner() test_runner.TestRunner {
	steps := []test_runner.TestRunnerStep{}

	for _, testerContextTestCase := range tester.context.TestCases {
		definitionTestCase := tester.definition.TestCaseBySlug(testerContextTestCase.Slug)

		steps = append(steps, test_runner.TestRunnerStep{
			TestCase:        definitionTestCase,
			TesterLogPrefix: testerContextTestCase.TesterLogPrefix,
			Title:           testerContextTestCase.Title,
		})
	}

	return test_runner.NewTestRunner(steps, tester.context.SubmissionDir)
}

func (tester Tester) getAntiCheatRunner() test_runner.TestRunner {
	steps := []test_runner.TestRunnerStep{}

	for index, testCase := range tester.definition.AntiCheatTestCases {
		steps = append(steps, test_runner.TestRunnerStep{
			TestCase:        testCase,
			TesterLogPrefix: fmt.Sprintf("ac-%d", index+1),
			Title:           fmt.Sprintf("AC%d", index+1),
		})
	}

	return test_runner.NewQuietTestRunner(steps, tester.context.SubmissionDir) // We only want Critical logs to be emitted for anti-cheat tests
}

func (tester Tester) getQuietExecutable() *executable.Executable {
	return executable.NewExecutable(tester.context.ExecutablePath)
}

func (tester Tester) getExecutable() *executable.Executable {
	return executable.NewVerboseExecutable(tester.context.ExecutablePath, logger.GetLogger(true, "[your_program] ").Plainln)
}

func (tester Tester) validateContext() error {
	for _, testerContextTestCase := range tester.context.TestCases {
		testerDefinitionTestCase := tester.definition.TestCaseBySlug(testerContextTestCase.Slug)

		if testerDefinitionTestCase.Slug != testerContextTestCase.Slug {
			return fmt.Errorf("tester context does not have test case with slug %s", testerContextTestCase.Slug)
		}
	}

	return nil
}

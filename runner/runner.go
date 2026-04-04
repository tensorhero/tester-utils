package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bootcraft-cn/tester-utils/executable"
	"github.com/bootcraft-cn/tester-utils/logger"
)

// Runner 提供类似 check50 的链式 API 来运行和测试程序
// 支持两种模式:
// 1. 阻塞模式: Stdin("input") 发送输入并等待程序结束
// 2. 交互模式: Start() 启动程序，SendLine() 发送输入，Reject() 检查程序是否拒绝输入
//
// 用法示例:
//
//	// 阻塞模式
//	runner.Run("./mario").Stdin("4").Stdout(expected).Exit(0)
//
//	// 交互模式 (用于测试输入拒绝)
//	runner.Run("./mario").Start().SendLine("-1").Reject().SendLine("4").Stdout(expected).Exit(0)
type Runner struct {
	workDir    string
	command    string
	args       []string
	env        []string
	timeout    time.Duration
	usePty     bool
	logger     *logger.Logger
	result     *executable.ExecutableResult
	err        error
	executable *executable.Executable
	started    bool
	stdout     *bytes.Buffer // 用于交互模式收集输出
}

// Run 创建一个新的 Runner 实例
func Run(workDir string, command string, args ...string) *Runner {
	return &Runner{
		workDir: workDir,
		command: command,
		args:    args,
		timeout: 10 * time.Second,
		usePty:  false,
		stdout:  bytes.NewBuffer(nil),
	}
}

// WithLogger 设置 logger
func (r *Runner) WithLogger(l *logger.Logger) *Runner {
	r.logger = l
	return r
}

// WithTimeout 设置超时时间
func (r *Runner) WithTimeout(t time.Duration) *Runner {
	r.timeout = t
	return r
}

// WithEnv 设置环境变量
func (r *Runner) WithEnv(env ...string) *Runner {
	r.env = append(r.env, env...)
	return r
}

// WithPty 启用 PTY 模式（用于交互式测试）
func (r *Runner) WithPty() *Runner {
	r.usePty = true
	return r
}

// createExecutable 创建并配置 executable
func (r *Runner) createExecutable() *executable.Executable {
	cmdPath := r.command

	// 判断是否为本地可执行文件
	// 1. 包含路径分隔符（如 ./hello, ../bin/test, path/to/file）
	// 2. 或者是绝对路径
	// 3. 或者在 workDir 中存在同名文件
	isLocalExecutable := strings.HasPrefix(cmdPath, "./") ||
		strings.HasPrefix(cmdPath, "../") ||
		strings.Contains(cmdPath, "/") ||
		filepath.IsAbs(cmdPath) ||
		fileExistsInDir(r.workDir, cmdPath)

	var fullPath string
	if isLocalExecutable {
		// 本地可执行文件：拼接 workDir
		if !filepath.IsAbs(cmdPath) && !strings.HasPrefix(cmdPath, "./") {
			cmdPath = "./" + cmdPath
		}
		fullPath = filepath.Join(r.workDir, cmdPath)
	} else {
		// 系统命令（如 python3, bash, clang）：直接传给 executable
		// executable.resolveAbsolutePath 会通过 exec.LookPath 查找
		fullPath = cmdPath
	}

	e := executable.NewExecutable(fullPath)
	e.WorkingDir = r.workDir
	e.TimeoutInMilliseconds = int(r.timeout.Milliseconds())
	e.ShouldUsePty = r.usePty

	return e
}

// fileExistsInDir 检查文件是否存在于指定目录中
func fileExistsInDir(dir, name string) bool {
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Start 启动程序但不等待结束（交互模式）
func (r *Runner) Start() *Runner {
	if r.err != nil {
		return r
	}

	if r.logger != nil {
		r.logger.Debugf("starting %s...", r.command)
	}

	r.executable = r.createExecutable()
	err := r.executable.Start(r.args...)
	if err != nil {
		r.err = err
		return r
	}

	r.started = true
	return r
}

// SendLine 发送一行输入（交互模式，需要先调用 Start）
func (r *Runner) SendLine(input string) *Runner {
	if r.err != nil {
		return r
	}

	if !r.started {
		r.err = fmt.Errorf("program not started, call Start() first")
		return r
	}

	if r.logger != nil {
		r.logger.Debugf("sending input %q...", input)
	}

	// 使用 executable 的 SendLine 方法发送输入
	if err := r.executable.SendLine(input); err != nil {
		r.err = fmt.Errorf("failed to send input: %v", err)
	}

	return r
}

// Reject 检查程序是否拒绝输入（继续等待而不是退出）
// 类似 check50 的 reject()，检查程序在收到输入后是否继续运行等待更多输入
func (r *Runner) Reject(rejectTimeout ...time.Duration) *Runner {
	if r.err != nil {
		return r
	}

	timeout := 1 * time.Second
	if len(rejectTimeout) > 0 {
		timeout = rejectTimeout[0]
	}

	if r.logger != nil {
		r.logger.Debugf("checking that input was rejected (waiting %v)...", timeout)
	}

	// 检查程序是否在 timeout 内还活着
	if r.executable != nil && r.started {
		// 轮询检查程序是否退出
		checkInterval := 50 * time.Millisecond
		elapsed := time.Duration(0)

		for elapsed < timeout {
			if r.executable.HasExited() {
				r.err = &RejectError{
					Message: "expected program to reject input and wait for more, but it exited",
				}
				return r
			}
			time.Sleep(checkInterval)
			elapsed += checkInterval
		}
		// 程序仍在运行，这是期望的行为（输入被拒绝，程序等待更多输入）
	}

	return r
}

// Stdin 发送输入并运行程序（阻塞式）
func (r *Runner) Stdin(input string) *Runner {
	if r.err != nil {
		return r
	}

	if r.logger != nil {
		r.logger.Debugf("sending input %q...", input)
	}

	r.executable = r.createExecutable()

	// 运行程序
	result, err := r.executable.RunWithStdin([]byte(input+"\n"), r.args...)
	r.result = &result
	if err != nil && err.Error() != "execution timed out" {
		r.err = err
	}

	return r
}

// Execute 不带输入运行程序
func (r *Runner) Execute() *Runner {
	if r.err != nil {
		return r
	}

	r.executable = r.createExecutable()

	result, err := r.executable.Run(r.args...)
	r.result = &result
	r.err = err

	return r
}

// WaitForExit 等待程序结束（交互模式）
func (r *Runner) WaitForExit() *Runner {
	if r.err != nil {
		return r
	}

	if r.executable != nil && r.started {
		result, err := r.executable.Wait()
		r.result = &result
		if err != nil && err.Error() != "execution timed out" {
			r.err = err
		}
		r.started = false
	}

	return r
}

// Kill 终止程序
func (r *Runner) Kill() *Runner {
	if r.executable != nil && r.started {
		r.executable.Kill()
		r.started = false
	}
	return r
}

// readAvailableOutput 读取当前可用的输出（非阻塞）
func (r *Runner) readAvailableOutput(reader io.Reader, timeout time.Duration) string {
	buf := make([]byte, 4096)
	done := make(chan int, 1)

	go func() {
		n, _ := reader.Read(buf)
		done <- n
	}()

	select {
	case n := <-done:
		return string(buf[:n])
	case <-time.After(timeout):
		return ""
	}
}

// Stdout 检查标准输出是否包含期望内容
func (r *Runner) Stdout(expected string) *Runner {
	if r.err != nil {
		return r
	}
	if r.result == nil {
		r.err = fmt.Errorf("program not yet executed")
		return r
	}

	actual := normalizeOutput(string(r.result.Stdout))

	if expected != "" {
		if !strings.Contains(actual, expected) {
			r.err = &Mismatch{
				Expected: expected,
				Actual:   actual,
				Message:  fmt.Sprintf("expected output to contain %q", expected),
			}
		}
	}

	return r
}

// StdoutRegex 使用正则表达式检查标准输出
func (r *Runner) StdoutRegex(pattern string) *Runner {
	if r.err != nil {
		return r
	}
	if r.result == nil {
		r.err = fmt.Errorf("program not yet executed")
		return r
	}

	actual := normalizeOutput(string(r.result.Stdout))
	re, err := regexp.Compile(pattern)
	if err != nil {
		r.err = fmt.Errorf("invalid regex pattern: %v", err)
		return r
	}

	if !re.MatchString(actual) {
		r.err = &Mismatch{
			Expected: pattern,
			Actual:   actual,
			Message:  fmt.Sprintf("expected output to match pattern %q", pattern),
		}
	}

	return r
}

// StdoutExact 检查标准输出是否完全匹配
func (r *Runner) StdoutExact(expected string) *Runner {
	if r.err != nil {
		return r
	}
	if r.result == nil {
		r.err = fmt.Errorf("program not yet executed")
		return r
	}

	actual := strings.TrimSpace(normalizeOutput(string(r.result.Stdout)))
	expected = strings.TrimSpace(expected)

	if actual != expected {
		r.err = &Mismatch{
			Expected: expected,
			Actual:   actual,
			Message:  "output mismatch",
		}
	}

	return r
}

// normalizeOutput 标准化输出（移除 PTY 的 \r\n 转换为 \n）
func normalizeOutput(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// Exit 检查退出码
func (r *Runner) Exit(code int) *Runner {
	if r.err != nil {
		return r
	}
	if r.result == nil {
		r.err = fmt.Errorf("program not yet executed")
		return r
	}

	if r.result.ExitCode != code {
		r.err = &ExitCodeMismatch{
			Expected: code,
			Actual:   r.result.ExitCode,
			Stdout:   normalizeOutput(string(r.result.Stdout)),
			Stderr:   normalizeOutput(string(r.result.Stderr)),
		}
	}

	return r
}

// Error 返回链式调用中累积的错误
func (r *Runner) Error() error {
	return r.err
}

// Result 返回执行结果
func (r *Runner) Result() *executable.ExecutableResult {
	return r.result
}

// GetStdout 返回标准输出内容
func (r *Runner) GetStdout() string {
	if r.result == nil {
		return ""
	}
	return normalizeOutput(string(r.result.Stdout))
}

// RejectError 表示程序未能正确拒绝无效输入
type RejectError struct {
	Message string
}

func (e *RejectError) Error() string {
	return e.Message
}

// Mismatch 表示期望值与实际值不匹配
type Mismatch struct {
	Expected string
	Actual   string
	Message  string
	Help     string
}

func (m *Mismatch) Error() string {
	if m.Message != "" {
		return m.Message
	}
	return fmt.Sprintf("expected %q, got %q", m.Expected, m.Actual)
}

// ExitCodeMismatch 表示退出码不匹配
type ExitCodeMismatch struct {
	Expected int
	Actual   int
	Stdout   string
	Stderr   string
}

func (e *ExitCodeMismatch) Error() string {
	msg := fmt.Sprintf("expected exit code %d, got %d", e.Expected, e.Actual)
	if e.Stderr != "" {
		msg += fmt.Sprintf("\nStderr: %s", e.Stderr)
	}
	return msg
}

// CompileC 编译 C 文件
func CompileC(workDir, source, output string, flags ...string) error {
	args := []string{"-o", output, source}
	args = append(args, flags...)

	cmd := exec.Command("clang", args...)
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return &CompileError{
			Source: source,
			Output: string(out),
			Err:    err,
		}
	}
	return nil
}

// CompileError 表示编译错误
type CompileError struct {
	Source string
	Output string
	Err    error
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("failed to compile %s: %s\n%s", e.Source, e.Err, e.Output)
}

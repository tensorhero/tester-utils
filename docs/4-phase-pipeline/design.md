# tester-utils 改进设计文档

> 版本: v1.1 | 日期: 2026-02-25 | 状态: 草案

## 1. 设计概览

### 1.1 核心思想

将 TestCase 的执行从单一的 `TestFunc` 扩展为 **4 阶段管道**：

```
┌─ Phase 1 ──────┐   ┌─ Phase 2 ────┐   ┌─ Phase 3 ─────┐   ┌─ Phase 4 ──┐
│ RequiredFiles   │ → │ CompileStep  │ → │ BeforeFunc    │ → │ TestFunc   │
│ (声明式文件检查) │   │ (声明式编译)  │   │ (自定义 Hook) │   │ (实际测试)  │
└─────────────────┘   └──────────────┘   └───────────────┘   └────────────┘
        ↓ 失败                ↓ 失败              ↓ 失败           ↓ 失败/成功
    跳过后续阶段          跳过后续阶段        跳过后续阶段        ── Teardown ──→
```

**任何阶段失败 → 跳过后续阶段 → 执行 TeardownFuncs → 报告错误。**

### 1.2 设计原则

1. **声明优于命令** — 能用结构体字段声明的，不要求写代码
2. **零值即不执行** — 所有新字段可选，`nil`/空切片 = 跳过该阶段
3. **向后兼容** — 不改现有 API 签名，不破坏现有 tester

## 2. 数据结构变更

### 2.1 tester_definition.go

```go
package tester_definition

import (
    "time"
    "github.com/tensorhero-cn/tester-utils/test_case_harness"
)

// CompileStep 声明编译步骤
type CompileStep struct {
    // Language 编译语言: "c", "make"
    // "c" → 调用 clang
    // "make" → 调用 make <Target>
    Language string

    // Source 源文件（Language="c" 时必填）
    Source string

    // Output 编译输出目标。
    // Language="c" 时：输出二进制文件名（如 "hello"），编译产物写入 {SubmissionDir}/{Output}。
    // Language="make" 时：make target 名称（如 "speller"），等价于 `make {Output}`。
    Output string

    // Flags 额外编译参数（追加到默认 flags 之后，不是覆盖）
    Flags []string

    // IncludeParentDir 是否添加 -I.. 引入父目录（用于 tensorhero.h）
    IncludeParentDir bool
}

// TestCase represents a test case that'll be run against the user's code.
type TestCase struct {
    // Slug is the unique identifier for this test case.
    Slug string

    // TestFunc is the function that'll be run against the user's code.
    TestFunc func(testCaseHarness *test_case_harness.TestCaseHarness) error

    // Timeout is the maximum amount of time that the test case can run for.
    Timeout time.Duration

    // === 新增字段 ===

    // RequiredFiles 声明该 stage 必须存在的提交文件。
    // 框架在 TestFunc 之前自动检查，任一缺失则报错并跳过 TestFunc。
    // 零值（nil/空）= 跳过检查。
    RequiredFiles []string

    // CompileStep 声明编译步骤。
    // 框架在文件检查之后、TestFunc 之前自动执行编译。
    // nil = 跳过编译。
    CompileStep *CompileStep

    // BeforeFunc 自定义前置校验函数。
    // 在声明式检查（RequiredFiles、CompileStep）之后、TestFunc 之前执行。
    // nil = 跳过。返回 error 则跳过 TestFunc。
    BeforeFunc func(testCaseHarness *test_case_harness.TestCaseHarness) error
}

// TesterDefinition 不变，保持向后兼容
type TesterDefinition struct {
    ExecutableFileName       string
    LegacyExecutableFileName string
    TestCases                []TestCase
    AntiCheatTestCases       []TestCase
}
```

### 2.2 字段设计决策

| 决策点 | 选择 | 理由 |
|-------|------|------|
| RequiredFiles 类型 | `[]string` | 简单直接，文件名列表，不需要 glob |
| CompileStep 类型 | `*CompileStep`（指针） | nil 表示跳过，值类型无法区分零值 |
| BeforeFunc 参数 | `*TestCaseHarness` | 与 TestFunc 共享同一实例 |
| CompileStep.Language | 字符串枚举 | 可扩展，比 iota 更可读 |
| CompileStep.Flags | `[]string`（可选） | 默认 flags 内置，Flags 为追加（append）而非覆盖 |

## 3. TestRunner 执行流改造

### 3.1 test_runner.go Run() 方法

```go
func (r TestRunner) Run(isDebug bool, executable *executable.Executable) bool {
    for index, step := range r.steps {
        if index != 0 {
            fmt.Println("")
        }

        testCaseHarness := test_case_harness.TestCaseHarness{
            Logger:        r.getLoggerForStep(isDebug, step),
            SubmissionDir: r.submissionDir,
            Executable:    executable.Clone(),
        }

        logger := testCaseHarness.Logger
        logger.Infof("Running tests for %s", step.Title)

        // ========== Phase 1: RequiredFiles ==========
        if len(step.TestCase.RequiredFiles) > 0 {
            if err := r.checkRequiredFiles(&testCaseHarness, step.TestCase.RequiredFiles); err != nil {
                r.reportTestError(err, isDebug, logger)
                testCaseHarness.RunTeardownFuncs()
                return false
            }
        }

        // ========== Phase 2: CompileStep（带 30s 超时） ==========
        if step.TestCase.CompileStep != nil {
            if err := r.runCompileStepWithTimeout(&testCaseHarness, step.TestCase.CompileStep, 30*time.Second); err != nil {
                r.reportTestError(err, isDebug, logger)
                testCaseHarness.RunTeardownFuncs()
                return false
            }
        }

        // ========== Phase 3: BeforeFunc（带 recover 防 panic） ==========
        if step.TestCase.BeforeFunc != nil {
            if err := r.safeRunBeforeFunc(&testCaseHarness, step.TestCase.BeforeFunc); err != nil {
                r.reportTestError(err, isDebug, logger)
                testCaseHarness.RunTeardownFuncs()
                return false
            }
        }

        // ========== Phase 4: TestFunc ==========
        stepResultChannel := make(chan error, 1)
        go func() {
            err := step.TestCase.TestFunc(&testCaseHarness)
            stepResultChannel <- err
        }()

        timeout := step.TestCase.CustomOrDefaultTimeout()

        var err error
        select {
        case stageErr := <-stepResultChannel:
            err = stageErr
        case <-time.After(timeout):
            err = fmt.Errorf("timed out, test exceeded %d seconds", int64(timeout.Seconds()))
        }

        if err != nil {
            r.reportTestError(err, isDebug, logger)
        } else {
            logger.Successf("Test passed.")
        }

        testCaseHarness.RunTeardownFuncs()

        if err != nil {
            return false
        }
    }

    return true
}
```

### 3.2 checkRequiredFiles 实现

```go
// checkRequiredFiles 检查所有必需文件是否存在
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
```

### 3.3 runCompileStep 实现

```go
// runCompileStep 执行编译步骤
func (r TestRunner) runCompileStep(harness *test_case_harness.TestCaseHarness, cs *CompileStep) error {
    logger := harness.Logger
    workDir := harness.SubmissionDir

    switch cs.Language {
    case "c":
        logger.Infof("Compiling %s...", cs.Source)
        if err := compileC(workDir, cs); err != nil {
            return fmt.Errorf("%s does not compile: %v", cs.Source, err)
        }
        logger.Successf("%s compiles", cs.Source)
        return nil

    case "make":
        logger.Infof("Running make %s...", cs.Output)
        cmd := exec.Command("make", cs.Output)
        cmd.Dir = workDir
        if out, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("make %s failed: %s\n%s", cs.Output, err, string(out))
        }
        logger.Successf("make %s succeeds", cs.Output)
        return nil

    default:
        return fmt.Errorf("unsupported compile language: %s", cs.Language)
    }
}

// runCompileStepWithTimeout 带超时保护的编译
func (r TestRunner) runCompileStepWithTimeout(harness *test_case_harness.TestCaseHarness, cs *CompileStep, timeout time.Duration) error {
    done := make(chan error, 1)
    go func() {
        done <- r.runCompileStep(harness, cs)
    }()
    select {
    case err := <-done:
        return err
    case <-time.After(timeout):
        return fmt.Errorf("compilation timed out after %v", timeout)
    }
}

// safeRunBeforeFunc 带 recover 的 BeforeFunc 执行，防止 panic 传播
func (r TestRunner) safeRunBeforeFunc(harness *test_case_harness.TestCaseHarness, fn func(*test_case_harness.TestCaseHarness) error) (err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("BeforeFunc panicked: %v", r)
        }
    }()
    return fn(harness)
}

// compileC 编译 C 文件
// 注意：tester-utils/runner 包中已有 runner.CompileC()（无默认 flags）。
// 本函数封装了默认 flags（-lm -Wall -Werror）和 IncludeParentDir 逻辑，
// 适用于框架层自动编译。复杂 stage 仍可在 BeforeFunc 中直接调用 runner.CompileC()。
func compileC(workDir string, cs *CompileStep) error {
    args := []string{"-o", cs.Output, cs.Source, "-lm", "-Wall", "-Werror"}
    args = append(args, cs.Flags...)
    if cs.IncludeParentDir {
        args = append(args, "-I..")
    }

    cmd := exec.Command("clang", args...)
    cmd.Dir = workDir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("%s\nOutput:\n%s", err, string(out))
    }
    return nil
}
```

## 4. llm100x-tester 迁移示例

### 4.1 hello.go：改造前后对比

#### 编译产物衔接机制

CompileStep 将编译输出写入 `{SubmissionDir}/{CompileStep.Output}`（如 `/work/hello/hello`）。
TestFunc 通过 `runner.Run(harness.SubmissionDir, "hello")` 引用——`runner.Run` 检测到文件存在于 workDir 下时自动补全路径为 `./hello`。
这是一个隐式约定：**CompileStep.Output 与 runner.Run 的 command 参数必须一致**。

**改造前（45 行）：**

```go
func testHello(harness *test_case_harness.TestCaseHarness) error {
    logger := harness.Logger
    workDir := harness.SubmissionDir

    // ---- 以下为样板代码（框架应自动处理）----
    logger.Infof("Checking hello.c exists...")
    if !harness.FileExists("hello.c") {
        return fmt.Errorf("hello.c does not exist")
    }
    logger.Successf("hello.c exists")

    logger.Infof("Compiling hello.c...")
    if err := helpers.CompileC(workDir, "hello.c", "hello", true); err != nil {
        return fmt.Errorf("hello.c does not compile: %v", err)
    }
    logger.Successf("hello.c compiles")
    // ---- 样板代码结束 ----

    // 真正的测试逻辑
    testCases := []struct{ name, expected string }{
        {"Emma", "Emma"}, {"Rodrigo", "Rodrigo"},
    }
    for _, tc := range testCases {
        r := runner.Run(workDir, "hello").Stdin(tc.name).Stdout(tc.expected).Exit(0)
        if err := r.Error(); err != nil { return err }
    }
    return nil
}
```

**改造后（25 行）：**

```go
func helloTestCase() tester_definition.TestCase {
    return tester_definition.TestCase{
        Slug:          "hello",
        Timeout:       30 * time.Second,
        RequiredFiles: []string{"hello.c"},                    // 框架自动检查
        CompileStep: &tester_definition.CompileStep{           // 框架自动编译
            Language: "c", Source: "hello.c", Output: "hello",
            IncludeParentDir: true,
        },
        TestFunc: testHello,  // 只剩纯测试逻辑
    }
}

func testHello(harness *test_case_harness.TestCaseHarness) error {
    testCases := []struct{ name, expected string }{
        {"Emma", "Emma"}, {"Rodrigo", "Rodrigo"},
    }
    for _, tc := range testCases {
        r := runner.Run(harness.SubmissionDir, "hello").
            WithTimeout(5 * time.Second).
            Stdin(tc.name).Stdout(tc.expected).Exit(0)
        if err := r.Error(); err != nil {
            return fmt.Errorf("test failed for input %q: %v", tc.name, err)
        }
    }
    return nil
}
```

### 4.2 各类 Stage 迁移模式

| Stage 类型 | RequiredFiles | CompileStep | BeforeFunc | 示例 |
|-----------|---------------|-------------|------------|------|
| **简单 C** | `["hello.c"]` | `{Language:"c", Source:"hello.c", Output:"hello"}` | nil | hello, cash, credit |
| **C + tensorhero.h** | `["mario.c"]` | `{Language:"c", ..., IncludeParentDir:true}` | nil | mario-less/more, scrabble |
| **C + 多文件** | `["helpers.c","bmp.h","helpers.h","testing.c"]` | `{Language:"c", Source:"testing.c helpers.c", ...}` | 用 BeforeFunc 自定义编译 | filter-less/more |
| **C + make** | `["dictionary.c"]` | `{Language:"make", Output:"speller"}` | nil | speller |
| **Python** | `["hello.py"]` | nil（不需要编译） | nil | sentimental-* |
| **SQL** | `["1.sql","2.sql",...,"7.sql"]` | nil | nil | songs, movies |
| **Flask** | `["app.py"]` | nil | `beforeFinance`（venv 探测、服务启动） | finance |
| **文本** | `["answers.txt"]` | nil | nil | sort |
| **C + 测试文件组装** | `["inheritance.c"]` | nil | `beforeInheritance`（main重命名+组合编译） | inheritance |

### 4.3 复杂 stage 用 BeforeFunc

```go
// finance: 需要 venv 探测 + Flask 服务启动
func financeTestCase() tester_definition.TestCase {
    return tester_definition.TestCase{
        Slug:          "finance",
        Timeout:       120 * time.Second,
        RequiredFiles: []string{"app.py"},
        BeforeFunc:    beforeFinance,      // 自定义前置
        TestFunc:      testFinance,
    }
}

func beforeFinance(harness *test_case_harness.TestCaseHarness) error {
    // venv 探测、环境准备等
    // ...
    return nil
}
```

## 5. files_config 消费路径设计

### 5.1 P1-A 阶段：硬编码对齐

将 stage.yml 的 `files_config.required` 硬编码为 `TestCase.RequiredFiles`：

```
stage.yml                          tester 代码
─────────                          ──────────
files_config:                      RequiredFiles: []string{
  required: [caesar.c]       →         "caesar.c",
  allowed: ["*.h"]                 }
  blocked: ["*.py"]
```

验证手段：CI 中跑 `ValidateTesterDefinitionAgainstYAML` 的扩展版本，
检查 `TestCase.RequiredFiles` 与对应 `stage.yml` 的 `files_config.required` 一致。

### 5.2 P1-B 阶段：数据驱动（后续）

```
Worker                          Tester 容器
──────                          ──────────
docker run ... \
  -e TENSORHERO_FILES_CONFIG='     tester-utils 解析 JSON
    {"required":["caesar.c"],   → Phase 0: 自动校验
     "allowed":["*.h"],             required: 文件存在检查
     "blocked":["*.py"]}'          allowed: glob 白名单
                                   blocked: glob 黑名单
```

框架在 `TestRunner.Run()` 最前面增加 Phase 0：

```go
// Phase 0: 环境变量驱动的 files_config 校验
if filesConfigJSON := os.Getenv("TENSORHERO_FILES_CONFIG"); filesConfigJSON != "" {
    if err := r.validateFilesConfig(filesConfigJSON); err != nil {
        return false
    }
}
```

**P1-A 和 P1-B 的优先级与合并策略：**

| 场景 | Phase 0（数据驱动） | Phase 1（硬编码） | 行为 |
|------|-------------------|-------------------|------|
| 仅 P1-A（无 env var） | 跳过 | 执行 RequiredFiles | 当前阶段 |
| 仅 P1-B（有 env var） | 执行 required + allowed + blocked | 跳过 | 后续阶段 |
| P1-A + P1-B 共存 | 执行（以 env var 为准） | **跳过** | env var 优先 |

**原则**：`TENSORHERO_FILES_CONFIG` 环境变量一旦存在，其 `required` 字段 **取代** `TestCase.RequiredFiles`（而非合并），因为数据源应该是 single source of truth。Phase 1 的 RequiredFiles 仅作为 env var 不存在时的回退。

## 6. 文件组织

```
tensorhero-tester-utils/
├── tester_definition/
│   └── tester_definition.go    # 修改: 增加 RequiredFiles, CompileStep, BeforeFunc
├── test_runner/
│   ├── test_runner.go          # 修改: Run() 增加 Phase 1-3
│   ├── compile.go              # 新增: compileC(), runCompileStep(), runCompileStepWithTimeout()
│   ├── prechecks.go            # 新增: checkRequiredFiles()
│   ├── safeguards.go           # 新增: safeRunBeforeFunc()（defer/recover）
│   └── test_runner_test.go     # 新建: Phase 1-4 前置校验 + 兼容性测试
├── test_case_harness/
│   └── test_case_harness.go    # 不变
└── docs/
    ├── requirements.md
    ├── design.md
    └── todo.md
```

## 7. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| CompileStep 覆盖不了所有编译场景 | filter-less 需要复杂 clang flags | 用 BeforeFunc 处理，CompileStep 覆盖 80%；复杂场景在 BeforeFunc 中调用 `runner.CompileC()` |
| RequiredFiles 与 stage.yml 不同步 | 硬编码可能过时 | CI 验证脚本检查一致性（T3.1） |
| Phase 1-3 增加执行开销 | 文件检查 < 1ms，编译是必须的开销 | 无实际影响 |
| CompileStep hang（#include 循环、磁盘问题） | 编译永不返回 | `runCompileStepWithTimeout` 强制 30s 超时 |
| BeforeFunc panic | 导致整个进程崩溃（Phase 3 在主 goroutine，不受 TestFunc timeout 保护） | `safeRunBeforeFunc` 用 `defer/recover` 捕获 panic 并转为 error |
| Quiet 模式下日志泄露 | anti-cheat 测试不应暴露信息 | Phase 1-3 使用 `harness.Logger`，已通过 `getLoggerForStep` 自动适配 quiet 模式 |

### 7.1 与已有 runner.CompileC 的关系

`runner/runner.go` 中已有 `runner.CompileC(workDir, source, output, flags...)`（无默认 flags，返回 `*CompileError`）。
本设计新增的 `test_runner/compile.go` 中 `compileC()` 封装了默认 flags（`-lm -Wall -Werror`）和 `IncludeParentDir` 逻辑。

**共存策略：**
- 简单 C stage → 声明 `CompileStep`，框架调用内置 `compileC()`
- 复杂场景（如 filter-less 的特殊 flags）→ `BeforeFunc` 中直接调用 `runner.CompileC(workDir, source, output, flags...)`
- 两者不冲突，不需要合并或删除任何一个

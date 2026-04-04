# auto-lang：多语言自动检测与编译支持 — 设计文档

> 版本: v1.0 | 日期: 2026-03-14 | 状态: 草案

## 1. 设计概览

### 1.1 核心思想

在现有 4-Phase Pipeline 的 **Phase 2（CompileStep）** 中扩展语言支持，新增 `"java"`、`"python"` 和 `"auto"` 三种编译模式。`"auto"` 模式通过文件存在性检测学生提交的语言，自动分发到对应的编译/运行路径。

```
Phase 2: CompileStep
┌──────────────────────────────────────────────────────────┐
│  Language?                                                │
│  ├── "c"       → clang（现有）                            │
│  ├── "make"    → make（现有）                             │
│  ├── "java"    → javac（新增）                            │
│  ├── "python"  → py_compile（新增）                       │
│  └── "auto"    → detectLanguage() → 递归分发（新增）       │
│                                                           │
│  "auto" 检测完成后：                                       │
│     harness.DetectedLang = &DetectedLanguage{...}         │
│     ↓                                                     │
│  Phase 4: TestFunc                                        │
│     lang := harness.DetectedLang                          │
│     runner.Run(workDir, lang.RunCmd, lang.RunArgs...)     │
└──────────────────────────────────────────────────────────┘
```

### 1.2 设计原则

1. **最小改动** — 仅新增字段和 switch case，不改现有 API 签名
2. **声明优于命令** — tester 作者用结构体声明多语言规则，不写 if/else
3. **零值即不执行** — 新增字段均为可选，nil/空 = 跳过
4. **递归分发** — `"auto"` 检测完成后构造具体语言的 CompileStep，递归调用 `runCompileStep`

> **扩展性说明**：新增语言需在 `compile.go` 的 switch 中添加 case，并重新编译。这是 Go switch 的固有约束，但新增一个语言仅需 ~20 行代码，可接受。

## 2. 数据结构变更

### 2.1 新增 LanguageRule 结构体

```go
// tester_definition/tester_definition.go

// LanguageRule 定义一条语言检测规则，用于 CompileStep.Language="auto" 模式。
// 框架按 AutoDetect 列表顺序检测 DetectFile，首个匹配的规则生效。
type LanguageRule struct {
    // DetectFile 检测文件是否存在于提交目录中（相对路径）。
    // 例如 "NDArray.java"、"num4py/ndarray.py"
    DetectFile string

    // Language 匹配后使用的编译语言（"java"、"python"、"c"、"make"）。
    // 将递归传给 runCompileStep 执行实际编译。
    Language string

    // Source 该语言对应的主源文件（传给 CompileStep.Source）。
    // 为空时默认使用 DetectFile。
    Source string

    // Flags 该语言对应的编译参数（传给 CompileStep.Flags）。
    Flags []string

    // RunCmd 运行命令（如 "java"、"python3"）。
    // TestFunc 通过 harness.DetectedLang.RunCmd 获取。
    RunCmd string

    // RunArgs 运行参数（如 ["-cp", ".", "TestE01"]）。
    // TestFunc 通过 harness.DetectedLang.RunArgs 获取。
    RunArgs []string
}
```

### 2.2 CompileStep 新增 AutoDetect 字段

```go
type CompileStep struct {
    // Language 编译语言: "c", "make", "java", "python", "auto"
    //   "java"   → 调用 javac
    //   "python" → 调用 python3 -m py_compile（语法检查）
    //   "auto"   → 按 AutoDetect 规则自动检测语言
    Language string

    Source           string
    // Output 编译输出文件名（仅 "c" 模式需要，Java/Python/auto 模式忽略此字段）。
    Output           string
    Flags            []string
    IncludeParentDir bool

    // AutoDetect 自动语言检测规则列表（仅 Language="auto" 时使用）。
    // 按列表顺序检测 DetectFile，首个匹配的规则生效。
    // Language 不是 "auto" 时忽略此字段。
    AutoDetect []LanguageRule
}
```

### 2.3 TestCaseHarness 新增 DetectedLang 字段

```go
// test_case_harness/test_case_harness.go

// DetectedLanguage 存储自动检测到的语言运行时信息。
// 由框架 Phase 2 (CompileStep auto) 填充，TestFunc 消费。
type DetectedLanguage struct {
    Language string   // 检测到的语言（"java", "python" 等）
    RunCmd   string   // 运行命令（"java", "python3" 等）
    RunArgs  []string // 运行参数（如 ["-cp", ".", "TestE01"]）
}

type TestCaseHarness struct {
    Logger        *logger.Logger
    SubmissionDir string
    Executable    *executable.Executable

    // DetectedLang 是 CompileStep Language="auto" 检测到的语言信息。
    // 非 auto 模式时为 nil。TestFunc 中可用于获取 RunCmd/RunArgs。
    DetectedLang *DetectedLanguage

    teardownFuncs []func()
}
```

### 2.4 字段设计决策

| 决策点                           | 选择                          | 理由                                    |
| -------------------------------- | ----------------------------- | --------------------------------------- |
| DetectFile vs 多文件检测         | 单文件                        | 一个标志文件即可区分语言，无需复杂 glob |
| AutoDetect 顺序                  | 列表顺序，首个匹配            | 简单确定性，无歧义                      |
| RunCmd/RunArgs 放在 LanguageRule | 是                            | 编译和运行信息内聚在同一规则中          |
| DetectedLang 指针                | `*DetectedLanguage`           | nil 表示非 auto 模式，安全判空          |
| "auto" 递归分发                  | 构造具体 CompileStep 递归调用 | 复用现有 java/python/c 编译逻辑，零重复 |

## 3. 编译逻辑扩展

### 3.1 compile.go 新增 Java 编译

```go
// test_runner/compile.go

case "java":
    logger.Infof("Compiling %s...", cs.Source)
    if err := compileJava(workDir, cs); err != nil {
        return fmt.Errorf("%s does not compile: %v", cs.Source, err)
    }
    logger.Successf("%s compiles", cs.Source)
    return nil
```

```go
// compileJava 调用 javac 编译 Java 源文件。
// Source 为主源文件，Flags 中可传递额外的 .java 文件。
// 输出 .class 文件写入 workDir（-d 参数控制）。
func compileJava(workDir string, cs *tester_definition.CompileStep) error {
    args := []string{"-d", "."}
    args = append(args, cs.Flags...)
    args = append(args, cs.Source)

    cmd := exec.Command("javac", args...)
    cmd.Dir = workDir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("%s\nOutput:\n%s", err, string(out))
    }
    return nil
}
```

**设计要点**：

- `-d .`：.class 文件输出到当前目录（`cmd.Dir = workDir`），与源文件同级（便于 `java -cp .` 运行）
- `Flags` 复用：传额外 .java 文件（如 Test Driver），与 C 的 Flags 传额外 .c 文件模式一致
- `Source` 放最后：javac 参数顺序 = options + source files

### 3.2 compile.go 新增 Python 语法检查

```go
case "python":
    logger.Infof("Checking %s syntax...", cs.Source)
    if err := checkPythonSyntax(workDir, cs); err != nil {
        return fmt.Errorf("%s has syntax errors: %v", cs.Source, err)
    }
    logger.Successf("%s syntax OK", cs.Source)
    return nil
```

```go
// checkPythonSyntax 调用 python3 -m py_compile 进行语法检查。
// 不生成可执行文件，仅验证语法正确性。
func checkPythonSyntax(workDir string, cs *tester_definition.CompileStep) error {
    cmd := exec.Command("python3", "-m", "py_compile", cs.Source)
    cmd.Dir = workDir
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("%s\nOutput:\n%s", err, string(out))
    }
    return nil
}
```

**设计要点**：

- Python 是解释型语言，`py_compile` 仅做语法检查（import 错误不会报）
- 对齐 Java 的日志风格：`Checking X syntax...` → `X syntax OK`
- `Flags` 暂不使用，预留扩展

### 3.3 compile.go 新增 auto 检测分发

```go
case "auto":
    if len(cs.AutoDetect) == 0 {
        return fmt.Errorf("CompileStep Language=\"auto\" but AutoDetect is empty")
    }
    rule, err := detectLanguage(workDir, cs.AutoDetect)
    if err != nil {
        return err
    }
    logger.Infof("Detected language: %s (found %s)", rule.Language, rule.DetectFile)
    // 将检测结果存入 harness（LanguageRule → DetectedLanguage 转换）
    harness.DetectedLang = &test_case_harness.DetectedLanguage{
        Language: rule.Language,
        RunCmd:   rule.RunCmd,
        RunArgs:  rule.RunArgs,
    }
    // 构造具体语言的 CompileStep，递归分发
    resolved := &tester_definition.CompileStep{
        Language: rule.Language,
        Source:   rule.Source,
        Flags:    rule.Flags,
        Output:   cs.Output,
    }
    return r.runCompileStep(harness, resolved)
```

### 3.4 新建 language.go — 语言检测逻辑

```go
// test_runner/language.go

package test_runner

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/bootcraft-cn/tester-utils/tester_definition"
)

// detectLanguage 按规则列表顺序检测文件存在性，返回首个匹配的规则。
// 若无规则匹配，返回错误并列出所有预期文件。
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
```

**复杂度**：O(n)，n = 规则数（通常 2-3 条），忽略不计。

## 4. TestRunner 执行流变更

### 4.1 Phase 2 增强

`test_runner.go` 中 Phase 2 的代码**不需要改动**——`runCompileStep` 内部的 switch 扩展已足够。唯一额外动作是 `"auto"` case 中设置 `harness.DetectedLang`，这发生在 `runCompileStep` 内部。

```
Phase 2 完整流程:
  CompileStep != nil?
    ├── Language="c"     → compileC()
    ├── Language="make"  → make
    ├── Language="java"  → compileJava()        ← 新增
    ├── Language="python"→ checkPythonSyntax()   ← 新增
    └── Language="auto"  → detectLanguage()      ← 新增
                            ├── 设置 harness.DetectedLang
                            └── 递归 → runCompileStep(resolved)
```

### 4.2 TestFunc 中使用 DetectedLang

```go
// tinynum-tester 中的使用示例
func testE01(harness *test_case_harness.TestCaseHarness) error {
    lang := harness.DetectedLang
    logger := harness.Logger
    workDir := harness.SubmissionDir

    // 运行 Test Driver（语言无关）
    r := runner.Run(workDir, lang.RunCmd, lang.RunArgs...).
        WithLogger(logger).
        WithTimeout(10 * time.Second).
        Execute().
        Exit(0)

    if err := r.Error(); err != nil {
        return err
    }

    // 解析结构化输出（语言无关）
    results := parseStructuredOutput(r.GetStdout())
    // ... 断言
}
```

## 5. tinynum-tester 声明式配置示例

### 5.1 E01 Storage & Shape

```go
func e01TestCase() tester_definition.TestCase {
    return tester_definition.TestCase{
        Slug:          "storage-and-shape",
        Timeout:       30 * time.Second,
        // RequiredFiles 不列 Test Driver — 它们来自 starter，由 AutoDetect 间接保证。
        // Phase 1 仅检查语言无关的通用文件（若有需要在此列出）。
        CompileStep: &tester_definition.CompileStep{
            Language: "auto",
            AutoDetect: []tester_definition.LanguageRule{
                {
                    DetectFile: "NDArray.java",
                    Language:   "java",
                    Source:     "NDArray.java",
                    Flags:      []string{"tests/TestE01.java"},
                    RunCmd:     "java",
                    RunArgs:    []string{"-cp", ".", "TestE01"},
                },
                {
                    DetectFile: "num4py/ndarray.py",
                    Language:   "python",
                    Source:     "num4py/ndarray.py",
                    RunCmd:     "python3",
                    RunArgs:    []string{"tests/test_e01.py"},
                },
            },
        },
        TestFunc: testE01,
    }
}
```

### 5.2 提取公共规则工厂（tinynum-tester 内部）

15 个关卡的 AutoDetect 规则高度相似，可在 tinynum-tester 中提取工厂函数：

```go
// internal/stages/common.go

func javaRule(testDriver string) tester_definition.LanguageRule {
    // testDriver 如 "TestE01"
    return tester_definition.LanguageRule{
        DetectFile: "NDArray.java",
        Language:   "java",
        Source:     "NDArray.java",
        Flags:      []string{fmt.Sprintf("tests/%s.java", testDriver)},
        RunCmd:     "java",
        RunArgs:    []string{"-cp", ".", testDriver},
    }
}

func pythonRule(testScript string) tester_definition.LanguageRule {
    // testScript 如 "test_e01"
    return tester_definition.LanguageRule{
        DetectFile: "num4py/ndarray.py",
        Language:   "python",
        Source:     "num4py/ndarray.py",
        RunCmd:     "python3",
        RunArgs:    []string{fmt.Sprintf("tests/%s.py", testScript)},
    }
}

func autoCompileStep(javaTestDriver, pythonTestScript string) *tester_definition.CompileStep {
    return &tester_definition.CompileStep{
        Language: "auto",
        AutoDetect: []tester_definition.LanguageRule{
            javaRule(javaTestDriver),
            pythonRule(pythonTestScript),
        },
    }
}
```

每关声明缩减为一行：

```go
func e05TestCase() tester_definition.TestCase {
    return tester_definition.TestCase{
        Slug:        "unary-math",
        Timeout:     30 * time.Second,
        CompileStep: autoCompileStep("TestE05", "test_e05"),
        TestFunc:    testE05,
    }
}
```

## 6. 与现有代码的交互

### 6.1 新增/修改文件清单

| 文件                                     | 类型     | 变更内容                                                               |
| ---------------------------------------- | -------- | ---------------------------------------------------------------------- |
| `tester_definition/tester_definition.go` | 修改     | 新增 `LanguageRule` 结构体 + `CompileStep.AutoDetect` 字段             |
| `test_case_harness/test_case_harness.go` | 修改     | 新增 `DetectedLanguage` 结构体 + `DetectedLang *DetectedLanguage` 字段 |
| `test_runner/compile.go`                 | 修改     | switch 新增 `"java"` / `"python"` / `"auto"` 三个 case + 三个函数      |
| `test_runner/language.go`                | **新建** | `detectLanguage()` 函数                                                |
| `test_runner/compile_test.go`            | **新建** | Java/Python/auto 编译相关单元测试                                      |
| `test_runner/language_test.go`           | **新建** | 语言检测单元测试                                                       |

### 6.2 不变的文件

| 文件                               | 说明                                         |
| ---------------------------------- | -------------------------------------------- |
| `test_runner/test_runner.go`       | Phase 2 调用 `runCompileStep` 的代码无需改动 |
| `test_runner/prechecks.go`         | Phase 1 不变                                 |
| `test_runner/safeguards.go`        | Phase 3 BeforeFunc 不变                      |
| `runner/runner.go`                 | Runner API 不变                              |
| `tester.go`                        | 入口不变                                     |
| `tester_context/tester_context.go` | 上下文解析不变                               |

### 6.3 import 依赖

`test_case_harness` 需要 import `tester_definition`（由于 `DetectedLang *tester_definition.LanguageRule`）。

**循环依赖检查**：

- `tester_definition` → `test_case_harness`：TestFunc 参数类型（已有）
- `test_case_harness` → `tester_definition`：DetectedLang 字段（**新增**）

这会形成**循环 import**。解决方案：

**方案 A：LanguageRule 定义在独立包中**

```
tester_definition/language_rule.go  → 定义 LanguageRule（独立包太碎，不推荐）
```

**方案 B：LanguageRule 定义在 test_case_harness 中**

```go
// test_case_harness/test_case_harness.go
type DetectedLanguage struct {
    Language string
    RunCmd   string
    RunArgs  []string
}

type TestCaseHarness struct {
    ...
    DetectedLang *DetectedLanguage
}
```

CompileStep.AutoDetect 仍用 `LanguageRule`（在 tester_definition 中），auto case 在 compile.go 中将 `LanguageRule` 转换为 `DetectedLanguage` 赋值给 harness。

**方案 C（推荐）：将 LanguageRule 的运行时部分拆为接口**

最简单的做法——在 `test_case_harness` 中只定义一个轻量结构体存运行时信息，避免跨包依赖：

```go
// test_case_harness/test_case_harness.go

// DetectedLanguage 存储自动检测到的语言运行时信息。
// 由框架 Phase 2 (CompileStep auto) 填充，TestFunc 消费。
type DetectedLanguage struct {
    Language string   // 检测到的语言（"java", "python" 等）
    RunCmd   string   // 运行命令（"java", "python3" 等）
    RunArgs  []string // 运行参数（如 ["-cp", ".", "TestE01"]）
}
```

```go
// tester_definition/tester_definition.go

type LanguageRule struct {
    DetectFile string
    Language   string
    Source     string
    Flags      []string
    RunCmd     string
    RunArgs    []string
}
```

compile.go 中的转换：

```go
// "auto" case 中
harness.DetectedLang = &test_case_harness.DetectedLanguage{
    Language: rule.Language,
    RunCmd:   rule.RunCmd,
    RunArgs:  rule.RunArgs,
}
```

**选择方案 C**：零循环依赖，最小跨包耦合，`LanguageRule` 是配置侧（tester_definition），`DetectedLanguage` 是运行时侧（test_case_harness）。

## 7. 错误场景处理

| 场景                               | 行为         | 错误信息                                                                                                |
| ---------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------- |
| `"auto"` 但 AutoDetect 为空        | Phase 2 失败 | `CompileStep Language="auto" but AutoDetect is empty`                                                   |
| 无规则匹配（两种语言文件都不存在） | Phase 2 失败 | `cannot detect language: none of [NDArray.java (java), num4py/ndarray.py (python)] found in submission` |
| Java 编译失败                      | Phase 2 失败 | `NDArray.java does not compile: <javac output>`                                                         |
| Python 语法错误                    | Phase 2 失败 | `num4py/ndarray.py has syntax errors: <py_compile output>`                                              |
| javac 不在 PATH 中                 | Phase 2 失败 | `javac: command not found`（exec.Command 自动报错）                                                     |
| python3 不在 PATH 中               | Phase 2 失败 | `python3: command not found`                                                                            |
| 编译超时（>30s）                   | Phase 2 失败 | `compilation timed out after 30s`（复用现有超时机制）                                                   |

## 8. 测试策略

### 8.1 单元测试

| 测试文件                          | 测试场景                                                 |
| --------------------------------- | -------------------------------------------------------- |
| `test_runner/language_test.go`    | detectLanguage：首条匹配、第二条匹配、无匹配、空规则列表 |
| `test_runner/compile_test.go`     | compileJava 成功/失败、checkPythonSyntax 成功/失败       |
| `test_runner/test_runner_test.go` | auto 完整流程：检测→编译→TestFunc 获取 DetectedLang      |

### 8.2 集成测试

在 tinynum-tester 中用 E01 作为 POC：

- Java 提交目录 → 检测 java → javac 编译 → 运行 TestE01 → 通过
- Python 提交目录 → 检测 python → py_compile → 运行 test_e01.py → 通过
- 空目录 → 检测失败 → 清晰错误信息

# auto-lang：多语言自动检测与编译支持 — 实施 TODO

> 版本: v1.0 | 日期: 2026-03-14 | 状态: 待实施

## 整体排期

| 阶段                           | 工作量    | 依赖   |
| ------------------------------ | --------- | ------ |
| Step 1: Java + Python 编译支持 | ~2h       | 无     |
| Step 2: auto 语言检测          | ~2h       | Step 1 |
| Step 3: 回归验证               | ~0.5h     | Step 2 |
| Step 4: tinynum-tester E01 POC | ~2h       | Step 2 |
| **总计**                       | **~6.5h** |        |

---

## Step 1: Java + Python 编译支持

### T1.1 CompileStep switch 新增 "java" case

- **文件**: `test_runner/compile.go`
- **内容**:
  - 新增 `case "java"` 分支，调用 `compileJava()`
  - 新增 `compileJava(workDir, cs)` 函数：`javac -d {workDir} {Flags...} {Source}`
  - 日志：`Compiling {Source}...` → `{Source} compiles`
- **约束**:
  - `-d .` 确保 .class 输出到当前目录（`cmd.Dir = workDir`）
  - `Flags` 传额外 .java 文件（与 C 的 Flags 传额外 .c 文件模式一致）
  - `Source` 放在 args 最后（javac 参数顺序要求）
- **行数**: ~20 行
- **工时**: 0.5h
- **状态**: ⬜ 待实施

### T1.2 CompileStep switch 新增 "python" case

- **文件**: `test_runner/compile.go`
- **内容**:
  - 新增 `case "python"` 分支，调用 `checkPythonSyntax()`
  - 新增 `checkPythonSyntax(workDir, cs)` 函数：`python3 -m py_compile {Source}`
  - 日志：`Checking {Source} syntax...` → `{Source} syntax OK`
- **约束**:
  - 仅做语法检查，不生成可执行文件
  - 编译错误包含完整的 py_compile 输出
- **行数**: ~20 行
- **工时**: 0.5h
- **状态**: ⬜ 待实施

### T1.3 基础单元测试（Java + Python）

- **文件**: `test_runner/compile_test.go`（新建）
- **测试场景**:
  - [ ] compileJava 成功：临时目录 + 合法 .java → 编译通过
  - [ ] compileJava 失败：语法错误的 .java → 返回 error 含 javac 输出
  - [ ] compileJava 多文件：Source + Flags 额外文件 → 全部编译
  - [ ] checkPythonSyntax 成功：合法 .py → 通过
  - [ ] checkPythonSyntax 失败：语法错误的 .py → 返回 error 含 py_compile 输出
- **前提**: 测试环境需要 `javac` 和 `python3` 可用
- **工时**: 0.5h
- **状态**: ⬜ 待实施

**Step 1 检查点**：`go test ./test_runner/...` 通过，llm100x-tester 无影响

---

## Step 2: auto 语言检测

### T2.1 新增 LanguageRule 结构体

- **文件**: `tester_definition/tester_definition.go`
- **内容**:
  - 新增 `LanguageRule` 结构体（DetectFile, Language, Source, Flags, RunCmd, RunArgs）
  - `CompileStep` 新增 `AutoDetect []LanguageRule` 字段
- **约束**:
  - 所有字段为值类型，零值安全
  - AutoDetect 为 nil/空时忽略（仅 Language="auto" 时读取）
- **行数**: ~25 行
- **工时**: 0.25h
- **状态**: ⬜ 待实施

### T2.2 新增 DetectedLanguage 结构体

- **文件**: `test_case_harness/test_case_harness.go`
- **内容**:
  - 新增 `DetectedLanguage` 结构体（Language, RunCmd, RunArgs）
  - `TestCaseHarness` 新增 `DetectedLang *DetectedLanguage` 字段
- **约束**:
  - `DetectedLanguage` 定义在 `test_case_harness` 包中，避免循环 import
  - 非 auto 模式时 `DetectedLang` 为 nil
- **行数**: ~10 行
- **工时**: 0.25h
- **状态**: ⬜ 待实施

### T2.3 新建 language.go — 检测逻辑

- **文件**: `test_runner/language.go`（新建）
- **内容**:
  - `detectLanguage(workDir string, rules []LanguageRule) (*LanguageRule, error)`
  - 按列表顺序检测 `filepath.Join(workDir, rule.DetectFile)` 是否存在
  - 首个匹配返回 `&rules[i]`
  - 无匹配返回错误，列出所有预期文件
- **行数**: ~25 行
- **工时**: 0.25h
- **状态**: ⬜ 待实施

### T2.4 compile.go 新增 "auto" case

- **文件**: `test_runner/compile.go`
- **内容**:
  - `case "auto"` 分支：
    1. 校验 `AutoDetect` 非空
    2. 调用 `detectLanguage()`
    3. 日志 `Detected language: {lang} (found {file})`
    4. 设置 `harness.DetectedLang = &DetectedLanguage{...}`
    5. 构造具体 CompileStep，递归调用 `runCompileStep()`
  - `runCompileStep` 签名调整：需接收 `harness` 参数以设置 `DetectedLang`
- **关键变更**: `runCompileStep` 当前签名为 `(harness, cs) error`，已接收 harness，无需改签名
- **行数**: ~20 行
- **工时**: 0.25h
- **状态**: ⬜ 待实施

### T2.5 语言检测单元测试

- **文件**: `test_runner/language_test.go`（新建）
- **测试场景**:
  - [ ] 首条规则匹配（Java 文件存在）
  - [ ] 第二条规则匹配（仅 Python 文件存在）
  - [ ] 无规则匹配 → 错误信息包含所有预期文件
  - [ ] 空规则列表 → 错误
  - [ ] 两种文件都存在 → 返回首条规则（Java 优先）
- **工时**: 0.5h
- **状态**: ⬜ 待实施

### T2.6 auto 完整流程测试

- **文件**: `test_runner/test_runner_test.go`（扩展现有）
- **测试场景**:
  - [ ] auto + Java 文件存在 → 检测 java → compileJava → TestFunc 中 DetectedLang.RunCmd="java"
  - [ ] auto + Python 文件存在 → 检测 python → checkPythonSyntax → TestFunc 中 DetectedLang.RunCmd="python3"
  - [ ] auto + 无文件 → Phase 2 失败 → TestFunc 不执行
  - [ ] auto + Java 编译失败 → Phase 2 失败 → TestFunc 不执行
  - [ ] 非 auto 模式 → DetectedLang 为 nil（向后兼容）
- **工时**: 0.5h
- **状态**: ⬜ 待实施

**Step 2 检查点**：`go test ./...` 全部通过，llm100x-tester 无影响

---

## Step 3: 回归验证

### T3.1 现有测试全部通过

- **命令**: `go test ./...`
- **预期**: 所有现有 23 个单元测试 + 新增测试全部通过
- **工时**: 0.25h
- **状态**: ⬜ 待实施

### T3.2 llm100x-tester 编译兼容性

- **命令**: `cd llm100x-tester && go build ./...`
- **预期**: 编译通过，无需任何改动
- **说明**: llm100x-tester 不使用新字段，向后兼容
- **工时**: 0.25h
- **状态**: ⬜ 待实施

---

## Step 4: tinynum-tester E01 POC

> 此步骤属于 tinynum-tester 项目，不在 tester-utils 仓库中，但用于验证框架改动。

### T4.1 创建 tinynum-tester 骨架

- **目录**: `tinynum-tester/`
- **内容**:
  - `main.go`：12 行入口（对齐 llm100x-tester）
  - `go.mod`：依赖 tester-utils（本地 replace 开发）
  - `internal/stages/stages.go`：GetDefinition()
  - `internal/stages/common.go`：javaRule / pythonRule / autoCompileStep 工厂函数
  - `internal/helpers/structured_output.go`：解析 `TEST:xxx\nRESULT:xxx`
  - `internal/helpers/float_compare.go`：浮点容差比较
- **工时**: 0.5h
- **状态**: ⬜ 待实施

### T4.2 实现 E01 Storage & Shape 测试

- **文件**: `internal/stages/e01_storage.go`
- **内容**:
  - `e01TestCase()` 使用 `autoCompileStep("TestE01", "test_e01")`
  - `testE01()` 使用 `harness.DetectedLang` 运行 Test Driver
  - 解析结构化输出，验证 zeros/ones/fromArray/size/ndim/shape/toString
- **工时**: 0.5h
- **状态**: ⬜ 待实施

### T4.3 端到端验证

- **场景 A**：Java 提交 → `TENSORHERO_REPOSITORY_DIR=java-solution ./tinynum-tester storage-and-shape`
- **场景 B**：Python 提交 → `TENSORHERO_REPOSITORY_DIR=python-solution ./tinynum-tester storage-and-shape`
- **场景 C**：空目录 → 清晰错误信息
- **工时**: 0.5h
- **状态**: ⬜ 待实施

---

## 依赖关系图

```
T1.1 ──┐
       ├── T1.3 ──┐
T1.2 ──┘          │
                  ├── T2.4 ── T2.6 ── T3.1 ── T3.2
T2.1 ──┐          │
       ├── T2.3 ──┘
T2.2 ──┘    │
            └── T2.5
                          T3.2 ── T4.1 ── T4.2 ── T4.3
```

- T1.1 和 T1.2 可并行
- T2.1 和 T2.2 可并行（与 T1.x 也可并行）
- T2.4 依赖 T1.x（java/python case 已存在）和 T2.x（LanguageRule 已定义）
- Step 4 依赖 Step 1-3 全部完成

## 变更文件汇总

| 文件                                     | 操作     | 行数        | Step |
| ---------------------------------------- | -------- | ----------- | ---- |
| `tester_definition/tester_definition.go` | 修改     | +25         | 2    |
| `test_case_harness/test_case_harness.go` | 修改     | +10         | 2    |
| `test_runner/compile.go`                 | 修改     | +60         | 1+2  |
| `test_runner/language.go`                | **新建** | ~25         | 2    |
| `test_runner/compile_test.go`            | **新建** | ~80         | 1    |
| `test_runner/language_test.go`           | **新建** | ~60         | 2    |
| `test_runner/test_runner_test.go`        | 修改     | +50         | 2    |
| **tester-utils 合计**                    |          | **~310 行** |      |

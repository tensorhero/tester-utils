# auto-lang：多语言自动检测与编译支持 — 需求文档

> 版本: v1.0 | 日期: 2026-03-14 | 状态: 草案

## 1. 背景

### 1.1 现状

当前 `tester-utils` 的 `CompileStep` 支持两种语言：`"c"` 和 `"make"`。每个 TestCase 绑定一种固定的编译方式，无法根据学生提交的语言动态切换。

这在 llm100x-tester（单语言 C/Python/SQL 课程）中没有问题，但 **tinynum** 等多语言课程（同一道题 Java/Python 均可完成）无法用现有框架声明式支持。

### 1.2 tinynum 课程的需求

tinynum 是一个"Build Your Own NumPy"课程，支持 Java 和 Python 两种语言。15 个关卡的**测试逻辑基本相同**（创建 NDArray → 调用方法 → 验证输出），只有编译/运行命令不同：

| 步骤 | Java                                         | Python                                             |
| ---- | -------------------------------------------- | -------------------------------------------------- |
| 检测 | `NDArray.java` 存在                          | `num4py/ndarray.py` 存在                           |
| 编译 | `javac -d . NDArray.java tests/TestE01.java` | 无需编译（可选：`python3 -m py_compile` 语法检查） |
| 运行 | `java -cp . TestE01`                         | `python3 tests/test_e01.py`                        |
| 输出 | 相同的 `TEST:xxx\nRESULT:xxx` 结构化格式     | 同左                                               |

如果没有框架层支持，tinynum-tester 每关都要写 if/else 分支处理两种语言，15 关共计 ~30 处重复的语言判断代码。

### 1.3 驱动力

- **tinynum MVP** 需要 Java + Python 双语言支持（阻塞项）
- 未来课程（tinytorch 等）也会是多语言的，框架层支持可复用
- 现有 `CompileStep` 的扩展点（`Language` 字段 + `switch` 分发）天然适合新增语言

## 2. 需求目标

### R1: CompileStep 支持 Java 编译（P0 — 阻塞 MVP）

**描述**：`CompileStep.Language` 新增 `"java"` 选项，框架自动调用 `javac` 编译。

**验收标准**：

- `Language: "java"` 时，框架调用 `javac -d {workDir} {Flags...} {Source}`
- `Source` 为主源文件（如 `NDArray.java`）
- `Flags` 可传递额外源文件（如 `tests/TestE01.java`）
- 编译失败时输出完整的 javac 错误信息
- 编译受 30s 超时保护（复用现有 `runCompileStepWithTimeout`）

### R2: CompileStep 支持 Python 语法检查（P1 — 可选增强）

**描述**：`CompileStep.Language` 新增 `"python"` 选项，框架自动调用 `python3 -m py_compile` 进行语法检查。Python 为解释型语言，无需编译即可运行；此步骤仅提供早期语法校验，非阻塞项。`"auto"` 模式下 Python 规则可不设置编译步骤，直接运行。

**验收标准**：

- `Language: "python"` 时，框架调用 `python3 -m py_compile {Source}`
- 语法错误时输出完整的 Python 错误信息
- `Source` 为主源文件（如 `num4py/ndarray.py`）
- 编译受 30s 超时保护

### R3: 自动语言检测（P0 — 阻塞 MVP）

**描述**：`CompileStep.Language` 新增 `"auto"` 选项，根据提交目录中的文件自动检测语言，分发到对应的编译+运行方式。

**验收标准**：

- `Language: "auto"` 时，按 `AutoDetect` 规则列表顺序检测文件存在性
- 首个匹配的规则决定编译语言和运行命令
- 无任何规则匹配时，返回清晰错误（列出所有预期文件）
- 检测结果通过 `TestCaseHarness.DetectedLang` 暴露给 TestFunc
- TestFunc 中可通过 `DetectedLang.RunCmd` / `DetectedLang.RunArgs` 获取运行命令

### R4: TestCaseHarness 暴露检测结果（P0 — 阻塞 MVP）

**描述**：`TestCaseHarness` 新增 `DetectedLang` 字段，供 TestFunc 获取自动检测的语言信息。

**验收标准**：

- `Language: "auto"` 时，CompileStep 完成后 `DetectedLang` 被填充
- `Language` 不是 `"auto"` 时，`DetectedLang` 为 nil（向后兼容）
- TestFunc 中可安全判空后使用

### R5: 向后兼容（P0 — 约束）

**验收标准**：

- 所有现有 tester（llm100x-tester 等）不需要任何改动
- `CompileStep` 的 `"c"` 和 `"make"` 行为不变
- `TestCaseHarness` 新字段零值不影响现有行为
- 所有现有单元测试继续通过

## 3. 使用场景

### 场景 1：tinynum-tester E01（Java 学生）

```
学生提交目录:
  NDArray.java          ← 学生代码
  tests/TestE01.java    ← starter 提供

tester 执行:
  Phase 1: RequiredFiles → 检查 TestE01.java 存在
  Phase 2: CompileStep(auto) → 检测到 NDArray.java → Language=java
           → javac -d . NDArray.java tests/TestE01.java
  Phase 4: TestFunc → runner.Run("java", "-cp", ".", "TestE01")
           → 解析 stdout 验证
```

### 场景 2：tinynum-tester E01（Python 学生）

```
学生提交目录:
  num4py/ndarray.py     ← 学生代码
  tests/test_e01.py     ← starter 提供

tester 执行:
  Phase 1: RequiredFiles → 检查 test_e01.py 存在
  Phase 2: CompileStep(auto) → 检测到 num4py/ndarray.py → Language=python
           → 跳过编译（或可选 python3 -m py_compile 语法检查）
  Phase 4: TestFunc → runner.Run("python3", "tests/test_e01.py")
           → 解析 stdout 验证（格式与 Java 相同）
```

### 场景 3：llm100x-tester（无影响）

```
现有 CompileStep:
  Language: "c", Source: "hello.c", Output: "hello"

执行流程完全不变 — "c" 分支走原有 clang 编译路径。
```

## 4. 非目标

- **不支持 Go / TypeScript 编译**：tinynum MVP 仅 Java + Python，其他语言留给未来按需新增
- **不修改 Runner 断言 API**：不新增 `StdoutFloat` 等浮点断言，由 tinynum-tester 的 `internal/helpers/` 自行处理
- **不修改 tensorhero.yml 格式**：语言检测基于文件存在性，不依赖配置文件声明
- **不支持同一提交多语言混合**：一个提交目录只能是一种语言

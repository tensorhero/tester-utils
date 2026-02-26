# tester-utils 改进需求文档

> 版本: v1.1 | 日期: 2026-02-25 | 状态: 草案

## 1. 背景

### 1.1 现状问题

当前 `tester-utils` 框架缺少 **Test 级别的前置校验机制**。所有校验逻辑（文件存在检查、编译步骤）
全部内联写在每个 stage 的 `TestFunc` 中，导致以下问题：

1. **大量样板代码重复** — 30 个 stage 中有 18 次手写 `harness.FileExists()` 检查，15 次手写编译调用
2. **校验与测试逻辑耦合** — 文件存在检查、编译步骤混在测试函数里，职责不清
3. **files_config 从未被消费** — `stage.yml` 中定义的 `required/allowed/blocked` 同步到 DB 后无任何代码读取
4. **缺少框架级 Hook** — `TesterDefinition` 和 `TestCase` 无 `BeforeFunc/SetupFunc`，`TestRunner` 无前置拦截点
5. **编程语言硬绑定** — 每个 stage 固定一种语言（C 或 Python），无法支持同一 stage 多语言完成

### 1.2 数据佐证（llm100x-tester 源码审计）

| 重复模式 | 出现次数 | 涉及文件 |
|---------|---------|---------|
| 单文件存在检查 `FileExists("x.c")` | 18 次 | hello, caesar, cash, credit, mario 等 |
| 多文件存在检查 | 4 次 | filter-less, filter-more, speller, movies |
| `helpers.CompileC()` 编译 | 14 次 | hello, mario×2, cash, credit, scrabble, readability, caesar, substitution, plurality, runoff, tideman, volume, recover |
| `exec.Command("clang")` 直接编译 | 3 次 | filter-less, filter-more, inheritance（使用特殊 flags） |
| `exec.Command("make")` 编译 | 1 次 | speller |
| 数据文件检查 | 3 次 | recover(card.raw), songs(1-7.sql), movies(1-13.sql) |
| 环境/运行时检查 | 1 次 | finance(venv 探测) |

**编译操作合计 18 次**（14×CompileC + 3×直接 clang + 1×make）。
**粗估：约 150 行可消除的样板代码。**

### 1.3 stage.yml files_config 现状

30 个 stage.yml 全部定义了 `files_config`，但从未被运行时消费：

```yaml
# 示例: caesar/stage.yml
files_config:
  required: [caesar.c]     # 必须存在的文件
  allowed: ["*.h"]         # 允许的额外文件（glob）
  blocked: ["*.py"]        # 禁止的文件（glob）
```

数据流断链：`stage.yml → hellobyte-schema syncer → DB stages.files_config (JSONB) → ❌ 无消费端`

## 2. 需求目标

### R1: 声明式前置校验（P0 — 必须）

框架层支持在 `TestCase` 上声明前置条件，`TestRunner` 在执行 `TestFunc` 之前自动校验。

**验收标准：**
- TestCase 可声明 `RequiredFiles []string`，框架自动检查文件存在
- TestCase 可声明 `CompileStep`，框架自动执行编译
- TestCase 可声明 `BeforeFunc`，用于自定义前置逻辑
- 前置校验失败时，输出清晰错误信息并跳过 TestFunc
- Phase 1-3 需加 `defer/recover` 防止用户 BeforeFunc panic 导致进程崩溃
- Phase 2（CompileStep）需设置编译超时（默认 30s），防止编译 hang
- 日志格式统一（`[stage-N] Checking X exists...`）

### R2: files_config 运行时消费路径（P1 — 重要）

建立从 `files_config` 到 Tester 的消费路径，使 `required/allowed/blocked` 真正生效。

**验收标准：**
- P1-A（硬编码）：tester 代码中的 `RequiredFiles` 与 `stage.yml` 的 `files_config.required` 保持一致
- P1-B（数据驱动，后续）：Worker 通过 `HELLOBYTE_FILES_CONFIG` 环境变量注入 JSON，
  tester-utils 自动解析并在 TestFunc 前校验 `required/allowed/blocked`

### R3: BeforeFunc 自定义 Hook（P0 — 必须）

支持复杂场景的自定义前置逻辑（如 finance 的 venv 探测、inheritance 的测试文件组装）。

**验收标准：**
- `BeforeFunc` 在声明式检查之后、`TestFunc` 之前执行
- `BeforeFunc` 失败时跳过 `TestFunc`，执行 `TeardownFuncs`
- `BeforeFunc` 共享同一个 `TestCaseHarness` 实例

### R4: 完整生命周期 Hook（P2 — 规划）

参考 JUnit 5 / pytest 的完整生命周期：

```
BeforeAll → [BeforeEach → TestFunc → AfterEach] × N → AfterAll
```

**验收标准：**
- `TesterDefinition` 支持 `BeforeAll/AfterAll` 全局 Hook
- 已有的 `TeardownFuncs` 作为 `AfterEach` 继续保留
- `BeforeFunc` 作为 `BeforeEach` 的声明式快捷方式

> **注意：** R4 为远期规划，P0 阶段只需实现 R1 + R3。

### R5: 向后兼容性（P0 — 必须）

**验收标准：**
- 所有新字段为可选字段，零值即不执行
- 现有 tester（不使用新字段）行为完全不变
- `RunCLI()` 和 `Run()` 入口签名不变

## 3. 非需求（明确排除）

| 排除项 | 原因 |
|-------|------|
| `files_config.allowed/blocked` 的 glob 匹配 | P1-B 阶段处理，当前只处理 `required` |
| 多语言测试框架 | 独立需求，不在本次范围（见附录 A） |
| Worker 侧 `HELLOBYTE_FILES_CONFIG` 注入 | 需 Worker + API 协同改造，P1-B 阶段 |
| `TestCase` 依赖图（check50 风格 `@check(dependency)`） | 过度设计，当前线性执行足够 |

## 4. 业界参考

### 4.1 CS50 check50

```python
@check50.check()
def exists():
    check50.exists("hello.c")  # 内置前置检查

@check50.check(exists)         # 声明依赖
def compiles():
    check50.c.compile("hello.c")

@check50.check(compiles)
def correct():
    check50.run("./hello").stdin("David").stdout("hello, David")
```

**借鉴点：**
- 文件存在检查和编译是框架内置能力，不需要 stage 作者手写
- 依赖声明清晰但 Go 版本用线性执行即可

### 4.2 JUnit 5 生命周期

```
@BeforeAll   → 类级别，全部 test 之前执行一次
@BeforeEach  → 每个 test 之前执行
@Test        → 实际测试
@AfterEach   → 每个 test 之后执行
@AfterAll    → 类级别，全部 test 之后执行一次
```

**借鉴点：**
- 分层 Hook 设计
- Setup/Teardown 对称（当前只有 Teardown）

### 4.3 Exercism Test Runner

```
Pre-check → Test → Post-format
(file exists, syntax) → (run, assert) → (JSON output)
```

**借鉴点：**
- Pre-check 与 Test 阶段明确分离
- 标准化输出格式

## 附录 A: 多语言测试支持（独立需求，未来规划）

当前 llm100x-tester 的语言绑定模式：

| Stage 类型 | 语言 | 硬绑定方式 |
|-----------|------|-----------|
| Week 1-5 C stages | C | `CompileC()` + `runner.Run(workDir, "hello")` |
| Week 6 Python stages | Python | `runner.Run(workDir, "python3", "hello.py")` |
| Week 7 SQL stages | SQL | `sql.Open("sqlite3")` + 直接执行 SQL |
| Week 9 Flask stage | Python/Flask | `exec.Command("python3", "-m", "flask")` |

**问题：** 如果 hello stage 允许 C 和 Python 两种语言完成，
当前框架无法在单个 `TestCase` 中根据提交文件自动选择测试策略。

**远期方向：**
- `TestCase` 增加 `Variants []TestVariant`，每个 Variant 绑定一种语言
- 框架根据提交目录中的文件自动探测语言并选择对应 Variant
- 同 stage.yml 的 `files_config` 关联：不同语言有不同的 `required` 文件

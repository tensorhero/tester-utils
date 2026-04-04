# BootCraft Tester Utils — 架构设计

## 1. 概述

**tensorhero-tester-utils** 是一个用于构建课程自动评分测试器的 Go 框架。每门课程发布自己的测试器二进制文件，通过导入本库来使用框架能力。框架负责提交代码发现、进程执行、输出断言和结构化结果报告，课程作者只需专注于编写测试逻辑。

```
课程测试器二进制
  └── tensorhero-tester-utils（本库）
        ├── CLI 入口 & 环境变量解析
        ├── 四阶段测试流水线
        ├── 进程执行（管道 / PTY）
        └── 流式断言 API（Runner）
```

## 2. 模块一览

| 模块                    | 职责                                                                     |
| ----------------------- | ------------------------------------------------------------------------ |
| `tester.go`             | 入口。解析 CLI 参数，合并环境变量，编排阶段执行。                        |
| `tester_definition`     | 数据模型：`TesterDefinition`、`TestCase`、`CompileStep`。                |
| `tester_context`        | 解析运行时上下文：运行模式（JSON/STAGE/ALL）、提交目录、可执行文件路径。 |
| `test_runner`           | 四阶段流水线执行器，含超时控制。                                         |
| `test_case_harness`     | `TestFunc` 沙箱：提供日志器、可执行文件构建器、文件辅助工具、清理注册。  |
| `executable`            | 进程生命周期管理：启动、I/O 中继、内存限制（Linux）、终止。              |
| `runner`                | 输出断言的流式 API：`Run().Stdin().Stdout().Exit()`。                    |
| `logger`                | 带颜色的分级日志，全局互斥锁序列化，静默模式。                           |
| `linewriter`            | 基于 channel 的行缓冲；遇换行符或 500 ms 超时即刷新。                    |
| `random`                | 确定性随机数生成器，由 `BOOTCRAFT_RANDOM_SEED` 控制种子。               |
| `bytes_diff_visualizer` | 十六进制 + ASCII 差异渲染，ANSI 彩色标记首个差异字节。                   |
| `stdio_mocker`          | 测试工具：用临时文件替换 `os.Stdout/Stdin/Stderr`。                      |
| `testing`               | `ValidateTesterDefinitionAgainstYAML` — 校验测试器定义与课程 YAML 一致。 |

## 3. 执行流程

```
main() → tester.Run(definition)
           │
           ├─ ParseArgs(os.Args)
           ├─ MergeArgsIntoEnv()
           ├─ RunCLI()
           │    ├─ newTester(definition)
           │    │    └─ NewTesterContext(env)  // 解析模式、目录、可执行文件
           │    │
           │    ├─ runStages(testCases)
           │    │    └─ 对每个 TestCase：
           │    │         TestRunner.RunTest()  // 四阶段流水线
           │    │
           │    └─ runAntiCheatStages(antiCheatTestCases)  // 静默模式
           │
           └─ exit(0 | 1)
```

### 运行模式

| 模式      | 触发条件                            | 行为                                  |
| --------- | ----------------------------------- | ------------------------------------- |
| **JSON**  | 设置了 `BOOTCRAFT_TEST_CASES_JSON` | 运行 JSON 中指定的测试用例。          |
| **STAGE** | 设置了 `BOOTCRAFT_STAGE`           | 运行到指定阶段 slug（含该阶段）为止。 |
| **ALL**   | 均未设置                            | 按顺序运行全部测试用例。              |

## 4. 四阶段测试流水线

每个 `TestCase` 在 `TestRunner.RunTest()` 中依次执行四个阶段：

```
阶段 1：RequiredFiles
  └─ 验证提交目录中存在所需文件。

阶段 2：CompileStep
  └─ 编译提交代码（语言、源文件、编译标志、输出路径）。
     支持 IncludeParentDir 以进行多文件编译。

阶段 3：BeforeFunc
  └─ 可选的前置回调（如：初始化数据库、创建测试夹具）。

阶段 4：TestFunc                          ← 带超时
  └─ 核心测试逻辑在 TestCaseHarness 沙箱中运行。
     可访问：Logger、Executable 构建器、SubmissionDir、文件辅助工具。
     使用 time.NewTimer 实现干净的超时取消。
```

任一阶段失败，后续阶段将被跳过，测试用例报告失败。

## 5. 可执行文件子系统

`executable` 包管理子进程生命周期：

```
Executable
  ├── WorkingDir, Args, Env
  ├── StdioHandler（接口）
  │     ├── PipeStdioHandler   — 通过 LineWriter 实现行缓冲管道 I/O
  │     └── PTYStdioHandler    — 伪终端，用于交互式程序
  ├── MemoryLimit（默认 2 GB）
  └── LoggerFunc
```

### I/O 中继

`setupIORelay` 将子进程的 stdout/stderr 复制到日志器，每个流强制执行 **30 KB** 输出上限。输出通过 `LineWriter` 中继，后者在收到完整行或 500 ms 空闲超时后刷新。

### 内存监控（Linux）

```
memoryMonitor
  ├── 每 100 ms 轮询 /proc/<pid>/status（VmRSS）
  ├── 设置 RLIMIT_AS = 3× 限制值（虚拟内存安全网）
  └── RSS 超限时通过原子操作标记 oomKilled
```

非 Linux 平台上，`memoryMonitor` 为空操作（通过构建标签控制）。

### 安全性

`getSafeEnvironmentVariables()` 过滤掉所有匹配 `TENSORHERO_SECRET*` 的环境变量，防止评分环境中的密钥泄露到学生代码中。

## 6. Runner — 流式断言 API

`Runner` 提供类似 check50 的链式 API 来测试可执行文件：

```go
// 阻塞模式 — 完整生命周期
testCase.Run("./server").
    Stdin("GET /").
    Stdout("200 OK").
    Exit(0)

// 交互模式 — 逐步执行
r := testCase.Start("./repl")
r.SendLine("help")
r.Stdout("Available commands:")
r.SendLine("quit")
r.WaitForExit()
```

### 输出匹配器

| 方法             | 匹配方式           |
| ---------------- | ------------------ |
| `Stdout(s)`      | 包含子串           |
| `StdoutExact(s)` | 精确匹配           |
| `StdoutRegex(p)` | 正则表达式匹配     |
| `Exit(code)`     | 退出码等于指定值   |
| `Reject(code)`   | 退出码不等于指定值 |

匹配失败时，runner 会记录期望值与实际值。对于字节级差异，`BytesDiffVisualizer` 渲染十六进制 + ASCII 对比图。

## 7. 日志架构

```
Logger
  ├── 级别：Debug、Info、Success、Error
  ├── 颜色：黄色前缀、绿色成功、红色错误、青色调试
  ├── syncWriter → 全局互斥锁（序列化所有 logger 实例的写入）
  ├── 静默模式：抑制非关键输出（反作弊阶段使用）
  └── 二级前缀：嵌套上下文（如：阶段 → 步骤 → 详情）
```

## 8. 配置契约

### 环境变量

| 变量                         | 必需 | 说明                                      |
| ---------------------------- | ---- | ----------------------------------------- |
| `BOOTCRAFT_REPOSITORY_DIR`  | 是   | 提交代码目录路径。                        |
| `BOOTCRAFT_STAGE`           | 否   | 目标阶段 slug（STAGE 模式）。             |
| `BOOTCRAFT_TEST_CASES_JSON` | 否   | 测试用例 slug 的 JSON 数组（JSON 模式）。 |
| `BOOTCRAFT_RANDOM_SEED`     | 否   | 用于可复现测试的确定性种子。              |
| `BOOTCRAFT_SKIP_ANTI_CHEAT` | 否   | 设置后跳过反作弊阶段。                    |
| `BOOTCRAFT_STREAM_LOGS`     | 否   | 实时流式输出日志。                        |
| `BOOTCRAFT_RECORD_FIXTURES` | 否   | 录制测试夹具以供回放。                    |
| `TENSORHERO_SECRET`          | 否   | 从子进程环境中过滤的密钥。                |

### 提交配置（`bootcraft.yml`）

提交目录中的可选 YAML 文件：

```yaml
debug: true # 启用调试级别日志
```

## 9. 数据流图

```
┌──────────────┐   环境变量     ┌────────────────┐
│  CI / Runner │ ──────────────→│   测试器二进制   │
└──────────────┘                └───────┬────────┘
                                        │
                        ┌───────────────┼───────────────┐
                        ▼               ▼               ▼
                  TesterContext    TestRunner ×N    AntiCheat ×M
                  (模式、目录、   (每个四阶段)      (静默模式)
                   可执行文件)          │
                                        ▼
                                  TestCaseHarness
                                   │         │
                              Executable    Runner
                              (启动、       (断言
                               I/O 中继、    输出、
                               内存限制)     退出码)
                                   │
                                   ▼
                              学生提交进程
```

## 10. 关键设计决策

| 决策                     | 理由                                                       |
| ------------------------ | ---------------------------------------------------------- |
| 构建标签控制内存监控     | 仅 Linux 暴露 `/proc` RSS；其他平台为空操作。              |
| 30 KB 输出上限           | 防止学生程序耗尽评分器内存。                               |
| 全局 logger 互斥锁       | 简单的序列化方案，避免并发阶段输出交错。                   |
| LineWriter channel 刷新  | 解耦写入粒度与显示粒度；确保输出为完整行。                 |
| 流式 Runner API          | 借鉴 check50 用户体验 — 课程作者能编写可读的、顺序的断言。 |
| 反作弊静默模式           | 静默运行重复测试，防止学生逆向推断期望输出。               |
| `TENSORHERO_SECRET` 过滤 | 纵深防御：密钥永远不会泄露到子进程环境中。                 |
| 确定性随机数             | `BOOTCRAFT_RANDOM_SEED` 支持可复现的测试运行，便于调试。  |

## 11. 扩展点

- **新 `StdioHandler`**：实现该接口可添加管道和 PTY 之外的自定义 I/O 策略。
- **新 `CompileStep` 语言**：在编译阶段添加特定语言的编译逻辑。
- **自定义 `BeforeFunc`**：每个测试用例的任意前置逻辑（数据库初始化、夹具生成等）。
- **额外输出匹配器**：按需为 `Runner` 扩展新的断言方法。

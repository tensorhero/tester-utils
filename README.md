# BootCraft Tester Utils

BootCraft 课程测试工具的共享框架模块。

**基于：** [codecrafters-io/tester-utils](https://github.com/codecrafters-io/tester-utils)

## 功能特性

- **四阶段测试流水线** — 在 `TestFunc` 之前依次执行文件检查、编译、预置钩子
- **流式 Runner API** — check50 风格的程序测试，支持阻塞、交互、PTY 模式
- **灵活运行模式** — 平台派发（JSON）、单阶段、全量运行（默认）
- **CLI 支持** — `./tester hello`、`./tester -s hello -d ~/work`、`--help`
- **合理默认值** — `bootcraft.yml` 可选，工作目录默认为 `.`

## 快速开始

```go
package main

import (
    "os"
    tester_utils "github.com/bootcraft-cn/tester-utils"
    "github.com/bootcraft-cn/tester-utils/tester_definition"
)

func main() {
    definition := tester_definition.TesterDefinition{
        TestCases: []tester_definition.TestCase{
            {
                Slug:          "hello",
                RequiredFiles: []string{"hello.c"},
                CompileStep: &tester_definition.CompileStep{
                    Language: "c", Source: "hello.c", Output: "hello",
                },
                TestFunc: testHello,
            },
        },
    }
    os.Exit(tester_utils.Run(os.Args[1:], definition))
}
```

## 四阶段测试流水线

每个 `TestCase` 依次经过四个阶段。任意阶段失败后，跳过后续阶段，执行 `TeardownFuncs` 并上报错误。

```
阶段 1: RequiredFiles  →  阶段 2: CompileStep  →  阶段 3: BeforeFunc  →  阶段 4: TestFunc
（文件存在性检查）          （编译，30s 超时）        （自定义钩子 + panic 恢复）   （实际测试）
```

所有阶段均通过零值语义按需启用（`nil`/空 = 跳过）。

```go
tester_definition.TestCase{
    Slug:          "hello",
    RequiredFiles: []string{"hello.c"},                        // 阶段 1
    CompileStep: &tester_definition.CompileStep{               // 阶段 2
        Language: "c", Source: "hello.c", Output: "hello",
        IncludeParentDir: true,  // 添加 -I.. 以引用上层公共头文件
    },
    BeforeFunc: func(h *test_case_harness.TestCaseHarness) error {  // 阶段 3
        // 自定义初始化（如启动服务器、准备测试文件）
        return nil
    },
    TestFunc: testHello,                                       // 阶段 4
}
```

### CompileStep

| 语言     | 行为                                                     | 示例                |
| -------- | -------------------------------------------------------- | ------------------- |
| `"c"`    | `clang -o {Output} {Source} -lm -Wall -Werror` + `Flags` | C 阶段              |
| `"make"` | `make {Output}`                                          | speller（Makefile） |

默认 C 编译参数（`-lm -Wall -Werror`）始终生效；`Flags` 用于追加额外参数。

## Runner 包

用于测试程序的流式 API：

```go
import "github.com/bootcraft-cn/tester-utils/runner"

// 阻塞模式 — 发送 stdin，检查 stdout + 退出码
err := runner.Run(workDir, "hello").
    Stdin("Alice").
    Stdout("hello, Alice").
    Exit(0).
    Error()

// 交互模式 — 测试输入拒绝逻辑
err := runner.Run(workDir, "mario").
    Start().
    SendLine("-1").Reject().        // 期望程序重新提示输入
    SendLine("4").Stdout("#####").
    Exit(0).
    Error()

// PTY 模式
err := runner.Run(workDir, "mario").
    WithPty().
    Stdin("5").Stdout("#####").
    Exit(0).
    Error()

// 编译 C 源文件
err := runner.CompileC(workDir, "hello.c", "hello", "-I..")
```

## CLI 用法

```bash
./tester              # 运行所有测试
./tester hello        # 运行指定阶段
./tester -s hello     # 同上，使用参数形式
./tester -d ./work    # 指定工作目录
./tester --help       # 显示帮助
```

## 环境变量

| 变量                         | 说明                                         |
| ---------------------------- | -------------------------------------------- |
| `BOOTCRAFT_REPOSITORY_DIR`  | 工作目录（默认：`.`）                        |
| `BOOTCRAFT_STAGE`           | 按 slug 运行单个阶段（调试用）               |
| `BOOTCRAFT_TEST_CASES_JSON` | 完整 JSON 测试用例列表（Worker 派发时使用）  |
| `BOOTCRAFT_RANDOM_SEED`     | 固定随机种子，用于确定性随机数               |
| `BOOTCRAFT_SKIP_ANTI_CHEAT` | 设为 `true` 跳过反作弊测试用例               |
| `BOOTCRAFT_STREAM_LOGS`     | 设为 `1` 禁用颜色并将 stdout 重定向至 stderr |
| `BOOTCRAFT_RECORD_FIXTURES` | 设为 `true` 录制/更新测试 fixture            |

## 文档

- [GoDoc](https://pkg.go.dev/github.com/bootcraft-cn/tester-utils)
- [四阶段流水线设计](docs/4-phase-pipeline/design.md)

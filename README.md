# BootLab Tester Utils

BootLab 课程测试工具的共享框架模块。

**基于：** [codecrafters-io/tester-utils](https://github.com/codecrafters-io/tester-utils)

**主要改进：**

1. **灵活的运行模式** - 支持三种模式：
   - 完整 JSON 格式（平台调度）
   - 单个 stage 测试（开发调试）
   - 运行全部测试（本地自测，默认）

2. **CLI 参数支持** - 新增命令行参数解析：
   - 位置参数：`./tester hello`
   - 标志参数：`./tester -s hello -d ~/work`
   - 帮助/版本：`--help`, `--version`

3. **可选配置文件** - `bootlab.yml` 为可选（有合理默认值），而非强制要求

4. **默认工作目录** - `BOOTLAB_REPOSITORY_DIR` 默认为当前目录 `.`，无需显式设置

5. **新增 SubmissionDir** - `TestCaseHarness` 暴露学员提交目录，方便访问相对路径

6. **改进的 Runner API** - 增强的程序测试 API，支持本地可执行文件自动检测

---

## 快速开始

```go
package main

import (
    "os"
    tester_utils "github.com/bootlab-dev/tester-utils"
)

func main() {
    definition := GetDefinition() // 你的测试定义
    os.Exit(tester_utils.Run(os.Args[1:], definition))
}
```

## CLI 使用

```bash
# 运行所有测试
./tester

# 运行指定 stage
./tester hello
./tester -s hello
./tester --stage hello

# 指定工作目录
./tester -d ./my-solution hello

# 查看帮助
./tester --help
```

## Runner 包

流式 API 用于测试程序（类似 check50）：

```go
import "github.com/bootlab-dev/tester-utils/runner"

// 基本用法
err := runner.Run("./hello").
    Stdin("Alice").
    Stdout("hello, Alice").
    Exit(0)

// PTY 支持
err := runner.Run("./mario").
    WithPty().
    Stdin("5").
    Stdout("#####").
    Exit(0)

// 测试输入拒绝
err := runner.Run("./mario").
    Stdin("-1").
    Reject()
```

## 环境变量

**流式日志支持** (Worker 集成):

- `BOOTLAB_STREAM_LOGS=1` - 禁用颜色并将 stdout 重定向到 stderr，便于 Worker 捕获实时日志流

## 文档

详细 API 文档请查看 [GoDoc](https://pkg.go.dev/github.com/bootlab-dev/tester-utils)。

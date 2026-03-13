# tester-utils 改进实施 TODO

> 版本: v1.1 | 日期: 2026-02-25 | 状态: ✅ 完成

## 整体排期估算

| 阶段 | 工作量 | 依赖 |
|------|--------|------|
| Phase 1: 框架层改造 | 4h | 无 |
| Phase 2: llm100x-tester 迁移 | 4.5h | Phase 1 |
| Phase 3: CI 验证 | 1.5h | Phase 2 |
| **总计** | **~10h** | |

> **注**：工时按首次实现估算（含学习与调试），×1.5 安全系数已包含。熟练者可压缩至 ~7h。

---

## Phase 1: tester-utils 框架层改造

### T1.1 新增 CompileStep 和 BeforeFunc 类型定义
- **文件**: `tester_definition/tester_definition.go`
- **内容**:
  - 新增 `CompileStep` 结构体（Language, Source, Output, Flags, IncludeParentDir）
  - `TestCase` 增加 3 个可选字段：`RequiredFiles []string`, `CompileStep *CompileStep`, `BeforeFunc`
- **约束**: 所有字段零值即不执行，向后兼容
- **工时**: 0.5h
- **状态**: ✅ 完成

### T1.2 新增 prechecks.go
- **文件**: `test_runner/prechecks.go`（新建）
- **内容**:
  - `checkRequiredFiles(harness, files) error` — 遍历文件列表，调用 `harness.FileExists()`
  - 日志格式：`Checking {filename} exists...` → `{filename} exists` 或 error
- **工时**: 0.5h
- **状态**: ✅ 完成

### T1.3 新增 compile.go + safeguards.go
- **文件**: `test_runner/compile.go`（新建）, `test_runner/safeguards.go`（新建）
- **compile.go 内容**:
  - `runCompileStep(harness, compileStep) error` — 根据 Language 分发
  - `runCompileStepWithTimeout(harness, compileStep, timeout) error` — 带超时保护（默认 30s）
  - `compileC(workDir, compileStep) error` — 调用 clang，默认 flags: `-lm -Wall -Werror`
  - `compileMake(workDir, target) error` — 调用 make
  - 日志格式：`Compiling {source}...` → `{source} compiles` 或 error
- **safeguards.go 内容**:
  - `safeRunBeforeFunc(harness, fn) error` — 用 `defer/recover` 防止 BeforeFunc panic 导致进程崩溃
- **设计要点**:
  - 默认 flags 内置，`CompileStep.Flags` 追加（append，非覆盖）
  - `IncludeParentDir=true` 时追加 `-I..`
  - 编译错误包含完整的 compiler output
  - 与已有 `runner.CompileC()` 共存：框架层用内置 `compileC()`，复杂场景在 BeforeFunc 中调用 `runner.CompileC()`
- **工时**: 1.5h
- **状态**: ✅ 完成

### T1.4 改造 TestRunner.Run()
- **文件**: `test_runner/test_runner.go`
- **内容**: 在 `TestFunc` 执行前插入 Phase 1-3
  ```
  Phase 1: checkRequiredFiles (if RequiredFiles non-empty)
  Phase 2: runCompileStep     (if CompileStep non-nil)
  Phase 3: BeforeFunc          (if BeforeFunc non-nil)
  Phase 4: TestFunc            (原有逻辑)
  ```
- **关键行为**:
  - Phase 1-3 任何一步失败 → 调用 `RunTeardownFuncs()` → `return false`
  - Phase 2 有独立 30s 编译超时（`runCompileStepWithTimeout`），防止编译 hang
  - Phase 3 有 `defer/recover` 保护（`safeRunBeforeFunc`），防止 BeforeFunc panic 崩溃整个进程
  - Phase 1-3 使用 `harness.Logger`，自动适配 quiet 模式（anti-cheat 测试）
  - Phase 4 保持原有 goroutine + timeout 机制不变
- **工时**: 0.5h
- **状态**: ✅ 完成

### T1.5 单元测试
- **文件**: `test_runner/test_runner_test.go`（**新建**，当前不存在）
- **测试场景**:
  - [x] RequiredFiles 全通过 → TestFunc 正常执行
  - [x] RequiredFiles 有缺失 → TestFunc 不执行，返回 false
  - [x] CompileStep 编译成功 → TestFunc 正常执行
  - [x] CompileStep 编译失败 → TestFunc 不执行
  - [x] BeforeFunc 返回 nil → TestFunc 正常执行
  - [x] BeforeFunc 返回 error → TestFunc 不执行
  - [x] BeforeFunc panic → 被 recover 捕获，转为 error，TestFunc 不执行
  - [x] CompileStep 超时 → 返回超时错误，TestFunc 不执行
  - [x] 所有新字段为零值 → 行为与改造前完全一致
  - [x] Teardown 在 Phase 失败时仍被调用
  - [x] Quiet 模式下 Phase 1-3 日志被正确抑制
- **工时**: 1h
- **状态**: ✅ 完成（23 个单元测试全部通过）

---

## Phase 2: llm100x-tester 迁移

### T2.1 简单 C stages 迁移（13 个）
- **文件**: `internal/stages/` 下的 hello.go, mario_less.go, mario_more.go, cash.go,
  credit.go, scrabble.go, readability.go, caesar.go, substitution.go,
  plurality.go, runoff.go, tideman.go, volume.go
- **迁移模式**:
  - 提取 `FileExists` + `CompileC` 为 `RequiredFiles` + `CompileStep`
  - 删除 TestFunc 中的样板代码
  - TestFunc 只保留纯测试逻辑
- **示例** (hello.go):
  ```go
  // Before:
  func testHello(h) {
      if !h.FileExists("hello.c") { ... }  // 删除
      CompileC(workDir, "hello.c", ...)     // 删除
      runner.Run(...).Stdin("Emma")...       // 保留
  }

  // After:
  RequiredFiles: []string{"hello.c"},
  CompileStep: &CompileStep{Language:"c", Source:"hello.c", Output:"hello", IncludeParentDir:true},
  TestFunc: func(h) { runner.Run(...) }
  ```
- **工时**: 1.5h（机械重构，每个 ~7 分钟）
- **状态**: ✅ 完成（13 个 stage 全部迁移）

### T2.2 复杂 C stages 迁移（4 个）
- **文件**: filter_less.go, filter_more.go, speller.go, recover.go
- **迁移模式**:
  - filter-less/more: RequiredFiles 提取多文件，编译逻辑复杂（特殊 clang flags）→ 用 BeforeFunc
  - speller: RequiredFiles=["dictionary.c"], CompileStep={Language:"make", Output:"speller"}
  - recover: RequiredFiles=["recover.c","card.raw"], 编译用 BeforeFunc（特殊 flags）
- **工时**: 0.5h
- **状态**: ✅ 完成（4 个 stage 全部迁移）

### T2.3 Python stages 迁移（7 个）
- **文件**: sentimental_hello.go, sentimental_mario_less.go, sentimental_mario_more.go,
  sentimental_cash.go, sentimental_credit.go, sentimental_readability.go, dna.go
- **迁移模式**:
  - RequiredFiles=["hello.py"] (或 cash.py, dna.py 等)
  - 无 CompileStep
  - 删除 TestFunc 中的 `FileExists` 检查
- **工时**: 0.5h
- **状态**: ✅ 完成（7 个 stage 全部迁移）

### T2.4 SQL stages 迁移（3 个）
- **文件**: songs.go, movies.go, fiftyville.go
- **迁移模式**:
  - songs: RequiredFiles=["1.sql","2.sql",...,"7.sql","answers.txt"]
  - movies: RequiredFiles=["1.sql","2.sql",...,"13.sql"]
  - fiftyville: RequiredFiles=["log.sql","answers.txt"]
  - 无 CompileStep
- **工时**: 0.25h
- **状态**: ✅ 完成（3 个 stage 全部迁移）

### T2.5 特殊 stages 迁移（3 个）
- **文件**: sort.go, inheritance.go, finance.go
- **迁移模式**:
  - sort: RequiredFiles=["answers.txt"], 无其他变动
  - inheritance: RequiredFiles=["inheritance.c"], BeforeFunc 处理 main 重命名+组合编译
  - finance: RequiredFiles=["app.py"], BeforeFunc 处理 venv 探测 + Flask 服务启动
- **工时**: 0.25h
- **状态**: ✅ 完成（3 个 stage 全部迁移）

---

## Phase 3: CI 验证与清理

### T3.1 RequiredFiles 一致性验证
- **文件**: `testing/validate_tester_definition.go`（扩展现有函数）
- **内容**:
  - 扩展 `ValidateTesterDefinitionAgainstYAML`，检查 TestCase.RequiredFiles 与
    stage.yml 的 files_config.required 一致
  - 新增 `courseYAML` 结构体解析 `files_config` 字段
- **工时**: 0.5h
- **状态**: ⏭️ 跳过（评估后决定不做，投入产出比低）

### T3.2 清理 helpers/c_compiler.go
- **文件**: llm100x-tester `internal/helpers/c_compiler.go`
- **内容**:
  - 迁移后评估是否还有 stage 直接调用 `helpers.CompileC()`
  - 如果全部迁移到 CompileStep → 删除此文件
  - 如果部分复杂 stage 仍在 BeforeFunc 中调用 → 保留
- **工时**: 0.25h
- **状态**: ✅ 完成（零调用者，已删除）

### T3.3 回归测试
- **内容**:
  - 运行 `scripts/test-all-solutions.sh` 验证所有 stage 迁移后仍通过
  - 运行 `go test ./...` 确保单元测试通过
  - 手动测试至少 3 个代表性 stage（C/Python/SQL 各一个）
- **工时**: 0.25h
- **状态**: ✅ 完成（30/30 stage 全部通过）

---

## 依赖关系图

```
T1.1 ──→ T1.2 ──→ T1.4 ──→ T1.5 ──→ T2.1 ──→ T3.1
    ╲──→ T1.3 ──╱                ╲──→ T2.2 ──→ T3.2
                                  ╲──→ T2.3
                                  ╲──→ T2.4
                                  ╲──→ T2.5 ──→ T3.3
```

- T1.1 是所有后续任务的基础
- T1.2 和 T1.3 可并行
- T2.1-T2.5 可并行（互不依赖）
- T3 需要 T2 全部完成

---

## 后续规划（不在本次范围）

| 编号 | 任务 | 前置条件 | 优先级 |
|------|------|---------|--------|
| F1 | files_config 数据驱动（P1-B: TENSORHERO_FILES_CONFIG 环境变量） | Worker 支持注入 | P1 |
| F2 | files_config.allowed glob 白名单检查 | F1 | P1 |
| F3 | files_config.blocked glob 黑名单检查 | F1 | P1 |
| F4 | BeforeAll/AfterAll 全局 Hook | 有实际需求时 | P2 |
| F5 | 多语言测试支持（TestVariant） | 有支持多语言的课程时 | P2 |
| F6 | check50 风格依赖图（@check(dependency)） | 当前线性执行不够时 | P3 |

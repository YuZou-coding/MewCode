# MewCode 版本命令任务拆解

## 1. 建立版本模块测试

- 影响文件：`internal/version/version_test.go`
- 依赖任务：无
- 参考资料定位：Go linker `-X` 注入规则
- 内容：覆盖开发默认值和版本展示格式。

## 2. 实现版本模块

- 影响文件：`internal/version/version.go`
- 依赖任务：1
- 参考资料定位：`internal` 目录现有包组织方式
- 内容：提供可由 linker 覆盖的版本值和统一展示方法。

## 3. 建立命令行为测试

- 影响文件：`internal/command/command_test.go`
- 依赖任务：2
- 参考资料定位：`TestBuiltinsDispatchLocalAndAIPrompt`、`TestBuiltinsCompletionIncludesWorkers`
- 内容：覆盖注册、帮助、补全、无别名和默认输出。

## 4. 注册版本命令

- 影响文件：`internal/command/builtin.go`
- 依赖任务：2、3
- 参考资料定位：`Builtins`、`statusHandler`
- 内容：按现有本地命令模式注册并分发版本查询。

## 5. 更新使用与构建文档

- 影响文件：`README.md`
- 依赖任务：4
- 参考资料定位：命令系统、安装章节
- 内容：补充版本命令及 linker flags 注入示例。

## 6. 接入主流程

- 影响文件：`internal/command/builtin.go`、`cmd/mewcode/main.go`
- 依赖任务：1-5
- 参考资料定位：命令注册初始化与 TUI 命令分发流程
- 内容：确认版本命令通过现有注册中心自动进入交互式与 fallback 主流程，无需新增旁路。

## 7. 端到端验证

- 影响文件：`internal/command/command_test.go`、`README.md`
- 依赖任务：1-6
- 参考资料定位：完整 Go 测试、`go run` linker flags
- 内容：运行定向测试、全量测试，并实际构建注入版本后验证命令输出。

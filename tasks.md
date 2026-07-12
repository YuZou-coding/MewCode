# MewCode 三档权限模式任务拆解

## 1. 配置与模式类型

- 影响文件：`internal/config/config.go`、`internal/permissions/types.go`
- 依赖任务：无
- 参考资料定位：`Config.Parse`、`Checker`
- 内容：增加并校验 `permission_mode`，定义严格、默认、YOLO 三种模式。

## 2. 权限决策模式化

- 影响文件：`internal/permissions/checker.go`、`internal/permissions/rules.go`
- 依赖任务：1
- 参考资料定位：`Checker.Check`、`DecideByRules`
- 内容：保留硬边界，实现 deny 优先及三档模式决策。

## 3. 接入运行时与 worker

- 影响文件：`internal/app/app.go`
- 依赖任务：1、2
- 参考资料定位：`App.Run`、`cloneChecker`
- 内容：从配置初始化检查器，复制当前模式到后续 worker。

## 4. 接入权限命令与确认界面

- 影响文件：`internal/command/builtin.go`、`internal/tui/loop.go`、`internal/tuiapp/model.go`、`internal/tuiapp/panels.go`
- 依赖任务：2、3
- 参考资料定位：`/permissions`、`permissionPrompt`、`handlePermissionKey`
- 内容：提供模式切换与 reset；严格模式仅支持本次允许或拒绝。

## 5. 更新文档与示例配置

- 影响文件：`README.md`、`mewcode.example.yaml`、`mewcode.openai.example.yaml`
- 依赖任务：4
- 参考资料定位：权限章节、配置示例
- 内容：说明模式语义、边界和命令入口。

## 6. 端到端验证

- 影响文件：`internal/config/config_test.go`、`internal/permissions/permissions_test.go`、`internal/agent/agent_test.go`、`internal/tui/loop_test.go`、`internal/tuiapp/model_test.go`
- 依赖任务：1-5
- 参考资料定位：现有权限、TUI、Agent 测试
- 内容：验证配置、决策矩阵、模式切换、确认面板和全量测试。

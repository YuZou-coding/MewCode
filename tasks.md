# MewCode Claude Code 风格全屏 TUI 任务拆解

## 1. 固化 TUI 视觉状态与回归基线

- 影响文件：`internal/tuiapp/model_test.go`
- 依赖任务：无
- 参考资料定位：`New`、`View`、`renderBlock`、`statusLine`、`inputLine`
- 内容：为宽屏、标准宽度和窄屏建立布局断言，覆盖欢迎块、流式正文、工具状态、权限面板、命令面板和状态行。

## 2. 拆分并集中主题定义

- 影响文件：`internal/tuiapp/theme.go`、`internal/tuiapp/model.go`
- 依赖任务：1
- 参考资料定位：`model.go` 内所有 `lipgloss.NewStyle` 和颜色常量
- 内容：集中定义品牌色、语义色、弱文本、符号和降级策略，移除渲染函数中的零散样式创建。

## 3. 拆分 transcript 数据与渲染

- 影响文件：`internal/tuiapp/transcript.go`、`internal/tuiapp/model.go`、`internal/tuiapp/model_test.go`
- 依赖任务：1、2
- 参考资料定位：`TranscriptBlock`、`appendAssistantDelta`、`renderTranscript`、`renderBlock`
- 内容：将 block 更新和纯文本/样式渲染移出 Model，保留 `Transcript()` 测试兼容接口。

## 4. 实现轻量单列 transcript 视觉层级

- 影响文件：`internal/tuiapp/transcript.go`、`internal/tuiapp/theme.go`
- 依赖任务：3
- 参考资料定位：结构化 block 类型和流事件处理
- 内容：实现用户、助手、thinking、工具、usage、error、system 的符号、缩进和间距；欢迎块保持完整显示。

## 5. 完善工具状态摘要与失败展开

- 影响文件：`internal/tuiapp/model.go`、`internal/tuiapp/transcript.go`、`internal/app/app.go`
- 依赖任务：3、4
- 参考资料定位：`StreamToolStart`、`StreamToolResult`、Agent 到 TUI 事件桥接
- 内容：跟踪工具目标、状态和耗时；成功显示紧凑摘要，失败、权限拒绝和 Hook 拦截显示可读原因。

## 6. 实现 viewport 自动跟随控制

- 影响文件：`internal/tuiapp/model.go`、`internal/tuiapp/layout.go`、`internal/tuiapp/model_test.go`
- 依赖任务：3
- 参考资料定位：`refresh`、Bubble Tea viewport 键盘消息
- 内容：默认跟随新输出；用户向上滚动后保持位置，新输出到达时显示返回底部提示，并支持快捷返回。

## 7. 升级命令面板键盘交互

- 影响文件：`internal/tuiapp/panels.go`、`internal/tuiapp/model.go`、`internal/command/completion.go`、`internal/tuiapp/model_test.go`
- 依赖任务：2
- 参考资料定位：`commandPanel`、`complete`、`Registry.PanelItems`
- 内容：增加高亮索引、上下选择、实时过滤、Tab 补全、Enter 执行和 Esc 关闭；面板最多展示八项。

## 8. 升级权限面板与决策留痕

- 影响文件：`internal/tuiapp/panels.go`、`internal/tuiapp/model.go`、`internal/tuiapp/model_test.go`
- 依赖任务：2、3
- 参考资料定位：`permissionLine`、`handlePermissionKey`、`permissions.HITLChoice`
- 内容：渲染工具、目标、原因和操作区；处理长文本自适应截断；选择后追加简洁 permission transcript block。

## 9. 重组整体布局与响应式状态行

- 影响文件：`internal/tuiapp/layout.go`、`internal/tuiapp/theme.go`、`internal/tuiapp/model.go`、`internal/command/types.go`、`internal/tui/loop.go`
- 依赖任务：4、6、7、8
- 参考资料定位：`View`、`statusLine`、`inputLine`、`command.State`、`tui.Controller.Status`
- 内容：组合极简 header、viewport、互斥面板、输入/activity 和细状态行；从会话与 Git 环境提供可用的 context、分支状态，并按宽度逐项隐藏低优先级信息；无法取得的数据不渲染占位值。

## 10. 接入主流程

- 影响文件：`internal/tuiapp/model.go`、`internal/tuiapp/run.go`、`internal/app/app.go`
- 依赖任务：2-9
- 参考资料定位：全屏 TUI 启动分支、Agent 事件桥接和权限 prompt
- 内容：确认新组件使用现有事件流和控制器状态，fallback 行式 TUI 不受影响。

## 11. 更新使用文档

- 影响文件：`README.md`
- 依赖任务：7-10
- 参考资料定位：交互式 TUI、Tab 补全和权限命令章节
- 内容：说明命令面板键位、viewport 跟随提示、权限选择和长任务状态呈现。

## 12. 端到端验证

- 影响文件：`internal/tuiapp/model_test.go`、`internal/app/app_test.go`、`internal/e2e`
- 依赖任务：1-11
- 参考资料定位：现有 TUI 流式渲染、权限和会话回归测试
- 内容：验证完整流式轮次、工具成功与失败、权限决策、命令操作、滚动行为、中文编辑及全量测试。

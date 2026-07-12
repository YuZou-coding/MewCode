# MewCode MCP 状态栏计数规格

## 背景

MewCode 使用 lazy discovery，启动时只读取 MCP 配置，需要外部能力时才初始化对应 server。当前状态栏只显示已连接数量，用户无法区分“尚未连接”和“没有配置”。

## 目标用户

- 配置多个 MCP server 并希望观察 lazy discovery 状态的终端用户。
- 排查 MCP 配置是否加载、候选 server 是否被初始化的开发者。

## 能力清单

- 状态栏同时显示已初始化 MCP server 数量和已配置 MCP server 总数。
- 启动后、尚未发现任何 MCP 工具时，已连接数量为零，已配置数量反映加载结果。
- MCP server 成功初始化并进入进程缓存后，已连接数量随状态刷新增长。
- 没有 MCP 配置时，两个数量都显示为零。

## 非功能要求

- 保持 lazy discovery，不因展示配置总数而提前连接 MCP server。
- 状态数据保持结构化，由控制器提供，UI 不直接依赖 MCP manager。
- 状态栏延续现有响应式宽度规则，不增加新的终端行。

## 设计骨架

- MCP manager 分别提供缓存连接数量与配置数量。
- 控制器把两个数量写入通用 UI 状态。
- 状态栏在已有 MCP 区域渲染“已连接/已配置”。

## 不做范围

- 不显示单个 MCP server 名称或连接错误。
- 不改变 MCP 初始化、断线重连或工具发现行为。
- 不新增独立 MCP 状态页面。

## 完成定义

见 [checklist.md](checklist.md)。

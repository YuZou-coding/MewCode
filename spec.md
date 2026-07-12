# MewCode MCP 标准兼容与 HTTP 认证规格

## 背景

MewCode 当前发送的 MCP 初始化请求不符合标准协议，标准 server 会因缺少必要字段而拒绝握手。HTTP transport 也无法携带认证 Header，导致需要令牌的远端 server 返回未认证错误。与此同时，外部 server 故障不应拖慢或阻断终端 Agent 启动。

## 目标用户

- 使用标准 stdio 或 Streamable HTTP MCP server 的 MewCode 用户。
- 需要通过环境变量安全配置 HTTP MCP 认证信息的开发者。
- 配置多个外部 server，并要求单个 server 故障不影响主流程的用户。

## 能力清单

- MewCode 按 MCP 标准完成初始化请求、版本协商和初始化完成通知。
- stdio 与 HTTP transport 复用同一套 MCP 生命周期语义。
- HTTP server 可配置任意请求 Header。
- Header 值可引用运行环境中的变量，避免在配置文件中保存认证令牌。
- Header 环境变量缺失时，对应 server 返回可诊断错误，其他 server 和 MewCode 主流程继续运行。
- 启动时只加载 MCP server 配置，按需发现时才连接、认证和发现工具。
- 同一会话内复用已成功初始化的连接和工具发现结果。
- 所有用户可见错误和诊断输出隐藏 Header、环境变量展开值及认证令牌。
- 文档提供仅使用 HTTP transport 的 Context7 配置示例。

## 非功能要求

- 遵循 MCP 初始化生命周期和 JSON-RPC 消息顺序。
- 不改变用户级与项目级 server 配置的合并和同名覆盖规则。
- 认证数据不得进入日志、工具结果或错误文本。
- 单个 server 的配置、认证、握手或发现失败不得阻断启动和其他 server。
- 协议、配置、transport、失败隔离和脱敏行为均由自动化测试覆盖。

## 设计骨架

- 外部 Client 负责 MCP 初始化状态机、服务端结果校验和工具调用前置条件。
- JSON-RPC 调用边界同时支持有响应的请求和无响应的通知，两种 transport 保持一致行为。
- 配置加载层解析 Header 映射并标记环境变量引用；连接建立时从进程环境解析实际值。
- HTTP transport 仅负责把已解析 Header 附加到每个请求，不记录或拼接敏感值到错误中。
- 外部管理器按需创建 Client，并以逐 server 结果隔离连接、认证和发现错误。
- 已成功协商的协议版本和连接状态只保存在当前会话内。

## 不做范围

- 不实现 OAuth 登录、浏览器授权或令牌刷新。
- 不把认证令牌写入 MewCode 配置、缓存或会话文件。
- 不支持 Header 值中的复合模板、默认值表达式或命令替换。
- 不同时保留 Context7 的 HTTP 与 stdio 示例配置。
- 不持久化 MCP 连接或工具发现状态。

## 完成定义

见 [checklist.md](checklist.md)。

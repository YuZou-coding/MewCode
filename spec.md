# MewCode 通用 MCP 工具路由规格

## 背景

MewCode 已支持 lazy MCP discovery：启动时只加载 MCP 配置，模型需要远端能力时通过 `tool_search` 搜索并加载工具。但当前模型只得到一条泛化提示，容易在需要外部能力时直接依赖记忆或本地 shell，不能稳定判断何时应该先发现 MCP 工具。

这不是 React 或 Context7 的单点问题，而是所有通用 MCP 能力的路由问题：官方文档、浏览器、数据库、Issue 系统、云服务、知识库、设计系统、SaaS API 等都需要被模型主动发现和选择。

## 目标用户

- 在 MewCode 中配置多个 MCP server 的开发者。
- 希望模型自动选择合适外部能力，而不是手动提示具体工具名的用户。
- 需要最新文档、外部系统状态或专用数据源的终端 Coding Agent 用户。

## 能力清单

- 模型在任务需要外部系统、当前信息、官方文档、远端数据源或专用服务时，会优先考虑 `tool_search`。
- MCP server 配置可声明描述、能力分类、关键词和示例任务，帮助 lazy discovery 在不连接全部 server 前做粗筛。
- `tool_search` 同时利用 server 元数据和远端工具描述做匹配。
- `tool_search` 返回候选工具的推荐顺序、匹配原因和能力分类，帮助模型选择下一步工具。
- 多个候选工具同时匹配时，结果按相关性排序；模型默认选择推荐项，高风险或同分歧义时再询问用户。
- 找不到合适 MCP 工具时，模型必须明确说明未发现合适工具，再使用可用上下文或模型知识回退。
- 现有无元数据 MCP 配置继续可用，保持向后兼容。

## 非功能要求

- 不为 React、Context7 或任何单一 server 写死特殊逻辑。
- 不在启动阶段连接 MCP server，保持 lazy discovery。
- 匹配和排序逻辑应可测试、可解释、稳定。
- 新元数据不能影响认证 Header、环境变量解析或已有 stdio/http transport 行为。
- 工具返回不泄露密钥、Header 值或环境变量内容。

## 设计骨架

- 稳定系统提示增加通用 MCP 路由规则：遇到外部能力需求时先发现工具，不能静默假装查过。
- MCP server 配置增加可选元数据字段：`description`、`capabilities`、`keywords`、`examples`。
- `tool_search` 先根据 server 名称和元数据粗筛候选 server；必要时再连接候选 server 获取远端工具列表。
- 匹配结果对每个工具返回本地工具名、server、远端描述、能力分类、匹配原因、分数和推荐标记。
- 保持精确选择 `select:external_<server>_<tool>` 的行为：只连接匹配 server，无法解析 server 时返回 `tool_not_found`。

## 不做范围

- 不实现自动联网搜索 MCP marketplace。
- 不自动安装或修改用户 MCP server 配置。
- 不改变 MCP 协议生命周期、认证、transport 或 tool result 回灌机制。
- 不引入复杂向量检索或机器学习排序。
- 不改变权限系统对远端工具调用的检查。

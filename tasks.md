# MewCode MCP 标准兼容与 HTTP 认证任务拆解

## 1. 固化标准初始化消息测试

- 影响文件：`internal/external/client_test.go`、`internal/rpc/session_test.go`
- 依赖任务：无
- 参考资料定位：`Client.Initialize`、`rpcCaller`、`Session.Call`、`NewNotification`
- 内容：覆盖初始化必填字段、服务端协商结果、初始化完成通知顺序，以及通知无需响应的行为。

## 2. 扩展 JSON-RPC 通知发送能力

- 影响文件：`internal/rpc/session.go`、`internal/rpc/session_test.go`
- 依赖任务：1
- 参考资料定位：`Transport.Send`、`Session.Call`、`NewNotification`、`Encode`
- 内容：为 stdio session 增加只发送不等待响应的通知路径，复用现有编码和关闭状态处理。

## 3. 实现标准 MCP 初始化生命周期

- 影响文件：`internal/external/client.go`、`internal/external/client_test.go`
- 依赖任务：1、2
- 参考资料定位：`Client.Initialize`、`rpcCaller`、MCP initialization lifecycle、MewCode 版本读取
- 内容：发送标准初始化参数，解析并校验服务端协商版本与能力，随后发送初始化完成通知；失败时不进入已初始化状态。

## 4. 增加 Header 配置与环境变量解析

- 影响文件：`internal/external/config.go`、`internal/external/config_test.go`
- 依赖任务：无
- 参考资料定位：`ServerConfig`、`LoadServersFile`、`assignConfigField`、现有 `env` 嵌套映射解析
- 内容：解析 HTTP `headers` 映射，支持完整 `${ENV_NAME}` 引用并在连接前展开；缺失变量返回只包含变量名和 server 名的错误。

## 5. 将认证 Header 接入 HTTP transport

- 影响文件：`internal/external/http_transport.go`、`internal/external/http_transport_test.go`、`internal/external/client.go`
- 依赖任务：4
- 参考资料定位：`NewHTTPTransport`、`SendAndReceive`、`NewClientFromConfig`
- 内容：把展开后的 Header 附加到该 server 的每个 HTTP 请求，保留协议必需的 Content-Type 与 Accept，并验证自定义值不会串到其他 transport。

## 6. 建立敏感信息脱敏边界

- 影响文件：`internal/external/config.go`、`internal/external/http_transport.go`、`internal/external/client.go`、`internal/e2e/external_tools_e2e_test.go`
- 依赖任务：4、5
- 参考资料定位：HTTP 非成功状态错误、配置展开错误、`Manager.Client`、`ToolSearch.Execute`
- 内容：确保配置、HTTP 状态、JSON-RPC 和逐 server 搜索错误均不包含 Header 值、环境变量值或令牌；保留状态码、server 名和缺失变量名用于诊断。

## 7. 更新配置与 Context7 文档

- 影响文件：`README.md`、`mewcode.example.yaml`、`mewcode.openai.example.yaml`
- 依赖任务：3、4、5、6
- 参考资料定位：README 外部工具服务器章节、示例配置中的外部 server 路径说明
- 内容：记录标准握手、按需搜索、Header 环境变量语法和失败隔离；Context7 只给出 HTTP 配置示例，不写入真实令牌。

## 8. 接入主流程

- 影响文件：`internal/external/manager.go`、`internal/external/register.go`、`internal/external/tool_search.go`、`internal/app/app.go`、`internal/agent/agent.go`、对应测试文件
- 依赖任务：2-7
- 参考资料定位：`Manager.Client`、`ToolSearch.Execute`、`App.Run`、Agent 每轮工具定义构建
- 内容：将标准握手和认证 transport 接入按需搜索流程，确认启动零连接、逐 server 失败隔离、成功连接缓存和后续远端工具调用保持一致。

## 9. 端到端验证

- 影响文件：`internal/e2e/external_tools_e2e_test.go`、`checklist.md`
- 依赖任务：1-8
- 参考资料定位：现有 stdio 与 HTTP 外部工具测试、Context7 HTTP 行为
- 内容：以测试 server 验证 stdio 标准消息顺序、HTTP Header 认证、环境变量缺失、401 脱敏、其他 server 继续工作、缓存复用和全量测试；在可用凭据环境中手工验证 Context7 HTTP 配置。

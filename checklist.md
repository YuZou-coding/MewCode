# MewCode 三档权限模式验收清单

- [ ] 不配置 `permission_mode` 时，解析结果为 `default`。
- [ ] `permission_mode: strict`、`default`、`yolo` 均可解析；`unsafe` 返回 `invalid permission_mode: unsafe`。
- [ ] `strict` 下带 allow 规则的常规工具调用仍触发确认。
- [ ] `default` 下命中 allow 规则的工具调用不触发确认。
- [ ] `yolo` 下未命中规则的常规工具调用不触发确认。
- [ ] 任意模式下，命中 deny 规则的工具调用返回拒绝。
- [ ] 任意模式下，`rm -rf /` 返回 `dangerous_command`，项目外路径返回 `path_outside_sandbox`。
- [ ] `/permissions` 显示当前模式、启动默认模式和三类规则数量。
- [ ] `/permissions mode yolo`、`strict`、`default` 可在当前会话切换；`/permissions mode reset` 恢复配置模式。
- [ ] 严格模式的行式和全屏权限提示只显示 `n deny` 与 `y once`，输入 `s` 不创建会话规则。
- [ ] 后续启动的 worker 继承当前权限模式，但不共享主会话的临时规则。
- [ ] `go test -count=1 ./...` 通过。

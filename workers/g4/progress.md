# G4 (Claude) progress

- T13 完成：提交 7e41be830（checkpoint 682d62702 为主体，补透传路径无 beta 头时的 sanitize）。定向测试 + Bedrock/Beta/Vertex 回归通过。
- T14 完成：已移植 1a3ecd2b9、7a09a2eaf/748b84a15（去掉 Grok 部分）、efa5a2240、da4419e62；跳过 27600b1d2（本地未移植）。定向测试、Bedrock/T13 回归、service 全量（默认与 -tags=unit）无 FAIL 行。
- T26 完成：移植 f654c23eb。思维块过滤先用 gjson 借用视图判断是否需要过滤，不需要则原样返回；需要过滤时仍走原解码路径，输出字节不变。对照基准（450KB 签名历史，Apple M 系列）：Decoded 约 0.85-0.90 ms、904 KB、10629 allocs；Precheck 约 0.63-0.76 ms、0 B、0 allocs。差分测试、畸形 JSON 测试、20 秒 fuzz 均通过。

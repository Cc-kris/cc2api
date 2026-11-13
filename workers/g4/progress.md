# G4 (Claude) progress

- T13 完成：提交 7e41be830（checkpoint 682d62702 为主体，补透传路径无 beta 头时的 sanitize）。定向测试 + Bedrock/Beta/Vertex 回归通过。
- T14 完成：已移植 1a3ecd2b9、7a09a2eaf/748b84a15（去掉 Grok 部分）、efa5a2240、da4419e62；跳过 27600b1d2（本地未移植）。定向测试、Bedrock/T13 回归、service 全量（默认与 -tags=unit）无 FAIL 行。

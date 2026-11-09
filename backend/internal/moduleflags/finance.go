// Package moduleflags 集中存放按业务模块划分的编译期总开关。
//
// 这些开关用来「整体摘除」一个业务模块，而不是给单个接口加运行时判断：
// 关闭后，对应的 HTTP 路由不会被注册（请求直接 404），后台定时任务/自动化
// 流程也不会启动，前端入口同样不展示。
package moduleflags

// FinanceEnabled 是财务板块（Business & Finance）的总开关。
//
// [FINANCE-OFF] 2026-11-09：用户决定不再使用财务板块，这里硬关闭。
// 关闭效果：
//   - 后端不注册 /admin/finance/*、/admin/upstream-finance-protocols/*，
//     以及上游钱包的资金/汇率/成本相关端点；
//   - FinanceRuntime、UpstreamFinanceSyncRuntime 不启动（无 ticker、无 goroutine）；
//   - 前端不展示财务菜单与路由。
//
// 历史表结构与数据全部保留，恢复时把这里改成 true 重新编译即可。
const FinanceEnabled = false

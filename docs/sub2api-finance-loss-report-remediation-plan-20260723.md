# CCAI 财务报表与上游成本核算整改方案

## 文档信息

- 版本：v1.4
- 创建日期：2026-07-23
- 最后更新日期：2026-07-24
- 范围：CCAI 管理端财务报表、上游成本配置、上游模型定价同步、上游余额同步、历史盈亏回算
- 交付目标：管理员打开财务报表后，直接看到当前亏损、历史亏损、亏损来源、成本数据完整度和上游余额状态
- 本文状态：可直接进入开发

## 一、结论

当前财务报表不能作为盈亏判断依据。问题不在图表，而在成本数据链路：报表把客户实际扣费再次乘以上游平台倍率当成上游成本，客户分组倍率被混入成本；已经建立的模型倍率表没有接入服务、查询和页面；时间筛选只作用于趋势，不作用于顶部汇总；订阅消费额被当成收入；上游充值额只有一笔初始余额，没有真实充值流水。

本方案采用“上游站点—结算钱包—路由账号—模型价格版本—请求财务记录”五层模型：

1. 上游站点继续复用账号中的 Base URL，不重复录入。
2. 每个 CCAI 路由账号自动建立一个结算钱包，管理员可以把共享同一余额和费率的账号合并到同一个钱包。
3. 上游模型价格按钱包和生效时间保存，支持不同账号、不同模型、不同时间使用不同倍率或单价。
4. 每条用量日志生成独立财务记录，保存当时实际使用的上游模型、价格版本、上游成本、收入、利润和数据质量。
5. 财务报表只汇总财务记录，不再使用客户扣费反推上游成本。
6. 钱包余额、API Key 配额与模型定价通过不同适配器同步；不支持财务接口的站点进入手工配置，不把 `/v1/models` 或 API Key 配额当作钱包余额。
7. 当前报表在新数据覆盖率达标前保留但标记为“旧口径”，新报表达到启用门槛后替换 `/admin/finance`。
8. 保留现有 `accounts.rate_multiplier` 作为“账号计费倍率”，继续用于账号额度、账号用量统计和告警；该字段不进入财务利润计算。
9. 账号新增和编辑页新增独立的 `upstream_cost_multiplier`“上游倍率”；管理员只在账号详情维护，财务页不再提供倍率录入。
10. 每条请求在路由账号选定时锁定 `accounts.upstream_cost_multiplier`，并随用量日志写入 `usage_logs.upstream_cost_multiplier`；账号后来修改倍率只影响修改后选中该账号的新请求，不改变在途请求和历史盈亏。
11. 财务扫描、历史回算和报表汇总只读取用量日志中的倍率快照及请求发生时有效的价格版本，禁止关联账号当前倍率重算旧账。

## 二、完成标准

整改完成必须同时满足以下条件：

1. 财务页默认展示本月数据，顶部直接显示本月净利润或净亏损、今天净利润或净亏损、收入、上游成本、毛利率、成本覆盖率和未定价请求数。
2. 任意日期范围的顶部汇总、趋势、排行和明细使用同一时间条件。
3. 每一笔已确认上游成本都能追溯到用量日志、路由账号、结算钱包、实际上游模型和价格版本。
4. 价格修改只影响新请求；历史请求保持原结果。历史回算通过新的价格版本和明确的生效时间执行。
5. 未定价请求不再按客户收入估算成本，统一标记为“待定价”，并从已确认利润中剔除。
6. 财务页同时显示“已确认盈亏”和“未定价风险”，不把数据缺失展示成零成本或零利润。
7. 钱包余额自动同步失败不会覆盖最后一次成功余额；页面显示最后成功时间、失败原因、余额来源和数据类型。
8. 同一上游共享余额的多个账号只计算一次余额，不发生重复相加。
9. 余额消费、订阅收入、上游充值和现金流分开核算，不再把现金流差额命名为利润。
10. 新口径最近 7 天成本覆盖率达到 99%，抽样 100 条请求的人工复算误差不超过 0.000001 美元后，正式替换旧报表。
11. 账号新增、编辑和列表同时保留“账号计费倍率”和“上游倍率”，两者具有独立字段、独立帮助文案和独立计算职责。
12. 财务页只读展示上游倍率与来源，不存在第二个上游倍率输入入口。
13. 使用上游倍率计算成本时必须读取 `usage_logs.upstream_cost_multiplier` 快照；禁止使用 `account_rate_multiplier` 或账号当前值替代。
14. 同一请求无论执行多少次财务扫描或报表查询，使用的倍率、价格版本和计算结果必须保持一致；账号倍率变更不得引起历史利润漂移。
15. 老板进入财务首页后，无需切换页面即可看到今日、本月和所选期间的已确认盈亏、上一周期变化、主要亏损来源、上游资金可用天数和数据可信度。
16. 所有经营异常必须能够从预警直接下钻到客户、分组、模型、渠道、路由账号和单条使用记录，并保留处理状态与处理记录。

## 三、现状证据

### 3.1 当前代码口径

当前成本公式位于 `backend/internal/repository/upstream_repo.go`：

```text
普通请求上游成本 = usage_logs.actual_cost × upstream_platform_rates.rate_multiplier
```

`actual_cost` 是客户实际扣费，已经包含客户分组倍率或用户专属倍率。它不是上游基础成本。该公式会把客户销售价格当成采购成本基础。

现有“账号计费倍率”也不是上游倍率：

- `backend/ent/schema/account.go` 明确把 `accounts.rate_multiplier` 定义为账号维度计费倍率，不影响用户/API Key 扣费；
- 网关使用 `total_cost × account_rate_multiplier` 累加 API Key/Bedrock 账号额度、账号统计和额度告警；
- `usage_logs.account_rate_multiplier` 保存该账号统计倍率快照；
- 当前财务 SQL 使用的是 `upstream_platform_rates.rate_multiplier`，没有把账号计费倍率作为上游采购倍率。

因此，复用 `accounts.rate_multiplier` 会改变账号额度和统计口径，必须新增独立上游倍率字段。

当前实现还存在以下确定问题：

- `upstream_model_rates` 已在迁移 157 中创建，但 repository、service、handler 和前端均未读写该表。
- `GetFinanceStats` 的顶部汇总没有使用 `start`、`end`，趋势使用了日期范围，导致同一页面上下口径不同。
- 上游与历史请求通过账号“当前 Base URL”关联；账号删除或修改 Base URL 后，历史归属发生漂移或丢失。
- `account_stats_cost` 具备按账号和模型计算成本的基础能力，但当前覆盖率不足，且 Fast、按秒视频等计费维度未形成完整上游成本口径。
- 上游余额使用 `initial_balance - 计算成本`，没有上游充值流水和余额快照。
- 用户充值只统计部分卡密记录，没有覆盖全部支付订单、退款和订阅收入确认。

### 3.2 线上数据现状

2026-07-23 对 `ccai.xyz` 进行只读查询，得到以下结果：

| 项目 | 结果 |
| --- | ---: |
| 活跃 CCAI 上游账号 | 26 |
| 账号中不同 Base URL | 12 |
| 已建立上游记录 | 11 |
| 已填写初始余额的上游 | 1 |
| 已配置平台费率 | 3 |
| 已配置模型费率 | 0 |
| 账号费率不等于 1 的账号 | 0 |
| 最近 30 天用量日志 | 112,069 |
| 最近 30 天具有 `account_stats_cost` 的日志 | 3,725 |
| 最近 30 天无法关联现有上游记录的日志 | 11,377 |
| 最近 30 天余额计费日志 | 84,008 |
| 最近 30 天订阅计费日志 | 28,061 |
| 非管理员最近 30 天待定价日志 | 69,458 / 72,449，约 95.87% |

由此得出两个结论：

1. 当前系统没有足够的模型级采购价格，无法准确计算当前和历史盈亏。
2. 当前页面对大部分请求按 1 倍处理，实质上把客户收入当成上游成本，容易得到接近零利润的错误结果。

## 四、财务口径

### 4.1 页面默认口径：经营毛利

财务页默认展示经营毛利，不展示“充值差额利润”。

```text
经营收入 = 余额请求实际扣费收入 + 订阅期间确认收入
已覆盖收入 = 已确认成本请求对应收入 + 当期未分配订阅收入
已确认毛利 = 已覆盖收入 - 已确认上游成本
已确认毛利率 = 已确认毛利 ÷ 已覆盖收入 × 100%
待定价收入暴露 = 缺少已确认上游成本的请求对应收入
```

当成本覆盖率低于 99% 时，页面标题使用“已覆盖范围毛利”，不得把该金额命名为全站净利润。经营收入、已覆盖收入和待定价收入暴露同时展示，避免缺失成本造成利润高估。

页面统一使用美元作为核算币种。其他币种在资金事件发生时按当时汇率换算为美元，并保存原币种、原金额、汇率和美元金额。

### 4.2 余额计费收入

余额计费请求的已确认收入直接使用该请求的 `usage_logs.actual_cost`：

```text
余额请求收入 = actual_cost
余额请求利润 = actual_cost - upstream_cost
```

用户充值不是充值当日收入，而是用户余额负债。用户实际消费时才进入经营收入。

### 4.3 订阅收入

订阅请求的 `actual_cost` 只是名义消费额，不作为真实收入。订阅收入按支付订单的服务期直线确认：

```text
订阅每日确认收入 = (订单服务金额 - 已退款服务金额) ÷ 订阅天数
订阅期间利润 = 期间确认收入 - 期间订阅请求上游成本
```

处理规则：

- 支付订单使用 `payment_orders.amount` 作为服务金额。
- `pay_amount - amount` 进入支付附加费用，不混入模型经营毛利。
- 已退款金额从退款生效日起冲减未确认收入；已经确认部分生成负向财务调整记录。
- 管理员免费分配的订阅确认收入为 0，上游成本全部计入推广成本。
- 同一用户、同一分组存在续费或叠加订单时，每个订单按自己的服务天数独立确认，日收入相加。

订阅收入归属固定为两层：先按订单和日期生成收入确认记录，再把当日确认收入按该订单对应分组的请求 `usage_list_value` 占比分配到请求财务记录；当日所有请求的 `usage_list_value` 都为 0 时按请求数平均分配。当日没有请求时保留为“未分配订阅收入”，归属客户和订阅分组，不虚构请求记录。该分配结果固化保存，保证客户、分组、模型和单请求利润可以勾稽到订阅订单。

### 4.4 上游成本

Token 模型按请求实际使用量计算：

```text
上游成本 =
  输入 Token × 输入单价
  + 输出 Token × 输出单价
  + 缓存读取 Token × 缓存读取单价
  + 5 分钟缓存写入 Token × 5 分钟缓存写入单价
  + 1 小时缓存写入 Token × 1 小时缓存写入单价
  + 图片输出 Token × 图片输出单价
```

其他计费方式：

```text
按次成本 = 实际请求次数 × 每次单价
按图成本 = 实际图片数量 × 对应尺寸或档位单价
按秒成本 = 实际视频秒数 × 每秒单价
```

Fast 请求使用 `service_tier=priority` 对应的上游 Fast 单价；普通单价与 Fast 单价分开保存。上游没有 Fast 价格时，该请求进入待定价状态，不使用客户 Fast 售价反推成本。

上游成本价格来源按以下顺序确定：

1. 请求命中的结算钱包存在已同步或已审核的实际上游模型价格时，直接使用该价格，不再乘账号上游倍率；
2. 没有实际上游价格，但请求发生时有效的系统模型价格版本存在对应上游模型原价时，使用请求快照中的独立上游倍率计算；
3. 两种来源都不存在时标记为 `missing_price`，不使用客户销售价格反推。

账号上游倍率成本公式：

```text
上游单位成本 = 系统模型原始单位价 × usage_logs.upstream_cost_multiplier
上游请求成本 = 各用量分项 × 对应上游单位成本后求和
```

`usage_logs.upstream_cost_multiplier` 是请求路由完成、账号选定时从 `accounts.upstream_cost_multiplier` 锁定的财务快照。该值通过请求上下文传到用量记录写入链路，不在请求结束时重新查询账号。账号编辑页修改上游倍率后，只影响修改完成后新选中该账号的请求；已经选中账号的在途请求和已经写入的历史请求继续使用原快照。`usage_logs.account_rate_multiplier` 继续服务账号额度和账号统计，禁止进入上游成本公式。

倍率与价格的固定时点如下：

```text
请求选择路由账号
  → 锁定账号上游倍率
  → 调用上游并采集实际用量
  → 将锁定倍率随 usage_logs 一次写入
  → 财务扫描按 usage_logs.created_at 匹配当时有效的价格版本
  → 生成不可漂移的 usage_finance_records
```

精确上游单价存在时，仍保存倍率快照用于审计，但成本直接使用精确单价，不再乘倍率。精确单价不存在时，成本使用请求发生时有效的系统模型原价版本乘倍率快照。任何历史价格版本缺失都进入 `missing_price`，禁止拿当前模型价格补旧账。

### 4.5 成本状态

每条请求必须属于以下一种状态：

| 状态 | 含义 | 是否进入已确认利润 |
| --- | --- | --- |
| `confirmed` | 来自上游同步价格、经过审核的手工精确价格，或非空独立上游倍率快照乘系统模型原价；倍率来源标记为 `account_upstream_multiplier` | 是 |
| `estimated` | 历史请求缺少倍率快照，但存在管理员确认并带生效区间的历史倍率，可进行独立模拟回算 | 单独展示，不进入已确认利润 |
| `missing_price` | 找不到模型价格 | 否 |
| `missing_profile` | 路由账号没有结算钱包 | 否 |
| `unsupported_usage` | 当前价格规则不覆盖该计费形态 | 否 |
| `excluded` | 管理员流量、测试流量或明确排除流量 | 否 |

财务页不把后三类状态计为零成本。

## 五、数据模型

### 5.1 账号计费倍率与上游倍率分离

两个倍率保留为独立字段，禁止互相复用：

| 字段 | 页面名称 | 当前/目标职责 | 是否进入财务利润 |
| --- | --- | --- | --- |
| `accounts.rate_multiplier` | 账号计费倍率 | 账号额度消耗、账号用量统计和账号额度告警 | 否 |
| `accounts.upstream_cost_multiplier` | 上游倍率 | 缺少精确上游价格时计算采购成本 | 是 |
| `usage_logs.account_rate_multiplier` | 账号计费倍率快照 | 保持账号额度与统计历史口径 | 否 |
| `usage_logs.upstream_cost_multiplier` | 上游倍率快照 | 固化请求发生时的采购倍率 | 是 |

`accounts.upstream_cost_multiplier` 固定规则：

- 新增账号页面默认填入 `1.0000` 并要求提交；
- 现有账号迁移为 `NULL`，禁止从 `accounts.rate_multiplier` 自动复制；
- 允许 `0`，表示该账号的采购成本为零；
- 只允许有限数字且必须大于等于 `0`，最多保存 4 位小数；
- 不影响用户扣费、分组倍率、用户专属倍率、模型广场倍率价、账号额度和账号用量统计；
- 财务页只读展示，不允许直接修改；
- 账号创建或更新接口新增独立 API 字段 `upstream_cost_multiplier`。

倍率快照执行以下不可变规则：

1. 取值时点固定为路由账号选定时，不是请求结束时，也不是财务扫描时。
2. 账号倍率与账号标识一起进入请求上下文，所有成功、失败、重试和流式请求的用量记录写入路径复用同一个快照。
3. `usage_logs.upstream_cost_multiplier` 只允许在插入用量记录时赋值；账号编辑、财务扫描、历史回算和报表查询均不得更新该字段。
4. 管理员在请求执行期间修改账号倍率时，在途请求保留选中账号时的旧倍率；保存成功后新选中该账号的请求使用新倍率。
5. 账号保存成功后立即更新或失效账号缓存，确保后续路由读取新倍率；缓存刷新不能反向影响已经创建的请求上下文。
6. 财务成本计算和历史报表只读取该财务快照，不读取账号当前值，也不读取 `account_rate_multiplier`。
7. 倍率属于采购成本敏感信息，只出现在管理员账号接口、管理员用量详情和财务接口中，不进入客户侧普通用量记录响应。

示例：

| 时间 | 事件 | 使用记录中的倍率 | 财务处理 |
| --- | --- | ---: | --- |
| 10:00 | 请求 A 选中账号，账号上游倍率为 0.3000 | 0.3000 | 永久按 0.3000 核算 |
| 14:00 | 管理员把账号上游倍率改为 0.2500 | 不修改请求 A | 历史结果不变 |
| 14:01 | 请求 B 选中同一账号 | 0.2500 | 永久按 0.2500 核算 |

所选期间的上游成本等于每条使用记录按自己的倍率快照计算后求和，不能先取账号当前倍率再乘期间总用量。

### 5.2 `upstreams`：上游站点

保留现有表，职责限定为站点级信息：

- Base URL
- 站点名称
- 支持的财务适配器
- 定价同步状态
- 余额同步状态
- 最后成功时间和错误摘要

`SyncFromAccounts` 继续按规范化 Base URL 自动创建站点，不要求管理员重新输入 URL。

删除 `upstreams.rate_multiplier` 参与新成本计算的职责。该站点级字段只保留一版兼容期，迁移完成后删除；账号级采购倍率统一使用新的 `accounts.upstream_cost_multiplier`。

### 5.3 `upstream_wallets`：结算钱包

一个上游站点允许存在多个结算钱包。结算钱包代表实际扣款和余额归属，不代表路由账号。

核心字段：

| 字段 | 用途 |
| --- | --- |
| `id` | 钱包 ID |
| `upstream_id` | 所属上游站点 |
| `name` | 钱包名称 |
| `pricing_adapter` | `manual`、`newapi`、`legacy_openai` |
| `pricing_group` | 上游计费分组 |
| `balance_adapter` | `manual`、`newapi_user`、`none` |
| `quota_adapter` | `legacy_openai`、`none` |
| `balance_scope_key` | 共享余额去重标识 |
| `finance_access_token_encrypted` | 只保存财务接口额外凭据，AES-256-GCM 加密 |
| `currency` | 原始结算币种 |
| `enabled` | 是否参与财务核算 |
| `last_pricing_sync_at/status/error` | 定价同步状态 |
| `last_balance_sync_at/status/error` | 余额同步状态 |

初次同步为每个活跃账号自动建立一个钱包。管理员可以把多个账号合并到同一钱包，合并后共享余额和价格版本。

### 5.4 `upstream_wallet_accounts`：钱包与路由账号关系

核心字段：

- `wallet_id`
- `account_id`
- `effective_from`
- `effective_to`

同一时间一个账号只能属于一个钱包。使用生效时间保存历史归属，避免账号改名、改 URL 或更换钱包后改变历史报表。

### 5.5 `upstream_model_price_versions`：模型价格版本

现有 `upstream_model_rates` 生产数据为 0 条。本次迁移把它扩展为版本化价格表，并保留旧字段兼容导入。

核心字段：

| 字段 | 用途 |
| --- | --- |
| `wallet_id` | 所属结算钱包 |
| `model_pattern` | 实际上游模型名，支持精确值和尾部 `*` |
| `billing_mode` | `token`、`per_request`、`image`、`per_second` |
| `input_price` / `output_price` | 普通 Token 单价 |
| `cache_read_price` | 缓存读取单价 |
| `cache_write_5m_price` / `cache_write_1h_price` | 缓存写入单价 |
| `image_output_price` | 图片输出 Token 单价 |
| `per_request_price` / `per_second_price` | 按次、按秒单价 |
| `fast_input_price` / `fast_output_price` / `fast_cache_read_price` | Fast 单价 |
| `uniform_multiplier` | 兼容历史钱包倍率版本；新请求统一读取独立上游倍率快照，不在财务页编辑 |
| `source` | `upstream_sync`、`manual_price`、`account_upstream_multiplier`、`import` |
| `effective_from` / `effective_to` | 生效区间 |
| `source_payload` | 上游公开计费字段，不保存凭据 |
| `checksum` | 同步去重和审计 |

匹配顺序固定为：

1. 钱包 + 实际上游模型精确价格
2. 钱包 + 实际上游模型最长前缀通配符
3. 请求账号上游倍率快照 × 系统官方模型原价
4. 待定价

禁止回退到 `actual_cost`。

系统模型原价目录参与倍率成本计算时也必须版本化：原价调整新增版本并记录生效时间，不得原地覆盖旧版本。由系统原价乘倍率生成的财务记录，其 `price_version_id` 指向请求发生时有效的原价版本，`upstream_cost_multiplier_snapshot` 保存该请求自己的倍率快照。

### 5.6 `usage_finance_records`：请求财务记录

该表与 `usage_logs` 一对一，不修改用量日志的只追加属性。

核心字段：

- `usage_log_id`
- `usage_created_at`
- `user_id`
- `group_id`
- `channel_id`
- `account_id`
- `wallet_id`
- `upstream_id`
- `requested_model`
- `upstream_model`
- `service_tier`
- `business_type`：`balance`、`subscription`、`grant`、`admin`
- `billing_type`
- `revenue_amount`
- `usage_list_value`
- `upstream_cost`
- `gross_profit`
- `gross_margin`
- `cost_status`
- `price_version_id`
- `pricing_source`
- `upstream_cost_multiplier_snapshot`
- `calculation_detail`
- `calculated_at`

`calculation_detail` 保存各计费分项、Token 数量和单价，不保存密钥和请求正文。

财务记录由后台扫描器生成：每 30 秒读取尚无财务记录的用量日志，按 `(created_at, id)` 游标批量处理，插入时使用唯一键保证多实例幂等。扫描器的输入固定来自历史用量记录：`upstream_model`、Token/缓存/图片/视频用量、`service_tier`、`created_at` 和 `upstream_cost_multiplier`。已确认成本的查询禁止关联 `accounts.upstream_cost_multiplier`，也禁止把账号当前值复制进旧用量记录。

扫描器按 `usage_logs.created_at` 解析价格版本，并把最终采用的 `price_version_id`、倍率快照、各分项单位价和计算结果写入财务记录。后续报表只汇总 `usage_finance_records`，不在查询报表时临时重算。只有管理员明确新增带历史生效区间的价格版本并启动回算任务时，才允许生成新的计算版本；原计算明细保留审计记录。

该流程不进入网关请求主链路，不增加用户请求延迟。请求主链路只负责把已锁定的倍率快照随 `usage_logs` 一次落库。

### 5.7 `upstream_balance_snapshots`：余额快照

核心字段：

- `wallet_id`
- `balance_scope_key`
- `remaining_balance`
- `currency`
- `balance_kind`：`wallet_cash` 或 `token_quota`
- `source`
- `source_total_limit`
- `source_used_amount`
- `captured_at`
- `sync_status`

页面“钱包余额”只读取 `balance_kind=wallet_cash` 的最近一次成功快照。`token_quota` 只进入账号配额监控，不进入上游现金余额、资产或现金流。总钱包余额按 `balance_scope_key` 去重。

### 5.8 `upstream_fund_events`：上游资金流水

核心字段：

- `wallet_id`
- `event_type`：`opening_balance`、`topup`、`refund`、`adjustment`
- `original_amount`
- `currency`
- `exchange_rate_to_usd`
- `amount_usd`
- `occurred_at`
- `note`

现有 `initial_balance` 迁移为一条 `opening_balance` 事件。后续“上游充值总额”和现金流只读取资金流水，不再累加初始余额字段。

### 5.9 `subscription_revenue_recognitions`：订阅收入确认

核心字段：

- `payment_order_id`
- `user_id`
- `group_id`
- `recognition_date`
- `recognized_revenue`
- `refunded_revenue`
- `unallocated_revenue`
- `allocation_status`
- `calculation_detail`

该表按订单和日期唯一。请求财务记录保存分配后的订阅收入，所有请求分配额加未分配收入必须等于当日订阅确认收入。

### 5.10 `account_upstream_multiplier_changes`：上游倍率变更审计

核心字段：

- `account_id`
- `old_multiplier`
- `new_multiplier`
- `effective_at`
- `operator_user_id`
- `change_reason`
- `created_at`

账号创建写入第一条审计记录；账号编辑上游倍率时必须填写变更原因。该表只负责审计，不参与请求成本计算，财务计算仍以 `usage_logs.upstream_cost_multiplier` 为准。

### 5.11 `finance_alerts` 与 `upstream_bill_reconciliations`

`finance_alerts` 保存预警类型、严重级别、关联维度、影响金额、影响请求数、状态、负责人和处理记录；相同预警条件持续存在时更新同一未关闭事件，不重复创建。

`upstream_bill_reconciliations` 保存钱包、账期、上游账单成本、系统已确认成本、差异金额、差异率、账单来源、对账状态和审核人。没有可验证上游账单来源的钱包不生成差异率，状态显示 `unsupported`。

## 六、上游倍率与余额同步

### 6.1 账号页维护上游倍率

账号新增和编辑页在现有“账号计费倍率”旁新增独立“上游倍率”字段，表单字段为 `form.upstream_cost_multiplier`：

```text
字段名称：上游倍率
默认值：1.0000
输入范围：大于等于 0，最多 4 位小数
帮助文案：用于财务利润计算；缺少精确上游价格时，上游成本 = 系统模型原价 × 上游倍率。该值不影响客户扣费、分组倍率、账号额度和账号用量统计。
```

账号列表保留“账号计费倍率”列，并新增“上游倍率”列。新增、编辑和批量编辑使用独立 `upstream_cost_multiplier` 字段；导入账号和 CRS 同步没有提供该值时保存为未配置，不得拿 `rate_multiplier` 自动填充。

财务报表的请求明细显示只读字段：

- 上游倍率快照；
- 倍率来源：账号配置；
- 账号名称；
- 跳转“编辑账号”入口。

财务页面不提供上游倍率输入框。管理员需要修改时统一进入账号详情页，保存后仅对新请求生效。

编辑已有账号的上游倍率时必须填写变更原因，页面同时显示当前倍率和保存后的生效时间。保存完成后可查看倍率变更历史，包括旧值、新值、操作人、变更原因和生效时间。

账号倍率保存成功后，账号服务必须同步更新路由缓存。缓存边界以“账号被请求选中”为准：选中前读取新值，选中后保留请求上下文中的旧值，避免账号编辑与在途请求发生账务竞态。

### 6.2 适配器接口

新增统一接口：

```text
Probe(ctx, upstream, account) -> capabilities
FetchPricing(ctx, wallet) -> price catalog + raw metadata
FetchBalance(ctx, wallet) -> wallet balance snapshot
FetchQuota(ctx, wallet) -> API key quota snapshot
```

能力结果固定包含：

- `pricing_supported`
- `pricing_scope`：`public` 或 `account`
- `balance_supported`
- `balance_scope`：`wallet` 或 `token`
- `balance_kind`：`wallet_cash` 或 `token_quota`
- `requires_extra_credential`
- `adapter`
- `tested_at`
- `error_code`

### 6.3 NewAPI 定价适配器

读取：

- `GET /api/status`
- `GET /api/pricing`

Token 模型转换公式：

```text
输入单价 = model_ratio × group_ratio ÷ quota_per_unit
输出单价 = model_ratio × completion_ratio × group_ratio ÷ quota_per_unit
缓存读取单价 = 输入单价 × cache_ratio
缓存写入单价 = 输入单价 × create_cache_ratio
```

按次模型使用 `model_price × group_ratio`，再按 `/api/status` 的币种和汇率字段统一换算为美元。

同步前必须给钱包选择 `pricing_group`。公开接口返回所有分组，不会告诉系统某个 API Key 实际属于哪个分组；没有选择分组时只展示可选分组，不写入有效价格。

### 6.4 Legacy OpenAI Billing 配额适配器

读取：

- `GET /dashboard/billing/subscription`
- `GET /dashboard/billing/usage?start_date=...&end_date=...`

该接口计算 API Key 兼容配额：

```text
剩余配额 = hard_limit_usd - total_usage ÷ 100
```

同步同时保存原始额度上限和已用量字段，用于账号配额监控。该结果的 `balance_kind` 固定为 `token_quota`，禁止写入钱包余额。

### 6.5 NewAPI 用户钱包余额适配器

现有模型 API Key 不能访问 `/api/user/self`。启用该适配器时，管理员只填写一次上游用户中心访问令牌，令牌加密存入结算钱包。Base URL、模型 API Key 和账号关系继续复用现有账号数据。只有用户中心接口明确返回现金余额和币种时，结果才标记为 `wallet_cash`。

### 6.6 手工模式

没有财务接口的上游使用手工模式：

- 支持精确模型、尾部通配符、按次、按图、按秒和 Fast 价格。
- 支持 CSV/JSON 导入和导出。
- 每次保存创建新价格版本，必须填写生效时间。
- 支持手工录入余额和上游充值流水。
- 统一上游倍率不在财务页录入，统一从账号详情的“上游倍率”读取。
- 账号名称中的“0.3x”“0.08/张”等文本只作为页面提示，不自动进入财务计算。

### 6.7 同步频率

- 钱包余额与 API Key 配额：每 10 分钟同步一次，管理员手工刷新不受此限制。
- 模型定价：每 6 小时同步一次，管理员手工刷新不受此限制。
- 连续 3 次失败后将钱包标记为同步异常并发送管理员告警。
- 同步失败保留最后一次成功结果，不写入零值。
- 响应体上限 1 MiB，连接超时 5 秒，总超时 10 秒，最多跟随 2 次同源重定向。

## 七、本次真实上游探测结果

### 7.1 测试方法

- 时间：2026-07-23
- 环境：线上 `ccai.xyz`
- 账号范围：26 个活跃上游账号、12 个不同 Base URL
- 操作范围：只读 GET 请求
- 凭据：复用现有账号 API Key，仅在服务器内存中使用；测试输出未包含密钥、余额数值和完整响应正文
- 探测接口：公开定价、用户资料、余额、Legacy Billing、模型列表

### 7.2 结果矩阵

| 上游 | 模型定价自动拉取 | 现有 API Key 拉取真实钱包余额 | 结论 |
| --- | --- | --- | --- |
| `ai.silkroadai.io` | 支持，`/api/pricing` 返回完整模型倍率、输出倍率、缓存倍率和分组倍率 | 不支持；Legacy Billing 只返回兼容配额 | 定价可自动同步；钱包余额使用用户中心令牌或手工记录 |
| `zzshu.cc` | 支持，返回完整模型倍率与分组倍率 | 不支持；Legacy Billing 只返回兼容配额，重复探测出现 403 | 定价可自动同步；钱包余额使用用户中心令牌或手工记录 |
| `img.xmu.la` | 接口结构存在，但当前模型价格列表为空 | 不支持 | 使用手工价格和手工余额 |
| `api.xtokenmirror.com` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `asian-acc.we-token.cc` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `hk.getelucid.com` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `mdkj.lol` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `sub.kedaya.xyz` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `sub2api.1006000.xyz` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `tp.xtokenmirror.com` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `www.recycleai.vip` | 探测路径返回前端 HTML，不是财务 JSON 接口 | 不支持 | 使用手工价格和手工余额 |
| `www.xiongxiongai.online` | 未发现财务定价 JSON 接口 | 不支持 | 使用手工价格和手工余额 |

### 7.3 探测结论

1. 当前 12 个上游中，2 个能够自动拉取有效公开模型定价，占 16.67%。
2. 另有 1 个上游提供定价接口结构，但模型列表为空，不能用于成本核算。
3. 公开定价接口在带 API Key 和不带 API Key 时响应完全一致，因此它是站点公开价，不是账号个性化采购价。
4. NewAPI 的 `/api/user/self`、`/api/user/profile` 和 `/api/user/balance` 对现有模型 API Key 返回 `Unauthorized, invalid access token`。模型 API Key 不能直接读取用户中心钱包余额。
5. 两个站点的 Legacy Billing 接口返回 `hard_limit_usd` 和 `total_usage`。其中一次实测上限为 100,000,000，且不同日期范围的 `total_usage` 完全相同；该数据是兼容配额，不是可验证的现金余额。
6. 使用现有模型 API Key，12 个上游中没有一个能够可靠拉取真实钱包余额。真实余额必须通过上游用户中心令牌、上游专用财务接口或手工余额流水获得。
7. `/v1/models` 只证明模型接口可用，不包含采购倍率和账户余额，不能作为财务数据源。
8. 剩余 10 个上游使用手工价格，或接入该上游提供的专用财务接口。

## 八、财务报表页面

### 8.1 老板视角复核结论

现有页面只有充值、消费、上游成本、利润率六个累计数字和一张趋势图，无法回答当前是否亏损、亏损来自哪里、上游资金还能支撑多久以及数字是否可信。整改后的 `/admin/finance` 固定为五个页签：

1. `经营总览`：老板首屏，一屏回答当前盈亏、历史变化、主要亏损来源、资金风险和数据可信度。
2. `利润分析`：按客户、分组、模型、渠道、上游、账号和业务类型分析单位经济性。
3. `亏损追踪`：只展示亏损记录、亏损原因和处理闭环。
4. `资金与余额`：展示客户付款、退款、上游资金流水、钱包余额和可用天数。
5. `数据质量`：展示成本覆盖、余额同步、价格完整度和账单差异。

经营毛利、现金流和余额风险在数据和页面上完全分开。客户付款不直接命名为收入，上游充值不进入利润，API Key 配额不命名为钱包余额。

### 8.2 经营总览首屏

首屏固定为三行，默认时间范围为本月，另提供今天、昨天、最近 7 天、最近 30 天和自定义范围。所有期间指标同时显示上一等长周期的金额变化和变化率。

#### 第一行：核心经营结果

| 指标 | 口径 | 状态展示 |
| --- | --- | --- |
| 今日已确认毛利 | 今日已覆盖收入减已确认上游成本 | 负数红色，显示较昨日变化 |
| 本月已确认毛利 | 本月至今已覆盖收入减已确认上游成本 | 负数红色，显示较上月同期变化 |
| 所选期间经营收入 | 余额请求实际扣费收入加订阅期间确认收入 | 拆分余额消费与订阅收入 |
| 所选期间已覆盖收入 | 已确认成本请求对应收入加未分配订阅收入 | 与已确认成本使用同一覆盖范围 |
| 所选期间已确认成本 | `confirmed` 请求的上游成本 | 同时显示未确认成本风险 |
| 已确认毛利率 | 已确认毛利除以已覆盖收入 | 小于 0 为红色，0% 至 10% 为黄色 |
| 成本覆盖率 | 已确认成本请求数除以应计成本请求数 | 低于 99% 固定红色警告 |
| 亏损请求数与金额 | 单条请求或分摊后收入低于成本的记录 | 显示占全部请求比例 |
| 历史累计净毛利 | 全历史已覆盖收入减已确认成本 | 展示累计盈亏方向 |
| 历史亏损总额 | 已确认毛利小于 0 的请求亏损绝对值合计 | 可跳转亏损追踪，不与累计净毛利混用 |

订阅请求的收入使用期间确认收入，不使用 `usage_logs.actual_cost` 充当收入。免费分配、赠送额度和管理员流量单独标识，不混入付费客户收入。

#### 第二行：经营趋势与亏损来源

左侧趋势图同时展示：

- 经营收入与已覆盖收入折线；
- 已确认上游成本折线；
- 单期毛利柱状图，亏损为红色；
- 累计毛利折线；
- 上一等长周期对比虚线；
- 成本覆盖率副轴。

支持小时、天、周、月粒度。点击任意时间点，将整页筛选到该时间桶并刷新下方排行和明细。

右侧展示“亏损来源前五名”，可切换客户、分组、模型、渠道、上游站点和路由账号。每行展示收入、已确认成本、毛利、毛利率、亏损请求数和成本覆盖率，默认按毛利从低到高排序。

#### 第三行：资金安全与待处理异常

资金安全区展示：

- 上游已同步钱包余额合计；
- 未来 7 天预计上游支出；
- 最低钱包可用天数；
- 7 天内需要充值的钱包数；
- 客户未消费余额负债；
- 钱包余额最后成功同步时间。

待处理异常区展示：

- 当前亏损中的客户、分组、模型、渠道和账号；
- 缺上游倍率快照的使用记录；
- 缺历史价格版本的使用记录；
- 余额或定价同步失败；
- 上游实际账单与系统成本差异超限；
- 连续 3 天负毛利；
- 成本覆盖率低于 99%。

每条异常显示严重级别、发生时间、影响金额、影响请求数、负责人、处理状态和最后处理记录，并直接打开过滤后的明细。

### 8.3 利润分析

利润分析支持以下维度，所有维度复用同一时间范围和成本状态筛选：

- 客户；
- 客户分组；
- 请求模型与实际上游模型；
- 渠道；
- 上游站点；
- 结算钱包与路由账号；
- 余额消费、订阅、赠送和管理员业务类型；
- 普通、Fast、缓存、图片、视频和按次计费类型。

每行固定展示收入、已确认成本、估算成本、待定价请求数、毛利、毛利率、请求数、输入成本、输出成本、缓存读写成本、Fast 附加成本、图片/视频成本和成本覆盖率。

下钻路径固定为：

```text
期间
  → 客户/分组/模型/渠道/上游任一分析维度
  → 关联路由账号
  → 单条使用记录
  → 收入分项、用量分项、价格版本、倍率快照和成本计算明细
```

### 8.4 亏损追踪

亏损追踪独立成页，不依赖用户手工设置利润小于零筛选。列表字段固定包含：

- 发生时间；
- 客户与分组；
- 请求模型与实际上游模型；
- 渠道、上游站点、结算钱包和路由账号；
- 业务类型与计费类型；
- 已确认收入、上游成本、亏损金额和毛利率；
- 上游倍率快照、价格版本和成本状态；
- 亏损原因、处理状态、负责人和处理记录。

系统按计算证据归类亏损原因：销售单位价低于采购单位成本、上游价格上涨、路由到高成本账号、Fast/缓存/图片/视频成本高于售价、订阅确认收入不足、赠送或免费流量、精确价格缺失、倍率快照缺失、历史归属缺失。数据缺失类原因不进入已确认亏损金额，单独进入风险金额。

### 8.5 资金与余额

资金页分为现金流、客户负债和上游钱包三部分：

| 部分 | 展示内容 |
| --- | --- |
| 现金流 | 客户实际付款、客户退款、支付手续费、上游充值、上游退款、其他调整、已知净现金流 |
| 客户负债 | 客户未消费余额、未履约订阅余额、退款待处理金额 |
| 上游钱包 | 当前钱包现金余额、今日消耗、近 7 日日均消耗、预计可用天数、充值预警线、余额来源和最后同步状态 |

```text
预计可用天数 = 最新成功钱包现金余额 ÷ 近 7 日已确认日均上游成本
```

近 7 日没有已确认成本时显示“无法计算”，不显示无限天数或 0 天。钱包可用天数低于 7 天标红，7 至 14 天标黄。余额超过两个同步周期未成功更新时标记为过期，不进入“已同步余额合计”。

API Key 配额、已使用配额、钱包现金余额和系统估算成本使用独立列和独立汇总，任何情况下都不相加。

### 8.6 数据质量与对账

数据质量页展示：

- 有上游倍率快照的使用记录占比；
- 有请求发生时价格版本的记录占比；
- 已确认成本、估算成本和待完善成本的请求数与金额；
- 未绑定钱包、历史归属不明确和不支持计费形态的记录；
- 定价同步、钱包余额同步和 API Key 配额同步的成功率；
- 上游实际账单与系统计算成本的金额差和差异率；
- 最后财务扫描时间、最后完整回算时间和最新可用账期。

页面金额固定使用三种状态：`已确认`、`估算`、`待完善`。缺失数据返回 `null` 和原因码，前端显示“无法计算”，禁止转换为 `0`。

### 8.7 预警与处理闭环

系统固定生成以下预警：

- 今日或本月已确认毛利小于 0；
- 任一客户、分组、模型、渠道、上游或账号毛利率小于 0；
- 连续 3 个自然日已确认毛利小于 0；
- 钱包预计可用天数低于 14 天；
- 钱包余额低于配置的充值预警线；
- 上游倍率上调后新请求转为亏损；
- 成本覆盖率低于 99%；
- 历史用量缺上游倍率快照或有效价格版本；
- 上游实际账单与系统成本的差异率超过 1%，且差异金额超过 1 美元；
- 余额或定价同步连续失败两次。

预警状态固定为 `open`、`acknowledged`、`resolved`。处理时保存处理人、处理时间、处理说明和关联配置变更；解除预警不删除历史记录。

### 8.8 现有页面字段处置

| 现有字段或板块 | 处理结果 |
| --- | --- |
| 用户充值总金额 | 从经营利润卡片移除，改名“客户实际付款”并迁入资金页 |
| 上游充值总金额 | 从经营利润卡片移除，改为上游资金流水，不再累加初始余额 |
| 用户消耗金额 | 改为“经营收入”，并拆分余额消费与订阅期间确认收入 |
| 已确认收入 | 分为“经营收入”和“已覆盖收入”，避免缺成本请求抬高利润 |
| 上游消耗金额 | 改为“已确认上游成本”，只汇总请求财务记录 |
| 已消耗利润 | 改为“已确认毛利”，同时展示待定价风险 |
| 利润率 | 改为“已确认毛利率”，分母只使用已覆盖收入 |
| 单一趋势图 | 改为收入、成本、单期毛利、累计毛利、覆盖率和上一周期对比 |
| 缺失数据默认显示 0 | 删除；改为“无法计算”及原因 |

旧页面在新账本覆盖率达标前只读保留并明确标记“旧口径，不用于经营决策”。

## 九、接口设计

### 9.1 财务报表

```text
GET /api/v1/admin/finance/overview
GET /api/v1/admin/finance/trend
GET /api/v1/admin/finance/breakdown
GET /api/v1/admin/finance/details
GET /api/v1/admin/finance/losses
GET /api/v1/admin/finance/funds
GET /api/v1/admin/finance/data-quality
GET /api/v1/admin/finance/cash-flow
GET /api/v1/admin/finance/alerts
PUT /api/v1/admin/finance/alerts/:id
GET /api/v1/admin/finance/reconciliations
```

公共筛选字段：

- `start_date`
- `end_date`
- `granularity`
- `upstream_id`
- `wallet_id`
- `account_id`
- `user_id`
- `channel_id`
- `model`
- `group_id`
- `billing_type`
- `business_type`
- `cost_status`

所有接口使用左闭右开时间范围 `[start_date, end_date)`，后端统一按站点时区解析并转换为 UTC 查询。

### 9.2 账号上游倍率

复用现有账号接口，不新增财务倍率接口：

```text
POST /api/v1/admin/accounts
PUT  /api/v1/admin/accounts/:id
```

请求和响应新增独立字段，现有 `rate_multiplier` 保持原语义：

```json
{
  "rate_multiplier": 1.0,
  "upstream_cost_multiplier": 0.3
}
```

后端分别校验两个字段。`rate_multiplier` 继续服务账号额度和账号统计；`upstream_cost_multiplier` 只服务财务采购成本。修改 `upstream_cost_multiplier` 时请求体必须包含 `upstream_cost_multiplier_change_reason`，服务端在同一事务中保存账号值和倍率变更审计。管理员财务接口和管理员用量详情只返回请求发生时的上游倍率快照，不接受倍率写入；客户侧用量接口不返回上游倍率、采购单价和上游成本。

### 9.3 上游财务配置

```text
GET    /api/v1/admin/upstreams/:id/wallets
POST   /api/v1/admin/upstreams/:id/wallets
PUT    /api/v1/admin/upstream-wallets/:id
DELETE /api/v1/admin/upstream-wallets/:id
POST   /api/v1/admin/upstream-wallets/:id/accounts
POST   /api/v1/admin/upstream-wallets/:id/probe
POST   /api/v1/admin/upstream-wallets/:id/sync-pricing
POST   /api/v1/admin/upstream-wallets/:id/sync-balance
GET    /api/v1/admin/upstream-wallets/:id/prices
POST   /api/v1/admin/upstream-wallets/:id/prices/import
POST   /api/v1/admin/upstream-wallets/:id/fund-events
GET    /api/v1/admin/upstream-wallets/:id/sync-history
```

敏感凭据在响应中只返回 `configured: true/false`，绝不返回密文或明文。

上游财务配置接口不提供统一倍率写入字段。钱包精确模型价格、余额和资金流水继续在本模块维护；账号默认上游倍率统一通过账号创建、更新接口的 `upstream_cost_multiplier` 字段维护。

### 9.4 历史回算

```text
POST /api/v1/admin/finance/backfill/preview
POST /api/v1/admin/finance/backfill/run
GET  /api/v1/admin/finance/backfill/:job_id
```

预览返回预计覆盖日志数、待定价模型、归属不明确账号和日期范围。执行任务按批次写入 `usage_finance_records`，支持暂停、继续和幂等重跑。

旧接口 `GET /api/v1/admin/finance/stats` 保留一个发布周期，只返回旧结构并附带 `deprecated: true` 和口径说明。前端切换后删除旧接口与旧 SQL。

## 十、代码落点

### 10.1 数据库

- `backend/migrations/169_finance_cost_ledger_and_upstream_sync.sql`
  - 新建钱包、账号关系、价格版本、请求财务记录、订阅收入确认、余额快照、资金流水、倍率变更审计、财务预警、账单对账和回算任务表。
  - 把现有 `initial_balance` 转为 `opening_balance` 资金事件。
  - 为 26 个活跃账号自动建立钱包和账号关系。
  - 保留原表字段，不在首个版本中执行破坏性删除。
  - 新增可空 `accounts.upstream_cost_multiplier decimal(10,4)`，现有账号不从 `rate_multiplier` 回填。
  - 新增可空 `usage_logs.upstream_cost_multiplier decimal(10,4)`，与现有 `account_rate_multiplier` 并存。
  - 该字段为插入时快照字段；不设置账号更新级联、数据库回填触发器或财务回算更新逻辑。

### 10.2 后端模型与仓储

- 调整 `backend/ent/schema/account.go`、`backend/ent/schema/usage_log.go`、`backend/internal/service/account.go` 和账号 DTO
  - 保留现有 `rate_multiplier`/`account_rate_multiplier` 语义，新增 `upstream_cost_multiplier` 及请求快照字段。
  - 管理员 DTO 返回倍率快照；客户侧用量 DTO 明确排除采购倍率字段。
- 调整 `backend/internal/handler/admin/account_handler.go` 和账号服务校验
  - 所有账号创建、更新和批量更新路径独立校验并保存上游倍率，不覆盖账号计费倍率。
- `backend/internal/service/upstream_management_models.go`
  - 上游站点只保留站点级职责。
- 新增 `backend/internal/service/finance_models.go`
  - 财务汇总、趋势、排行、明细、数据质量 DTO，明细包含只读上游倍率快照和来源。
- 新增 `backend/internal/service/upstream_finance_models.go`
  - 钱包、价格版本、余额快照、资金流水 DTO。
- 重构 `backend/internal/repository/upstream_repo.go`
  - 删除旧成本表达式在新接口中的使用。
- 新增 `backend/internal/repository/finance_repo.go`
  - 财务记录、订阅收入确认、报表聚合、亏损追踪、预警、对账和回算任务查询。
- 新增 `backend/internal/repository/upstream_finance_repo.go`
  - 钱包、价格版本、余额和资金流水持久化。

### 10.3 后端服务

- 新增 `backend/internal/service/upstream_finance_adapter.go`
  - 适配器接口、能力探测和统一错误码。
- 新增 `backend/internal/service/upstream_finance_newapi.go`
  - NewAPI 定价与用户余额适配器。
- 新增 `backend/internal/service/upstream_finance_legacy_openai.go`
  - Legacy Billing 余额适配器。
- 新增 `backend/internal/service/finance_cost_calculator.go`
  - Token、缓存、Fast、图片、视频、按次和按秒成本计算；精确上游价格优先，缺少精确价格时使用系统原价乘独立上游倍率快照。
- 调整各平台使用记录写入链路
  - 在路由账号选定时把 `account.UpstreamCostMultiplier` 写入请求上下文；请求完成写入 `usage_logs` 时使用上下文中的锁定值，不重新查询账号。
  - 覆盖普通、流式、重试、WebSocket、图片、视频和失败用量记录路径；原有 `account_rate_multiplier` 写入和账号额度逻辑保持不变。
- 新增 `backend/internal/service/finance_usage_scanner.go`
  - 每 30 秒按历史用量记录及 `created_at` 有效价格版本生成请求财务记录；已确认计算不读取账号当前倍率。
- 新增 `backend/internal/service/upstream_finance_sync_runner.go`
  - 定价和余额定时同步。
- 新增 `backend/internal/service/finance_backfill_service.go`
  - 历史预览、批量回算和进度记录。
- 新增 `backend/internal/service/finance_revenue_recognition.go`
  - 订阅收入按订单和日期确认，并把可分配收入固化到请求财务记录。
- 新增 `backend/internal/service/finance_alert_service.go`
  - 生成经营亏损、资金可用天数、成本覆盖率、同步失败和账单差异预警，维护处理闭环。
- 新增 `backend/internal/service/upstream_reconciliation_service.go`
  - 按钱包和账期核对上游实际账单与系统已确认成本。
- 调整 `backend/internal/service/account_stats_pricing.go`
  - 抽取可复用的计价分项；补齐 `service_tier`、5 分钟/1 小时缓存、图片和按秒计费。
- 调整 `backend/internal/service/wire.go`、`backend/cmd/server/wire.go` 和生成文件
  - 注入新仓储、服务、扫描器和同步运行器。

### 10.4 Handler 与路由

- 新增 `backend/internal/handler/admin/finance_handler.go`
- 新增 `backend/internal/handler/admin/upstream_finance_handler.go`
- 调整 `backend/internal/server/routes/admin.go`
- 旧 `UpstreamHandler.FinanceStats` 在兼容期内保留。

### 10.5 前端

- 调整 `frontend/src/components/account/CreateAccountModal.vue`
  - 保留“账号计费倍率”及 `form.rate_multiplier`，新增“上游倍率”及 `form.upstream_cost_multiplier`。
- 调整 `frontend/src/components/account/EditAccountModal.vue`
  - 分别回填和保存账号计费倍率、上游倍率，两个输入互不联动；上游倍率发生变化时要求填写变更原因并展示变更历史。
- 调整 `frontend/src/views/admin/AccountsView.vue` 及账号列表列配置
  - 保留“账号计费倍率”列并新增“上游倍率”列。
- 调整 `frontend/src/components/account/BulkEditAccountModal.vue` 和账号导入路径
  - 新增独立 `upstream_cost_multiplier`，导入缺失时保持未配置，不复用 `rate_multiplier`。
- 重构 `frontend/src/views/admin/FinanceStatsView.vue`
  - 实现经营总览、利润分析、亏损追踪、资金与余额、数据质量五个页签；上游倍率只读展示，不提供倍率输入框。
- 扩展 `frontend/src/views/admin/upstreams/UpstreamManagementView.vue`
  - 增加结算钱包、账号归属、定价同步、余额同步和资金流水入口。
- 新增 `frontend/src/views/admin/upstreams/UpstreamWalletDetailView.vue`
- 新增 `frontend/src/components/finance/FinanceSummaryCards.vue`
- 新增 `frontend/src/components/finance/ProfitTrendChart.vue`
- 新增 `frontend/src/components/finance/LossBreakdownTable.vue`
- 新增 `frontend/src/components/finance/FinanceDataQualityPanel.vue`
- 新增 `frontend/src/components/finance/FinanceFundsPanel.vue`
- 新增 `frontend/src/components/finance/FinanceAlertsPanel.vue`
- 新增 `frontend/src/components/finance/FinanceReconciliationTable.vue`
- 新增 `frontend/src/components/finance/UpstreamPricingEditor.vue`
  - 只编辑精确模型价格和阶梯价格，不提供统一上游倍率输入。
- 新增 `frontend/src/api/admin/finance.ts`
- 扩展 `frontend/src/api/admin/upstreams.ts`
- 调整 `frontend/src/router/index.ts`、`frontend/src/i18n/locales/zh.ts` 和英文语言文件。

## 十一、实施顺序

### 阶段 1：停止错误口径扩散

1. 在旧财务页增加“旧口径，不用于真实盈亏判断”说明。
2. 修复旧顶部汇总不使用日期范围的问题。
3. 在新报表完成前，禁止新增基于旧 `upstreamCostExpr` 的功能。

### 阶段 2：建立结算钱包和价格版本

1. 执行迁移 169。
2. 从账号 Base URL 自动创建缺失上游。
3. 为每个活跃账号创建默认钱包并建立生效关系。
4. 保留现有“账号计费倍率”，在账号新增、编辑、列表和批量编辑中增加独立“上游倍率”。
5. 上线手工精确模型价格、Fast、图片和视频价格编辑器；财务页不提供统一倍率输入。
6. 上线 NewAPI 和 Legacy Billing 能力探测。

### 阶段 3：生成新财务记录

1. 上线成本计算器和财务扫描器。
2. 成本计算器优先使用精确上游价格，缺少精确价格时读取 `usage_logs.upstream_cost_multiplier` 快照；禁止读取 `account_rate_multiplier`。
3. 所有用量记录写入路径在账号选定时锁定倍率并保存快照，新请求开始生成 `usage_finance_records`。
4. 订阅收入确认服务生成按日收入及请求分配结果。
5. 页面先展示数据质量，不替换旧财务数据。
6. 对每种计费方式和订阅收入分配执行固定样本复算。

### 阶段 4：历史回算

1. 管理员为每个钱包补齐历史精确价格版本和生效时间；历史倍率缺失项必须补录带明确生效区间的历史倍率规则，账号当前倍率不能作为旧记录的已确认成本依据。
2. 运行回算预览，处理未绑定账号和未定价模型。
3. 分月执行历史回算。
4. 回算完成后核对每日收入、成本、利润和覆盖率。

### 阶段 5：切换财务报表

满足以下启用门槛后切换 `/admin/finance`：

- 最近 7 天成本覆盖率不低于 99%。
- 所有活跃钱包完成计费配置或被明确排除。
- 所有使用倍率成本口径的活跃账号均已在账号详情保存正确上游倍率。
- 100 条抽样请求人工复算全部通过。
- 余额计费收入与 `actual_cost` 汇总一致。
- 订阅日确认收入与有效支付订单、退款和服务天数一致。
- 同一时间范围内汇总、趋势、排行和明细可以互相勾稽。
- 老板首屏同时通过当前盈亏、历史变化、前五亏损来源、资金可用天数和数据可信度验收。
- 每一条财务预警都能打开对应明细，并可完成确认、处理和关闭。

### 阶段 6：删除旧口径

1. 删除旧前端字段和图表。
2. 删除 `upstreamCostExpr` 在财务模块中的使用。
3. 删除旧 `/finance/stats` 接口。
4. 下一次迁移删除不再使用的站点级成本倍率字段。

## 十二、历史数据处理规则

1. 历史日志使用 `upstream_model`；为空时使用 `requested_model`，并标记模型来源为回退。
2. 历史账号即使已经软删除，也按 `account_id` 读取，不再要求 `deleted_at IS NULL`。
3. 账号历史归属使用 `upstream_wallet_accounts` 的生效区间。
4. 价格使用 `usage_created_at` 落入的价格版本。
5. `usage_logs.upstream_cost_multiplier` 非空时直接作为历史上游倍率快照，禁止被账号当前倍率覆盖。
6. `usage_logs.account_rate_multiplier` 不得作为上游倍率使用。
7. 历史上游倍率快照为空时，禁止使用账号当前 `upstream_cost_multiplier` 回填、预览或确认历史成本；只有管理员补录带明确生效区间的历史倍率规则后，才能生成单独标记为 `estimated` 的模拟结果。
8. 找不到历史价格版本时保持待定价，不使用当前价格覆盖历史。
9. 找不到历史 Base URL 或钱包归属时标记 `missing_profile`。
10. 历史回算不会修改 `usage_logs`、用户余额、订阅用量和支付订单。
11. 每个回算任务记录参数、执行人、开始时间、结束时间、处理数量和错误数量。

## 十三、安全与稳定性

- 财务访问令牌使用现有 `SecretEncryptor` 的 AES-256-GCM 实现。
- 前端和日志不返回密文、明文、Cookie 或完整上游响应。
- 上游响应正文最多读取 1 MiB，错误日志只保存状态码、适配器错误码和截断后的安全摘要。
- Base URL 继续经过 HTTP/HTTPS 校验；同步客户端限制重定向并执行目标地址安全检查。
- 定时任务使用批量、游标和唯一键，保证多实例运行幂等。
- 价格同步采用版本新增，不覆盖正在使用的历史价格。
- 同步失败不写零余额、不清空价格、不改变历史利润。
- 财务详情不展示用户请求正文和上游密钥。

## 十四、测试方案

### 14.1 成本计算单元测试

覆盖：

- 输入、输出、缓存读、5 分钟缓存写、1 小时缓存写
- 普通和 Fast
- 长上下文分段
- 精确模型和最长前缀通配符
- 账号上游倍率为 `0`、`1`、小数和 4 位精度值
- 精确上游价格存在时不重复乘账号上游倍率
- 缺少精确价格时使用用量日志中的账号上游倍率快照
- 修改账号上游倍率后，历史财务记录保持不变，新请求使用新倍率
- 请求选中账号后、写入用量记录前修改倍率，在途请求仍保存选中时的旧倍率
- 同一批请求跨越倍率变更时点时，变更前后分别使用各自快照，期间成本按逐条计算结果求和
- 修改账号当前倍率后重复执行财务扫描和报表查询，既有财务结果保持一致
- 改变账号计费倍率不改变财务上游成本
- 图片按次、图片数量、图片尺寸档位
- 视频按秒
- 缺价格、缺钱包、零 Token、失败请求
- 金额舍入和高精度小数

### 14.2 上游适配器测试

使用 `httptest.Server` 固定验证：

- NewAPI 正常价格响应
- 分组不存在
- 空模型列表
- 非 JSON、HTML 回退页
- 401、403、429、500
- 超时、重定向、超大响应
- Legacy Billing 配额与用量计算
- 配额结果禁止进入钱包现金余额
- 旧余额保留与同步失败状态

### 14.3 仓储与迁移测试

- 全新数据库执行迁移
- 从迁移 168 升级
- `initial_balance` 转资金事件
- 钱包账号关系时间区间不重叠
- 价格版本时间区间不重叠
- 财务记录重复扫描不重复插入
- 软删除账号历史回算
- 多实例并发回算
- 迁移新增 `upstream_cost_multiplier`，现有账号保持 `NULL`，不从 `rate_multiplier` 自动复制
- 账号 `upstream_cost_multiplier` 正确写入 `usage_logs.upstream_cost_multiplier`
- 原有 `rate_multiplier` 继续写入 `account_rate_multiplier`，两个快照互不覆盖
- 历史用量日志的非空上游倍率快照在回算中保持原值
- 财务扫描器计算已确认成本时不关联账号当前上游倍率
- 精确价格存在时保留倍率快照但不重复相乘，价格版本缺失时不使用当前目录价格补账
- 历史快照为空时不允许用账号当前倍率生成已确认成本
- 订阅按日确认收入等于请求分配收入加未分配收入
- 上游倍率变更同时保存旧值、新值、生效时间、操作人和原因
- 相同财务预警不重复创建，关闭后再次触发会生成新事件
- 钱包账单对账按账期幂等，`unsupported` 钱包不生成虚假差异率

### 14.4 报表勾稽测试

对同一时间范围验证：

```text
汇总收入 = 趋势收入合计 = 排行收入合计
汇总成本 = 趋势成本合计 = 排行成本合计
汇总利润 = 汇总收入 - 汇总成本
已确认请求数 + 待定价请求数 + 排除请求数 = 全部请求数
订阅确认收入 = 已分配订阅收入 + 未分配订阅收入
现金流 = 客户实际付款 - 客户退款 - 支付手续费 - 上游充值 + 上游退款 + 其他调整
```

### 14.5 前端测试

- 新增账号同时显示“账号计费倍率”和“上游倍率”，上游倍率默认值为 `1.0000`
- 编辑账号分别回填和保存两个倍率，修改其中一个不改变另一个
- 上游倍率发生变化时未填写变更原因不能提交，保存后变更历史完整显示
- 账号列表和批量编辑同时显示两个倍率及各自文案
- 非数字、负数和超过 4 位小数无法提交
- 财务明细只读显示倍率快照与来源，不存在倍率输入框
- 管理员用量详情显示倍率快照，客户侧用量列表和详情不返回该字段
- 财务明细“编辑账号”跳转到正确账号
- 正利润、零利润、负利润颜色和文案
- 覆盖率低于 99% 的固定警告
- 日期筛选和上一周期对比
- 客户、分组、模型、渠道、上游和账号六种亏损排行维度
- 亏损明细展开
- 上游钱包可用天数及 7 天、14 天预警状态
- 客户未消费余额负债与钱包现金余额分开展示
- 预警确认、处理和关闭流程
- 缺失金额显示“无法计算”，不显示为 0
- 定价同步失败和余额同步失败
- 手工精确价格版本新增，界面不包含统一倍率输入
- 钱包合并后的余额去重
- 空数据、加载失败和权限拒绝

### 14.6 线上验收

1. 选择 2 个支持公开定价同步的上游完成模型定价同步。
2. 为这 2 个上游分别配置手工钱包余额或用户中心财务令牌。
3. 选择 2 个手工上游：在账号详情设置上游倍率，并在财务配置录入需要覆盖的精确模型价格。
4. 连续观察 7 天新财务记录。
5. 每天抽样普通、Fast、缓存、图片和订阅请求。
6. 比对上游账单、CCAI 用量日志和财务记录。
7. 达到启用门槛后切换页面。

## 十五、回滚方案

- 首个版本只新增表和新接口，不删除旧字段。
- 新扫描器、同步器和新页面可以独立停止，不影响网关请求和用户扣费。
- 回滚时停止后台扫描器与同步器，恢复旧前端路由。
- 新财务表保留用于排查，不反向修改用量、余额或订单。
- 旧接口保留一个发布周期，页面切换失败时立即恢复。
- 删除旧字段只在新报表稳定运行一个发布周期后执行。

## 十六、不进入本次整改的内容

- 不解析账号名称自动生成倍率。
- 不抓取上游网页后台或模拟网页登录。
- 不把模型可用性接口当作财务接口。
- 不修改客户模型广场的销售价格逻辑。
- 不改变现有用户扣费、余额扣减和订阅限额流程。
- 不在请求主链路同步调用上游财务接口。

## 十七、最终验收视图

管理员进入财务报表后，首屏能够直接回答以下问题：

1. 今天、本月和所选期间是盈利还是亏损，金额是多少。
2. 亏损来自哪个上游、哪个钱包、哪个路由账号和哪个实际模型。
3. 亏损发生在普通价格、Fast、缓存、图片还是视频计费。
4. 当前结论覆盖了多少请求，还有多少请求缺少采购价格。
5. 每个上游当前剩余余额是多少，数据来自自动同步还是手工记录，最后更新时间是什么。
6. 历史哪一天开始亏损，累计亏损是多少，价格变化前后发生了什么。
7. 经营毛利与现金流分别是多少，不再把充值差额误当利润。
8. 每笔成本使用了哪个账号上游倍率快照，倍率来自哪个账号；需要调整时可以直接跳转账号详情，财务页无需重复输入。

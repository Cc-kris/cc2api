import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";

import FinanceStatsView from "../FinanceStatsView.vue";

const api = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getTrend: vi.fn(),
  getBreakdown: vi.fn(),
  getLosses: vi.fn(),
  getFunds: vi.fn(),
  createExport: vi.fn(),
  getExport: vi.fn(),
  downloadExport: vi.fn(),
}));

vi.mock("@/api/admin", () => ({ adminAPI: { finance: api } }));
vi.mock("vue-chartjs", () => ({
  Line: {
    props: ["data", "options"],
    template:
      '<div data-testid="finance-line-chart">{{ JSON.stringify(data) }}</div>',
  },
}));
vi.mock("chart.js", () => ({
  CategoryScale: {},
  Chart: { register: vi.fn() },
  Legend: {},
  LineElement: {},
  LinearScale: {},
  PointElement: {},
  Tooltip: {},
}));

const quality = {
  status: "partial",
  exact_count: 90,
  estimated_count: 4,
  missing_price_count: 2,
  missing_multiplier_count: 1,
  missing_usage_count: 1,
  non_billable_count: 0,
  unpriced_revenue: "12.50",
  cost_coverage_rate: "0.98",
};
const metric = (amount: string | null) => ({
  amount,
  currency: "USD",
  previous_amount: "80",
  change_rate: "0.25",
  status: amount === null ? "partial" : "complete",
});
const overview = {
  range: {
    start_date: "2026-07-01",
    end_date: "2026-07-27",
    timezone: "Asia/Shanghai",
  },
  revenue: metric("100"),
  upstream_cost: metric("70"),
  profit: metric("30"),
  margin_rate: null,
  today_profit: metric("2"),
  month_profit: metric("20"),
  historical_profit: metric("300"),
  historical_loss_amount: "25",
  estimated_cost_risk: "3",
  unconfirmed_exact_cost: "1.5",
  unpriced_revenue_risk: "12.50",
  loss_amount: "5",
  loss_request_count: 2,
  payment_net_cash: "50",
  upstream_net_cash: "-20",
  wallet_cash_total: "260",
  token_quota_wallet_count: 1,
  quality,
  open_alert_count: 1,
  generated_at: "2026-07-27T08:00:00Z",
};
const trend = [
  {
    bucket_start: "2026-07-25T16:00:00Z",
    bucket_end: "2026-07-27T00:00:00Z",
    revenue: "100",
    upstream_cost: "70",
    profit: "30",
    loss_amount: "5",
    margin_rate: "0.3",
    request_count: 10,
    quality,
  },
];
const breakdown = [
  {
    dimension_key: "7",
    dimension_name: "重点客户",
    revenue: "100",
    upstream_cost: "70",
    input_cost: "20",
    output_cost: "25",
    cache_cost: "5",
    fast_cost: "10",
    image_cost: "4",
    video_cost: "3",
    other_cost: "3",
    profit: "30",
    margin_rate: "0.3",
    loss_amount: "5",
    request_count: 10,
    exact_count: 9,
    estimated_count: 0,
    missing_count: 1,
  },
];
const losses = [
  {
    usage_log_id: 88,
    request_id: "req-loss",
    usage_created_at: "2026-07-26T08:00:00Z",
    user_id: 7,
    user_name: "亏损客户",
    group_id: 2,
    group_name: "企业组",
    channel_id: 3,
    channel_name: "渠道 A",
    account_id: 4,
    account_name: "上游账号 A",
    wallet_id: 5,
    wallet_name: "钱包 A",
    upstream_id: 6,
    upstream_name: "上游 A",
    requested_model: "gpt-5",
    upstream_model: "gpt-5",
    sales_pricing_version: "v1",
    revenue: "1",
    upstream_cost: "2",
    profit: "-1",
    margin_rate: "-1",
    cost_status: "exact",
    loss_amount: "1",
    loss_reason: "negative_profit",
    alert_id: 9,
    status: "open",
    assignee_id: 11,
    handled_by: null,
    handled_note: "",
    handled_at: null,
  },
];
const funds = {
  wallet_cash: [
    {
      wallet_id: 4,
      wallet_name: "现金钱包 B",
      balance_scope_key: "newapi-live",
      balance: "260",
      currency: "USD",
      daily_cost: "10",
      available_days: "26",
      collected_at: "2026-07-27T08:00:00Z",
      sync_status: "success",
      included_in_total: true,
      stale: false,
    },
    {
      wallet_id: 5,
      wallet_name: "现金钱包 A",
      balance_scope_key: "newapi-main",
      balance: "260",
      currency: "USD",
      daily_cost: "10",
      available_days: "26",
      collected_at: "2026-07-27T08:00:00Z",
      sync_status: "success",
      included_in_total: false,
      stale: false,
    },
  ],
  token_quota: [
    {
      wallet_id: 6,
      wallet_name: "配额钱包 A",
      total_quota: "1000000",
      used_quota: "100",
      remaining_quota: "999900",
      currency: "Token",
      collected_at: "2026-07-27T08:00:00Z",
      sync_status: "success",
    },
  ],
  customer_balance: "42",
  customer_cash: {
    payment: "100",
    refund: "10",
    payment_fees: "2",
    net_cash: "88",
  },
  upstream_cash: { topup: "60", topup_available: true, topup_event_count: 1, net_cash_available: true, event_count: 1, refund: "5", adjustment: "0", net_cash: "-55" },
  stale_wallet_count: 0,
  failed_sync_count: 0,
};
function resolvedDefaults() {
  api.getOverview.mockResolvedValue(overview);
  api.getTrend.mockResolvedValue(trend);
  api.getBreakdown.mockResolvedValue({
    items: breakdown,
    total: 1,
    page: 1,
    page_size: 100,
  });
  api.getLosses.mockResolvedValue({
    items: losses,
    total: 1,
    page: 1,
    page_size: 100,
  });
  api.getFunds.mockResolvedValue(funds);
  api.createExport.mockResolvedValue({
    id: 77,
    type: "finance_export",
    status: "completed",
    progress: "1",
    processed_count: 1,
    success_count: 1,
    failed_count: 0,
    report: "breakdown",
    format: "csv",
    file_size: 100,
    row_count: 1,
    expires_at: "2026-07-27T08:15:00Z",
    created_at: "2026-07-27T08:00:00Z",
    started_at: "2026-07-27T08:00:01Z",
    finished_at: "2026-07-27T08:00:02Z",
    error_summary: null,
    download_url: "/api/v1/admin/finance/exports/77/download?token=signed",
  });
  api.getExport.mockResolvedValue({});
  api.downloadExport.mockResolvedValue(new Blob(["csv"]));
}

function mountPage() {
  return mount(FinanceStatsView, {
    global: {
      stubs: {
        AppLayout: { template: "<div><slot /></div>" },
        AccountFinanceSettings: {
          emits: ["changed"],
          template: '<div data-testid="account-finance-settings">上游账号采购倍率<button data-testid="account-finance-changed" @click="$emit(\'changed\')">保存采购倍率</button></div>',
        },
        Pagination: {
          props: ["page", "pageSize", "total"],
          emits: ["update:page", "update:pageSize"],
          template:
            '<div><span data-testid="pagination-total">共 {{ total }} 条</span><button data-testid="pagination-next" @click="$emit(\'update:page\', page + 1)">下一页</button></div>',
        },
      },
    },
  });
}

describe("FinanceStatsView", () => {
  beforeEach(() => {
    Object.values(api).forEach((mock) => mock.mockReset());
    resolvedDefaults();
  });

  it("uses one filter scope and clearly marks incomplete profit data", async () => {
    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.text()).toContain("财务总览");
    expect(wrapper.text()).toContain("盈利分析");
    expect(wrapper.text()).toContain("上游账号");
    expect(wrapper.text()).not.toContain("需要处理");
    expect(wrapper.text()).not.toContain("财务初始化");
    expect(
      wrapper.get('[data-testid="finance-coverage-risk"]').text(),
    ).toContain("不能代表全站净利润");
    expect(wrapper.text()).toContain("无法计算");
    expect(wrapper.text()).toContain("客户消费金额");
    expect(wrapper.text()).toContain("钱在哪里");
    expect(wrapper.text()).not.toContain("日常只需要看这里");
    expect(wrapper.text()).not.toContain("先看这三个关系");
    expect(wrapper.get('[data-testid="finance-line-chart"]').text()).toContain(
      "利润",
    );

    const base = api.getOverview.mock.calls[0][0];
    expect(base).toMatchObject({
      granularity: "day",
      data_scope: "all",
    });
    expect(base.timezone).toBe("Asia/Shanghai");
    expect(wrapper.get('[data-testid="finance-line-chart"]').text()).toContain(
      "07/26",
    );
    expect(api.getTrend).toHaveBeenCalledWith(base);
    expect(api.getFunds).toHaveBeenCalledWith(base);
  });

  it("does not turn an overview API failure into zero-valued finance cards", async () => {
    api.getOverview.mockRejectedValue({ message: "经营总览接口不可用" });
    const wrapper = mountPage();
    await flushPromises();

    expect(wrapper.text()).toContain("经营总览接口不可用");
    expect(wrapper.find('[data-testid="finance-summary"]').exists()).toBe(
      false,
    );
    expect(wrapper.text()).not.toContain("$0.00");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "上游账号")!
      .trigger("click");
    expect(wrapper.text()).toContain("客户本期充值");
    expect(wrapper.text()).toContain("现金钱包 B");
  });

  it("renders loss details and the merged upstream finance source", async () => {
    const wrapper = mountPage();
    await flushPromises();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "盈利分析")!
      .trigger("click");
    expect(wrapper.text()).toContain("亏损客户");
    expect(wrapper.text()).toContain("上游账号 A");
    expect(wrapper.text()).not.toContain("系统记录状态");

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "上游账号")!
      .trigger("click");
    expect(wrapper.text()).toContain("现金钱包 B");
    expect(wrapper.text()).not.toContain("现金钱包 A");
    expect(wrapper.text()).not.toContain("Token 配额");
    expect(wrapper.text()).toContain("上游账号采购倍率");

    api.getOverview.mockClear();
    api.getFunds.mockClear();
    await wrapper.get('[data-testid="account-finance-changed"]').trigger("click");
    await flushPromises();
    expect(api.getOverview).toHaveBeenCalledTimes(1);
    expect(api.getFunds).toHaveBeenCalledTimes(1);
  });

  it("reloads the profit analysis when its dimension changes", async () => {
    const wrapper = mountPage();
    await flushPromises();
    api.getBreakdown.mockClear();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "盈利分析")!
      .trigger("click");

    const options = wrapper
      .get('[data-testid="finance-dimension"]')
      .findAll("option")
      .map((option) => option.element.value);
    expect(options).toEqual([
      "user",
      "group",
      "requested_model",
      "account",
    ]);

    await wrapper.get('[data-testid="finance-dimension"]').setValue("account");
    await flushPromises();

    expect(api.getBreakdown).toHaveBeenCalledTimes(2);
    expect(api.getBreakdown.mock.calls[1][0]).toMatchObject({
      dimension: "account",
      sort_by: "profit",
      sort_order: "asc",
    });
  });

  it("keeps the server total and loads later pages instead of truncating after 100 rows", async () => {
    api.getBreakdown.mockResolvedValue({
      items: breakdown,
      total: 120,
      page: 1,
      page_size: 50,
    });
    const wrapper = mountPage();
    await flushPromises();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "盈利分析")!
      .trigger("click");
    expect(wrapper.get('[data-testid="pagination-total"]').text()).toContain(
      "120",
    );
    api.getBreakdown.mockClear();

    await wrapper.get('[data-testid="pagination-next"]').trigger("click");
    await flushPromises();

    expect(api.getBreakdown).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2, page_size: 50 }),
    );
  });

  it("creates and downloads an asynchronous CSV export with the active filters", async () => {
    const createObjectURL = vi.fn(() => "blob:finance-export");
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: createObjectURL,
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: revokeObjectURL,
    });
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => undefined);
    const wrapper = mountPage();
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "盈利分析")!
      .trigger("click");

    await wrapper.get('[data-testid="finance-export-create"]').trigger("click");
    await flushPromises();
    expect(api.createExport).toHaveBeenCalledWith(
      expect.objectContaining({
        report: "breakdown",
        format: "csv",
        filters: expect.objectContaining({
          dimension: "user",
          data_scope: "all",
        }),
      }),
    );
    expect(
      wrapper.get('[data-testid="finance-export-status"]').text(),
    ).toContain("共 1 行");

    await wrapper
      .get('[data-testid="finance-export-download"]')
      .trigger("click");
    await flushPromises();
    expect(api.downloadExport).toHaveBeenCalledWith(
      "/api/v1/admin/finance/exports/77/download?token=signed",
    );
    expect(createObjectURL).toHaveBeenCalled();
    expect(click).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:finance-export");
  });

  it("does not expose one-time initialization or unactionable alert handling", async () => {
    const wrapper = mountPage();
    await flushPromises();
    const labels = wrapper.findAll("button").map((button) => button.text());
    expect(labels).not.toContain("需要处理");
    expect(labels).not.toContain("财务设置");
    expect(wrapper.text()).not.toContain("财务初始化");
  });
});

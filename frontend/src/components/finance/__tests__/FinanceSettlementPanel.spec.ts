import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";

import FinanceSettlementPanel from "../FinanceSettlementPanel.vue";

const api = vi.hoisted(() => ({
  getSettlements: vi.fn(),
  getSettlement: vi.fn(),
  retrySettlement: vi.fn(),
  reallocateSettlement: vi.fn(),
}));

vi.mock("@/api/admin", () => ({ adminAPI: { finance: api } }));

const interval = {
  id: 7,
  owner_type: "account",
  owner_id: 12,
  account_id: 12,
  scope_key: "account-12",
  previous_snapshot_id: 100,
  current_snapshot_id: 101,
  period_start: "2026-07-29T00:00:00Z",
  period_end: "2026-07-30T00:00:00Z",
  unit_semantics: "fiat_currency",
  currency: "USD",
  fx_rate_version_id: 5,
  fx_rate_to_usd: "1",
  fx_source: "currency_identity",
  list_cost_delta: "10",
  actual_cost_delta: "2.2",
  observed_multiplier: "0.22",
  status: "settled",
  current_revision: 2,
  request_count: 1,
  segment_count: 1,
  standard_cost_total: "10",
  allocated_cost_total: "2.2",
  difference_amount: "0",
};

const detail = {
  interval,
  allocations: [
    {
      id: 31,
      settlement_interval_id: 7,
      usage_log_id: 88,
      request_id: "req-settlement",
      attempt_no: 1,
      revision: 2,
      standard_cost_weight: "10",
      allocation_rate: "1",
      allocated_cost: "2.2",
      created_at: "2026-07-30T00:01:00Z",
    },
  ],
};

describe("FinanceSettlementPanel", () => {
  beforeEach(() => {
    api.getSettlements.mockReset().mockResolvedValue({ items: [interval], total: 1, page: 1, page_size: 20 });
    api.getSettlement.mockReset().mockResolvedValue(detail);
    api.retrySettlement.mockReset().mockResolvedValue(detail);
    api.reallocateSettlement.mockReset().mockResolvedValue({
      interval: { ...interval, current_revision: 3 },
      allocations: detail.allocations,
    });
  });

  it("shows exact interval amounts and creates a revision-protected reallocation", async () => {
    const wrapper = mount(FinanceSettlementPanel);
    await flushPromises();

    expect(wrapper.text()).toContain("0.22x");
    expect(wrapper.text()).toContain("已结算");
    await wrapper.findAll("button").find((button) => button.text() === "查看分摊")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("req-settlement");
    expect(wrapper.get('[data-testid="finance-settlement-fx-evidence"]').text()).toContain("版本 #5");

    await wrapper.get('input[placeholder="说明重新分摊的业务原因"]').setValue("修正标准成本权重");
    await wrapper.get("form").trigger("submit");
    await flushPromises();

    expect(api.reallocateSettlement).toHaveBeenCalledWith(7, {
      expected_revision: 2,
      reason: "修正标准成本权重",
    });
    expect(wrapper.text()).toContain("当前修订 v3");
  });

  it("retries a review interval without exposing reallocation first", async () => {
    const reviewInterval = { ...interval, status: "needs_review", current_revision: 1 };
    api.getSettlements.mockResolvedValue({ items: [reviewInterval], total: 1, page: 1, page_size: 20 });
    api.getSettlement.mockResolvedValue({ interval: reviewInterval, allocations: [] });
    const wrapper = mount(FinanceSettlementPanel);
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "查看分摊")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("重新结算");
    expect(wrapper.text()).not.toContain("创建新修订");
    await wrapper.findAll("button").find((button) => button.text() === "重新结算")!.trigger("click");
    await flushPromises();
    expect(api.retrySettlement).toHaveBeenCalledWith(7);
  });

  it("keeps interval deltas in source currency and renders allocations in USD", async () => {
    const cnyInterval = {
      ...interval,
      currency: "CNY",
      list_cost_delta: "10",
      actual_cost_delta: "2.2",
      allocated_cost_total: "0.308",
      difference_amount: "0.001",
      fx_rate_to_usd: "0.14",
    };
    api.getSettlements.mockResolvedValue({ items: [cnyInterval], total: 1, page: 1, page_size: 20 });
    api.getSettlement.mockResolvedValue({
      interval: cnyInterval,
      allocations: [{ ...detail.allocations[0], allocated_cost: "0.308" }],
    });

    const wrapper = mount(FinanceSettlementPanel);
    await flushPromises();
    expect(wrapper.text()).toContain("¥10.00");
    expect(wrapper.text()).toContain("$0.00");

    await wrapper.findAll("button").find((button) => button.text() === "查看分摊")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("$0.31");
    expect(wrapper.text()).toContain("分摊金额统一为 USD");
  });
});

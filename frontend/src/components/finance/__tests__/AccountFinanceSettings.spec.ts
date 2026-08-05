import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";

import AccountFinanceSettings from "../AccountFinanceSettings.vue";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  update: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}));

vi.mock("@/api/admin", () => ({
  adminAPI: { accounts: { list: mocks.list, update: mocks.update } },
}));
vi.mock("@/stores/app", () => ({
  useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError }),
}));

const account = {
  id: 7,
  name: "OpenAI 生图账号",
  platform: "openai",
  type: "apikey",
  upstream_cost_multiplier: "0.8000",
  status: "active",
};

describe("AccountFinanceSettings", () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
    mocks.list.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 1000 });
    mocks.update.mockResolvedValue({ ...account, upstream_cost_multiplier: "0.7500" });
  });

  it("loads the multiplier already stored on each account and does not expose URL maintenance", async () => {
    const wrapper = mount(AccountFinanceSettings);
    await flushPromises();

    expect(mocks.list).toHaveBeenCalledWith(1, 1000, {
      lite: "true",
      sort_by: "name",
      sort_order: "asc",
    });
    expect(wrapper.text()).toContain("OpenAI 生图账号");
    expect(wrapper.text()).toContain("0.8000");
    expect(wrapper.text()).not.toContain("Base URL");
    expect(wrapper.text()).not.toContain("初始化");
  });

  it("updates only the selected account multiplier with an automatic audit reason", async () => {
    const wrapper = mount(AccountFinanceSettings);
    await flushPromises();

    await wrapper.get("button.text-primary-700").trigger("click");
    await wrapper.get('input[type="number"]').setValue("0.75");
    await wrapper.findAll("button").find((button) => button.text() === "保存")!.trigger("click");
    await flushPromises();

    expect(mocks.update).toHaveBeenCalledWith(7, {
      upstream_cost_multiplier: "0.7500",
      upstream_cost_multiplier_change_reason: "经营与财务修改上游采购倍率",
    });
    expect(wrapper.text()).toContain("0.7500");
    expect(wrapper.find('input[type="number"]').exists()).toBe(false);
    expect(mocks.showSuccess).toHaveBeenCalledWith("采购倍率已保存");
  });

  it("rejects multipliers with more than four decimal places", async () => {
    const wrapper = mount(AccountFinanceSettings);
    await flushPromises();

    await wrapper.get("button.text-primary-700").trigger("click");
    await wrapper.get('input[type="number"]').setValue("0.12345");
    await wrapper.findAll("button").find((button) => button.text() === "保存")!.trigger("click");

    expect(wrapper.text()).toContain("最多 4 位小数");
    expect(mocks.update).not.toHaveBeenCalled();
  });

  it("defaults an account without a stored multiplier to 1 instead of zero", async () => {
    mocks.list.mockResolvedValue({
      items: [{ ...account, upstream_cost_multiplier: null }],
      total: 1,
      page: 1,
      page_size: 1000,
    });
    const wrapper = mount(AccountFinanceSettings);
    await flushPromises();

    await wrapper.get("button.text-primary-700").trigger("click");

    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe("1.0000");
  });
});

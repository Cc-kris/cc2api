import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
const read = (p: string) => readFileSync(fileURLToPath(new URL(p, import.meta.url)), "utf8");
const css = read("./style.css");
const sources: Record<string, string> = {
  style: css,
  content: read("./views/admin/ContentCenterView.vue"),
  publicContent: read("./views/public/PublicContentView.vue"),
  finance: read("./views/admin/FinanceStatsView.vue"),
};
const bannedGreen = /emerald-|#10b981\b|#059669\b|#047857\b|#065f46\b|#d1fae5\b|#00c896\b|#7af0c4\b|#84cc16\b|rgba?\(\s*16\s*,\s*185\s*,\s*129/gi;

describe("neo-lab v0.4 全站 token 与原生控件", () => {
  it("声明设计契约 token", () => {
    const tokens = ["--v04-primary: #CCFF00", "--v04-primary-hover: #D8FF33", "--v04-accent: #CCFF00",
      "--v04-surface: #FFFFFF", "--v04-success: #16A34A", "--v04-page: #121316", "--v04-surface: #1F2937", "--v04-link: #60A5FA"];
    tokens.forEach((t) => expect(css).toContain(t));
  });
  it("允许文件无旧绿残留", () => {
    Object.entries(sources).forEach(([name, src]) => {
      const hits = src.match(bannedGreen) ?? [];
      expect(hits, `${name} 残留: ${hits.join(",")}`).toHaveLength(0);
    });
  });
  it("选中/成功态使用契约色", () => {
    expect(css).toMatch(/\.switch-checked\s*\{[^}]*background-color:\s*#CCFF00/i);
    expect(css).toMatch(/\.task-status\.chip-success\s*\{[^}]*#16A34A/i);
  });
  it("原生下拉使用自定义箭头与焦点环", () => {
    expect(css).toMatch(/select:not\(\[multiple\]\)\s*\{[^}]*appearance:\s*none/i);
    expect(css).toMatch(/select:not\(\[multiple\]\)\s*\{[^}]*background-image:\s*url\("data:image\/svg\+xml/);
    const patterns = [/html\.dark select:not\(\[multiple\]\)/, /select:not\(\[multiple\]\):focus-visible/, /select:not\(\[multiple\]\) option\s*\{/];
    patterns.forEach((p) => expect(css).toMatch(p));
  });
  it("登录/404 与后台暗色动作按钮不回退为旧蓝白样式", () => {
    const primary = css.match(/html\.dark \.btn-primary,\s*html\.dark button\.bg-primary-600,[\s\S]*?\{([^}]*)\}/);
    expect(primary?.[1]).toContain("background-color: #CCFF00 !important");
    expect(primary?.[1]).toContain("color: #121316 !important");
    expect(primary?.[1]).toContain("border-color: #121316 !important");
    const secondaryHover = css.match(/html\.dark \.btn-secondary:hover:not\(:disabled\),\s*html\.dark \.btn-ghost:hover:not\(:disabled\)\s*\{([^}]*)\}/);
    expect(secondaryHover?.[1]).toContain("border-color: #E2E8F0 !important");
    expect(css).toMatch(/html\.dark \.tab-active\s*\{[^}]*color:\s*#CCFF00/i);
  });
  it("输入框无下划线且表格分隔线保留", () => {
    const control = /(^|[\s,>+~(])(input|textarea|select)(?=[\s,.:#[>+~)]|$)/;
    const blocks = [...css.matchAll(/([^{}]+)\{([^{}]*)\}/g)];
    const underline = blocks.filter(([, sel, body]) => control.test(sel) && /border-bottom(?:-width|-style|-color)?\s*:/.test(body));
    expect(underline.map(([, sel]) => sel.trim()), "输入/下拉使用了下划线边框").toEqual([]);
    const rows = blocks.filter(([, sel]) => sel.includes(".table tbody tr"));
    expect(rows.some(([, , body]) => /border-bottom/.test(body)), "表格行分隔线丢失").toBe(true);
  });
});

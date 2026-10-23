import { apiClient } from "../client";
export const contentAdminAPI = {
  listNews: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/content/news", { params }),
  listSources: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/content/news-sources", { params }),
  getSource: (id: number) => apiClient.get(`/admin/content/news-sources/${id}`),
  createSource: (body: unknown) =>
    apiClient.post("/admin/content/news-sources", body),
  updateSource: (id: number, body: unknown) =>
    apiClient.put(`/admin/content/news-sources/${id}`, body),
  toggleSource: (id: number, enabled: boolean) =>
    apiClient.post(
      `/admin/content/news-sources/${id}/${enabled ? "enable" : "disable"}`,
    ),
  deleteSource: (id: number) =>
    apiClient.delete(`/admin/content/news-sources/${id}`),
  restoreSource: (id: number) =>
    apiClient.post(`/admin/content/news-sources/${id}/restore`),
  fetchSource: (id: number) =>
    apiClient.post(`/admin/content/news-sources/${id}/fetch`, {
      idempotencyKey: `manual-${id}-${Date.now()}`,
    }),
  getNews: (id: number) => apiClient.get(`/admin/content/news/${id}`),
  updateNews: (id: number, body: unknown) =>
    apiClient.put(`/admin/content/news/${id}`, body),
  restoreNews: (id: number) =>
    apiClient.post(`/admin/content/news/${id}/restore`),
  updateAutomation: (body: unknown) =>
    apiClient.put("/admin/content/automation-settings", body),
  publishNews: (id: number) =>
    apiClient.post(`/admin/content/news/${id}/publish`),
  unpublishNews: (id: number) =>
    apiClient.post(`/admin/content/news/${id}/unpublish`),
  deleteNews: (id: number) => apiClient.delete(`/admin/content/news/${id}`),
  bulkNews: (body: unknown) => apiClient.post("/admin/content/news/bulk", body),
  listArticles: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/content/articles", { params }),
  getArticle: (id: number) => apiClient.get(`/admin/content/articles/${id}`),
  createArticle: (body: unknown) =>
    apiClient.post("/admin/content/articles", body),
  publishArticle: (id: number) =>
    apiClient.post(`/admin/content/articles/${id}/publish`),
  updateArticle: (id: number, body: unknown) =>
    apiClient.put(`/admin/content/articles/${id}`, body),
  unpublishArticle: (id: number) =>
    apiClient.post(`/admin/content/articles/${id}/unpublish`),
  deleteArticle: (id: number) =>
    apiClient.delete(`/admin/content/articles/${id}`),
  bulkArticles: (body: unknown) =>
    apiClient.post("/admin/content/articles/bulk", body),
  listTranslations: (id: number, params?: Record<string, unknown>) =>
    apiClient.get(`/admin/content/articles/${id}/translations`, { params }),
  createTranslation: (id: number, body: unknown) =>
    apiClient.post(`/admin/content/articles/${id}/translations`, body),
  updateTranslation: (id: number, body: unknown) =>
    apiClient.put(`/admin/content/articles/translations/${id}`, body),
  publishTranslation: (id: number) =>
    apiClient.post(`/admin/content/articles/translations/${id}/publish`),
  unpublishTranslation: (id: number) =>
    apiClient.post(`/admin/content/articles/translations/${id}/unpublish`),
  createTranslationTask: (id: number, body: unknown) =>
    apiClient.post(`/admin/content/articles/${id}/translations/tasks`, body),
  analytics: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics", { params }),
  analyticsOverview: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics/overview", { params }),
  analyticsRankings: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics/rankings", { params }),
  listFetchRuns: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/content/fetch-runs", { params }),
  // 后端强制要求 Idempotency-Key 头（缺失即 400），重试按钮此前 100% 失效
  retryFetchRun: (id: number) =>
    apiClient.post(`/admin/content/fetch-runs/${id}/retry`, undefined, {
      headers: { "Idempotency-Key": `retry-run-${id}-${Date.now()}` },
    }),
  getAutomation: () => apiClient.get("/admin/content/automation-settings"),
  createWritingTask: (body: unknown) =>
    apiClient.post("/admin/content/articles/writing-tasks", body),
  listWritingTasks: (limit = 20) =>
    apiClient.get("/admin/content/articles/writing-tasks", { params: { limit } }),
  getWritingTask: (id: string) =>
    apiClient.get(`/admin/content/articles/writing-tasks/${id}`),
  cancelWritingTask: (id: string) =>
    apiClient.post(`/admin/content/articles/writing-tasks/${id}/cancel`),
  deleteWritingTask: (id: string) =>
    apiClient.delete(`/admin/content/articles/writing-tasks/${id}`),
  uploadContentImage: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return apiClient.post("/admin/content/upload", form, {
      headers: { "Content-Type": "multipart/form-data" },
    });
  },
  models: (platform?: string) =>
    apiClient.get("/admin/content/articles/models", { params: { platform } }),
  getAIConfig: () =>
    apiClient.get("/admin/content/articles/ai-config"),
  updateAIConfig: (body: unknown) =>
    apiClient.put("/admin/content/articles/ai-config", body),
  fetchAIModels: (body: unknown) =>
    apiClient.post("/admin/content/articles/ai-config/fetch-models", body),
  getTaxonomy: () =>
    apiClient.get("/admin/content/articles/taxonomy"),
  mutateTaxonomy: (body: { scope: string; action: string; name: string; newName?: string }) =>
    apiClient.post("/admin/content/articles/taxonomy/mutate", body),
  analyticsTrends: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics/trends", { params }),
  analyticsCTR: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics/click-through-rate", { params }),
  analyticsExport: (params?: Record<string, unknown>) =>
    apiClient.get("/admin/bi/analytics/export.csv", {
      params,
      responseType: "blob",
    }),
};
export default contentAdminAPI;

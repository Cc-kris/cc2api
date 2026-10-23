<template>
  <AppLayout>
    <div class="admin-page">
    <div class="top-actions-bar">
      <div class="top-tabs-title">内容中心</div>
      <button class="button" @click="refresh">刷新</button>
    </div>
    <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    <div class="tabs">
      <button
        v-for="tab in tabs"
        :key="tab"
        :class="{ active: current === tab }"
        @click="current = tab"
      >
        {{ tab }}
      </button>
    </div>
    <section v-if="current === '新闻动态'" class="workspace">
      <div class="subnav">
        <button
          :class="{ selected: newsView === 'items' }"
          @click="newsView = 'items'"
        >
          已抓取新闻</button
        ><button
          :class="{ selected: newsView === 'sources' }"
          @click="newsView = 'sources'"
        >
          新闻源</button
        ><button
          :class="{ selected: newsView === 'runs' }"
          @click="newsView = 'runs'"
        >
          抓取任务</button
        ><button
          :class="{ selected: newsView === 'automation' }"
          @click="newsView = 'automation'"
        >
          自动化设置
        </button>
      </div>
      <div v-if="newsView === 'items'">
        <div class="toolbar">
          <input
            v-model="newsSearch"
            placeholder="搜索标题或摘要"
            @keyup.enter="loadNews"
          /><select v-model="newsStatus" @change="loadNews">
            <option value="">全部状态</option>
            <option value="published">已发布</option>
            <option value="draft">草稿</option>
            <option value="pending_review">待审核</option></select
          ><input
            v-model="newsLocale"
            placeholder="语言"
            @keyup.enter="loadNews"
          /><input
            v-model="newsSourceId"
            type="number"
            min="1"
            placeholder="来源 ID"
            @keyup.enter="loadNews"
          /><input v-model="newsFrom" type="date" /><input
            v-model="newsTo"
            type="date"
          /><button class="button" @click="loadNews">查询</button>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>
                  <input
                    v-model="allNewsSelected"
                    type="checkbox"
                    @change="toggleAllNews"
                  />
                </th>
                <th>标题</th>
                <th>来源</th>
                <th>状态</th>
                <th>发布时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in news" :key="item.id">
                <td>
                  <input
                    v-model="selectedNewsIds"
                    :value="item.id"
                    type="checkbox"
                  />
                </td>
                <td class="title-cell">
                  <a
                    v-if="item.originalUrl"
                    class="title-link"
                    :href="item.originalUrl"
                    target="_blank"
                    rel="noopener noreferrer"
                    :title="'打开原文：' + item.originalUrl"
                  >{{ item.title }}</a>
                  <button v-else class="title-link" type="button" @click="editNews(item)">
                    {{ item.title }}
                  </button>
                  <small v-if="item.summary" class="summary-clamp">{{ item.summary }}</small>
                  <small class="locale-tag">{{ item.locale }}</small>
                </td>
                <td>{{ item.sourceName || "-" }}</td>
                <td>
                  <span class="status">{{ item.status }}</span>
                </td>
                <td>{{ formatDate(item.publishedAt) }}</td>
                <td class="actions-cell actions-sticky">
                    <a
                      v-if="item.originalUrl"
                      class="link"
                      :href="item.originalUrl"
                      target="_blank"
                      rel="noopener noreferrer"
                    >原文</a>
                    <button type="button" class="link" @click="editNews(item)">编辑</button>
                    <button type="button" class="link" @click="publishNews(item.id)" v-if="item.status !== 'published'">发表</button>
                    <button type="button" class="link danger" @click="deleteNews(item.id)">删除</button>
                  </td>
              </tr>
            </tbody>
          </table>
          <p v-if="!news.length" class="empty">暂无新闻</p>
          <div class="table-footer">
            <div class="bulk-actions">
              <button
                class="link"
                :class="{ 'is-muted': !selectedNewsIds.length }"
                @click="bulkNews('unpublish')"
              >
                批量下架
              </button>
              <button
                class="danger link"
                :class="{ 'is-muted': !selectedNewsIds.length }"
                @click="bulkNews('delete')"
              >
                批量删除
              </button>
            </div>
            <div class="pagination">
              <button class="link" :disabled="newsPage <= 1" @click="newsPage--; loadNews()">上一页</button>
              <span class="page-info">第 {{ newsPage }} 页 · 共 {{ newsTotal }} 条</span>
              <button class="link" :disabled="!newsHasNext" @click="newsPage++; loadNews()">下一页</button>
            </div>
          </div>
        </div>
        <div
          v-if="selectedNews.id"
          class="modal-backdrop z-overlay"
          @click.self="selectedNews = {}"
        >
          <div ref="newsEditorRef" class="news-editor news-editor-modal news-editor-flex">
          <div class="section-heading modal-fixed-header">
            <div>
              <h2>新闻详情编辑</h2>
              <p class="muted">编辑抓取正文、摘要、分类标签和 SEO 字段后再发布。</p>
            </div>
            <button class="link" @click="selectedNews = {}">关闭</button>
          </div>
          <div class="modal-scroll-body">
          <div class="form-grid">
            <label
              >标题<input v-model="selectedNews.title" maxlength="240"
            /></label>
            <label
              >Slug<input v-model="selectedNews.slug" maxlength="240"
            /></label>
            <label>语言<input v-model="selectedNews.locale" /></label>
            <label
              >分类<input
                v-model="selectedNews.category"
                list="news-category-options"
                placeholder="如 模型发布 / 工程架构"
              /><datalist id="news-category-options">
                <option v-for="c in taxonomy.newsCategories" :key="c" :value="c" />
              </datalist></label>
            <label
              >标签<input
                v-model="selectedNewsTagsText"
                placeholder="多个标签用英文逗号分隔"
            /></label>
            <label
              >原文链接<input
                v-model="selectedNews.originalUrl"
                placeholder="https://..."
            /></label>
            <label
              >SEO 标题<input v-model="selectedNews.seoTitle" maxlength="160"
            /></label>
            <label
              >SEO 描述<input
                v-model="selectedNews.seoDescription"
                maxlength="300"
            /></label>
            <label>来源名称<input v-model="selectedNews.sourceName" /></label>
          </div>
          <div class="two-col-editor">
            <div class="editor-config-col">
              <div class="config-block">
                <div class="config-title">摘要</div>
                <textarea v-model="selectedNews.summary" class="short-text" placeholder="摘要"></textarea>
              </div>
            </div>
            <div class="editor-main-col">
              <div class="editor-label">新闻正文（富文本）</div>
              <AnnouncementRichTextEditor v-model="selectedNews.bodyHtml" placeholder="新闻正文..." />
            </div>
          </div>
          </div>
          <div class="form-actions modal-fixed-footer">
            <button class="button" @click="saveNews">保存新闻</button>
            <button class="secondary-button" @click="selectedNews = {}">
              取消
            </button>
          </div>
          </div>
        </div>
      </div>
      <div v-else-if="newsView === 'sources'">
        <div class="toolbar source-toolbar">
          <input
            v-model="sourceForm.name"
            :class="{ 'input-invalid': sourceFormErrors.name }"
            placeholder="来源名称（必填）"
            @input="sourceFormErrors.name = ''"
            @blur="validateSourceField('name')"
          /><input
            v-model="sourceForm.url"
            :class="{ 'input-invalid': sourceFormErrors.url }"
            placeholder="公开 RSS/Atom/JSON URL（必填）"
            @input="sourceFormErrors.url = ''"
            @blur="validateSourceField('url')"
          /><select v-model="sourceForm.sourceType">
            <option value="rss">RSS</option>
            <option value="atom">Atom</option>
            <option value="public_json">Public JSON</option></select
          ><button
            class="button"
            :disabled="sourceCreateDisabled"
            :title="sourceCreateBlockedReason"
            @click="createSource"
          >
            新增新闻源
          </button>
          <textarea v-if="sourceForm.sourceType === 'public_json'" v-model="sourceForm.mappingJsonText" class="short-text" placeholder='JSON 字段映射，例如 {"title":"$.headline","body":"$.article.text","url":"$.article.href"}'></textarea>
        </div>
        <div v-if="sourceFormErrors.name || sourceFormErrors.url" class="field-errors">
          <small v-if="sourceFormErrors.name" class="error-text">来源名称必填</small>
          <small v-if="sourceFormErrors.url" class="error-text">{{ sourceFormErrors.url }}</small>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>类型</th>
                <th>调度</th>
                <th>URL</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="source in sources" :key="source.id">
                <td>{{ source.name }}</td>
                <td>{{ source.sourceType }} · {{ source.language || "-" }}</td>
                <td>
                  <small>{{ sourceScheduleText(source) }}</small>
                  <small v-if="source.nextRunAt" class="muted">下次：{{ formatDate(source.nextRunAt) }}</small>
                </td>
                <td class="url">{{ source.url }}</td>
                <td>
                  {{ source.enabled ? "启用" : "停用"
                  }}<small v-if="source.nextRunAt"
                    >下次：{{ formatDate(source.nextRunAt) }}</small
                  ><small v-if="source.lastError" class="error-text">{{
                    source.lastError
                  }}</small>
                </td>
                <td>
                  <button class="link" @click="editSource(source)">编辑</button>
                  <button class="link" @click="toggleSource(source)">
                    {{ source.enabled ? "停用" : "启用" }}</button
                  ><button class="link" @click="fetchSource(source.id)">
                    立即抓取</button
                  ><button class="danger link" @click="deleteSource(source.id)">
                    删除</button
                  ><button
                    v-if="source.deletedAt"
                    class="link"
                    @click="restoreSource(source.id)"
                  >
                    恢复
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <div class="table-footer">
            <div></div>
            <div class="pagination">
              <button class="link" :disabled="sourcePage <= 1" @click="sourcePage--; loadSources()">上一页</button>
              <span class="page-info">第 {{ sourcePage }} 页 · 共 {{ sourceTotal }} 条</span>
              <button class="link" :disabled="!sourceHasNext" @click="sourcePage++; loadSources()">下一页</button>
            </div>
          </div>
        </div>
        <div v-if="selectedSource.id" class="news-editor">
          <div class="section-heading">
            <h2>编辑新闻源</h2>
            <button class="link" @click="selectedSource = {}">关闭</button>
          </div>
          <div class="form-grid">
            <label>名称<input v-model="selectedSource.name" /></label
            ><label>Feed URL<input v-model="selectedSource.url" /></label
            ><label>语言<input v-model="selectedSource.language" /></label
            ><label
              >间隔（分钟）<input
                v-model.number="selectedSource.intervalMinutes"
                type="number"
                min="1" /></label
            ><label
              >每分钟速率<input
                v-model.number="selectedSource.rateLimitPerMinute"
                type="number"
                min="1" /></label
            ><label>分类<input v-model="selectedSource.category" /></label>
            ><label v-if="selectedSource.sourceType === 'public_json'">JSON 字段映射<textarea v-model="selectedSource.mappingJsonText" class="short-text" placeholder='{"title":"$.headline","body":"$.article.text"}'></textarea></label>
          </div>
          <button class="button" @click="saveSource">保存新闻源</button>
        </div>
      </div>
      <div v-else-if="newsView === 'runs'">
        <div class="table-wrap runs-table-wrap">
          <table class="runs-table">
            <colgroup>
              <col style="width: 72px" />
              <col style="width: 150px" />
              <col style="width: 84px" />
              <col style="width: 92px" />
              <col style="width: 132px" />
              <col style="width: 132px" />
              <col style="width: 64px" />
              <col style="width: 64px" />
              <col style="width: 64px" />
              <col />
              <col style="width: 84px" />
            </colgroup>
            <thead>
              <tr>
                <th>任务</th>
                <th>来源</th>
                <th>触发</th>
                <th>状态</th>
                <th>开始</th>
                <th>结束</th>
                <th>新增</th>
                <th>重复</th>
                <th>失败</th>
                <th class="cell-error-summary">错误摘要</th>
                <th class="actions-th actions-cell">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="run in runs" :key="run.id">
                <td>#{{ run.id }}</td>
                <td class="cell-ellipsis" :title="sourceName(run.sourceId)">{{ sourceName(run.sourceId) }}</td>
                <td>{{ run.triggerType || "-" }}</td>
                <td>{{ run.status }}</td>
                <td>{{ formatDateTime(run.startedAt) }}</td>
                <td>{{ formatDateTime(run.finishedAt) }}</td>
                <td>{{ run.createdCount }}</td>
                <td>{{ run.duplicateCount }}</td>
                <td>{{ run.failedCount }}</td>
                <td class="error-text cell-error-summary" :title="run.errorSummary || '-'">{{ run.errorSummary || "-" }}</td>
                <td class="actions-cell actions-sticky">
                  <button
                    v-if="
                      [
                        'failed',
                        'partial',
                        'cancelled',
                        'interrupted',
                      ].includes(run.status)
                    "
                    class="link"
                    @click="retryRun(run.id)"
                  >
                    重试
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <div class="table-footer">
            <div></div>
            <div class="pagination">
              <button class="link" :disabled="runPage <= 1" @click="runPage--; loadRuns()">上一页</button>
              <span class="page-info">第 {{ runPage }} 页 · 共 {{ runTotal }} 条</span>
              <button class="link" :disabled="!runHasNext" @click="runPage++; loadRuns()">下一页</button>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="settings">
        <label class="switch"
          ><input v-model="automation.enabled" type="checkbox" /><span
            >启用定时抓取与自动发布</span
          ></label
        >
        <div class="form-grid">
          <label>时区<input v-model="automation.timezone" /></label
          ><label>Cron<input v-model="automation.fetchCron" /></label
          ><label
            >发布模式<select v-model="automation.publishMode">
              <option value="manual_review">人工审核</option>
              <option value="auto_publish">自动发布</option>
            </select></label
          ><label
            >每日抓取上限（0=不限）<input
              v-model.number="automation.dailyFetchLimit"
              type="number"
              min="0" /></label
          ><label
            >每日发布上限（0=不限）<input
              v-model.number="automation.dailyPublishLimit"
              type="number"
              min="0" /></label
          ><label
            >发布时间窗开始<input
              v-model="automation.publishWindowStart"
              type="time"
              placeholder="09:00" /></label
          ><label
            >发布时间窗结束<input
              v-model="automation.publishWindowEnd"
              type="time"
              placeholder="22:00" /></label
          ><label
            >去重窗口（小时，必填）<input
              v-model.number="automation.dedupeWindowHours"
              type="number"
              min="1"
              :class="{ 'input-invalid': automationErrors.dedupeWindowHours }"
              @input="automationErrors.dedupeWindowHours = ''" /></label
          ><label
            >目标语言<input
              v-model="automation.targetLocalesText"
              placeholder="zh-CN,en-US"
          /></label>
        </div>
        <div v-if="automationErrors.dedupeWindowHours" class="field-errors">
          <small class="error-text">{{ automationErrors.dedupeWindowHours }}</small>
        </div>
        <button class="button" @click="saveAutomation">保存自动化设置</button>
      </div>

      <!-- 删除新闻确认弹窗（新闻与文章是两套接口，不能复用文章的处理函数） -->
      <div v-if="newsDeleteConfirm.show" class="modal-backdrop z-overlay" @click.self="newsDeleteConfirm.show = false">
        <div class="inline-tax-dialog">
          <h3>确认删除这条新闻？</h3>
          <p class="muted" style="margin:0">删除后不可恢复（可在回收站还原）。</p>
          <div class="inline-tax-actions">
            <button class="secondary-button" @click="newsDeleteConfirm.show = false">取消</button>
            <button
              class="button primary"
              :disabled="newsDeleteConfirm.deleting"
              @click="confirmDeleteNews"
            >{{ newsDeleteConfirm.deleting ? "删除中..." : "确认删除" }}</button>
          </div>
        </div>
      </div>
    </section>
        <section v-else-if="current === '深度解析'" class="workspace articles-pane">
      <div class="table-top-actions">
        <div class="toolbar flex-grow">
          <input
            v-model="articleSearch"
            placeholder="搜索文章标题或摘要..."
            class="search-input"
            @keyup.enter="loadArticles"
          />
          <select v-model="articleStatus" @change="loadArticles">
            <option value="">全部状态</option>
            <option value="published">已发布</option>
            <option value="draft">草稿</option>
            <option value="pending_review">待审核</option>
          </select>
          <select v-model="articleModel" @change="loadArticles">
            <option value="">全部模型</option>
            <option v-for="m in articleModelFilterOptions" :key="m" :value="m">
              {{ m }}
            </option>
          </select>
          <small v-if="modelsLoadError" class="error-text">{{ modelsLoadError }}</small>
          <input v-model="articleLocale" placeholder="语言 (zh-CN)" @keyup.enter="loadArticles" style="width: 110px;" />
          <input v-model="articleCategory" placeholder="分类" @keyup.enter="loadArticles" style="width: 100px;" />
          <input v-model="articleFrom" type="date" />
          <input v-model="articleTo" type="date" />
          <button class="button" @click="loadArticles">查询</button>
        </div>
        <div class="create-action">
          <button class="button primary new-btn" @click="openNewArticle">
            + 新建文章
          </button>
        </div>
      </div>

      <!-- 表格内容 -->
      <div class="table-wrap">
        <table>
              <thead>
                <tr>
                  <th>
                    <input
                      v-model="allArticlesSelected"
                      type="checkbox"
                      @change="toggleAllArticles"
                    />
                  </th>
                  <th>标题</th>
                  <th>语言</th>
                  <th>状态</th>
                  <th>精选</th>
                  <th>阅读数</th>
                  <th>发布时间</th>
                  <th class="actions-th">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in articles" :key="item.id">
                  <td>
                    <input
                      v-model="selectedArticleIds"
                      :value="item.id"
                      type="checkbox"
                    />
                  </td>
                  <td class="title-cell">
                    <button class="title-link" @click="editArticle(item)">
                      {{ item.title }}
                    </button>
                  </td>
                  <td><span class="locale-tag">{{ item.locale }}</span></td>
                  <td><span class="status">{{ item.status }}</span></td>
                  <td class="featured-cell" @click.stop>
                    <input
                      type="checkbox"
                      class="featured-toggle"
                      title="设为精选 / 取消精选（仅 1 篇）"
                      :checked="isArticleRowFeatured(item)"
                      :disabled="articleFeatureBusyId === item.id"
                      @change="toggleArticleFeatured(item, $event)"
                    />
                  </td>
                  <td>{{ item.readCount || 0 }}</td>
                  <td>{{ formatDate(item.publishedAt) }}</td>
                  <td class="actions-cell">
                    <button
                      class="link"
                      @click="editArticle(item)"
                    >
                      编辑
                    </button>
                    <button
                      class="link"
                      @click="
                        item.status === 'published'
                          ? unpublishArticle(item.id)
                          : publishArticle(item.id)
                      "
                    >
                      {{
                        item.status === "published" ? "下架" : "发布"
                      }}</button
                    ><button
                      class="danger link"
                      @click="deleteArticle(item.id)"
                    >
                      删除
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
            <p v-if="!articles.length" class="empty">暂无文章</p>
            <div class="table-footer">
              <div class="bulk-actions">
                <button
                  class="link"
                  :disabled="!selectedArticleIds.length"
                  @click="bulkArticles('unpublish')"
                >
                  批量下架
                </button>
                <button
                  class="danger link"
                  :disabled="!selectedArticleIds.length"
                  @click="bulkArticles('delete')"
                >
                  批量删除
                </button>
              </div>
              <div class="pagination">
                <button class="link" :disabled="articlePage <= 1" @click="articlePage--; loadArticles()">上一页</button>
                <span class="page-info">第 {{ articlePage }} 页 · 共 {{ articleTotal }} 条</span>
                <button class="link" :disabled="!articleHasNext" @click="articlePage++; loadArticles()">下一页</button>
              </div>
            </div>
          </div>

      <!-- 独立文章编辑弹窗（新建/编辑统一，居中，左右布局） -->
      <div v-if="showArticleModal" class="modal-backdrop" @click.self="showArticleModal = false">
        <div class="modal-drawer article-editor-modal">
          <div class="modal-drawer-header">
            <div class="header-title-wrap">
              <span class="header-badge">{{ articleForm.id ? '编辑模式' : '新建草稿' }}</span>
              <h3>{{ articleForm.id ? ("编辑文章 #" + articleForm.id) : "撰写新文章" }}</h3>
            </div>
            <button type="button" class="close-btn" title="关闭" @click="showArticleModal = false">✕</button>
          </div>
          <div class="modal-drawer-body">
            <div class="article-editor-columns">
              <!-- 左列：标题 / 摘要 / 正文（源码与渲染预览切换） -->
              <div class="editor-col-left">
                <div class="field-group">
                  <label class="field-label-prominent">文章标题 <span class="required-star">*</span></label>
                  <input
                    v-model="articleForm.title"
                    class="editor-title-input"
                    maxlength="160"
                    placeholder="输入引人入胜的文章标题..."
                  />
                </div>

                <div class="field-group mt-3">
                  <label class="field-label-prominent">文章摘要 (Excerpt)</label>
                  <textarea
                    v-model="articleForm.excerpt"
                    class="short-text-area"
                    rows="3"
                    placeholder="简明扼要地概括文章核心亮点，用于列表展示与社交分享..."
                  ></textarea>
                </div>

                <div class="field-group mt-3">
                  <div class="body-editor-head">
                    <label class="field-label-prominent">正文内容 <span class="required-star">*</span></label>
                    <div class="body-mode-switch">
                      <button
                        type="button"
                        :class="{ active: bodyEditMode === 'editor' }"
                        @click="switchBodyMode('editor')"
                      >富文本编辑</button>
                      <button
                        type="button"
                        :class="{ active: bodyEditMode === 'source' }"
                        @click="switchBodyMode('source')"
                      >HTML 源码</button>
                      <button
                        type="button"
                        :class="{ active: bodyEditMode === 'preview' }"
                        @click="switchBodyMode('preview')"
                      >渲染预览</button>
                    </div>
                  </div>
                  <AnnouncementRichTextEditor
                    v-if="bodyEditMode === 'editor'"
                    v-model="articleForm.bodyMarkdown"
                    placeholder="开始撰写文章内容，支持粘贴富文本/源码片段..."
                  />
                  <textarea
                    v-else-if="bodyEditMode === 'source'"
                    v-model="articleForm.bodyMarkdown"
                    class="html-source-area"
                    rows="14"
                    placeholder="粘贴或编辑 HTML 源码，例如 <h2>章节标题</h2><p>正文...</p>"
                  ></textarea>
                  <div
                    v-else
                    class="body-render-preview"
                    v-html="renderedBodyPreview"
                  ></div>
                </div>
              </div>

              <!-- 右列：分类/标签/SEO/精选/封面 -->
              <div class="editor-col-right">
                <div class="meta-card">
                  <div class="meta-card-header">
                    <span class="meta-card-icon">⚙️</span>
                    <h4>分类与标签</h4>
                  </div>
                  <div class="meta-card-body">
                    <div class="form-row">
                      <label>分类
                        <div class="select-with-create">
                          <select v-model="articleForm.category">
                            <option value="">未分类</option>
                            <option v-for="c in taxonomy.articleCategories" :key="c" :value="c">{{ c }}</option>
                          </select>
                          <button
                            type="button"
                            class="mini-create-btn"
                            title="新建分类"
                            @click="openInlineTaxonomyDialog('article_category')"
                          >＋ 新建</button>
                        </div>
                      </label>
                    </div>
                    <div class="form-row">
                      <label>标签 (逗号分隔)
                        <input v-model="articleForm.tags" placeholder="ai, llm, tutorial" />
                      </label>
                      <button
                        type="button"
                        class="mini-create-btn mt-1"
                        title="新建标签"
                        @click="openInlineTaxonomyDialog('tag')"
                      >＋ 新建标签</button>
                    </div>
                  </div>
                </div>

                <div class="meta-card mt-3">
                  <div class="meta-card-header">
                    <span class="meta-card-icon">🔍</span>
                    <h4>SEO 搜索引擎优化</h4>
                  </div>
                  <div class="meta-card-body">
                    <div class="seo-score-row">
                      <div class="seo-score-badge" :class="seoScoreLevel">
                        <span class="seo-score-num">{{ seoScore }}</span>
                        <span class="seo-score-label">SEO 评分</span>
                      </div>
                      <ul class="seo-score-tips">
                        <li v-for="tip in seoScoreTips" :key="tip">{{ tip }}</li>
                      </ul>
                    </div>
                    <button
                      type="button"
                      class="button primary seo-auto-btn"
                      @click="generateSeoMeta"
                    >✨ SEO 自动化：生成标题 / 描述 / 关键词</button>
                    <div class="form-row">
                      <label>SEO 标题
                        <input v-model="articleForm.seoTitle" maxlength="160" placeholder="默认继承文章标题" />
                      </label>
                    </div>
                    <div class="form-row">
                      <label>SEO 描述 (Meta Description)
                        <textarea v-model="articleForm.seoDescription" rows="2" maxlength="300" placeholder="留空默认使用文章摘要..."></textarea>
                      </label>
                    </div>
                    <div class="form-row">
                      <label>SEO 关键词 (Meta Keywords，逗号分隔)
                        <input v-model="articleForm.seoKeywords" placeholder="由 SEO 自动化生成或手动填写" />
                      </label>
                    </div>
                    <!-- SERP 实时预览 -->
                    <div class="serp-preview">
                      <div class="serp-title">{{ articleForm.seoTitle || articleForm.title || '文章标题预览' }}</div>
                      <div class="serp-url">cc-ai.xyz › articles › {{ articleForm.slug || 'slug' }}</div>
                      <div class="serp-desc">{{ articleForm.seoDescription || articleForm.excerpt || 'SEO 描述预览将显示在这里...' }}</div>
                    </div>
                    <details class="advanced-seo">
                      <summary>高级选项（Slug / 语言 / Canonical / 作者）</summary>
                      <div class="form-row">
                        <label>Slug (URL 路径)
                          <input v-model="articleForm.slug" maxlength="180" placeholder="如 my-first-post (留空自动生成)" />
                        </label>
                      </div>
                      <div class="form-row">
                        <label>语言
                          <input v-model="articleForm.locale" placeholder="zh-CN" />
                        </label>
                      </div>
                      <div class="form-row">
                        <label>Canonical 标准链接
                          <input v-model="articleForm.canonicalUrl" placeholder="留空自动使用本站原始 URL" />
                        </label>
                      </div>
                      <div class="form-row">
                        <label>作者 ID
                          <input v-model.number="articleForm.authorId" type="number" min="1" placeholder="默认当前用户" />
                        </label>
                      </div>
                    </details>
                  </div>
                </div>

                <div class="meta-card mt-3">
                  <div class="meta-card-header">
                    <span class="meta-card-icon">⭐</span>
                    <h4>推荐设置</h4>
                  </div>
                  <div class="meta-card-body">
                    <label class="featured-switch-row">
                      <span>首页精选展示</span>
                      <button
                        type="button"
                        class="switch-btn"
                        :class="{ on: articleForm.featured }"
                        role="switch"
                        :aria-checked="articleForm.featured"
                        @click="articleForm.featured = !articleForm.featured"
                      >
                        <span class="switch-knob"></span>
                      </button>
                    </label>
                    <small class="muted">开启后文章将优先展示在首页精选位。</small>
                  </div>
                </div>

                <div class="meta-card mt-3">
                  <div class="meta-card-header">
                    <span class="meta-card-icon">🖼️</span>
                    <h4>封面图 (Cover)</h4>
                  </div>
                  <div class="meta-card-body">
                    <small class="muted">建议比例 16:9 或 2:1，不超过 5MB</small>
                    <div class="upload-control-row mt-2">
                      <label class="upload-file-btn">
                        <input type="file" accept="image/*" class="sr-only" @change="handleImageUpload($event, 'coverImage')" />
                        <span>本地上传</span>
                      </label>
                      <button
                        v-if="articleForm.coverImage"
                        type="button"
                        class="btn-clear"
                        @click="articleForm.coverImage = ''"
                      >清除</button>
                    </div>
                    <div v-if="articleForm.coverImage" class="image-preview-box mt-2">
                      <img :src="articleForm.coverImage" alt="封面预览" />
                    </div>
                    <div class="mt-2">
                      <label class="alt-label">Alt 替代文本：
                        <input v-model="articleForm.coverAlt" placeholder="描述图片供无障碍与 SEO" />
                      </label>
                    </div>
                    <details class="advanced-seo mt-2">
                      <summary>社交分享图 (OG Image)</summary>
                      <div class="upload-control-row mt-2">
                        <label class="upload-file-btn">
                          <input type="file" accept="image/*" class="sr-only" @change="handleImageUpload($event, 'ogImage')" />
                          <span>本地上传</span>
                        </label>
                        <button
                          v-if="articleForm.ogImage"
                          type="button"
                          class="btn-clear"
                          @click="articleForm.ogImage = ''"
                        >清除</button>
                      </div>
                      <div v-if="articleForm.ogImage" class="image-preview-box mt-2">
                        <img :src="articleForm.ogImage" alt="OG 图片预览" />
                      </div>
                    </details>
                  </div>
                </div>
              </div>
            </div>

            <!-- 翻译管理子卡片 -->
            <div v-if="articleForm.id" class="translations-card mt-4">
              <div class="trans-header">
                <div>
                  <h4>多语言译文管理</h4>
                  <small>为海外用户构建多语言版本，支持一键 AI 全自动互译</small>
                </div>
                <div class="translation-actions">
                  <select v-model="translationLocale" class="locale-select">
                    <option value="en-US">English (en-US)</option>
                    <option value="zh-CN">简体中文 (zh-CN)</option>
                    <option value="ja-JP">日本語 (ja-JP)</option>
                    <option value="ko-KR">한국어 (ko-KR)</option>
                  </select>
                  <button class="button" @click="saveTranslation">保存译文</button>
                  <button class="secondary-button sparkle-btn" @click="generateTranslation">✨ AI 智能翻译</button>
                </div>
              </div>
              <div v-if="translations.length" class="translations-chips">
                <div
                  v-for="t in translations"
                  :key="t.id"
                  class="trans-chip"
                >
                  <span class="trans-chip-title" @click="editTranslation(t)">🌐 {{ t.locale }} - {{ t.title || '无标题' }}</span>
                  <span class="status-badge" :class="'badge-' + t.status">{{ t.status }}</span>
                  <button v-if="t.status !== 'published'" type="button" class="link-action text-blue-600" @click="publishTranslation(t)">发布</button>
                  <button v-else type="button" class="link-action text-amber-600" @click="unpublishTranslation(t)">下架</button>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-drawer-footer">
            <button class="secondary-button" @click="showArticleModal = false">取消</button>
            <button v-if="articleForm.id" class="secondary-button" type="button" @click="previewArticle">新窗口预览</button>
            <button class="button primary" :disabled="articleSaving" @click="saveArticleAndClose">
              {{ articleSaving ? "保存中..." : "保存" }}
            </button>
          </div>
        </div>
      </div>

      <!-- 新建分类/标签迷你弹窗（禁止 window.prompt） -->
      <div v-if="inlineTaxDialog.show" class="modal-backdrop z-overlay" @click.self="inlineTaxDialog.show = false">
        <div class="inline-tax-dialog">
          <h3>{{ inlineTaxDialog.scope === 'tag' ? '新建标签' : (inlineTaxDialog.scope === 'news_category' ? '新建新闻分类' : '新建文章分类') }}</h3>
          <input
            v-model="inlineTaxDialog.name"
            :placeholder="inlineTaxDialog.scope === 'tag' ? '输入标签名称' : (inlineTaxDialog.scope === 'news_category' ? '输入新闻分类名称' : '输入文章分类名称')"
            maxlength="60"
            @keyup.enter="submitInlineTaxDialog"
          />
          <div class="inline-tax-actions">
            <button class="secondary-button" @click="inlineTaxDialog.show = false">取消</button>
            <button
              class="button primary"
              :disabled="inlineTaxDialog.saving || !inlineTaxDialog.name.trim()"
              @click="submitInlineTaxDialog"
            >{{ inlineTaxDialog.saving ? "创建中..." : "创建" }}</button>
          </div>
        </div>
      </div>

      <!-- 删除文章确认弹窗（替代 window.confirm） -->
      <div v-if="articleDeleteConfirm.show" class="modal-backdrop z-overlay" @click.self="articleDeleteConfirm.show = false">
        <div class="inline-tax-dialog">
          <h3>确认删除这篇文章？</h3>
          <p class="muted" style="margin:0">删除后不可恢复（可在回收站还原）。</p>
          <div class="inline-tax-actions">
            <button class="secondary-button" @click="articleDeleteConfirm.show = false">取消</button>
            <button
              class="button primary"
              :disabled="articleDeleteConfirm.deleting"
              @click="confirmDeleteArticle"
            >{{ articleDeleteConfirm.deleting ? "删除中..." : "确认删除" }}</button>
          </div>
        </div>
      </div>

    </section>

    <!-- ==================== 分类与标签 ==================== -->
    <section v-else-if="current === '分类与标签'" class="workspace taxonomy-pane">
      <div class="subnav">
        <button
          v-for="v in taxonomyViews"
          :key="v.key"
          :class="{ selected: taxonomyView === v.key }"
          @click="switchTaxonomyView(v.key)"
        >
          {{ v.label }}
        </button>
      </div>
      <div class="toolbar">
        <input
          v-model="taxonomySearch"
          :placeholder="'搜索' + currentTaxonomyEntity + '...'"
          class="search-input"
          @input="taxonomyPage = 1"
        />
        <button class="button" @click="openTaxonomyDialog('create')">
          + 新增{{ currentTaxonomyEntity }}
        </button>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>所属范围</th>
              <th class="col-actions">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="name in pagedTaxonomyItems" :key="name">
              <td>{{ name }}</td>
              <td>{{ currentTaxonomyScopeLabel }}</td>
              <td>
                <button class="link" type="button" @click="openTaxonomyDialog('edit', name)">编辑</button>
                <button class="link danger" type="button" @click="openTaxonomyDialog('delete', name)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="!filteredTaxonomyItems.length" class="empty">
          {{ taxonomySearch ? "没有匹配的结果" : "暂无" + currentTaxonomyEntity }}
        </p>
        <div class="table-footer">
          <div class="pagination">
            <button class="link" :disabled="taxonomyPage <= 1" @click="taxonomyPage--">上一页</button>
            <span class="page-info">第 {{ taxonomyPage }} / {{ taxonomyPageCount }} 页 · 共 {{ filteredTaxonomyItems.length }} 条</span>
            <button class="link" :disabled="taxonomyPage >= taxonomyPageCount" @click="taxonomyPage++">下一页</button>
          </div>
        </div>
      </div>

      <!-- 分类/标签 新增·编辑·删除 居中弹窗（禁止内联展开） -->
      <div v-if="taxonomyDialog.show" class="taxonomy-backdrop" @click.self="closeTaxonomyDialog">
        <div class="taxonomy-dialog">
          <div class="taxonomy-dialog-header">
            <h3>{{ taxonomyDialogTitle }}</h3>
            <button class="link" type="button" @click="closeTaxonomyDialog">✕</button>
          </div>
          <div class="taxonomy-dialog-body">
            <template v-if="taxonomyDialog.mode === 'delete'">
              <p>
                确认删除{{ currentTaxonomyEntity }}
                <strong>「{{ taxonomyDialog.originalName }}」</strong> 吗？
              </p>
              <p class="muted">删除后，引用该名称的文章/新闻将自动回退到默认分类或移除该标签。</p>
            </template>
            <template v-else>
              <label class="taxonomy-field">
                {{ currentTaxonomyEntity }}名称 <span class="required-star">*</span>
                <input
                  v-model="taxonomyDialog.name"
                  :placeholder="'请输入' + currentTaxonomyEntity + '名称'"
                  @keyup.enter="submitTaxonomyDialog"
                />
              </label>
            </template>
          </div>
          <div class="taxonomy-dialog-footer">
            <button class="secondary-button" type="button" @click="closeTaxonomyDialog">取消</button>
            <button
              class="button primary"
              :class="{ danger: taxonomyDialog.mode === 'delete' }"
              :disabled="taxonomyDialog.saving"
              type="button"
              @click="submitTaxonomyDialog"
            >
              {{ taxonomyDialog.saving ? "提交中..." : taxonomyDialog.mode === "delete" ? "确认删除" : "保存" }}
            </button>
          </div>
        </div>
      </div>
    </section>

    <section v-else-if="current === 'AI 创作'" class="workspace ai-studio-pane">
      <div class="panel-header-desc">
        <div class="ai-banner-content">
          <h2>🤖 AI 智能创作工作台</h2>
          <p class="muted">利用多模型协同全自动生成长篇深度干货与行业洞察，生成的草稿可一键无缝载入至文章库进行人工精修与多渠道发布。</p>
        </div>
      </div>

      

      <div class="ai-studio-grid">
        <!-- 左侧：任务列表（纯列表+操作） -->
        <div class="ai-col ai-col-left">
          <div class="ai-col-header">
            <h3>生成任务</h3>
            <button class="button primary new-btn-mini" @click="resetAiStudio">+ 新增</button>
          </div>
          <div class="task-list">
            <div
              v-for="t in writingTaskList"
              :key="t.taskId || t.task_id || t.id"
              class="task-item"
              :class="{ active: (writingTask.taskId || writingTask.task_id || writingTask.id) === (t.taskId || t.task_id || t.id) }"
              @click="selectWritingTask(t)"
            >
              <div class="task-item-topic">{{ t.topic || "(无主题)" }}</div>
              <div class="task-item-meta">
                <span class="status-pill" :class="'status-' + t.status">{{ taskStatusText(t.status) }}</span>
                <span class="task-date">{{ formatDate(t.createdAt || t.created_at) }}</span>
                <button
                  class="task-delete-btn"
                  title="删除任务记录"
                  @click.stop="deleteTaskRecord(t)"
                >✕</button>
              </div>
            </div>
            <div v-if="!writingTaskList.length" class="empty-task-placeholder">
              <span>📭 暂无生成任务</span>
            </div>
          </div>
          <div v-if="writingTask.id" class="current-task-card">
            <div class="current-task-header">
              <span class="task-label">选中任务</span>
              <span class="status-pill" :class="'status-' + writingTask.status">{{ taskStatusText(writingTask.status) }}</span>
            </div>
            <div class="task-detail-topic">{{ writingTask.topic || "-" }}</div>
            <div v-if="writingTask.errorCode" class="error-banner">{{ writingTask.errorCode }}</div>
            <div class="task-btns">
              <button v-if="['queued','running'].includes(writingTask.status)" class="danger-btn-sm" @click="cancelTask">停止生成</button>
              <button v-if="writingTask.status === 'failed'" class="retry-btn-sm" @click="createTask">重新尝试</button>
            </div>
          </div>
        </div>

        <!-- 中间：所有配置参数 -->
        <div class="ai-col ai-col-mid">
          <div class="ai-col-header">
            <h3>生成配置</h3>
          </div>
          <div class="ai-form-card">
            <div class="field-group">
              <label class="block-label">选题背景与核心大纲 (Brief) <span class="required-star">*</span></label>
              <textarea
                v-model="brief.topic"
                placeholder="输入希望探讨的主题、受众定位、重点论据或结构要求..."
                class="ai-topic-input"
              ></textarea>
            </div>
            <div class="field-group">
              <label class="block-label">生成模型 <span class="required-star">*</span></label>
              <select v-model="brief.model" class="ai-model-select">
                <option value="" disabled>请选择模型</option>
                <option v-for="m in aiModelOptions" :key="m" :value="m">{{ m }}</option>
              </select>
              <small v-if="modelsLoadError" class="error-text">{{ modelsLoadError }}</small>
              <small v-else-if="!aiModelOptions.length" class="muted">模型列表为空，请检查 /admin/content/articles/models 接口</small>
            </div>
            <div class="form-row-grid">
              <div class="field-group">
                <label class="block-label">目标语言</label>
                <select v-model="brief.language" class="ai-select">
                  <option value="zh-CN">中文</option>
                  <option value="en-US">English</option>
                </select>
              </div>
              <div class="field-group">
                <label class="block-label">期望生成字数</label>
                <input v-model.number="brief.wordCount" type="number" placeholder="2000" />
              </div>
            </div>
            <div class="form-row-grid">
              <div class="field-group">
                <label class="block-label">核心关键词</label>
                <input v-model="brief.keywords" placeholder="以英文逗号分隔" />
              </div>
              <div class="field-group">
                <label class="block-label">指定渠道/账户（可选）</label>
                <input v-model="brief.providerKeyRef" placeholder="留空自动智能负载均衡" />
              </div>
            </div>
            <div class="field-group">
              <label class="block-label">勾选参考新闻（可选）</label>
              <div class="ref-news-list">
                <label
                  v-for="item in news.slice(0, 10)"
                  :key="'ref-' + item.id"
                  class="ref-news-item"
                >
                  <input v-model="selectedRefNewsIds" :value="item.id" type="checkbox" />
                  <span class="ref-news-title">{{ item.title }}</span>
                </label>
                <p v-if="!news.length" class="muted ref-news-empty">暂无新闻可引用</p>
              </div>
            </div>
            <div class="field-group">
              <label class="block-label">手动粘贴参考文本（可选）</label>
              <textarea
                v-model="referenceContent"
                placeholder="粘贴参考资料、背景资料或竞品原文，AI 生成时会作为参考上下文..."
                class="ai-ref-textarea"
              ></textarea>
            </div>
            <div class="ai-actions">
              <button
                class="button primary generate-btn"
                :disabled="!brief.topic || !brief.model || ['queued','running'].includes(writingTask.status)"
                @click="createTask"
              >
                🚀 开始生成
              </button>
            </div>
          </div>
        </div>

        <!-- 右侧：生成结果与流式预览 -->
        <div class="ai-col ai-col-right">
          <div class="ai-col-header">
            <h3>生成结果</h3>
            <button
              v-if="writingTaskResult?.title || writingTaskArticleId"
              class="button primary load-article-btn"
              @click="openGeneratedArticle"
            >
              📝 载入文章库
            </button>
          </div>
          <div class="ai-preview-card">
            <div v-if="writingTask.status === 'queued' || writingTask.status === 'running'" class="ai-generating">
              <div class="spinner"></div>
              <p class="generating-title">AI 正在深度思考并撰写正文...</p>
              <small class="muted">模型正组织逻辑大纲与论据，预计需要 20~40 秒</small>
            </div>
            <div v-else-if="writingTask.status === 'failed'" class="ai-failed">
              <span class="failed-icon">⚠️</span>
              <h4>生成未成功</h4>
              <p class="error-text">{{ writingTask.errorCode || "模型调用发生异常，请检查配置或重试" }}</p>
              <button class="button secondary mt-3" @click="createTask">重新生成</button>
            </div>
            <div v-else-if="writingTask.status === 'succeeded' && (writingTaskResult || taskArticleError)" class="ai-result">
              <div class="result-header">
                <h4>{{ writingTaskResult?.title || '已生成文章' }}</h4>
                <small v-if="writingTaskArticleId" class="muted">文章 #{{ writingTaskArticleId }}</small>
              </div>
              <p v-if="taskArticleError" class="error-text">{{ taskArticleError }}</p>
              <div
                v-if="writingTaskBodyHtml"
                class="ai-result-body"
                v-html="writingTaskBodyHtml"
              ></div>
              <p v-else-if="!taskArticleError" class="muted">任务已完成，但返回内容为空，请到「深度解析」查看该文章。</p>
              <div class="result-actions">
                <button class="button primary" @click="openGeneratedArticle">✅ 采纳（载入文章库）</button>
                <button class="button secondary" @click="createTask">🔄 重新生成</button>
              </div>
            </div>
            <div v-else class="ai-empty">
              <div class="empty-icon">💡</div>
              <h4>{{ writingTask.status === 'succeeded' ? '任务已完成（无正文摘要）' : '等待指令输入' }}</h4>
              <p class="muted">
                {{ writingTask.status === 'succeeded'
                  ? '该任务已完成，点击左侧任务可查看关联文章；也可直接在「深度解析」中查找。'
                  : '在左侧填写创作需求并点击中栏「开始生成」，AI 生成的专业长文草稿将实时展示于此' }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>

<section v-else class="workspace">
      <div class="bi-toolbar">
        <label
          >开始日期<input v-model="biFilters.from" type="date" @change="loadBI"
        /></label>
        <label
          >结束日期<input v-model="biFilters.to" type="date" @change="loadBI"
        /></label>
        <label
          >语言<input
            v-model="biFilters.locale"
            placeholder="全部语言"
            @change="loadBI"
        /></label>
        <label
          >来源<select v-model="biFilters.source_id" @change="loadBI">
            <option value="">全部来源</option>
            <option
              v-for="source in sources"
              :key="source.id"
              :value="String(source.id)"
            >
              {{ source.name }}
            </option>
          </select></label
        >
        <label
          >内容类型<select v-model="biFilters.content_type" @change="loadBI">
            <option value="">全部</option>
            <option value="article">文章</option>
            <option value="news">新闻</option>
          </select></label
        >
        <label
          >位置<input
            v-model="biFilters.placement"
            placeholder="全部位置"
            @keyup.enter="loadBI"
        /></label>
        <button class="secondary-button" @click="exportBI">导出 CSV</button>
      </div>
      <div class="metrics">
        <div class="metric-card m-blue">
          <span class="metric-icon">👥</span>
          <div class="metric-info"><small>访客</small><strong>{{ overview.uniqueVisitors || 0 }}</strong></div>
        </div>
        <div class="metric-card m-green">
          <span class="metric-icon">📄</span>
          <div class="metric-info"><small>页面浏览</small><strong>{{ overview.pageViews || 0 }}</strong></div>
        </div>
        <div class="metric-card m-purple">
          <span class="metric-icon">📖</span>
          <div class="metric-info"><small>内容阅读</small><strong>{{ overview.reads || 0 }}</strong></div>
        </div>
        <div class="metric-card m-amber">
          <span class="metric-icon">🎯</span>
          <div class="metric-info"><small>点击率</small><strong>{{ ((overview.ctr || 0) * 100).toFixed(2) }}%</strong></div>
        </div>
      </div>
      <div class="chart-grid">
        <div class="chart-card">
          <h3>访问与阅读趋势</h3>
          <div class="chart-box">
            <LineChart :data="trendChartData" :options="trendChartOptions" />
          </div>
        </div>
        <div class="chart-card">
          <h3>文章阅读排行 TOP5</h3>
          <div class="chart-box">
            <BarChart :data="articleRankingChartData" :options="horizontalBarOptions" />
          </div>
        </div>
        <div class="chart-card">
          <h3>新闻阅读占比</h3>
          <div class="chart-box">
            <DoughnutChart :data="newsShareChartData" :options="doughnutOptions" />
          </div>
        </div>
        <div class="chart-card">
          <h3>点击率（按展示位置）</h3>
          <div class="chart-box">
            <BarChart :data="ctrChartData" :options="verticalBarOptions" />
          </div>
        </div>
      </div>
    </section>
  </div>
  </AppLayout>

  <!-- 通用自绘确认框（替代 window.confirm） -->
  <div v-if="uiConfirm.show" class="modal-backdrop z-overlay" @click.self="uiConfirm.show = false">
    <div class="ui-confirm-dialog" role="dialog" aria-modal="true">
      <h3>{{ uiConfirm.title }}</h3>
      <p v-if="uiConfirm.message" class="muted ui-confirm-message">{{ uiConfirm.message }}</p>
      <div class="ui-confirm-actions">
        <button class="secondary-button" :disabled="uiConfirm.busy" @click="uiConfirm.show = false">取消</button>
        <button
          class="button"
          :class="uiConfirm.danger ? 'danger-button' : 'primary'"
          :disabled="uiConfirm.busy"
          @click="runUiConfirm"
        >{{ uiConfirm.busy ? "处理中..." : uiConfirm.okText }}</button>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import AnnouncementRichTextEditor from '@/components/admin/announcements/AnnouncementRichTextEditor.vue'
import { announcementEditorInitialHtml, normalizeAnnouncementEditorHtml } from '@/utils/announcementContent'
import { computed, nextTick, onMounted, ref, watch } from "vue";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Tooltip,
  Legend,
  Filler,
} from "chart.js";
import { Line as LineChart, Bar as BarChart, Doughnut as DoughnutChart } from "vue-chartjs";
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, ArcElement, Tooltip, Legend, Filler);
import contentAPI from "@/api/admin/content";
import { useAppStore } from "@/stores";
import { marked } from "marked";
import DOMPurify from "dompurify";

const appStore = useAppStore();

/* ==================== 通用交互件：toast + 自绘确认框 ==================== */
function toast(type: "success" | "error" | "info" | "warning", message: string) {
  try {
    appStore.showToast(type, message, 3200);
  } catch {
    /* store 异常时退化为控制台，避免阻断业务流程 */
    // eslint-disable-next-line no-console
    console.warn(`[${type}] ${message}`);
  }
}

type UiConfirmState = {
  show: boolean;
  title: string;
  message: string;
  okText: string;
  danger: boolean;
  busy: boolean;
  onOk: null | (() => Promise<void> | void);
};
const uiConfirm = ref<UiConfirmState>({
  show: false,
  title: "",
  message: "",
  okText: "确认",
  danger: false,
  busy: false,
  onOk: null,
});
function askConfirm(opts: {
  title: string;
  message?: string;
  okText?: string;
  danger?: boolean;
  onOk: () => Promise<void> | void;
}) {
  uiConfirm.value = {
    show: true,
    title: opts.title,
    message: opts.message || "",
    okText: opts.okText || "确认",
    danger: Boolean(opts.danger),
    busy: false,
    onOk: opts.onOk,
  };
}
async function runUiConfirm() {
  const c = uiConfirm.value;
  if (!c.onOk || c.busy) return;
  c.busy = true;
  try {
    await c.onOk();
  } finally {
    c.busy = false;
    uiConfirm.value = { ...uiConfirm.value, show: false, onOk: null };
  }
}

const tabs = ["新闻动态", "深度解析", "AI 创作", "分类与标签", "BI 分析"];
const current = ref("新闻动态");
const newsView = ref("items");
const news = ref<any[]>([]);
const newsPage = ref(1);
const newsTotal = ref(0);
const newsHasNext = ref(false);
const selectedNews = ref<any>({});
const newsEditorRef = ref<HTMLElement | null>(null);
const selectedNewsIds = ref<number[]>([]);
const allNewsSelected = ref(false);
const sources = ref<any[]>([]);
const sourcePage = ref(1);
const sourceTotal = ref(0);
const sourceHasNext = ref(false);
const selectedSource = ref<any>({});
const runs = ref<any[]>([]);
const runPage = ref(1);
const runTotal = ref(0);
const runHasNext = ref(false);
const articles = ref<any[]>([]);
const articlePage = ref(1);
const articleTotal = ref(0);
const articleHasNext = ref(false);
const selectedArticleIds = ref<number[]>([]);
const allArticlesSelected = ref(false);
const trends = ref<any[]>([]);
const rankings = ref<any[]>([]);
const ctrRows = ref<any[]>([]);
const overview = ref<any>({});
const biFilters = ref({
  from: new Date(Date.now() - 30 * 864e5).toISOString().slice(0, 10),
  to: new Date().toISOString().slice(0, 10),
  locale: "",
  content_type: "",
  placement: "",
  source_id: "",
});
const writingTask = ref<any>({});
const writingTaskList = ref<any[]>([]);
const imageUploading = ref<"" | "coverImage" | "ogImage">("");
let writingPollTimer: ReturnType<typeof setTimeout> | undefined;
const models = ref<any[]>([]);

// Chart.js data computed
const trendChartData = computed(() => ({
  labels: trends.value.map((r: any) => formatDate(r.date)),
  datasets: [
    { label: "页面浏览", data: trends.value.map((r: any) => r.pageViews || 0), borderColor: "#6366F1", backgroundColor: "rgba(99,102,241,0.1)", fill: true, tension: 0.4 },
    { label: "内容阅读", data: trends.value.map((r: any) => r.reads || 0), borderColor: "#10B981", backgroundColor: "rgba(16,185,129,0.1)", fill: true, tension: 0.4 },
  ],
}));
const articleRankingChartData = computed(() => ({
  labels: rankings.value.filter((r: any) => r.contentType === "article").slice(0, 5).map((r: any) => (r.title || r.pagePath || "").slice(0, 20)),
  datasets: [{ label: "阅读数", data: rankings.value.filter((r: any) => r.contentType === "article").slice(0, 5).map((r: any) => r.reads || 0), backgroundColor: ["#6366F1", "#8B5CF6", "#EC4899", "#F59E0B", "#10B981"], borderRadius: 8 }],
}));
const newsShareChartData = computed(() => {
  const newsReads = rankings.value.filter((r: any) => r.contentType === "news").reduce((s: number, r: any) => s + (r.reads || 0), 0);
  const articleReads = rankings.value.filter((r: any) => r.contentType === "article").reduce((s: number, r: any) => s + (r.reads || 0), 0);
  return {
    labels: ["新闻", "文章"],
    datasets: [{ data: [newsReads, articleReads], backgroundColor: ["#6366F1", "#10B981"], borderWidth: 0 }],
  };
});
const ctrChartData = computed(() => ({
  labels: ctrRows.value.map((r: any) => r.placement || "未标注"),
  datasets: [{ label: "CTR %", data: ctrRows.value.map((r: any) => parseFloat(((r.ctr || 0) * 100).toFixed(2))), backgroundColor: "#F59E0B", borderRadius: 8 }],
}));
const trendChartOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: "top" as const } }, scales: { y: { beginAtZero: true } } };
const horizontalBarOptions = { indexAxis: "y" as const, responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } };
const doughnutOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: "bottom" as const } } };
const verticalBarOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true } } };
// C3/D2：模型下拉不再回落硬编码假名；接口失败时为空列表 + 提示
const modelsLoadError = ref("");
const fallbackModels: Array<{ id: string; name: string }> = [];

const allAvailableModels = computed(() => {
  const loaded = models.value
    .map((m: any) => {
      if (typeof m === "string") return { id: m, name: m };
      return { id: m?.id || m?.name || "", name: m?.name || m?.id || "" };
    })
    .filter((m: any) => Boolean(m.id));
  if (loaded.length) return loaded; // 接口有真实模型时只展示真实模型，避免脏数据
  return fallbackModels;
});

// AI 创作模型下拉改为跟随接口（历史硬编码 11 个名字多数在后端不存在）
const aiModelOptions = computed(() => allAvailableModels.value.map((m: any) => m.id));


const translations = ref<any[]>([]);
const translationLocale = ref("en-US");
const translationForm = ref<any>({
  id: 0,
  title: "",
  slug: "",
  excerpt: "",
  bodyMarkdown: "",
  seoTitle: "",
  seoDescription: "",
});
const errorMessage = ref("");
const newsSearch = ref("");
const newsStatus = ref("");
const newsLocale = ref("");
const newsSourceId = ref("");
const newsFrom = ref("");
const newsTo = ref("");
const articleSearch = ref("");
const articleStatus = ref("");
const articleLocale = ref("");
const articleCategory = ref("");
const articleTags = ref("");
const articleModel = ref("");
// D2：存量文章的 model 名（如 grok-4.7）可能不在上游模型目录里，合并后筛选才可用
const articleModelNames = ref<string[]>([]);
const articleModelFilterOptions = computed(() => {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const m of allAvailableModels.value) {
    if (!m.id || seen.has(m.id)) continue;
    seen.add(m.id);
    out.push(m.id);
  }
  for (const m of articleModelNames.value) {
    if (!m || seen.has(m)) continue;
    seen.add(m);
    out.push(m);
  }
  return out;
});
const articleAuthorId = ref("");
const articleFrom = ref("");
const articleTo = ref("");
const articleMinReads = ref("");
const articleMaxReads = ref("");
const sourceForm = ref<any>({ name: "", url: "", sourceType: "rss", mappingJsonText: "" });
const sourceFormErrors = ref<{ name: string; url: string }>({ name: "", url: "" });
// S5：字段级校验（红字提示 + 提交按钮禁用）
function validateSourceField(field: "name" | "url") {
  const name = String(sourceForm.value.name || "").trim();
  const url = String(sourceForm.value.url || "").trim();
  if (field === "name") {
    sourceFormErrors.value.name = name ? "" : "必填";
    return;
  }
  if (!url) sourceFormErrors.value.url = "URL 必填";
  else if (!/^https?:\/\//i.test(url)) sourceFormErrors.value.url = "URL 需以 http:// 或 https:// 开头";
  else sourceFormErrors.value.url = "";
}
const sourceCreateBlockedReason = computed(() => {
  const name = String(sourceForm.value.name || "").trim();
  const url = String(sourceForm.value.url || "").trim();
  if (!name && !url) return "请填写来源名称与 URL";
  if (!name) return "请填写来源名称";
  if (!url) return "请填写公开 RSS/Atom/JSON URL";
  return "";
});
const sourceCreateDisabled = computed(() => !!sourceCreateBlockedReason.value);
// 调度文案：源自己的 intervalMinutes 才是真实调度，之前误把全局 fetchCron 混进同一行（S6 互相矛盾）
// S6：调度文案统一以 Cron 为准（不再写死 60 分钟/只读 intervalMinutes）
function cronToScheduleText(cron: string): string {
  const parts = String(cron || "").trim().split(/\s+/);
  if (parts.length < 5) return "";
  // 兼容 6 段（含秒，如 `0 */15 * * * *`）与 5 段标准 Cron
  const hasSeconds = parts.length >= 6;
  const [p0, p1, p2] = parts;
  const min = hasSeconds ? p1 : p0;
  const hour = hasSeconds ? p2 : p1;
  // 0 */15 * * * *  → 每 15 分钟
  const stepMin = min.match(/^\*\/(\d+)$/);
  if (stepMin && (hour === "*" || hour === undefined)) return `每 ${stepMin[1]} 分钟抓取一次`;
  const stepHour = hour && hour.match(/^\*\/(\d+)$/);
  if (stepHour) return `每 ${stepHour[1]} 小时抓取一次`;
  if (min === "*" && hour === "*") return "每分钟抓取一次";
  if (hour === "*") return `每小时第 ${min} 分钟抓取一次`;
  return `每天 ${String(hour).padStart(2, "0")}:${String(min).padStart(2, "0")} 抓取一次`;
}
function sourceScheduleText(source: any) {
  const fromCron = cronToScheduleText(source?.fetchCron || source?.cronExpression || "");
  if (fromCron) return fromCron;
  const minutes = Number(source?.intervalMinutes || source?.scheduleMinutes || 0);
  if (minutes > 0) return `每 ${minutes} 分钟抓取一次`;
  const label = String(source?.scheduleLabel || "").trim();
  if (label) return label;
  return "未设置抓取间隔";
}
// R3：抓取任务列表按 sourceId 查源名，避免只显示数字 ID
function sourceName(id: any) {
  const hit = sources.value.find((s: any) => String(s.id) === String(id));
  return hit?.name || (id ? `来源 #${id}` : "-");
}
const automation = ref<any>({
  enabled: false,
  timezone: "Asia/Shanghai",
  fetchCron: "0 */15 * * * *",
  publishMode: "manual_review",
  dailyFetchLimit: 0,
  dailyPublishLimit: 0,
  publishWindowStart: "09:00",
  publishWindowEnd: "22:00",
  dedupeWindowHours: 24,
  targetLocalesText: "zh-CN",
});
const automationErrors = ref<{ dedupeWindowHours: string }>({ dedupeWindowHours: "" });
const brief = ref<any>({
  topic: "",
  model: "",
  language: "zh-CN",
  keywords: "",
  wordCount: 1200,
  providerKeyRef: "",
});
/* ===== AI 创作 - 参考内容 ===== */
const AI_MODEL_CONFIG_KEY = "cc-ai-model-config";
const selectedRefNewsIds = ref<number[]>([]);
const referenceContent = ref("");

function applySavedModelConfig() {
  try {
    const raw = localStorage.getItem(AI_MODEL_CONFIG_KEY);
    if (!raw) return;
    const saved = JSON.parse(raw);
    if (saved && typeof saved === "object") {
      if (saved.model && !brief.value.model) brief.value.model = saved.model;
      if (saved.wordCount && !brief.value.wordCount)
        brief.value.wordCount = Number(saved.wordCount);
    }
  } catch {
    /* 配置损坏时忽略，使用默认值 */
  }
}
const showArticleModal = ref(false);
const articleForm = ref<any>({
  id: 0,
  version: 0,
  title: "",
  slug: "",
  locale: "zh-CN",
  excerpt: "",
  bodyMarkdown: "",
  seoTitle: "",
  seoDescription: "",
  seoKeywords: "",
  featured: false,
  coverImage: "",
  coverAlt: "",
  category: "",
  tags: "",
  authorId: undefined,
  canonicalUrl: "",
  ogImage: "",
  status: "draft",
});

/* ==================== 文章编辑弹窗增强状态 ==================== */
const bodyEditMode = ref<"editor" | "source" | "preview">("editor");
const articleSaving = ref(false);
const inlineTaxDialog = ref<{
  show: boolean;
  scope: string;
  name: string;
  saving: boolean;
}>({ show: false, scope: "article_category", name: "", saving: false });

const renderedBodyPreview = computed(() =>
  DOMPurify.sanitize(String(articleForm.value.bodyMarkdown || ""), {
    USE_PROFILES: { html: true },
  }),
);

function switchBodyMode(mode: "editor" | "source" | "preview") {
  bodyEditMode.value = mode;
}

/* SEO 评分（本地启发式规则，0-100） */
const seoScoreTips = computed<string[]>(() => {
  const f = articleForm.value;
  const tips: string[] = [];
  const title = String(f.seoTitle || f.title || "");
  const desc = String(f.seoDescription || f.excerpt || "");
  const body = String(f.bodyMarkdown || "").replace(/<[^>]*>/g, "");
  if (title.length < 10 || title.length > 60) tips.push("SEO 标题建议 10-60 字符");
  if (desc.length < 50 || desc.length > 160) tips.push("Meta 描述建议 50-160 字符");
  if (!f.coverImage) tips.push("缺少封面图");
  if (f.coverImage && !f.coverAlt) tips.push("封面图缺少 Alt 文本");
  if (body.length < 300) tips.push("正文过短（建议 ≥300 字符）");
  if (!/<h[23][\s>]/i.test(String(f.bodyMarkdown || ""))) tips.push("正文缺少 H2/H3 小标题结构");
  if (!f.category) tips.push("未选择分类");
  if (!String(f.tags || "").trim()) tips.push("未设置标签");
  return tips;
});
const seoScore = computed(() => Math.max(0, 100 - seoScoreTips.value.length * 12));
const seoScoreLevel = computed(() =>
  seoScore.value >= 80 ? "score-good" : seoScore.value >= 50 ? "score-mid" : "score-bad",
);

/* SEO 自动化：按最佳实践一键生成标题/描述/关键词 */
function generateSeoMeta() {
  const f = articleForm.value;
  const bodyText = String(f.bodyMarkdown || "")
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  const baseTitle = String(f.title || "").trim();
  if (baseTitle) {
    f.seoTitle = baseTitle.length > 55 ? `${baseTitle.slice(0, 55)}…` : `${baseTitle} | CCAI`;
    if (f.seoTitle.length > 60) f.seoTitle = baseTitle.slice(0, 57) + "...";
  }
  const descSource = String(f.excerpt || "").trim() || bodyText;
  if (descSource) {
    f.seoDescription = descSource.slice(0, 150).replace(/[。；;]\s*[^。；;]*$/, "") + (descSource.length > 150 ? "。" : "");
  }
  const words = (baseTitle + " " + bodyText.slice(0, 400))
    .split(/[\s,，、。；;:：!！?？·\-—/()（）[\]]+/)
    .map((w) => w.trim())
    .filter((w) => w.length >= 2);
  const freq = new Map<string, number>();
  for (const w of words) freq.set(w, (freq.get(w) || 0) + 1);
  const keywords = [...freq.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 8)
    .map(([w]) => w);
  const tagKws = String(f.tags || "").split(",").map((t) => t.trim()).filter(Boolean);
  f.seoKeywords = [...new Set([...tagKws, ...keywords])].slice(0, 10).join(", ");
}

/* 弹窗内新建分类/标签（复用 taxonomy mutate 接口） */
function openInlineTaxonomyDialog(scope: string) {
  inlineTaxDialog.value = { show: true, scope, name: "", saving: false };
}
async function submitInlineTaxDialog() {
  const d = inlineTaxDialog.value;
  const name = d.name.trim();
  if (!name) return;
  d.saving = true;
  await runAdmin(async () => {
    await contentAPI.mutateTaxonomy({ scope: d.scope, action: "add", name });
    await loadTaxonomy();
    if (d.scope === "article_category") articleForm.value.category = name;
    else {
      const tags = String(articleForm.value.tags || "").split(",").map((t: string) => t.trim()).filter(Boolean);
      if (!tags.includes(name)) tags.push(name);
      articleForm.value.tags = tags.join(", ");
    }
    d.show = false;
  }, "创建失败");
  d.saving = false;
}

const unwrap = (v: any) => {
  if (Array.isArray(v)) return v;
  if (Array.isArray(v?.models)) return v.models;
  if (Array.isArray(v?.data?.models)) return v.data.models;
  if (Array.isArray(v?.data?.items)) return v.data.items;
  if (Array.isArray(v?.items)) return v.items;
  if (Array.isArray(v?.data)) return v.data;
  return [];
};
const pageMeta = (v: any) => {
  const candidates = [v?.data?.data, v?.data, v].filter(
    (item) => item && typeof item === "object" && !Array.isArray(item),
  );
  const payload =
    candidates.find(
      (item) =>
        Array.isArray(item.items) ||
        typeof item.total === "number" ||
        typeof item.hasNext === "boolean",
    ) || {};
  return {
    page: Number(payload.page || 1),
    total: Number(payload.total || 0),
    hasNext: Boolean(payload.hasNext),
  };
};
const formatDate = (v: string) => (v ? new Date(v).toLocaleDateString() : "-");
const formatDateTime = (v: string) =>
  v ? new Date(v).toLocaleString() : "-";
async function runAdmin(action: () => Promise<void>, message: string) {
  errorMessage.value = "";
  try {
    await action();
  } catch (error) {
    errorMessage.value = error instanceof Error && error.message ? error.message : message;
  }
}
const parseMappingJSON = (raw: unknown): Record<string, string> => {
  if (!String(raw || "").trim()) return {};
  let value: unknown;
  try {
    value = JSON.parse(String(raw));
  } catch {
    throw new Error("JSON 字段映射格式无效，请输入合法 JSON");
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("JSON 字段映射必须是对象");
  }
  const allowed = new Set([
    "title",
    "summary",
    "body",
    "content",
    "description",
    "url",
    "link",
    "external_id",
    "id",
    "source",
    "source_name",
    "image_url",
    "published_at",
  ]);
  const output: Record<string, string> = {};
  for (const [key, path] of Object.entries(value as Record<string, unknown>)) {
    if (!allowed.has(key) || typeof path !== "string" || !path.trim().startsWith("$")) {
      throw new Error(`JSON 字段映射无效：${key} 必须使用 $. 开头的路径`);
    }
    output[key] = path.trim();
  }
  return output;
};
async function loadNews(pageOrEvent: number | Event = newsPage.value) {
  const page = typeof pageOrEvent === "number" ? pageOrEvent : 1;
  newsPage.value = Math.max(1, page);
  const { data } = await contentAPI.listNews({
    page: newsPage.value,
    page_size: 20,
    search: newsSearch.value,
    status: newsStatus.value,
    locale: newsLocale.value,
    source_id: newsSourceId.value || undefined,
    from: newsFrom.value || undefined,
    to: newsTo.value || undefined,
  });
  news.value = unwrap(data);
  const meta = pageMeta(data);
  newsTotal.value = meta.total;
  newsHasNext.value = meta.hasNext;
  allNewsSelected.value = false;
  selectedNewsIds.value = [];
}
function toggleAllNews() {
  selectedNewsIds.value = allNewsSelected.value
    ? news.value.map((item) => item.id)
    : [];
}
function toggleAllArticles() {
  selectedArticleIds.value = allArticlesSelected.value
    ? articles.value.map((item) => item.id)
    : [];
}
async function bulkNews(action: string) {
  const label = action === "delete" ? "批量删除" : "批量下架";
  if (!selectedNewsIds.value.length) {
    toast("warning", "请先选择要操作的内容");
    return;
  }
  askConfirm({
    title: `确认${label}这 ${selectedNewsIds.value.length} 条新闻？`,
    message: action === "delete" ? "删除后可在回收站还原。" : "下架后前台将不再展示。",
    okText: `确认${label}`,
    danger: action === "delete",
    onOk: async () => {
      await runAdmin(async () => {
        await contentAPI.bulkNews({ ids: selectedNewsIds.value, action });
        selectedNewsIds.value = [];
        allNewsSelected.value = false;
        await loadNews(newsPage.value);
      }, "新闻批量操作失败");
    },
  });
}
async function bulkArticles(action: string) {
  const label = action === "delete" ? "批量删除" : "批量下架";
  if (!selectedArticleIds.value.length) {
    toast("warning", "请先选择要操作的内容");
    return;
  }
  askConfirm({
    title: `确认${label}这 ${selectedArticleIds.value.length} 篇文章？`,
    message: action === "delete" ? "删除后可在回收站还原。" : "下架后前台将不再展示。",
    okText: `确认${label}`,
    danger: action === "delete",
    onOk: async () => {
      await runAdmin(async () => {
        await contentAPI.bulkArticles({ ids: selectedArticleIds.value, action });
        selectedArticleIds.value = [];
        allArticlesSelected.value = false;
        await loadArticles(articlePage.value);
      }, "文章批量操作失败");
    },
  });
}
async function loadSources(page = sourcePage.value) {
  sourcePage.value = Math.max(1, page);
  const { data } = await contentAPI.listSources({
    page: sourcePage.value,
    page_size: 20,
    include_deleted: true,
  });
  sources.value = unwrap(data);
  const meta = pageMeta(data);
  sourceTotal.value = meta.total;
  sourceHasNext.value = meta.hasNext;
}
async function loadRuns(page = runPage.value) {
  runPage.value = Math.max(1, page);
  const { data } = await contentAPI.listFetchRuns({
    page: runPage.value,
    page_size: 20,
  });
  runs.value = unwrap(data);
  const meta = pageMeta(data);
  runTotal.value = meta.total;
  runHasNext.value = meta.hasNext;
}
async function loadArticles(pageOrEvent: number | Event = articlePage.value) {
  const page = typeof pageOrEvent === "number" ? pageOrEvent : 1;
  articlePage.value = Math.max(1, page);
  const { data } = await contentAPI.listArticles({
    page: articlePage.value,
    page_size: 20,
    search: articleSearch.value,
    status: articleStatus.value,
    locale: articleLocale.value,
    category: articleCategory.value,
    tags: articleTags.value,
    model: articleModel.value,
    author_id: articleAuthorId.value || undefined,
    from: articleFrom.value || undefined,
    to: articleTo.value || undefined,
    min_reads: articleMinReads.value || undefined,
    max_reads: articleMaxReads.value || undefined,
  });
  articles.value = unwrap(data);
  // D2：把当前结果里出现过的 model 名并入筛选下拉，保证存量数据可被筛出
  const seenModels = new Set(articleModelNames.value);
  for (const a of (articles.value as any[])) {
    const mv = String((a as any)?.model || "").trim();
    if (mv && !seenModels.has(mv)) {
      seenModels.add(mv);
      articleModelNames.value = [...articleModelNames.value, mv];
    }
  }
  const meta = pageMeta(data);
  articleTotal.value = meta.total;
  articleHasNext.value = meta.hasNext;
  allArticlesSelected.value = false;
  selectedArticleIds.value = [];
}
async function loadModels() {
  try {
    const { data } = await contentAPI.models("");
    const payload: any = (data as any)?.data ?? data;
    const rawList: any[] = Array.isArray(payload)
      ? payload
      : payload?.Models || payload?.models || payload?.items || [];
    models.value = rawList
      .map((m: any) => (typeof m === "string" ? { id: m, name: m } : m))
      .filter((m: any) => Boolean(m?.id || m?.name));
    modelsLoadError.value = "";
  } catch {
    models.value = [];
    modelsLoadError.value = "模型列表加载失败，请稍后重试";
  }
  if (!brief.value.model && aiModelOptions.value.length) {
    brief.value.model = aiModelOptions.value[0];
  }
}
async function loadTranslations(id: number) {
  const { data } = await contentAPI.listTranslations(id);
  translations.value = unwrap(data);
}
async function loadBI() {
  const params = { ...biFilters.value };
  const [o, t, r, c] = await Promise.all([
    contentAPI.analyticsOverview(params),
    contentAPI.analyticsTrends(params),
    contentAPI.analyticsRankings({ ...params, page: 1, page_size: 10 }),
    contentAPI.analyticsCTR(params),
  ]);
  overview.value = o.data?.data || o.data || {};
  trends.value = unwrap(t.data);
  rankings.value = unwrap(r.data);
  ctrRows.value = unwrap(c.data);
}
async function editNews(item: any) {
  try {
    const { data } = await contentAPI.getNews(item.id);
    selectedNews.value = data?.data || data || { ...item };
  } catch {
    selectedNews.value = { ...item };
    errorMessage.value = "新闻详情加载失败，已显示列表数据";
  }
  await nextTick();
  newsEditorRef.value?.scrollIntoView({ behavior: "smooth", block: "start" });
}
function stripEditorHtml(html: string): string {
  const div = document.createElement("div");
  div.innerHTML = html;
  return (div.textContent || div.innerText || "").replace(/\s+/g, " ").trim();
}

async function saveNews() {
  if (!selectedNews.value.id) return;
  try {
    await contentAPI.updateNews(selectedNews.value.id, {
      ...selectedNews.value,
      sourceId: selectedNews.value.sourceId || selectedNews.value.sourceID,
      category: String(selectedNews.value.category || "").trim(),
      tags: parseTagsText(selectedNewsTagsText.value),
      originalUrl: String(selectedNews.value.originalUrl || "").trim(),
      summary:
        String(selectedNews.value.summary || "").trim() ||
        stripEditorHtml(
          String(selectedNews.value.bodyHtml || selectedNews.value.bodyHTML || ""),
        ).slice(0, 140),
      bodyHtml: normalizeAnnouncementEditorHtml(
        String(selectedNews.value.bodyHtml || selectedNews.value.bodyHTML || ""),
      ),
    });
    await loadNews();
    await editNews(selectedNews.value);
    toast("success", "新闻已保存");
  } catch {
    errorMessage.value = "新闻保存失败，请检查标题、Slug 和正文";
    toast("error", "新闻保存失败，请检查标题、Slug 和正文");
  }
}

/* ===== 新闻弹窗：标签数组 ↔ 逗号分隔文本 ===== */
function parseTagsText(text: string): string[] {
  return String(text || "")
    .split(/[,，]/)
    .map((v) => v.trim())
    .filter(Boolean);
}
const selectedNewsTagsText = computed({
  get: () => {
    const tags = selectedNews.value?.tags;
    if (Array.isArray(tags)) return tags.join(", ");
    return tags ? String(tags) : "";
  },
  set: (val: string) => {
    if (!selectedNews.value) return;
    selectedNews.value.tags = parseTagsText(val);
  },
});

async function loadAutomation() {
  const { data } = await contentAPI.getAutomation();
  const payload: any = data?.data || data || {};
  automation.value = {
    ...automation.value,
    ...payload,
    // 后端字段为 PublishWindowStart/End（HH:MM），去重窗口必填
    publishWindowStart: payload.publishWindowStart || payload.PublishWindowStart || "09:00",
    publishWindowEnd: payload.publishWindowEnd || payload.PublishWindowEnd || "22:00",
    dedupeWindowHours:
      payload.dedupeWindowHours ?? payload.DedupeWindowHours ?? 24,
    targetLocalesText: Array.isArray(payload.targetLocales)
      ? payload.targetLocales.join(",")
      : payload.targetLocalesText ||
        (Array.isArray(payload.TargetLocales) ? payload.TargetLocales.join(",") : "zh-CN"),
  };
}

// ---- 新闻行的发表 / 删除：必须走 /admin/content/news 接口（新闻与文章是两套表） ----
async function publishNews(id: number) {
  await runAdmin(async () => {
    await contentAPI.publishNews(id);
    await loadNews();
    if (selectedNews.value.id === id) await editNews(selectedNews.value);
  }, "新闻发表失败");
}
const newsDeleteConfirm = ref<{ show: boolean; id: number; deleting: boolean }>({
  show: false,
  id: 0,
  deleting: false,
});
function deleteNews(id: number) {
  newsDeleteConfirm.value = { show: true, id, deleting: false };
}
async function confirmDeleteNews() {
  const c = newsDeleteConfirm.value;
  if (!c.id || c.deleting) return;
  c.deleting = true;
  await runAdmin(async () => {
    await contentAPI.deleteNews(c.id);
    if (selectedNews.value.id === c.id) selectedNews.value = {};
    await loadNews();
    c.show = false;
  }, "新闻删除失败");
  c.deleting = false;
}

async function refresh() {
  await Promise.all([
    loadNews(),
    loadSources(),
    loadRuns(),
    loadArticles(),
    loadModels(),
    loadBI(),
    loadAutomation(),
    loadTaxonomy(),
  ]);
}
async function createSource() {
  const name = String(sourceForm.value.name || "").trim();
  const url = String(sourceForm.value.url || "").trim();
  sourceFormErrors.value = { name: "", url: "" };
  if (!name) sourceFormErrors.value.name = "必填";
  if (!url) {
    sourceFormErrors.value.url = "URL 必填";
  } else if (!/^https?:\/\/\S+$/i.test(url)) {
    sourceFormErrors.value.url = "URL 需以 http:// 或 https:// 开头";
  }
  if (sourceFormErrors.value.name || sourceFormErrors.value.url) {
    toast("error", "请先补全新闻源名称与 URL");
    return;
  }
  await runAdmin(async () => {
    const mappingJson = parseMappingJSON(sourceForm.value.mappingJsonText);
    await contentAPI.createSource({
      ...sourceForm.value,
      name,
      url,
      mappingJson,
      language: "zh-CN",
      enabled: true,
      intervalMinutes: 15,
      rateLimitPerMinute: 60,
      authorizationNote: "公开 feed，已获授权",
      robotsPolicy: "遵守 robots 规则",
    });
    sourceForm.value = { name: "", url: "", sourceType: "rss", mappingJsonText: "" };
    await loadSources();
    toast("success", "新闻源已创建（默认启用，每 15 分钟抓取）");
  }, "新闻源创建失败");
}
async function toggleSource(s: any) {
  await runAdmin(async () => {
    await contentAPI.toggleSource(s.id, !s.enabled);
    await loadSources();
  }, "新闻源状态更新失败");
}
async function editSource(source: any) {
  try {
    const { data } = await contentAPI.getSource(source.id);
    selectedSource.value = data?.data || data || { ...source };
    selectedSource.value.mappingJsonText = JSON.stringify(selectedSource.value.mappingJson || {}, null, 2);
  } catch {
    selectedSource.value = { ...source };
    errorMessage.value = "新闻源详情加载失败";
  }
}
async function saveSource() {
  if (!selectedSource.value.id) return;
  await runAdmin(async () => {
    const payload = { ...selectedSource.value, mappingJson: parseMappingJSON(selectedSource.value.mappingJsonText) };
    await contentAPI.updateSource(
      selectedSource.value.id,
      payload,
    );
    await loadSources();
    await editSource(selectedSource.value);
  }, "新闻源保存失败");
}
async function restoreSource(id: number) {
  await runAdmin(async () => {
    await contentAPI.restoreSource(id);
    await loadSources();
  }, "新闻源恢复失败");
}
async function fetchSource(id: number) {
  await runAdmin(async () => {
    await contentAPI.fetchSource(id);
    await loadRuns();
  }, "新闻源抓取任务创建失败");
}
async function retryRun(id: number) {
  await runAdmin(async () => {
    await contentAPI.retryFetchRun(id);
    await loadRuns();
  }, "抓取任务重试失败");
}
async function deleteSource(id: number) {
  askConfirm({
    title: "确认删除这个新闻源？",
    message: "删除后该源不再抓取，可在列表中通过「恢复」还原。",
    okText: "确认删除",
    danger: true,
    onOk: async () => {
      await runAdmin(async () => {
        await contentAPI.deleteSource(id);
        await loadSources();
      }, "新闻源删除失败");
    },
  });
}


async function saveAutomation() {
  const dedupe = Number(automation.value.dedupeWindowHours);
  automationErrors.value = { dedupeWindowHours: "" };
  if (!Number.isFinite(dedupe) || dedupe <= 0) {
    automationErrors.value.dedupeWindowHours = "去重窗口必填，且必须大于 0 小时（后端强校验）";
    toast("error", "去重窗口小时数必须大于 0");
    return;
  }
  if (!String(automation.value.timezone || "").trim()) {
    toast("error", "时区必填");
    return;
  }
  const body: Record<string, unknown> = {
    enabled: Boolean(automation.value.enabled),
    timezone: String(automation.value.timezone).trim(),
    fetchCron: String(automation.value.fetchCron || "").trim(),
    publishMode: automation.value.publishMode || "manual_review",
    dailyFetchLimit: Number(automation.value.dailyFetchLimit || 0),
    dailyPublishLimit: Number(automation.value.dailyPublishLimit || 0),
    // A3：后端 UnmarshalJSON 只从 publishStartAt/publishEndAt（支持 "09:00" 时钟格式）
    // 解析并覆盖 publishWindowStart/End，因此两者都要带上，否则时间窗会被清空。
    publishWindowStart: String(automation.value.publishWindowStart || ""),
    publishWindowEnd: String(automation.value.publishWindowEnd || ""),
    publishStartAt: String(automation.value.publishWindowStart || ""),
    publishEndAt: String(automation.value.publishWindowEnd || ""),
    dedupeWindowHours: dedupe,
    targetLocales: String(automation.value.targetLocalesText || "")
      .split(/[,，]/)
      .map((v: string) => v.trim())
      .filter(Boolean),
  };
  await runAdmin(async () => {
    await contentAPI.updateAutomation(body);
    await loadAutomation();
    toast("success", "自动化设置已保存");
  }, "自动化设置保存失败");
}
// C6：任务 result 现在是 "article:13" 这类字符串，直接 JSON.parse 会抛错导致右栏整体不渲染
const taskArticle = ref<any>({});
const taskArticleError = ref("");
const writingTaskArticleId = computed(() => {
  const t: any = writingTask.value || {};
  const direct = t.articleId ?? t.article_id;
  if (direct) return Number(direct);
  const r = t.result;
  if (typeof r === "number") return r;
  if (typeof r === "string") {
    const m = r.match(/article[:#\s]*(\d+)/i);
    if (m) return Number(m[1]);
  }
  if (r && typeof r === "object") {
    const v = r.articleId ?? r.article_id ?? r.id;
    if (v && Number.isFinite(Number(v))) return Number(v);
  }
  return 0;
});
const writingTaskPayload = computed<any>(() => {
  const r: any = writingTask.value?.result;
  if (!r) return null;
  if (typeof r === "object") return r;
  if (typeof r !== "string") return null;
  const text = r.trim();
  if (!text.startsWith("{") && !text.startsWith("[")) return null; // "article:13" 之类的引用串
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
});
const writingTaskResult = computed<any>(() => {
  const payload = writingTaskPayload.value || {};
  const article = taskArticle.value || {};
  const merged: any = { ...article, ...payload };
  if (writingTaskArticleId.value) merged.articleId = writingTaskArticleId.value;
  if (!merged.title && article.title) merged.title = article.title;
  if (!merged.bodyHtml && (article.bodyMarkdown || article.bodyText)) {
    merged.bodyHtml = article.bodyMarkdown || article.bodyText;
  }
  return Object.keys(merged).length ? merged : null;
});
const writingTaskBodyHtml = computed(() => {
  const r: any = writingTaskResult.value;
  if (!r) return "";
  const raw = r.bodyHtml || r.bodyMarkdown || r.body || r.bodyText || "";
  return raw ? DOMPurify.sanitize(String(raw)) : "";
});
async function loadTaskArticle(id: number) {
  if (!id) {
    taskArticle.value = {};
    taskArticleError.value = "";
    return;
  }
  try {
    const { data } = await contentAPI.getArticle(id);
    taskArticle.value = data?.data || data || {};
    taskArticleError.value = "";
  } catch {
    taskArticle.value = {};
    taskArticleError.value = `关联文章 #${id} 已不存在或读取失败（可能已被删除）`;
  }
}
watch(
  () => writingTaskArticleId.value,
  (id) => {
    void loadTaskArticle(id);
  },
  { immediate: true },
);

function taskStatusText(status: string): string {
  const map: Record<string, string> = {
    queued: "排队中",
    running: "生成中",
    succeeded: "已完成",
    failed: "失败",
    cancelled: "已取消",
  };
  return map[status] || status || "-";
}

async function loadWritingTasks() {
  try {
    const { data } = await contentAPI.listWritingTasks(20);
    const list = data?.data || data || [];
    // 后端返回的任务主键字段为 id，这里归一化出 taskId，供选中/删除/轮询逻辑统一使用
    writingTaskList.value = (Array.isArray(list) ? list : []).map((t: any) => ({
      ...t,
      taskId: t.taskId || t.task_id || t.id,
    }));
  } catch {
    writingTaskList.value = [];
  }
}

function selectWritingTask(t: any) {
  stopWritingPoll();
  writingTask.value = t;
  const id = t.taskId || t.task_id || t.id;
  if (id && ["queued", "running"].includes(t.status)) {
    pollWritingTask(id);
  }
}

function resetAiStudio() {
  stopWritingPoll();
  writingTask.value = {};
  brief.value.topic = "";
}

async function createTask() {
  try {
    const refNews = news.value.filter((item) =>
      selectedRefNewsIds.value.includes(item.id),
    );
    const refNewsText = refNews
      .map((item) => `【新闻】${item.title}${item.summary ? "：" + item.summary : ""}`)
      .join("\n\n");
    const referenceText = (referenceContent.value || "").trim();
    const response = await contentAPI.createWritingTask({
      ...brief.value,
      keywords: String(brief.value.keywords || "")
        .split(",")
        .map((v: string) => v.trim())
        .filter(Boolean),
      contentType: "article",
      sourceNews: refNews,
      sourceMaterial: refNewsText,
      referenceContent: referenceText,
      sourceContent: [refNewsText, referenceText].filter(Boolean).join("\n\n---\n\n"),
    });
    writingTask.value = response.data?.data || response.data || {};
    brief.value.topic = "";
    void loadWritingTasks();
    if (writingTask.value.id) pollWritingTask(writingTask.value.id);
  } catch {
    errorMessage.value = "写作任务创建失败，请检查模型和账户配置";
  }
}
function stopWritingPoll() {
  if (writingPollTimer) clearTimeout(writingPollTimer);
  writingPollTimer = undefined;
}
async function pollWritingTask(id: string) {
  stopWritingPoll();
  try {
    const { data } = await contentAPI.getWritingTask(id);
    writingTask.value = data?.data || data || writingTask.value;
    if (["queued", "running"].includes(writingTask.value.status)) {
      writingPollTimer = setTimeout(() => void pollWritingTask(id), 1000);
    } else if (writingTask.value.status === "succeeded") {
      void loadWritingTasks();
      await loadArticles();
      if (writingTask.value.articleId) {
        const article = articles.value.find(
          (item) => item.id === writingTask.value.articleId,
        );
        if (article) editArticle(article);
      }
    }
  } catch {
    errorMessage.value = "写作任务状态获取失败，请稍后刷新";
  }
}
async function cancelTask() {
  if (!writingTask.value.id) return;
  try {
    const { data } = await contentAPI.cancelWritingTask(writingTask.value.id);
    writingTask.value = data?.data || data || writingTask.value;
    stopWritingPoll();
  } catch {
    errorMessage.value = "写作任务取消失败";
  }
}
async function exportBI() {
  try {
    const response = await contentAPI.analyticsExport(biFilters.value);
    const blob =
      response.data instanceof Blob
        ? response.data
        : new Blob([response.data], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "ccai-content-analytics.csv";
    link.click();
    URL.revokeObjectURL(url);
  } catch {
    errorMessage.value = "BI 导出失败";
  }
}
async function publishArticle(id: number) {
  await runAdmin(async () => {
    await contentAPI.publishArticle(id);
    await loadArticles();
  }, "文章发布失败");
}
/** 精选标记标签（与后端 isArticleFeatured 判定保持一致：tags 含「精选」/featured） */
const FEATURED_TAG = "精选";
const articleFeatureBusyId = ref<number | null>(null);

function isArticleRowFeatured(item: any): boolean {
  if (item?.isFeatured) return true;
  const tags: string[] = Array.isArray(item?.tags) ? item.tags : [];
  return tags.some((t) => String(t).trim() === FEATURED_TAG || String(t).trim().toLowerCase() === "featured");
}

async function toggleArticleFeatured(item: any, ev: Event) {
  if (!item?.id || articleFeatureBusyId.value === item.id) return;
  const checked = (ev.target as HTMLInputElement)?.checked ?? false;
  articleFeatureBusyId.value = item.id;
  try {
    await (contentAPI.mutateTaxonomy as any)({
      scope: "article_feature",
      action: checked ? "feature_article" : "unfeature_article",
      name: String(item.id),
    });
    // 本地同步：精选为单选，其他文章自动落选；后端已写入 tags + taxonomy 并失效公开缓存
    for (const a of articles.value) {
      const tags: string[] = Array.isArray(a.tags) ? [...a.tags] : [];
      const idx = tags.findIndex((t) => String(t).trim() === FEATURED_TAG || String(t).trim().toLowerCase() === "featured");
      if (a.id === item.id) {
        if (checked && idx < 0) a.tags = [...tags, FEATURED_TAG];
        if (!checked && idx >= 0) a.tags = tags.filter((_, i) => i !== idx);
        a.isFeatured = checked;
      } else if (checked) {
        if (idx >= 0) a.tags = tags.filter((_, i) => i !== idx);
        a.isFeatured = false;
      }
    }
  } catch (e) {
    (ev.target as HTMLInputElement).checked = !checked;
    errorMessage.value = "精选状态更新失败";
  } finally {
    articleFeatureBusyId.value = null;
  }
}

async function unpublishArticle(id: number) {
  await runAdmin(async () => {
    await contentAPI.unpublishArticle?.(id);
    await loadArticles();
  }, "文章下架失败");
}
const articleDeleteConfirm = ref<{ show: boolean; id: number; deleting: boolean }>({
  show: false,
  id: 0,
  deleting: false,
});
function deleteArticle(id: number) {
  articleDeleteConfirm.value = { show: true, id, deleting: false };
}
async function confirmDeleteArticle() {
  const c = articleDeleteConfirm.value;
  if (!c.id || c.deleting) return;
  c.deleting = true;
  await runAdmin(async () => {
    await contentAPI.deleteArticle(c.id);
    await loadArticles();
    c.show = false;
  }, "文章删除失败");
  c.deleting = false;
}
async function handleImageUpload(e: Event, field: 'coverImage' | 'ogImage') {
  const target = e.target as HTMLInputElement;
  const file = target?.files?.[0];
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    errorMessage.value = "请选择图片文件";
    target.value = '';
    return;
  }
  if (file.size > 5 * 1024 * 1024) {
    errorMessage.value = "图片不能超过 5MB";
    target.value = '';
    return;
  }
  imageUploading.value = field;
  try {
    await runAdmin(async () => {
      const { data } = await contentAPI.uploadContentImage(file);
      const payload = data?.data || data;
      const url = payload?.url || payload?.path;
      if (!url) throw new Error("上传响应缺少图片地址");
      articleForm.value[field] = url;
    }, "图片上传失败");
  } finally {
    imageUploading.value = "";
    target.value = '';
  }
}

async function deleteTaskRecord(t: any) {
  const id = t?.taskId || t?.task_id || t?.id;
  if (!id) return;
  askConfirm({
    title: "删除这条生成任务记录？",
    message: "已生成的文章不会被删除，仅移除任务记录。",
    okText: "确认删除",
    danger: true,
    onOk: async () => {
      await runAdmin(async () => {
        await contentAPI.deleteWritingTask(id);
        if ((writingTask.value.taskId || writingTask.value.task_id || writingTask.value.id) === id) {
          writingTask.value = {};
        }
        await loadWritingTasks();
        toast("success", "任务记录已删除");
      }, "任务删除失败");
    },
  });
}

async function saveArticleAndClose() {
  if (articleSaving.value) return;
  articleSaving.value = true;
  try {
    await saveArticle();
    showArticleModal.value = false;
  } catch (error) {
    errorMessage.value = error instanceof Error && error.message ? error.message : "文章保存失败";
  } finally {
    articleSaving.value = false;
  }
}

async function saveArticle() {
  const body = { ...articleForm.value };
  body.tags =
    typeof body.tags === "string"
      ? body.tags
          .split(",")
          .map((v: string) => v.trim())
          .filter(Boolean)
      : body.tags;
  // 精选开关与 tags 双向同步（后端精选 = tags 含「精选」/featured 或 taxonomy FeaturedArticleIDs）
  const tagList: string[] = Array.isArray(body.tags) ? [...body.tags] : [];
  const featuredTagIdx = tagList.findIndex(
    (t) => String(t).trim() === FEATURED_TAG || String(t).trim().toLowerCase() === "featured",
  );
  if (articleForm.value.featured) {
    if (featuredTagIdx < 0) tagList.push(FEATURED_TAG);
  } else if (featuredTagIdx >= 0) {
    tagList.splice(featuredTagIdx, 1);
  }
  body.tags = tagList;
  delete body.id;
  if (articleForm.value.id) {
    await contentAPI.updateArticle(articleForm.value.id, body);
  } else {
    await contentAPI.createArticle(body);
  }
  await loadArticles();
  resetArticle();
  bodyEditMode.value = "editor";
}
function editArticle(item: any) {
  bodyEditMode.value = "editor";
  showArticleModal.value = true;
  articleForm.value = {
    id: item.id,
    version: item.version || 0,
    title: item.title || "",
    slug: item.slug || "",
    locale: item.locale || "zh-CN",
    excerpt: item.excerpt || "",
    bodyMarkdown: announcementEditorInitialHtml(item.bodyMarkdown || item.bodyHtml || ""),
    seoTitle: item.seoTitle || "",
    seoDescription: item.seoDescription || "",
    seoKeywords: Array.isArray(item.seoKeywords) ? item.seoKeywords.join(", ") : item.seoKeywords || "",
    featured: Boolean(item.isFeatured ?? item.featured),
    coverImage: item.coverImage || "",
    coverAlt: item.coverAlt || "",
    category: item.category || "",
    tags: Array.isArray(item.tags) ? item.tags.join(", ") : item.tags || "",
    authorId: item.authorId,
    canonicalUrl: item.canonicalUrl || "",
    ogImage: item.ogImage || "",
    status: item.status || "draft",
  };
  void loadTranslations(item.id);
}
function editTranslation(item: any) {
  translationForm.value = {
    id: item.id,
    contentType: item.contentType,
    contentId: item.contentId,
    sourceVersion: item.sourceVersion,
    sourceLocale: item.sourceLocale,
    locale: item.locale,
    status: item.status,
    title: item.title || "",
    slug: item.slug || "",
    excerpt: item.excerpt || "",
    bodyMarkdown: item.bodyMarkdown || "",
    seoTitle: item.seoTitle || "",
    seoDescription: item.seoDescription || "",
  };
}
async function saveTranslation() {
  if (!translationForm.value.id) return;
  try {
    await contentAPI.updateTranslation(
      translationForm.value.id,
      translationForm.value,
    );
    await loadTranslations(articleForm.value.id);
  } catch (e) {
    errorMessage.value = "译文保存失败，请检查内容完整性";
  }
}
async function publishTranslation(item: any) {
  try {
    await contentAPI.publishTranslation(item.id);
    await loadTranslations(articleForm.value.id);
  } catch (e) {
    errorMessage.value = "译文发布失败，请检查源文章状态";
  }
}
async function unpublishTranslation(item: any) {
  try {
    await contentAPI.unpublishTranslation(item.id);
    await loadTranslations(articleForm.value.id);
  } catch (e) {
    errorMessage.value = "译文下架失败";
  }
}
async function generateTranslation() {
  if (!articleForm.value.id) return;
  await contentAPI.createTranslationTask(articleForm.value.id, {
    locale: translationLocale.value,
    model: brief.value.model,
    providerKeyRef: brief.value.providerKeyRef,
  });
  await loadTranslations(articleForm.value.id);
}
async function openGeneratedArticle() {
  const r = writingTaskResult.value;
  const articleId = writingTaskArticleId.value;
  if (!r && !articleId) return;
  // 任务已落库（result="article:N"）时，直接载入该文章，避免重复创建草稿
  if (articleId) {
    try {
      const { data } = await contentAPI.getArticle(articleId);
      const existing: any = data?.data || data;
      if (existing?.id) {
        showArticleModal.value = true;
        articleForm.value = {
          id: existing.id,
          version: existing.version || 0,
          title: existing.title || "",
          slug: existing.slug || "",
          locale: existing.locale || brief.value.language || "zh-CN",
          excerpt: existing.excerpt || existing.seoDescription || "",
          bodyMarkdown: announcementEditorInitialHtml(
            existing.bodyMarkdown || existing.bodyHtml || "",
          ),
          seoTitle: existing.seoTitle || "",
          seoDescription: existing.seoDescription || "",
          coverImage: existing.coverImage || "",
          coverAlt: existing.coverAlt || "",
          category: existing.category || brief.value.category || "架构与网关",
          tags: Array.isArray(existing.tags) ? existing.tags.join(",") : existing.tags || brief.value.keywords || "",
          authorId: existing.authorId,
          canonicalUrl: existing.canonicalUrl || "",
          ogImage: existing.ogImage || "",
          status: existing.status || "draft",
        };
        toast("info", `已载入文章 #${existing.id}：${existing.title || ""}`);
        return;
      }
    } catch {
      toast("warning", `文章 #${articleId} 无法读取，将按任务返回内容新建草稿`);
    }
  }
  showArticleModal.value = true;
  articleForm.value = {
    id: 0,
    version: 0,
    title: r.title || "",
    slug: r.slug || "",
    locale: brief.value.language || "zh-CN",
    excerpt: r.excerpt || r.summary || "",
    bodyMarkdown: announcementEditorInitialHtml(r.bodyHtml || r.bodyMarkdown || r.body || ""),
    seoTitle: r.seoTitle || r.seo_title || "",
    seoDescription: r.seoDescription || r.seo_description || "",
    coverImage: "",
    coverAlt: "",
    category: brief.value.category || "架构与网关",
    tags: brief.value.keywords || "",
    authorId: undefined,
    canonicalUrl: "",
    ogImage: "",
    status: "draft",
  };
}

function openNewArticle() {
  resetArticle();
  showArticleModal.value = true;
}

function resetArticle() {
  stopWritingPoll();
  articleForm.value = {
    id: 0,
    version: 0,
    title: "",
    slug: "",
    locale: "zh-CN",
    excerpt: "",
    bodyMarkdown: "",
    seoTitle: "",
    seoDescription: "",
    seoKeywords: "",
    featured: false,
    coverImage: "",
    coverAlt: "",
    category: "",
    tags: "",
    authorId: undefined,
    canonicalUrl: "",
    ogImage: "",
    status: "draft",
  };
  translationForm.value = {
    id: 0,
    title: "",
    slug: "",
    excerpt: "",
    bodyMarkdown: "",
    seoTitle: "",
    seoDescription: "",
  };
  translations.value = [];
}
function previewArticle() {
  const win = window.open("", "_blank");
  if (win) {
    const title = articleForm.value.title || "CCAI 文章预览";
    const body = String(articleForm.value.bodyMarkdown || "");
    const doc = win.document;
    doc.title = title;
    const main = doc.createElement("main");
    main.style.cssText =
      "max-width:760px;margin:40px auto;font:16px/1.8 sans-serif";
    const heading = doc.createElement("h1");
    heading.textContent = title;
    const rendered = doc.createElement("div");
    rendered.innerHTML = DOMPurify.sanitize(marked.parse(body) as string, {
      USE_PROFILES: { html: true },
    });
    main.append(heading, rendered);
    doc.body.appendChild(main);
  }
}
watch([current, newsView], refresh);
/* ==================== 分类与标签 (Taxonomy) ==================== */
type TaxonomyViewKey = "articleCategories" | "newsCategories" | "tags";
const taxonomyViews: { key: TaxonomyViewKey; label: string; entity: string; scopeLabel: string; scope: string }[] = [
  { key: "articleCategories", label: "文章分类管理", entity: "文章分类", scopeLabel: "文章分类", scope: "article_category" },
  { key: "newsCategories", label: "新闻分类管理", entity: "新闻分类", scopeLabel: "新闻分类", scope: "news_category" },
  { key: "tags", label: "标签管理", entity: "标签", scopeLabel: "标签（文章/新闻通用）", scope: "tag" },
];
const taxonomyView = ref<TaxonomyViewKey>("articleCategories");
const taxonomy = ref<{ articleCategories: string[]; newsCategories: string[]; tags: string[] }>({
  articleCategories: [],
  newsCategories: [],
  tags: [],
});
const taxonomySearch = ref("");
const taxonomyPage = ref(1);
const taxonomyPageSize = 10;
const taxonomyDialog = ref<{
  show: boolean;
  mode: "create" | "edit" | "delete";
  name: string;
  originalName: string;
  saving: boolean;
}>({ show: false, mode: "create", name: "", originalName: "", saving: false });

const currentTaxonomyView = computed(
  () => taxonomyViews.find((v) => v.key === taxonomyView.value) || taxonomyViews[0],
);
const currentTaxonomyEntity = computed(() => currentTaxonomyView.value.entity || currentTaxonomyView.value.label);
const currentTaxonomyScopeLabel = computed(
  () => currentTaxonomyView.value.scopeLabel || currentTaxonomyView.value.entity || "通用",
);
const taxonomyItems = computed<string[]>(() => taxonomy.value[taxonomyView.value] || []);
const filteredTaxonomyItems = computed(() => {
  const q = taxonomySearch.value.trim().toLowerCase();
  if (!q) return taxonomyItems.value;
  return taxonomyItems.value.filter((n) => n.toLowerCase().includes(q));
});
const taxonomyPageCount = computed(() =>
  Math.max(1, Math.ceil(filteredTaxonomyItems.value.length / taxonomyPageSize)),
);
const pagedTaxonomyItems = computed(() => {
  const start = (taxonomyPage.value - 1) * taxonomyPageSize;
  return filteredTaxonomyItems.value.slice(start, start + taxonomyPageSize);
});
const taxonomyDialogTitle = computed(() => {
  const label = currentTaxonomyEntity.value;
  if (taxonomyDialog.value.mode === "create") return `新增${label}`;
  if (taxonomyDialog.value.mode === "edit") return `编辑${label}`;
  return `删除${label}`;
});

function switchTaxonomyView(key: TaxonomyViewKey) {
  taxonomyView.value = key;
  taxonomySearch.value = "";
  taxonomyPage.value = 1;
}
async function loadTaxonomy() {
  try {
    const { data } = await contentAPI.getTaxonomy();
    const payload = data?.data || data || {};
    taxonomy.value = {
      articleCategories: payload.articleCategories || [],
      newsCategories: payload.newsCategories || [],
      tags: payload.tags || [],
    };
  } catch {
    errorMessage.value = "分类与标签加载失败";
  }
}
function openTaxonomyDialog(mode: "create" | "edit" | "delete", name = "") {
  taxonomyDialog.value = {
    show: true,
    mode,
    name: mode === "edit" ? name : "",
    originalName: name,
    saving: false,
  };
}
function closeTaxonomyDialog() {
  taxonomyDialog.value.show = false;
}
async function submitTaxonomyDialog() {
  const dialog = taxonomyDialog.value;
  const scope = currentTaxonomyView.value.scope;
  if (dialog.mode !== "delete" && !dialog.name.trim()) {
    errorMessage.value = `请输入${currentTaxonomyEntity.value}名称`;
    return;
  }
  dialog.saving = true;
  await runAdmin(async () => {
    if (dialog.mode === "create") {
      await contentAPI.mutateTaxonomy({ scope, action: "add", name: dialog.name.trim() });
    } else if (dialog.mode === "edit") {
      await contentAPI.mutateTaxonomy({
        scope,
        action: "rename",
        name: dialog.originalName,
        newName: dialog.name.trim(),
      });
    } else {
      await contentAPI.mutateTaxonomy({ scope, action: "delete", name: dialog.originalName });
    }
    dialog.show = false;
    await loadTaxonomy();
    toast(
      "success",
      dialog.mode === "create"
        ? `${currentTaxonomyEntity.value}已创建`
        : dialog.mode === "edit"
          ? `${currentTaxonomyEntity.value}已更新`
          : `${currentTaxonomyEntity.value}已删除`,
    );
  }, "分类/标签操作失败");
  dialog.saving = false;
}

onMounted(() => {
  refresh();
  void loadWritingTasks();
  void loadTaxonomy();
  void loadNews(1);
  applySavedModelConfig();
});
</script>
<style scoped>
.admin-page {
  max-width: 100%;
  margin: 0 auto;
  padding: 4px 0 32px;
  color: #121316;
  font-family: 'Plus Jakarta Sans', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 20px 24px;
  box-shadow: 5px 5px 0px #121316;
  margin-bottom: 20px;
}
.eyebrow {
  display: inline-block;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 0.08em;
  color: #121316;
  background: #CCFF00;
  border: 1.5px solid #121316;
  box-shadow: 2px 2px 0px #121316;
  padding: 2px 8px;
  border-radius: 6px;
  font-weight: 900;
}
.page-header h1 {
  font-size: 24px;
  font-weight: 900;
  color: #121316;
  margin: 8px 0 4px;
  letter-spacing: -0.5px;
}
.page-header p,
.muted {
  color: #5E6577;
  font-weight: 600;
  font-size: 13.5px;
}
.error-message {
  color: #121316;
  background: #FFE4E1;
  border: 2px solid #121316;
  box-shadow: 4px 4px 0px #FF3B30;
  border-radius: 12px;
  padding: 12px 16px;
  font-weight: 800;
  margin-bottom: 16px;
}
.tabs,
.subnav {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 8px;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 14px;
  padding: 6px;
  box-shadow: 4px 4px 0px #121316;
  margin: 0 0 20px 0;
}
.subnav {
  background: #F4F7FF;
  box-shadow: 3px 3px 0px #121316;
}
.tabs button,
.subnav button {
  background: transparent;
  border: 2px solid transparent;
  border-radius: 10px;
  padding: 8px 16px;
  color: #5E6577;
  font-weight: 800;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.tabs button:hover,
.subnav button:hover {
  color: #121316;
  background: #EDF4FF;
}
.tabs button.active,
.subnav button.selected {
  color: #121316 !important;
  background: #CCFF00 !important;
  border: 2px solid #121316 !important;
  box-shadow: 2px 2px 0px #121316 !important;
  font-weight: 900 !important;
}
.workspace {
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 20px;
  padding: 24px;
  box-shadow: 6px 6px 0px #121316;
}
.toolbar,
.form-grid {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 18px;
}
.toolbar input,
.toolbar select,
.form-grid input,
.form-grid select,
.settings input,
.settings select,
textarea {
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 9px 12px;
  background: #FFFFFF;
  color: #121316;
  font-weight: 600;
  font-size: 13px;
  box-shadow: 2px 2px 0px rgba(18, 19, 22, 0.15);
  transition: all 0.15s ease;
}
.toolbar input:focus,
.toolbar select:focus,
.form-grid input:focus,
.form-grid select:focus,
.settings input:focus,
.settings select:focus,
textarea:focus {
  outline: none;
  border-color: #2563EB;
  box-shadow: 3px 3px 0px #2563EB;
}
.toolbar input {
  min-width: 240px;
}
.button {
  padding: 9px 18px;
  border: 2px solid #121316;
  border-radius: 10px;
  background: #CCFF00;
  color: #121316;
  font-weight: 900;
  font-size: 13px;
  box-shadow: 3px 3px 0px #121316;
  cursor: pointer;
  transition: transform 0.14s ease, box-shadow 0.14s ease;
}
.button:hover:not(:disabled) {
  transform: translate(-1.5px, -1.5px);
  box-shadow: 4.5px 4.5px 0px #121316;
  background: #d8ff33;
}
.button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.secondary-button {
  padding: 9px 18px;
  border: 2px solid #121316;
  border-radius: 10px;
  background: #FFFFFF;
  color: #121316;
  font-weight: 800;
  font-size: 13px;
  box-shadow: 3px 3px 0px #121316;
  cursor: pointer;
  transition: transform 0.14s ease, box-shadow 0.14s ease;
}
.secondary-button:hover:not(:disabled) {
  transform: translate(-1.5px, -1.5px);
  box-shadow: 4.5px 4.5px 0px #121316;
  background: #EDF4FF;
}
.table-wrap {
  overflow-x: auto;
  border: 2px solid #121316;
  border-radius: 14px;
  box-shadow: 4px 4px 0px #121316;
  background: #FFFFFF;
}
table {
  width: 100%;
  border-collapse: collapse;
}
th {
  text-align: left;
  padding: 13px 16px;
  background: #EDF4FF;
  border-bottom: 2px solid #121316;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 800;
  color: #121316;
  white-space: nowrap;
}
td {
  text-align: left;
  padding: 13px 16px;
  border-bottom: 1.5px solid rgba(18, 19, 22, 0.1);
  color: #121316;
  font-weight: 600;
  font-size: 13.5px;
  vertical-align: middle;
  white-space: nowrap;
}
td.title-cell {
  white-space: normal;
  min-width: 240px;
  max-width: 460px;
}
.title-link {
  appearance: none;
  border: none;
  background: transparent;
  padding: 0;
  margin: 0;
  text-align: left;
  color: #121316;
  font-weight: 800;
  font-size: 14px;
  line-height: 1.45;
  cursor: pointer;
  display: inline-block;
}
.title-link:hover {
  color: #2563EB;
  text-decoration: underline;
}
.summary-clamp {
  display: -webkit-box !important;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.45;
  font-size: 12px;
  color: #5E6577;
  margin-top: 4px;
}
.locale-tag {
  display: inline-block !important;
  background: #F4F7FF;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 2px 7px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 800;
  color: #121316;
  margin-top: 4px;
}
td.actions-cell {
  white-space: nowrap;
}
.table-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 18px;
  background: #F9FBFF;
  border-top: 2px solid #121316;
}
.bulk-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.bulk-actions .link,
.pagination .link {
  margin-right: 0;
}
.pagination {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
}
.page-info {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12.5px;
  font-weight: 800;
  color: #121316;
}
tbody tr:hover {
  background: #F4F7FF;
}
td strong,
td small {
  display: block;
}
td small {
  color: #5E6577;
  margin-top: 4px;
  font-family: 'JetBrains Mono', monospace;
}
.link {
  border: 1.5px solid #121316;
  background: #EDF4FF;
  color: #2563EB;
  font-weight: 800;
  font-size: 12.5px;
  padding: 6px 12px;
  border-radius: 8px;
  box-shadow: 2px 2px 0px #121316;
  cursor: pointer;
  margin-right: 8px;
  transition: transform 0.12s ease, box-shadow 0.12s ease, opacity 0.12s ease;
}
.link:hover:not(:disabled) {
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0px #121316;
}
.link:disabled {
  opacity: 0.48;
  cursor: not-allowed;
  box-shadow: 1px 1px 0px rgba(18, 19, 22, 0.35);
}
.link.danger {
  background: #FFE4E1;
  color: #FF3B30;
}
.status {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 800;
  color: #047857;
  background: #D1FAE5;
  border: 1.5px solid #121316;
  padding: 3px 9px;
  border-radius: 6px;
}
.empty {
  padding: 44px;
  text-align: center;
  color: #5E6577;
  font-weight: 700;
}
.settings {
  max-width: 960px;
}
.settings label,
.form-grid label {
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: #121316;
  font-weight: 800;
  font-size: 13px;
  min-width: 180px;
}
.switch {
  display: inline-flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 20px;
  font-weight: 800;
  background: #F4F7FF;
  border: 2px solid #121316;
  padding: 8px 14px;
  border-radius: 10px;
  box-shadow: 2px 2px 0px #121316;
}
.editor-grid,
.bi-grid {
  display: grid;
  grid-template-columns: 1fr 1.3fr;
  gap: 24px;
}
.editor-grid h2,
.bi-grid h2 {
  margin-top: 0;
  font-weight: 900;
  font-size: 18px;
}
.editor-grid textarea {
  width: 100%;
  min-height: 160px;
  resize: vertical;
  margin-bottom: 18px;
}
.metrics {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 26px;
}
.metrics div {
  background: #F8FAFF;
  border: 2px solid #121316;
  padding: 18px 20px;
  border-radius: 14px;
  box-shadow: 4px 4px 0px #121316;
}
.metrics small,
.metrics strong {
  display: block;
}
.metrics small {
  color: #5E6577;
  font-weight: 800;
  font-size: 12px;
}
.metrics strong {
  font-family: 'JetBrains Mono', monospace;
  font-size: 28px;
  font-weight: 900;
  color: #121316;
  margin-top: 8px;
}
.bi-grid > div {
  border-top: 2px solid #121316;
  padding-top: 20px;
}
.trend-row {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1.5px dashed rgba(18, 19, 22, 0.18);
  font-weight: 700;
}
.trend-row span {
  overflow: hidden;
  text-overflow: ellipsis;
}
.editor-grid hr {
  border: 0;
  border-top: 2px dashed rgba(18, 19, 22, 0.2);
  margin: 26px 0;
}
.short-text {
  min-height: 80px !important;
}
.article-body {
  min-height: 260px !important;
}
.form-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.title-link {
  border: 0;
  background: none;
  padding: 0;
  color: #2563EB;
  font-weight: 800;
  cursor: pointer;
  text-align: left;
}
.title-link:hover {
  text-decoration: underline;
}
.translation-panel {
  border-top: 2px solid #121316;
  margin-top: 24px;
  padding-top: 18px;
}
.translation-row {
  display: flex;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1.5px solid rgba(18, 19, 22, 0.1);
}
.news-editor,
.task-status {
  margin-top: 20px;
  padding: 18px;
  border: 2px solid #121316;
  border-radius: 14px;
  background: #F8FAFF;
  box-shadow: 4px 4px 0px #121316;
}
/* 新闻编辑面板改为浮层，避免出现在表格下方、视口外导致"点编辑没反应" */
.news-editor.news-editor-modal {
  margin: 0;
  width: min(1240px, calc(100vw - 48px));
  max-height: 92vh;
  overflow-y: auto;
  text-align: left;
  box-shadow: 6px 6px 0px #121316;
}
.task-status {
  display: flex;
  align-items: center;
  gap: 14px;
  color: #5E6577;
  font-weight: 700;
}
.task-status strong {
  color: #121316;
}
.error-text {
  display: block;
  color: #FF3B30;
  font-weight: 700;
}
.bi-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: end;
  margin-bottom: 24px;
}
.bi-toolbar label {
  min-width: 140px;
  font-weight: 800;
  font-size: 12.5px;
}
.bi-toolbar input,
.bi-toolbar select {
  display: block;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 8px 12px;
  background: #FFFFFF;
  font-weight: 600;
}
.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.news-editor small {
  display: block;
  color: #5E6577;
  margin-top: 4px;
}
@media (max-width: 900px) {
  .editor-grid,
  .bi-grid {
    grid-template-columns: 1fr;
  }
  .metrics {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* ==================== 现代化内容中心交互系统 ==================== */
.admin-page {
  max-width: 1440px;
  margin: 0 auto;
  padding: 24px 32px;
}

/* 顶部操作区 */
.top-actions-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.page-title-mini {
  font-size: 16px;
  font-weight: 800;
  color: #121316;
}

/* 表格容器与 Neobrutalist 风格微调 */
.table-wrap {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 14px;
  box-shadow: 4px 4px 0px #121316;
  margin-top: 16px;
  max-height: calc(100vh - 280px);
}

.table-top-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.new-btn {
  white-space: nowrap;
  background: #2563EB !important;
  color: #FFFFFF !important;
  font-weight: 800;
  border: 2px solid #121316 !important;
  box-shadow: 3px 3px 0px #121316 !important;
  transition: all 0.15s ease;
}
.new-btn:hover {
  transform: translate(-1px, -1px);
  box-shadow: 4px 4px 0px #121316 !important;
}

/* 操作列收窄 + sticky */
th:last-child, td.actions-cell {
  min-width: 110px;
  width: 110px;
  white-space: nowrap;
  position: sticky;
  right: 0;
  background: #FFFFFF;
  box-shadow: -2px 0 6px rgba(0,0,0,0.03);
}
td.actions-cell .link {
  font-size: 12.5px;
  padding: 3px 6px;
  font-weight: 700;
}

/* 数据汇总卡片：彩色渐变 */
.metric-card {
  border-radius: 12px;
  padding: 16px;
  color: #FFFFFF;
  border: none;
  box-shadow: 0 2px 8px rgba(0,0,0,0.12);
}
.metric-card.metric-views { background: linear-gradient(135deg, #1D4ED8 0%, #3B82F6 100%); }
.metric-card.metric-reads { background: linear-gradient(135deg, #065F46 0%, #10B981 100%); }
.metric-card.metric-uv { background: linear-gradient(135deg, #5B21B6 0%, #8B5CF6 100%); }
.metric-card.metric-ctr { background: linear-gradient(135deg, #B45309 0%, #F59E0B 100%); }
.metric-icon {
  font-size: 22px;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255,255,255,0.2);
  border-radius: 10px;
}
.metric-info small { color: rgba(255,255,255,0.8); }
.metric-info strong { font-size: 24px; }

/* ==================== 文章编辑抽屉 (Drawer) ==================== */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(18, 19, 22, 0.45);
  backdrop-filter: blur(5px);
  z-index: 1000;
  display: flex;
  justify-content: center;
  align-items: center;
  animation: fadeIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

/* 居中弹窗：分类与标签（新增/编辑/删除） */
.taxonomy-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(18, 19, 22, 0.45);
  backdrop-filter: blur(5px);
  z-index: 1100;
  display: flex;
  align-items: center;
  justify-content: center;
  animation: fadeIn 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}
.taxonomy-dialog {
  width: 100%;
  max-width: 440px;
  background: #FAF8F5;
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.25);
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.taxonomy-dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.taxonomy-dialog-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.taxonomy-dialog-body p {
  margin: 0 0 8px;
  font-size: 14px;
}
.taxonomy-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  color: #55524d;
}
.taxonomy-field input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #d6d3cd;
  border-radius: 8px;
  font-size: 14px;
  box-sizing: border-box;
}
.taxonomy-dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.modal-drawer {
  width: 100%;
  max-width: 1240px;
  max-height: 90vh;
  background: #FAF8F5;
  border: 2.5px solid #121316;
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(18, 19, 22, 0.25);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  animation: modalPop 0.24s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes modalPop {
  from { opacity: 0; transform: translateY(12px) scale(0.98); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

@keyframes slideLeft {
  from { transform: translateX(100%); }
  to { transform: translateX(0); }
}

.modal-drawer-header {
  padding: 18px 28px;
  background: #FFFFFF;
  border-bottom: 2px solid #121316;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.02);
}

.header-title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-badge {
  font-size: 11px;
  font-weight: 800;
  padding: 2px 8px;
  border-radius: 6px;
  background: #CCFF00;
  color: #121316;
  border: 1.5px solid #121316;
  letter-spacing: 0.5px;
}

.modal-drawer-header h3 {
  margin: 0;
  font-size: 19px;
  font-weight: 900;
  color: #121316;
}

.close-btn {
  background: #F1F5F9;
  border: 1.5px solid #CBD5E1;
  font-size: 16px;
  cursor: pointer;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 900;
  color: #475569;
  transition: all 0.15s ease;
}
.close-btn:hover {
  background: #FEE2E2;
  border-color: #EF4444;
  color: #DC2626;
}

.modal-drawer-body {
  padding: 24px 28px 40px;
  overflow-y: auto;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.modal-drawer-footer {
  padding: 16px 28px;
  background: #FFFFFF;
  border-top: 2px solid #121316;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  box-shadow: 0 -2px 6px rgba(0, 0, 0, 0.02);
}

/* 主编辑区 */
.modal-main-section {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 14px;
  padding: 22px;
  box-shadow: 4px 4px 0px #121316;
}

.field-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label-prominent {
  font-size: 13.5px;
  font-weight: 800;
  color: #121316;
  display: flex;
  align-items: center;
  gap: 4px;
}

.required-star {
  color: #EF4444;
  font-weight: 900;
}

.editor-title-input {
  width: 100%;
  font-size: 17px;
  font-weight: 700;
  padding: 11px 14px;
  border: 2px solid #121316;
  border-radius: 10px;
  background: #FAFBFC;
  box-shadow: 2px 2px 0px rgba(18, 19, 22, 0.05);
  transition: all 0.15s ease;
}
.editor-title-input:focus {
  background: #FFFFFF;
  border-color: #2563EB;
  outline: none;
  box-shadow: 2px 2px 0px #2563EB;
}

.short-text-area {
  width: 100%;
  font-family: inherit;
  font-size: 13.5px;
  padding: 10px 14px;
  border: 1.5px solid #CBD5E1;
  border-radius: 10px;
  background: #FAFBFC;
  resize: vertical;
  transition: all 0.15s ease;
}
.short-text-area:focus {
  background: #FFFFFF;
  border-color: #2563EB;
  outline: none;
}

/* 配置卡片栅格 */
.modal-meta-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.meta-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 18px;
  box-shadow: 3px 3px 0px #121316;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.meta-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1.5px dashed #E2E8F0;
  padding-bottom: 10px;
}
.meta-card-icon {
  font-size: 16px;
}
.meta-card-header h4 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 800;
  color: #121316;
}

.meta-card-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-row label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12.5px;
  font-weight: 700;
  color: #334155;
}
.form-row input, .form-row textarea {
  border: 1.5px solid #CBD5E1;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  font-family: inherit;
}
.form-row input:focus, .form-row textarea:focus {
  border-color: #2563EB;
  outline: none;
}

.form-row-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.form-row-grid label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12.5px;
  font-weight: 700;
  color: #334155;
}
.form-row-grid input {
  border: 1.5px solid #CBD5E1;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
}
.form-row-grid input:focus {
  border-color: #2563EB;
  outline: none;
}

/* 视觉配图管理 */
.media-assets-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.media-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 16px;
  box-shadow: 3px 3px 0px #121316;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.media-card-header {
  display: flex;
  align-items: center;
  gap: 10px;
}
.media-icon {
  font-size: 20px;
}
.media-card-header h5 {
  margin: 0;
  font-size: 14px;
  font-weight: 800;
  color: #121316;
}
.media-card-header small {
  color: #64748B;
  font-size: 11.5px;
}

.upload-control-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.upload-control-row input {
  border: 1.5px solid #CBD5E1;
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 12.5px;
}

.upload-file-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: #EDF4FF;
  border: 1.5px solid #121316;
  border-radius: 8px;
  padding: 8px 12px;
  font-weight: 800;
  font-size: 12px;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
  transition: all 0.15s ease;
  white-space: nowrap;
}
.upload-file-btn:hover {
  background: #DBEAFE;
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0px #121316;
}

.btn-clear {
  background: #FEE2E2;
  color: #DC2626;
  border: 1.5px solid #121316;
  border-radius: 8px;
  padding: 8px 10px;
  font-weight: 800;
  font-size: 12px;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
}

.image-preview-box {
  margin-top: 4px;
  border: 1.5px solid #121316;
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 2px 2px 0px #121316;
  background: #000;
  max-height: 150px;
}
.image-preview-box img {
  width: 100%;
  height: 150px;
  object-fit: cover;
  display: block;
}

.alt-label {
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  display: flex;
  align-items: center;
  gap: 6px;
}
.alt-label input {
  flex: 1;
  border: 1px solid #CBD5E1;
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 12px;
}

/* 翻译管理卡片 */
.translations-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 18px;
  box-shadow: 3px 3px 0px #121316;
}

.trans-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  border-bottom: 1.5px dashed #E2E8F0;
  padding-bottom: 12px;
}
.trans-header h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
}
.trans-header small {
  color: #64748B;
  font-size: 12px;
}

.translation-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.locale-select {
  border: 1.5px solid #121316;
  border-radius: 8px;
  padding: 7px 10px;
  font-size: 13px;
  font-weight: 700;
  background: #FFFFFF;
}
.sparkle-btn {
  background: #FEF08A !important;
  color: #854D0E !important;
  border: 1.5px solid #121316 !important;
  font-weight: 800 !important;
  box-shadow: 2px 2px 0px #121316 !important;
}

.translations-chips {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 14px;
}

.trans-chip {
  background: #F8FAFC;
  border: 1.5px solid #121316;
  padding: 6px 12px;
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  box-shadow: 2px 2px 0px #121316;
}
.trans-chip-title {
  cursor: pointer;
}
.trans-chip-title:hover {
  text-decoration: underline;
  color: #2563EB;
}

.status-badge {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 800;
}
.badge-published {
  background: #DCFCE7;
  color: #166534;
}
.badge-draft {
  background: #F1F5F9;
  color: #475569;
}

.link-action {
  background: none;
  border: none;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  padding: 2px 4px;
}
.link-action:hover {
  text-decoration: underline;
}

/* ==================== AI 创作工作台 (AI Studio) ==================== */
.ai-studio-pane {
  max-width: 100%;
}

.panel-header-desc {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 14px;
  padding: 20px 24px;
  box-shadow: 4px 4px 0px #121316;
  margin-bottom: 20px;
}
.panel-header-desc h2 {
  margin: 0 0 6px 0;
  font-size: 21px;
  font-weight: 900;
  color: #121316;
}
.panel-header-desc p {
  margin: 0;
  font-size: 13.5px;
  color: #64748B;
  line-height: 1.5;
}

.ai-studio-grid {
  display: grid;
  grid-template-columns: 290px 400px 1fr;
  gap: 20px;
  align-items: stretch;
}

/* ===== 左栏需求输入卡片（E2）===== */
.ai-topic-card {
  padding: 12px 14px;
  gap: 8px;
}
.ai-topic-input {
  width: 100%;
  min-height: 110px;
  border: 2px solid #E2E8F0;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
  font-family: inherit;
  color: #121316;
  background: #FAFBFC;
  resize: vertical;
  outline: none;
  transition: border-color 0.15s;
  box-sizing: border-box;
}
.ai-topic-input:focus {
  border-color: #121316;
}

/* ===== 中栏参考内容（E2）===== */
.ref-news-list {
  max-height: 180px;
  overflow-y: auto;
  border: 2px solid #E2E8F0;
  border-radius: 8px;
  background: #FAFBFC;
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ref-news-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 8px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12.5px;
  font-weight: 600;
  color: #1E293B;
  transition: background 0.12s;
}
.ref-news-item:hover {
  background: #F1F5F9;
}
.ref-news-item input[type="checkbox"] {
  flex-shrink: 0;
  width: 14px;
  height: 14px;
  cursor: pointer;
}
.ref-news-title {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ref-news-empty {
  margin: 0;
  padding: 10px;
  text-align: center;
  font-size: 12px;
}
.ai-ref-textarea {
  width: 100%;
  min-height: 110px;
  border: 2px solid #E2E8F0;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 13px;
  font-family: inherit;
  color: #121316;
  background: #FAFBFC;
  resize: vertical;
  outline: none;
  transition: border-color 0.15s;
  box-sizing: border-box;
}
.ai-ref-textarea:focus {
  border-color: #121316;
}

/* ===== 右栏采纳/重新生成按钮（E2）===== */
.result-actions {
  display: flex;
  gap: 10px;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid #E2E8F0;
}

.ai-col {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 8px;
  border-bottom: 2px solid #121316;
}
.ai-col-header h3 {
  margin: 0;
  font-size: 15.5px;
  font-weight: 900;
  color: #121316;
}

.new-btn-mini {
  padding: 4px 10px !important;
  font-size: 12px !important;
  border-radius: 6px !important;
  box-shadow: 2px 2px 0px #121316 !important;
}

/* 左侧：任务队列卡片 */
.task-list {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 12px;
  box-shadow: 3px 3px 0px #121316;
  max-height: 380px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.task-item {
  border: 1.5px solid #E2E8F0;
  background: #FAFBFC;
  border-radius: 8px;
  padding: 10px 12px;
  cursor: pointer;
  transition: all 0.15s ease;
}
.task-item:hover {
  border-color: #121316;
  background: #F8FAFC;
  transform: translateY(-1px);
}
.task-item.active {
  border-color: #121316;
  background: #EFF6FF;
  box-shadow: 2px 2px 0px #121316;
}

.task-item-topic {
  font-weight: 800;
  font-size: 13px;
  color: #121316;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.task-item-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 6px;
  font-size: 11px;
}
.task-date {
  color: #94A3B8;
  font-size: 11px;
}
.task-delete-btn {
  border: none;
  background: transparent;
  color: #94A3B8;
  font-size: 12px;
  line-height: 1;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s, color 0.15s, background 0.15s;
}
.task-item:hover .task-delete-btn {
  opacity: 1;
}
.task-delete-btn:hover {
  color: #EF4444;
  background: rgba(239, 68, 68, 0.1);
}

.empty-task-placeholder {
  text-align: center;
  padding: 30px 10px;
  color: #94A3B8;
  font-size: 13px;
  font-weight: 700;
}

.current-task-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 14px;
  box-shadow: 3px 3px 0px #121316;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.current-task-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.task-label {
  font-size: 11.5px;
  font-weight: 800;
  color: #64748B;
  text-transform: uppercase;
}

.task-detail-topic {
  font-size: 13.5px;
  font-weight: 800;
  color: #121316;
}

.error-banner {
  background: #FEE2E2;
  border: 1px solid #FCA5A5;
  color: #B91C1C;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
}

.task-btns {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
.danger-btn-sm {
  background: #FEE2E2;
  color: #DC2626;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
}
.retry-btn-sm {
  background: #EDF4FF;
  color: #2563EB;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
}

/* 状态药丸 */
.status-pill {
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 800;
  border: 1px solid rgba(0, 0, 0, 0.1);
}
.status-queued, .status-running {
  background: #FEF3C7;
  color: #92400E;
}
.status-succeeded {
  background: #DCFCE7;
  color: #166534;
}
.status-failed {
  background: #FEE2E2;
  color: #991B1B;
}
.status-cancelled {
  background: #F1F5F9;
  color: #475569;
}

/* 中间：配置卡片 */
.ai-form-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 3px 3px 0px #121316;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.block-label {
  font-size: 13px;
  font-weight: 800;
  color: #121316;
  margin-bottom: 6px;
  display: block;
}

.ai-topic-input {
  width: 100%;
  min-height: 110px;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 12px 14px;
  font-family: inherit;
  font-size: 13.5px;
  box-shadow: 2px 2px 0px rgba(18, 19, 22, 0.05);
  resize: vertical;
}
.ai-topic-input:focus {
  outline: none;
  border-color: #2563EB;
}

.mid-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.mid-form label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12.5px;
  font-weight: 700;
  color: #334155;
}
.mid-form input, .mid-form select {
  border: 1.5px solid #CBD5E1;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 13px;
  font-family: inherit;
  background: #FAFBFC;
}
.mid-form input:focus, .mid-form select:focus {
  border-color: #2563EB;
  outline: none;
  background: #FFFFFF;
}

.generate-btn {
  width: 100%;
  padding: 12px !important;
  font-size: 14.5px !important;
  font-weight: 900 !important;
  background: #CCFF00 !important;
  color: #121316 !important;
  border: 2px solid #121316 !important;
  box-shadow: 3px 3px 0px #121316 !important;
}
.generate-btn:hover:not(:disabled) {
  background: #B8E600 !important;
  transform: translate(-1px, -1px);
  box-shadow: 4px 4px 0px #121316 !important;
}

/* 右侧：结果预览卡片 */
.load-article-btn {
  padding: 4px 10px !important;
  font-size: 12px !important;
  border-radius: 6px !important;
  background: #2563EB !important;
  color: #FFFFFF !important;
  box-shadow: 2px 2px 0px #121316 !important;
}

.ai-preview-card {
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 3px 3px 0px #121316;
  min-height: 520px;
  display: flex;
  flex-direction: column;
}

.ai-generating {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  flex: 1;
  padding: 60px 20px;
  text-align: center;
}
.spinner {
  width: 38px;
  height: 38px;
  border: 3.5px solid #E2E8F0;
  border-top-color: #2563EB;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
.generating-title {
  font-size: 15px;
  font-weight: 800;
  color: #121316;
  margin: 0;
}

.ai-failed {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: 40px 20px;
  text-align: center;
}
.failed-icon {
  font-size: 36px;
  margin-bottom: 8px;
}
.ai-failed h4 {
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 800;
}

.ai-result {
  display: flex;
  flex-direction: column;
  gap: 14px;
  flex: 1;
}
.result-header h4 {
  margin: 0;
  font-size: 17px;
  font-weight: 900;
  color: #121316;
  border-bottom: 1.5px dashed #E2E8F0;
  padding-bottom: 10px;
}

.ai-result-body {
  flex: 1;
  max-height: 480px;
  overflow-y: auto;
  border: 1.5px solid #E2E8F0;
  border-radius: 8px;
  padding: 16px;
  font-size: 13.5px;
  line-height: 1.7;
  color: #1E293B;
  background: #FAF8F5;
}
.ai-result-body img {
  max-width: 100%;
  border-radius: 6px;
}

.ai-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: 60px 20px;
  text-align: center;
}
.empty-icon {
  font-size: 40px;
  margin-bottom: 10px;
}
.ai-empty h4 {
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 800;
  color: #121316;
}
.ai-empty p {
  max-width: 320px;
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
}

/* 响应式断点适配 */
@media (max-width: 1200px) {
  .ai-studio-grid {
    grid-template-columns: 1fr;
  }
  .modal-meta-grid, .media-assets-grid {
    grid-template-columns: 1fr;
  }
}

/* ==================== 文章编辑弹窗（左右布局/SEO/上传） ==================== */
.article-editor-modal {
  width: min(1240px, 95vw);
  max-height: 92vh;
  display: flex;
  flex-direction: column;
}
.article-editor-modal .modal-drawer-body {
  overflow-y: auto;
  flex: 1;
}
.article-editor-columns {
  display: grid;
  grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr);
  gap: 20px;
  align-items: start;
}
@media (max-width: 960px) {
  .article-editor-columns {
    grid-template-columns: 1fr;
  }
}
.editor-col-right {
  display: flex;
  flex-direction: column;
  position: sticky;
  top: 0;
}
.body-editor-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.body-mode-switch {
  display: inline-flex;
  border: 2px solid #121316;
  border-radius: 10px;
  overflow: hidden;
  background: #fff;
}
.body-mode-switch button {
  border: none;
  background: transparent;
  padding: 5px 12px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  color: #121316;
  border-right: 2px solid #121316;
}
.body-mode-switch button:last-child {
  border-right: none;
}
.body-mode-switch button.active {
  background: #CCFF00;
}
.html-source-area {
  width: 100%;
  min-height: 320px;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 10px 12px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12.5px;
  line-height: 1.6;
  resize: vertical;
  box-sizing: border-box;
}
.body-render-preview {
  min-height: 320px;
  max-height: 480px;
  overflow-y: auto;
  border: 2px dashed #121316;
  border-radius: 10px;
  padding: 16px;
  background: #FAFAF6;
  font-size: 14.5px;
  line-height: 1.8;
  word-break: break-word;
}
.body-render-preview img {
  max-width: 100%;
}
.select-with-create {
  display: flex;
  gap: 6px;
  align-items: center;
}
.select-with-create select {
  flex: 1;
  min-width: 0;
}
.mini-create-btn {
  white-space: nowrap;
  border: 2px solid #121316;
  background: #fff;
  border-radius: 8px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
  box-shadow: 2px 2px 0 #121316;
  transition: transform 0.1s ease;
}
.mini-create-btn:hover {
  background: #CCFF00;
  transform: translate(-1px, -1px);
}
.seo-score-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 10px;
}
.seo-score-badge {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-width: 72px;
  padding: 8px;
  border: 2px solid #121316;
  border-radius: 12px;
  box-shadow: 3px 3px 0 #121316;
}
.seo-score-badge.score-good { background: #CCFF00; }
.seo-score-badge.score-mid { background: #FFE58F; }
.seo-score-badge.score-bad { background: #FFE4E1; }
.seo-score-num {
  font-size: 24px;
  font-weight: 900;
  line-height: 1;
}
.seo-score-label {
  font-size: 11px;
  font-weight: 800;
  margin-top: 4px;
}
.seo-score-tips {
  margin: 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.7;
  color: #5E6577;
  font-weight: 600;
}
.seo-auto-btn {
  width: 100%;
  margin-bottom: 12px;
}
.serp-preview {
  border: 1.5px solid #D8DCE6;
  border-radius: 10px;
  padding: 12px;
  margin: 10px 0;
  background: #fff;
}
.serp-title {
  color: #1a0dab;
  font-size: 16px;
  line-height: 1.35;
  font-weight: 600;
}
.serp-url {
  color: #006621;
  font-size: 12px;
  margin: 2px 0;
}
.serp-desc {
  color: #545454;
  font-size: 12.5px;
  line-height: 1.5;
}
.advanced-seo summary {
  cursor: pointer;
  font-size: 12.5px;
  font-weight: 800;
  color: #5E6577;
  margin: 6px 0;
}
.featured-switch-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 800;
  font-size: 13.5px;
  cursor: pointer;
}
.switch-btn {
  width: 46px;
  height: 26px;
  border: 2px solid #121316;
  border-radius: 999px;
  background: #E9ECF3;
  position: relative;
  cursor: pointer;
  transition: background 0.15s ease;
  padding: 0;
}
.switch-btn .switch-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #fff;
  border: 2px solid #121316;
  transition: left 0.15s ease;
}
.switch-btn.on {
  background: #CCFF00;
}
.switch-btn.on .switch-knob {
  left: 22px;
}
.z-overlay {
  z-index: 1000;
}
.inline-tax-dialog {
  background: #fff;
  border: 2.5px solid #121316;
  border-radius: 16px;
  box-shadow: 6px 6px 0 #121316;
  padding: 20px;
  width: min(400px, 90vw);
}
.inline-tax-dialog h3 {
  margin: 0 0 12px;
  font-size: 17px;
  font-weight: 900;
}
.inline-tax-dialog input {
  width: 100%;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 8px 10px;
  font-size: 14px;
  box-sizing: border-box;
}
.inline-tax-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;
}

/* ===== P1 验收修复新增样式 ===== */
/* 通用自绘确认框 */
.ui-confirm-dialog {
  width: min(460px, calc(100vw - 48px));
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 16px;
  box-shadow: 6px 6px 0px #121316;
  padding: 22px 24px;
  text-align: left;
}
.ui-confirm-dialog h3 {
  margin: 0 0 10px;
  font-size: 17px;
  font-weight: 800;
}
.ui-confirm-message {
  margin: 0 0 18px;
  font-size: 13.5px;
  line-height: 1.6;
}
.ui-confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
.danger-button {
  background: #E5484D;
  border-color: #121316;
  color: #fff;
}
/* 新闻编辑弹窗：头/脚固定，仅中间滚动（N6） */
.news-editor.news-editor-flex {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  max-height: 92vh;
}
.modal-fixed-header {
  position: sticky;
  top: 0;
  z-index: 2;
  background: #FFFFFF;
  padding-bottom: 10px;
  border-bottom: 2px solid #121316;
  margin-bottom: 12px;
}
.modal-scroll-body {
  flex: 1 1 auto;
  overflow-y: auto;
  padding-right: 4px;
  max-height: calc(92vh - 190px);
}
.modal-fixed-footer {
  position: sticky;
  bottom: 0;
  z-index: 2;
  background: #FFFFFF;
  padding-top: 12px;
  margin-top: 12px;
  border-top: 2px solid #121316;
}
/* 字段级校验提示（S5） */
.field-errors {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 4px 0 8px;
}
.field-errors .error-text {
  color: #E5484D;
  font-weight: 700;
}
.input-invalid {
  border-color: #E5484D !important;
  box-shadow: 0 0 0 2px rgba(229, 72, 77, 0.18);
}
/* 批量操作在未选中时视觉弱化但仍可点击（N10） */
.link.is-muted {
  opacity: 0.45;
  cursor: not-allowed;
}
/* 任务列表防溢出（A5）：固定列宽 + 省略号 + 错误摘要截断 */
.runs-table-wrap {
  overflow-x: auto;
}
.runs-table {
  table-layout: fixed;
  width: 100%;
  min-width: 1040px;
}
.runs-table th,
.runs-table td {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}
.runs-table th:last-child,
.runs-table td:last-child {
  position: sticky;
  right: 0;
  background: #FFFFFF;
  box-shadow: -6px 0 8px -6px rgba(18, 19, 22, 0.35);
  text-align: center;
}
.cell-ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

/* A5：错误摘要列限宽两行截断（全文见 title），避免独占整行把操作列挤出可视区 */
.cell-error-summary {
  max-width: 260px;
  min-width: 180px;
  white-space: normal;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  line-height: 1.45;
  word-break: break-word;
}
</style>

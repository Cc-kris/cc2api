<template>
  <AppLayout>
    <div class="admin-page">
    <div class="top-actions-bar">
      <div class="top-tabs-title">内容中心</div>
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
          /><Select
            v-model="newsStatus"
            :options="newsStatusOptions"
            class="toolbar-select"
            aria-label="新闻状态"
            data-test="news-status"
            @change="onNewsStatusSelect"
          /><input
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
                <th>语言</th>
                <th>状态</th>
                <th>译文</th>
                <th>发布时间</th>
                <th class="actions-th">操作</th>
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
                  <small v-if="displayNewsSummary(item)" class="summary-clamp">{{ displayNewsSummary(item) }}</small>
                  <small v-if="item.sourceName" class="muted">{{ item.sourceName }}</small>
                </td>
                <td><span class="locale-tag">{{ item.locale }}</span></td>
                <td>
                  <span class="status">{{ item.status }}</span>
                </td>
                <td class="trans-cell">
                  <template v-if="newsTranslationSummary(item.id).length">
                    <span
                      v-for="t in newsTranslationSummary(item.id)"
                      :key="t.id || t.targetLocale"
                      class="trans-chip-mini"
                      :class="'chip-' + translationStatusKey(t.status)"
                      :title="t.errorCode ? '错误：' + t.errorCode : ''"
                    >{{ localeShort(t.targetLocale) }}·{{ translationStatusText(t.status) }}</span>
                  </template>
                  <span v-else class="muted">未翻译</span>
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
                    <button
                      v-if="item.status !== 'published'"
                      type="button"
                      class="link news-publish-btn"
                      :disabled="!newsPublishable(item)"
                      :title="newsPublishable(item) ? '发布这条新闻' : '需先补全正文、摘要与来源名后再发布'"
                      :aria-label="newsPublishable(item) ? '发表' : '发表（需先补全正文、摘要与来源名）'"
                      @click="publishNews(item.id)"
                    >发表</button>
                    <button type="button" class="link danger" @click="deleteNews(item.id)">删除</button>
                  </td>
              </tr>
            </tbody>
          </table>
          <p v-if="listState.news.loading" class="list-state" role="status">正在加载新闻…</p>
          <p v-else-if="listState.news.error" class="list-state list-error" role="alert">{{ listState.news.error }} <button class="link" @click="loadNews()">重试</button></p>
          <p v-else-if="!news.length" class="empty" role="status">暂无新闻</p>
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
            <Pagination v-if="newsTotal > 0" :page="newsPage" :total="newsTotal" :page-size="20" :show-page-size-selector="false" @update:page="loadNews" />
          </div>
        </div>
        <div
          v-if="selectedNews.id"
          class="modal-backdrop z-overlay"
            @click.self="requestCloseNewsEditor"
        >
          <div ref="newsEditorRef" class="news-editor news-editor-modal" role="dialog" aria-modal="true" aria-labelledby="news-editor-title" @keydown.esc="requestCloseNewsEditor" @keydown.tab="trapNewsFocus" @input="newsFormTouched = true" @change="newsFormTouched = true" @click="newsFormTouched = true">
            <div class="section-heading modal-fixed-header">
              <div>
                <h2 id="news-editor-title">新闻详情编辑</h2>
                <p class="muted">编辑抓取正文、摘要、分类标签和 SEO 字段后再发布。</p>
              </div>
              <button type="button" class="dialog-close" aria-label="关闭新闻编辑" @click="requestCloseNewsEditor">×</button>
            </div>

            <!-- 译文任务：横向折叠条置于两栏之上，避免与正文编辑器在垂直方向重叠 -->
            <div class="news-translation-panel">
              <button
                type="button"
                class="news-trans-toggle"
                :aria-expanded="newsTranslationOpen ? 'true' : 'false'"
                @click="newsTranslationOpen = !newsTranslationOpen"
              >
                <span class="config-title">译文任务</span>
                <span class="news-trans-summary">
                  {{ newsTranslationTasks.length ? newsTranslationTasks.length + ' 个任务' : '暂无任务' }}
                </span>
                <span class="news-trans-toggle-text">{{ newsTranslationOpen ? '收起' : '展开' }}</span>
              </button>
              <div v-if="newsTranslationOpen" class="news-translation-tasks">
                <div
                  v-for="task in newsTranslationTasks"
                  :key="task.id"
                  class="news-translation-task"
                >
                  <span class="task-locale">{{ localeShort(task.targetLocale || task.locale) }}</span>
                  <span class="task-status" :class="'chip-' + translationStatusKey(task.status)">
                    {{ translationStatusText(task.status) }}
                  </span>
                  <small v-if="task.errorCode || task.error" class="task-error">
                    {{ task.errorCode || task.error }}
                  </small>
                </div>
                <small v-if="!newsTranslationTasks.length" class="muted">
                  暂无新闻译文任务；新闻的自动翻译由「自动化设置 → 目标语言」驱动。
                </small>
              </div>
            </div>

            <div class="modal-scroll-body news-editor-body">
              <div class="two-col-editor">
                <div class="editor-main-col">
                  <div class="config-block">
                    <div class="config-title">基础信息</div>
                    <div class="form-grid">
                      <label class="form-required-legend">必填项标有 *</label>
                      <label
                        >标题<input
                          v-model="selectedNews.title"
                          aria-label="标题"
                          maxlength="240"
                          :aria-invalid="newsTitleInvalid ? 'true' : undefined"
                          @blur="newsTitleTouched = true"
                      /></label>
                      <small v-if="newsTitleInvalid" class="field-error">标题不能为空</small>
                      <label class="field-line"
                        >摘要<textarea
                          v-model="selectedNews.summary"
                          class="news-summary-input"
                          rows="3"
                          placeholder="用简洁段落概括新闻核心内容"
                        ></textarea>
                      </label>
                      <label
                        >分类<input
                          class="combo-trigger"
                          readonly
                          role="combobox"
                          aria-haspopup="listbox"
                          aria-label="分类"
                          :aria-expanded="newsSelectOpen === 'category' ? 'true' : 'false'"
                          :value="newsCategoryLabelText"
                          @click="toggleNewsSelect('category')"
                          @keydown.enter.prevent="toggleNewsSelect('category')"
                        /><span
                          v-if="newsSelectOpen === 'category'"
                          class="combo-menu"
                          role="listbox"
                          aria-label="分类选项"
                          ><button
                            v-for="opt in newsCategorySelectOptions"
                            :key="'news-cat-' + opt.value"
                            type="button"
                            class="combo-option"
                            role="option"
                            :aria-selected="selectedNews.category === opt.value ? 'true' : 'false'"
                            @click.stop="selectedNews.category = opt.value; newsSelectOpen = ''"
                          >
                            {{ opt.label }}
                          </button></span
                        ></label
                      >
                      <label
                        >标签<span class="tag-input" data-test="tag-input"
                          ><span
                            v-for="(tag, tagIndex) in newsTagChips"
                            :key="'news-tag-' + tag"
                            class="tag-chip"
                            :data-tag="tag"
                            ><span
                              class="tag-remove"
                              role="button"
                              tabindex="0"
                              :title="'移除标签 ' + tag"
                              @click.stop="removeNewsTag(tagIndex)"
                              @keydown.enter.prevent.stop="removeNewsTag(tagIndex)"
                              @keydown.space.prevent.stop="removeNewsTag(tagIndex)"
                            ></span></span
                          ><input
                            v-model="newsTagDraft"
                            placeholder="输入标签后回车"
                            list="news-tag-suggestions"
                            @keydown.enter.prevent="commitNewsTag"
                            @keydown.backspace="handleNewsTagBackspace"
                        /></span></label>
                      <datalist id="news-tag-suggestions">
                        <option v-for="t in taxonomy.tags" :key="'news-tag-opt-' + t" :value="t"></option>
                      </datalist>
                    </div>
                  </div>
                  <div class="editor-label">新闻正文（富文本）</div>
                  <div class="editor-shell" data-test="news-rich-editor">
                    <AnnouncementRichTextEditor
                      v-model="selectedNews.bodyHtml"
                      :session-key="`${selectedNews.id}:${newsEditorSession}`"
                      placeholder="新闻正文..."
                    />
                  </div>
                </div>
                <div class="editor-config-col">
                  <div class="config-block">
                    <div class="config-title">发布与精选</div>
                    <div class="form-grid">
                      <label
                        >状态<input
                          class="combo-trigger"
                          readonly
                          role="combobox"
                          aria-haspopup="listbox"
                          aria-label="状态"
                          :aria-expanded="newsSelectOpen === 'status' ? 'true' : 'false'"
                          :value="newsStatusLabelText"
                          @click="toggleNewsSelect('status')"
                          @keydown.enter.prevent="toggleNewsSelect('status')"
                        /><span
                          v-if="newsSelectOpen === 'status'"
                          class="combo-menu"
                          role="listbox"
                          aria-label="状态选项"
                          ><button
                            v-for="opt in newsStatusSelectOptions"
                            :key="'news-status-' + opt.value"
                            type="button"
                            class="combo-option"
                            role="option"
                            :aria-selected="selectedNews.status === opt.value ? 'true' : 'false'"
                            @click.stop="selectedNews.status = opt.value; newsSelectOpen = ''"
                          >
                            {{ opt.label }}
                          </button></span
                        ></label
                      >
                      <label class="field-line">发布时间<input v-model="newsPublishedAtLocal" type="datetime-local" /></label>
                      <label class="field-line">语言<input v-model="selectedNews.locale" /></label>
                      <label class="field-line">Slug<input v-model="selectedNews.slug" maxlength="240" /></label>
                      <label class="field-line">来源名称<input v-model="selectedNews.sourceName" /></label>
                      <label class="field-line"
                        >来源链接<input v-model="selectedNews.originalUrl" placeholder="https://..."
                      /></label>
                      <div class="config-block config-block-inner">
                        <div class="config-title">同步语言</div>
                        <div class="locale-checkboxes" role="group" aria-label="同步语言">
                          <label
                            v-for="l in ADMIN_TARGET_LOCALES"
                            :key="'news-sync-' + l.value"
                            class="locale-checkbox"
                          >
                            <input type="checkbox" :value="l.value" v-model="newsSyncLocales" />
                            <span>{{ localeOptionLabel(l.value) }}</span>
                          </label>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div class="config-block">
                    <div class="config-title">封面</div>
                    <div class="form-grid">
                      <label class="field-line"
                        >封面图<input v-model="selectedNews.imageUrl" placeholder="https://... 建议 16:9"
                      /></label>
                    </div>
                  </div>
                  <div class="config-block">
                    <div class="config-title">
                      SEO
                      <button
                        type="button"
                        class="secondary-button seo-ai-button"
                        :disabled="newsSeoBusy || !selectedNews.id"
                        :title="selectedNews.id ? '用 AI 检查正文并生成 SEO 标题、描述与关键词' : '先保存新闻再生成'"
                        @click="generateNewsSEO"
                      >{{ newsSeoBusy ? 'AI 生成中…' : '✨ AI 生成 SEO' }}</button>
                    </div>
                    <div class="form-grid">
                      <label class="field-line">SEO 标题<input v-model="selectedNews.seoTitle" maxlength="160" /></label>
                      <label class="field-line"
                        >SEO 描述<textarea
                          v-model="selectedNews.seoDescription"
                          maxlength="300"
                          rows="3"
                        ></textarea>
                        <span class="field-hint" :class="{ 'hint-warn': newsSeoLength > 0 && (newsSeoLength < 120 || newsSeoLength > 160) }">
                          建议 120~160 字，当前 {{ newsSeoLength }} 字
                        </span>
                      </label>
                      <label v-if="selectedNews.seoKeywordsText" class="field-line"
                        >SEO 关键词<input v-model="selectedNews.seoKeywordsText" placeholder="AI 生成，逗号分隔" />
                      </label>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="form-actions modal-fixed-footer">
              <button type="button" class="secondary-button" @click="requestCloseNewsEditor">取消</button>
              <button type="button" class="button" aria-label="保存新闻" @click="saveNews">保存</button>
            </div>
          </div>
        </div>
        <!-- 未保存修改确认：先问再关，内容留在内存里，可随时继续编辑 -->
        <div
          v-if="newsCloseGuard.show"
          class="modal-backdrop z-overlay news-close-guard-backdrop"
          @click.self="resumeNewsEditing"
        >
          <div
            class="confirm-dialog news-close-guard-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="news-close-guard-title"
            @keydown.esc.stop="resumeNewsEditing"
            @keydown.tab="trapNewsGuardFocus"
          >
            <h3 id="news-close-guard-title">有未保存修改，确认离开？</h3>
            <p class="muted">离开后本次修改的标题、正文、分类标签与 SEO 字段将不会被保存。</p>
            <div class="confirm-dialog-actions">
              <button type="button" class="secondary-button" @click="confirmDiscardNews">确认离开</button>
              <button type="button" class="button primary" @click="resumeNewsEditing">继续编辑</button>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="newsView === 'sources'">
        <div class="toolbar source-toolbar">
          <button type="button" class="button source-create-button" @click="openCreateSource">+ 新增新闻源</button>
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
                  <template v-if="source.deletedAt">已删除</template>
                  <template v-else
                    >{{ source.enabled ? "启用" : "停用"
                    }}<small v-if="source.nextRunAt"
                      >下次：{{ formatDate(source.nextRunAt) }}</small
                    ><small v-if="source.lastError" class="error-text">{{
                      source.lastError
                    }}</small></template
                  >
                </td>
                <td>
                  <button
                    v-if="source.deletedAt"
                    class="link"
                    @click="restoreSource(source.id)"
                  >
                    恢复
                  </button>
                  <template v-else>
                    <button class="link" @click="editSource(source)">编辑</button>
                    <button class="link" @click="toggleSource(source)">
                      {{ source.enabled ? "停用" : "启用" }}</button
                    ><button class="link" @click="fetchSource(source.id)">
                      立即抓取</button
                    ><button class="danger link" @click="deleteSource(source.id)">
                      删除</button>
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="listState.sources.loading" class="list-state" role="status">正在加载新闻源…</p>
          <p v-else-if="listState.sources.error" class="list-state list-error" role="alert">{{ listState.sources.error }} <button class="link" @click="loadSources()">重试</button></p>
          <p v-else-if="!sources.length" class="empty" role="status">暂无新闻源</p>
          <div class="table-footer">
            <div></div>
            <Pagination v-if="sourceTotal > 0" :page="sourcePage" :total="sourceTotal" :page-size="20" :show-page-size-selector="false" @update:page="loadSources" />
          </div>
        </div>
        <div v-if="sourceDialogOpen" class="content-modal-layer" @mousedown.self="requestCloseSourceDialog">
          <div class="source-dialog" role="dialog" aria-modal="true" aria-labelledby="source-editor-title" @keydown.esc="requestCloseSourceDialog">
          <div class="section-heading">
            <h2 id="source-editor-title">{{ sourceDialogCreating ? "新增新闻源" : "编辑新闻源" }}</h2>
            <button type="button" class="dialog-close" aria-label="关闭新闻源弹窗" @click="requestCloseSourceDialog">×</button>
          </div>
          <div class="form-grid">
            <label>名称<span class="required-star" aria-hidden="true">*</span><input
                v-model="sourceDraft.name"
                :class="{ 'input-invalid': sourceFormErrors.name }"
                placeholder="来源名称（必填）"
                aria-required="true"
                @input="sourceFormErrors.name = ''"
                @blur="validateSourceField('name')" /><small v-if="sourceFormErrors.name" class="error-text" role="alert">来源名称必填</small></label
            ><label>链接 URL<span class="required-star" aria-hidden="true">*</span><input
                v-model="sourceDraft.url"
                :class="{ 'input-invalid': sourceFormErrors.url }"
                placeholder="公开 RSS/Atom/JSON URL（必填）"
                aria-required="true"
                @input="sourceFormErrors.url = ''"
                @blur="validateSourceField('url')" /><small v-if="sourceFormErrors.url" class="error-text" role="alert">{{ sourceFormErrors.url }}</small></label
            ><label>类型<span class="required-star" aria-hidden="true">*</span><Select
                v-model="sourceDraft.sourceType"
                :options="sourceTypeOptions"
                aria-label="新闻源类型"
                data-test="news-source-type" /></label
            ><label>默认分类<Select
                v-model="sourceDraft.category"
                :options="newsCategorySelectOptions"
                placeholder="不指定"
                aria-label="默认分类" /></label
            ><label>语言<input v-model="sourceDraft.language" placeholder="zh-CN" /></label
            ><label
              >抓取间隔（分钟）<input
                v-model.number="sourceDraft.intervalMinutes"
                class="interval-input"
                :class="{ 'input-invalid': sourceFormErrors.interval }"
                type="number"
                min="15"
                max="1440"
                :aria-invalid="sourceFormErrors.interval ? 'true' : undefined"
                :placeholder="'跟随全局（' + globalFetchIntervalMinutes() + ' 分钟）'"
                @input="sourceFormErrors.interval = ''"
                @blur="validateSourceField('interval')" /><small v-if="sourceFormErrors.interval" class="error-text" role="alert">{{ sourceFormErrors.interval }}</small></label
            ><label
              >每分钟速率<input
                v-model.number="sourceDraft.rateLimitPerMinute"
                type="number"
                min="1" /></label
            ><label
              >抓取规则<Select
                :model-value="null"
                :options="[]"
                disabled
                placeholder="暂无可用规则"
                aria-label="抓取规则" /><small class="field-hint">请先在抓取规则里创建</small></label
            ><label class="source-status-field"
              >状态<span class="switch slider-switch source-enabled-switch"
                ><span class="switch-track" aria-hidden="true"></span><input
                  v-model="sourceDraft.enabled"
                  class="switch-input"
                  type="checkbox"
                  role="switch"
                  aria-label="新闻源状态"
                /><span class="switch-text">{{ sourceDraft.enabled !== false ? "启用" : "停用" }}</span></span
              ></label
            ><label v-if="sourceDraft.sourceType === 'public_json'">JSON 字段映射<textarea v-model="sourceDraft.mappingJsonText" class="short-text" placeholder='{"title":"$.headline","body":"$.article.text","url":"$.article.href"}'></textarea></label>
          </div>
          <p v-if="errorMessage" class="error-text source-dialog-error" role="alert">{{ errorMessage }}</p>
          <div class="form-actions modal-fixed-footer"><button type="button" class="secondary-button" @click="requestCloseSourceDialog">取消</button><button type="button" class="button" @click="submitSourceDialog">保存新闻源</button></div>
          </div>
        </div>
        <!-- SRC-07：新闻源弹窗未保存确认（四种关闭方式共用） -->
        <div v-if="sourceCloseGuard.show" class="modal-backdrop z-overlay source-close-guard-backdrop" @click.self="resumeSourceEditing">
          <div class="confirm-dialog source-close-guard-dialog" role="dialog" aria-modal="true" aria-labelledby="source-close-guard-title" @keydown.esc.stop="resumeSourceEditing">
            <h3 id="source-close-guard-title">有未保存修改，确认离开？</h3>
            <p class="muted">离开后本次填写的名称、URL、间隔等设置将不会被保存。</p>
            <div class="confirm-dialog-actions">
              <button type="button" class="secondary-button" @click="confirmDiscardSource">确认离开</button>
              <button type="button" class="button primary" @click="resumeSourceEditing">继续编辑</button>
            </div>
          </div>
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
          <p v-if="listState.runs.loading" class="list-state" role="status">正在加载抓取任务…</p>
          <p v-else-if="listState.runs.error" class="list-state list-error" role="alert">{{ listState.runs.error }} <button class="link" @click="loadRuns()">重试</button></p>
          <p v-else-if="!runs.length" class="empty" role="status">暂无抓取任务</p>
          <div class="table-footer">
            <div></div>
            <Pagination v-if="runTotal > 0" :page="runPage" :total="runTotal" :page-size="20" :show-page-size-selector="false" @update:page="loadRuns" />
          </div>
        </div>
      </div>
      <div v-else class="settings">
        <label class="switch slider-switch"
          ><span class="switch-track" aria-hidden="true"></span><input
            v-model="automation.enabled"
            class="switch-input"
            type="checkbox"
            role="switch"
            :aria-checked="automation.enabled ? 'true' : 'false'"
          /><span class="switch-text">启用定时抓取与自动发布</span></label
        >
        <div class="form-grid">
          <label>时区<input v-model="automation.timezone" /></label
          ><label>Cron<input v-model="automation.fetchCron" /></label
          ><label
            >发布模式<Select
              v-model="automation.publishMode"
              :options="publishModeOptions"
              aria-label="发布模式"
              data-test="automation-publish-mode"
            /></label
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
          ><label class="target-locales-label"
            >目标语言（中文为源语言）
            <div class="locale-checkbox-group">
              <label
                v-for="loc in ADMIN_TARGET_LOCALES"
                :key="loc.value"
                class="locale-checkbox"
              >
                <input
                  v-model="automationTargetLocales"
                  type="checkbox"
                  :value="loc.value"
                />
                <span>{{ localeOptionLabel(loc.value) }}</span>
              </label>
            </div>
            <small class="muted">新闻抓取后自动按勾选语言生成译文任务；未勾选则不翻译。</small></label
          >
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
          <Select
            v-model="articleStatus"
            :options="articleStatusOptions"
            class="toolbar-select"
            aria-label="文章状态"
            data-test="article-status"
            @change="onArticleFilterSelect"
          />
          <Select
            v-model="articleModel"
            :options="articleModelSelectOptions"
            class="toolbar-select"
            aria-label="文章模型"
            data-test="article-model"
            @change="onArticleFilterSelect"
          />
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
                  <th>译文</th>
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
                  <td class="trans-cell">
                    <template v-if="articleTranslationSummary(item.id).length">
                      <span
                        v-for="t in articleTranslationSummary(item.id)"
                        :key="t.id || t.locale"
                        class="trans-chip-mini"
                        :class="'chip-' + translationStatusKey(t.status)"
                        :title="t.errorCode ? '错误：' + t.errorCode : ''"
                      >{{ localeShort(t.locale) }}·{{ translationStatusText(t.status) }}</span>
                    </template>
                    <span v-else class="muted">未翻译</span>
                  </td>
                  <td class="featured-cell" @click.stop>
                    <button
                      type="button"
                      class="switch featured-toggle"
                      v-featured-checked
                      role="switch"
                      :class="{ on: isArticleRowFeatured(item) }"
                      :aria-checked="isArticleRowFeatured(item) ? 'true' : 'false'"
                      :aria-label="isArticleRowFeatured(item) ? '取消精选' : '设为精选'"
                      :disabled="articleFeatureBusyId === item.id || (item.status !== 'published' && !isArticleRowFeatured(item))"
                      :title="item.status !== 'published' && !isArticleRowFeatured(item) ? '仅已发布文章可设为精选' : '设为精选 / 取消精选（仅 1 篇）'"
                      @click="toggleArticleFeatured(item, !isArticleRowFeatured(item))"
                    >
                      <span class="switch-knob"></span>
                    </button>
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
            <p v-if="listState.articles.loading" class="list-state" role="status">正在加载文章…</p>
          <p v-else-if="listState.articles.error" class="list-state list-error" role="alert">{{ listState.articles.error }} <button class="link" @click="loadArticles()">重试</button></p>
          <p v-else-if="!articles.length" class="empty" role="status">暂无文章</p>
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
              <Pagination v-if="articleTotal > 0" :page="articlePage" :total="articleTotal" :page-size="20" :show-page-size-selector="false" @update:page="loadArticles" />
            </div>
          </div>

      <!-- 独立文章编辑弹窗（新建/编辑统一，居中，左右布局） -->
      <div v-if="showArticleModal" class="modal-backdrop" @click.self="requestCloseArticle">
        <div
          class="modal-drawer article-editor-modal"
          role="dialog"
          aria-modal="true"
          aria-label="文章编辑"
          @keydown.esc.stop.prevent="requestCloseArticle"
          @keydown.tab="trapArticleFocus"
        >
          <div class="modal-drawer-header">
            <div class="header-title-wrap">
              <span class="header-badge">{{ articleForm.id ? '编辑模式' : '新建草稿' }}</span>
              <h3>{{ articleForm.id ? ("编辑文章 #" + articleForm.id) : "撰写新文章" }}</h3>
            </div>
            <button type="button" class="close-btn" title="关闭" @click="requestCloseArticle">✕</button>
          </div>
          <div class="modal-drawer-body">
            <div class="article-editor-columns">
              <!-- 左列：配置字段（标题 / 摘要 / 分类标签 / SEO / 推荐 / 封面 / 译文） -->
              <div
                class="editor-col-left"
                @keydown.capture="onEditorLeftKeydown"
                @focusin="onEditorLeftFocusIn"
              >
                <div class="meta-card">
                  <div class="meta-card-header"><h4>基础信息</h4></div>
                  <div class="meta-card-body">
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
                  </div>
                </div>
                <div class="meta-card">
                  <div class="meta-card-header">
                    <span class="meta-card-icon">⚙️</span>
                    <h4>分类与标签</h4>
                  </div>
                  <div class="meta-card-body">
                    <div class="form-row">
                      <label>分类
                        <div class="select-with-create">
                          <Select
                            v-model="articleForm.category"
                            :options="articleCategorySelectOptions"
                            class="w-full"
                            aria-label="文章分类"
                            data-test="article-category"
                          />
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
                      <label>标签
                        <TagInput
                          v-model="articleTagsInput"
                          :suggestions="taxonomy.tags"
                          :max="ARTICLE_TAG_LIMIT"
                          :tag-max-width="ARTICLE_TAG_MAX_WIDTH"
                          :limit-hint="`单篇最多 ${ARTICLE_TAG_LIMIT} 个标签，单个标签最长 ${ARTICLE_TAG_MAX_WIDTH} 字（中文按 1 字、英文按半角计）`"
                          placeholder="输入或选择标签"
                        />
                      </label>
                      <!-- 无独立「新建标签」按钮：输入即建词，保存时自动补写进标签库 -->
                    </div>
                  </div>
                </div>

                <div class="meta-card">
                  <div class="meta-card-header"><h4>正文</h4></div>
                  <div class="meta-card-body">
                <div class="field-group body-field-group">
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
                  <div class="article-editor-shell">
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
                  </div>
                </div>
              </div>

              <!-- 右列：精选 / 封面 / SEO / 翻译 -->
              <div class="editor-col-right">
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
                      <!-- 键盘可达：可见按钮承载焦点与点击，file input 只做隐藏的取值通道
                           （原先 label>input.sr-only 让 Tab 落在屏幕外 1747px 的绝对定位元素上） -->
                      <button type="button" class="upload-file-btn" @click="coverFileRef?.click()">本地上传</button>
                      <input
                        ref="coverFileRef"
                        type="file"
                        accept="image/*"
                        class="sr-only"
                        tabindex="-1"
                        aria-hidden="true"
                        @change="handleImageUpload($event, 'coverImage')"
                      />
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
                        <button type="button" class="upload-file-btn" @click="ogFileRef?.click()">本地上传</button>
                        <input
                          ref="ogFileRef"
                          type="file"
                          accept="image/*"
                          class="sr-only"
                          tabindex="-1"
                          aria-hidden="true"
                          @change="handleImageUpload($event, 'ogImage')"
                        />
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
                      :aria-disabled="seoGenerating || !articleBodyPlainText ? 'true' : 'false'"
                      :title="articleBodyPlainText ? '根据正文生成 SEO 描述与关键词' : '请先填写正文内容'"
                      @click="generateSeoMeta"
                    >{{ seoGenerating ? "生成中..." : "✨ SEO 自动化：生成描述 / 关键词" }}</button>
                    <!-- 覆盖确认是「弹窗层」的问答，挂到 body 上：既避免与编辑器底部的「取消」重名，
                         也让读屏软件把它当作独立 alertdialog 而不是左栏卡片里的普通内容 -->
                    <Teleport to="body">
                      <div
                        v-if="seoOverwriteConfirm"
                        class="action-confirm seo-overwrite-confirm seo-overwrite-overlay"
                        role="alertdialog"
                        aria-modal="true"
                        aria-label="SEO 覆盖确认"
                        @keydown.esc="cancelSeoOverwrite"
                      >
                        <p>已有内容（SEO 描述 / 关键词），是否覆盖？覆盖后将用本次生成结果替换。</p>
                        <div class="action-confirm-actions">
                          <button ref="seoOverwriteOkRef" type="button" class="button primary" @click="confirmSeoOverwrite">覆盖</button>
                          <button type="button" class="secondary-button" @click="cancelSeoOverwrite">取消</button>
                        </div>
                      </div>
                    </Teleport>
                    <div class="form-row">
                      <label>SEO 标题
                        <input v-model="articleForm.seoTitle" maxlength="160" placeholder="默认继承文章标题" />
                      </label>
                    </div>
                    <div class="form-row">
                      <label>SEO 描述 (Meta Description)
                        <textarea v-model="articleForm.seoDescription" rows="2" maxlength="300" placeholder="留空默认使用文章摘要..."></textarea>
                        <span class="field-hint" :class="{ 'hint-warn': seoDescLength > 0 && (seoDescLength < 120 || seoDescLength > 160) }">
                          建议 120~160 字，当前 {{ seoDescLength }} 字
                        </span>
                      </label>
                    </div>
                    <div class="form-row">
                      <label>SEO 关键词
                        <TagInput v-model="articleSeoKeywordsInput" placeholder="自动生成或手动添加" />
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

                <div v-if="articleForm.id" class="translations-card mt-4">
                  <div class="trans-header">
                    <div>
                      <h4>多语言译文管理</h4>
                      <small>为海外用户构建多语言版本，支持一键 AI 全自动互译</small>
                    </div>
                    <div class="translation-actions">
                      <button type="button" class="secondary-button" @click="selectedArticleLocales = ADMIN_TARGET_LOCALES.map((l) => l.value)">
                        全选语言
                      </button>
                      <button type="button" class="secondary-button" @click="selectedArticleLocales = []">
                        清空
                      </button>
                      <button
                        v-if="translationForm.id"
                        type="button"
                        class="secondary-button"
                        @click="saveTranslation"
                      >
                        保存当前译文
                      </button>
                      <button
                        type="button"
                        class="button primary sparkle-btn"
                        :disabled="translationBusy || !selectedArticleLocales.length"
                        @click="generateSelectedTranslations"
                      >
                        ✨ 生成所选语言译文（{{ selectedArticleLocales.length }}）
                      </button>
                    </div>
                  </div>
                  <div class="locale-checkboxes" role="group" aria-label="目标语言">
                    <label v-for="l in ADMIN_TARGET_LOCALES" :key="l.value" class="locale-checkbox">
                      <input
                        v-model="selectedArticleLocales"
                        type="checkbox"
                        :value="l.value"
                        :data-locale="l.value"
                      />
                      <span>{{ localeOptionLabel(l.value) }}</span>
                      <small>{{ l.value }}</small>
                    </label>
                  </div>
                  <div class="translation-rows">
                    <div
                      v-for="l in ADMIN_TARGET_LOCALES"
                      :key="'trans-row-' + l.value"
                      class="translation-row"
                      :data-locale="l.value"
                    >
                      <span class="trans-locale">{{ localeOptionLabel(l.value) }} <small>{{ l.value }}</small></span>
                      <span
                        class="status-badge"
                        :class="'badge-' + translationStatusKey(translationFor(l.value)?.status)"
                      >{{ translationStatusText(translationFor(l.value)?.status) }}</span>
                      <span v-if="translationFor(l.value)?.errorCode" class="error-text">
                        {{ translationFor(l.value)?.errorCode }}
                      </span>
                      <template v-if="translationFor(l.value)">
                        <button
                          v-if="translationFor(l.value).status === 'published'"
                          type="button"
                          class="link-action text-amber-600"
                          @click="unpublishTranslation(translationFor(l.value))"
                        >下架</button>
                        <button
                          v-else
                          type="button"
                          class="link-action text-blue-600"
                          @click="publishTranslation(translationFor(l.value))"
                        >发布</button>
                        <button
                          v-if="translationFor(l.value).status !== 'published'"
                          type="button"
                          class="link-action danger"
                          :disabled="translationBusy"
                          @click="cancelTranslation(l.value)"
                        >取消</button>
                        <button
                          type="button"
                          class="link-action"
                          @click="editTranslation(translationFor(l.value))"
                        >编辑</button>
                      </template>
                      <button
                        v-else
                        type="button"
                        class="link-action"
                        :disabled="translationBusy"
                        @click="generateTranslationFor(l.value)"
                      >{{ translationBusy ? "处理中…" : "重试/翻译" }}</button>
                    </div>
                  </div>
                  <div v-if="otherTranslations.length" class="translations-chips">
                    <div v-for="t in otherTranslations" :key="t.id" class="trans-chip">
                      <span class="trans-chip-title" @click="editTranslation(t)">🌐 {{ t.locale }} - {{ t.title || '无标题' }}</span>
                      <span class="status-badge" :class="'badge-' + translationStatusKey(t.status)">{{ translationStatusText(t.status) }}</span>
                      <button v-if="t.status !== 'published'" type="button" class="link-action text-blue-600" @click="publishTranslation(t)">发布</button>
                      <button v-else type="button" class="link-action text-amber-600" @click="unpublishTranslation(t)">下架</button>
                    </div>
                  </div>
                  <small class="translation-hint muted">
                    中文为源语言；勾选语言后生成会逐语言创建真实翻译任务，状态实时来自译文记录。
                  </small>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-drawer-footer">
            <button class="secondary-button" @click="requestCloseArticle">取消</button>
            <button v-if="articleForm.id" class="secondary-button" type="button" @click="previewArticle">新窗口预览</button>
            <button class="button primary" :disabled="articleSaving" @click="saveArticleAndClose">
              {{ articleSaving ? "保存中..." : "保存" }}
            </button>
          </div>
        </div>
      </div>

      <!-- 未保存修改确认：关闭动作先暂停（编辑器隐藏、内容留在内存），由用户决定继续编辑或丢弃 -->
      <div v-if="articleCloseGuard.show" class="modal-backdrop z-overlay" @click.self="resumeArticleEditing">
        <div class="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="article-close-guard-title">
          <h3 id="article-close-guard-title">有未保存修改，确认离开？</h3>
          <p class="muted">离开后本次填写的标题、正文、分类标签与 SEO 内容将不会被保存。</p>
          <div class="confirm-dialog-actions">
            <button type="button" class="secondary-button" @click="confirmDiscardArticle">确认离开</button>
            <button type="button" class="button primary" @click="resumeArticleEditing">继续编辑</button>
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
                <!-- 单个名称：用普通输入框。TagInput 是多值组件（字符串模式 emit "a,b"），
                     之前取 $event[0] 只拿到首字符，且 blur 100ms 延迟提交导致点「保存」时 name 仍为空。 -->
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

      

      <nav class="ai-subtabs" aria-label="AI 创作页面">
        <button :class="{ active: aiStudioView === 'article' }" @click="aiStudioView = 'article'">文章生成</button>
        <button :class="{ active: aiStudioView === 'config' }" @click="aiStudioView = 'config'">模型配置</button>
      </nav>
      <div v-show="aiStudioView === 'config'">
        <AIConfigPanel @configs-updated="onAIConfigsUpdated" />
      </div>
      <div v-if="aiStudioView === 'article' && !aiConfigs.some((c: any) => c.isDefault)" class="ai-config-required" role="status">
        <h3>请先配置模型</h3><p>设置默认模型后即可开始文章生成。</p>
        <button class="button primary" @click="aiStudioView = 'config'">前往模型配置</button>
      </div>
      <div v-else-if="aiStudioView === 'article' && aiConfigs.some((c: any) => c.isDefault)" class="ai-studio-grid">
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
              <Select
                :model-value="selectedAIModelKey"
                :options="aiModelOptions"
                value-key="value"
                label-key="label"
                aria-label="生成模型"
                placeholder="请选择模型"
                @update:model-value="selectAIModel"
              />
              <small v-if="modelsLoadError" class="error-text">{{ modelsLoadError }}</small>
              <small v-else-if="!aiModelOptions.length" class="muted">模型列表为空，请检查 /admin/content/articles/models 接口</small>
            </div>
            <div class="form-row-grid">
              <div class="field-group">
                <label class="block-label">目标语言</label>
                <Select
                  v-model="brief.language"
                  :options="briefLanguageOptions"
                  class="ai-select"
                  aria-label="目标语言"
                  data-test="brief-language"
                />
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
        <label>
          {{ t('contentV04.analytics.dateRange') }}
          <Select v-model="biRange" :options="biRangeOptions" data-test="bi-range" />
        </label>
        <label v-if="biRange === 'custom'">
          {{ t('contentV04.analytics.customRange') }}
          <input v-model="biFilters.from" type="date" :max="biFilters.to" data-test="bi-from" @change="loadBI" />
        </label>
        <label v-if="biRange === 'custom'">
          <span>&nbsp;</span>
          <input v-model="biFilters.to" type="date" :min="biFilters.from" data-test="bi-to" @change="loadBI" />
        </label>
        <label>
          {{ t('contentV04.analytics.locale') }}
          <Select v-model="biFilters.locale" :options="biLocaleOptions" data-test="bi-locale" />
        </label>
        <label>
          {{ t('contentV04.analytics.location') }}
          <Select v-model="biFilters.placement" :options="biPlacementOptions" data-test="bi-placement" />
        </label>
        <button class="secondary-button" data-test="bi-export" @click="exportBI">
          {{ t('contentV04.analytics.export') }}
        </button>
      </div>

      <p v-if="biError" class="bi-error" role="alert">
        <span>{{ t('contentV04.analytics.failed') }}</span>
        <button class="secondary-button" @click="loadBI">{{ t('contentV04.analytics.retry') }}</button>
      </p>

      <div class="metrics">
        <div class="metric-card m-blue">
          <span class="metric-icon">👥</span>
          <div class="metric-info">
            <small>{{ t('contentV04.analytics.today') }} · {{ t('contentV04.analytics.visitors') }}</small>
            <strong>{{ todayStats.visitors }}</strong>
          </div>
        </div>
        <div class="metric-card m-green">
          <span class="metric-icon">📄</span>
          <div class="metric-info">
            <small>{{ t('contentV04.analytics.today') }} · {{ t('contentV04.analytics.views') }}</small>
            <strong>{{ todayStats.views }}</strong>
          </div>
        </div>
        <div class="metric-card m-purple">
          <span class="metric-icon">📖</span>
          <div class="metric-info">
            <small>{{ t('contentV04.analytics.today') }} · {{ t('contentV04.analytics.reads') }}</small>
            <strong>{{ todayStats.reads }}</strong>
          </div>
        </div>
        <div class="metric-card m-amber">
          <span class="metric-icon">🎯</span>
          <div class="metric-info">
            <small>{{ t('contentV04.analytics.ctr') }}</small>
            <strong>{{ (todayStats.ctr * 100).toFixed(2) }}%</strong>
          </div>
        </div>
      </div>

      <div class="chart-grid">
        <div class="chart-card chart-wide">
          <h3>{{ t('contentV04.analytics.trend') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="trends.length" kind="line" :data="trendChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.articleTop') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="articleTopRows.length" kind="hbar" :data="articleTopChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.newsTop') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="newsTopRows.length" kind="hbar" :data="newsTopChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.placementCtr') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="ctrRows.length" kind="bar" :data="ctrChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.category') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="distributions.category.length" kind="doughnut" :data="categoryChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.language') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="distributions.locale.length" kind="doughnut" :data="localeChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.source') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="distributions.source.length" kind="doughnut" :data="sourceChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.referrer') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="distributions.referrer.length" kind="doughnut" :data="referrerChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card">
          <h3>{{ t('contentV04.analytics.duration') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="trends.length" kind="line" :data="readSecondsChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
          </div>
        </div>
        <div class="chart-card chart-wide">
          <h3>{{ t('contentV04.analytics.publishing') }}</h3>
          <div class="chart-box">
            <SvgChart v-if="trends.length" kind="bar" :data="publishingChartData" :height="260" />
            <p v-else class="chart-empty">{{ t('contentV04.analytics.noData') }}</p>
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
import { extractApiErrorMessage } from "@/utils/apiError";
import AppLayout from '@/components/layout/AppLayout.vue'
import AnnouncementRichTextEditor from '@/components/admin/announcements/AnnouncementRichTextEditor.vue'
import { announcementEditorInitialHtml, normalizeAnnouncementEditorHtml } from '@/utils/announcementContent'
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import contentAPI from "@/api/admin/content";
import AIConfigPanel from "@/components/admin/content/AIConfigPanel.vue";
import SvgChart from "@/components/admin/content/SvgChart.vue";
import Pagination from "@/components/common/Pagination.vue";
import Select from "@/components/common/Select.vue";
import TagInput from "@/components/common/TagInput.vue";
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
const aiStudioView = ref("article");
const aiConfigs = ref<any[]>([]);
function onAIConfigsUpdated(value: any[]) {
  aiConfigs.value = value || [];
  models.value = aiConfigs.value.flatMap((c: any) => (c.models || []).map((id: string) => ({ id, name: id })));
  const def = aiConfigs.value.find((c: any) => c.isDefault);
  if (def) {
    brief.value.model = def.defaultModel || def.models?.[0] || "";
    brief.value.providerKeyRef = def.id;
    selectedAIModelKey.value = `${def.id}::${brief.value.model}`;
  }
}
const current = ref("新闻动态");
const newsView = ref("items");
const news = ref<any[]>([]);
const newsPage = ref(1);
const newsTotal = ref(0);

const selectedNews = ref<any>({});
const newsEditorSession = ref(0);
const newsSeoBusy = ref(false);
const newsSeoLength = computed(() => String(selectedNews.value?.seoDescription || "").trim().length);

/** 让 AI 先检查正文、再写 SEO 标题/描述/关键词；结果直接回填编辑表单，保存时随表单一起落库。 */
async function generateNewsSEO() {
  const id = Number(selectedNews.value?.id || 0);
  if (!id || newsSeoBusy.value) return;
  newsSeoBusy.value = true;
  try {
    const res: any = await contentAPI.autoFillNewsSEO(id);
    const data = res?.data ?? res ?? {};
    if (data.seoTitle) selectedNews.value.seoTitle = String(data.seoTitle);
    if (data.seoDescription) selectedNews.value.seoDescription = String(data.seoDescription);
    if (Array.isArray(data.keywords) && data.keywords.length) {
      selectedNews.value.seoKeywordsText = data.keywords.join(", ");
    }
    toast("success", "已生成 SEO 标题 / 描述 / 关键词，确认后保存");
  } catch (err: any) {
    toast("error", extractApiErrorMessage(err, "AI 生成 SEO 失败，请检查 AI 配置"));
  } finally {
    newsSeoBusy.value = false;
  }
}
let newsEditRequest = 0;
const newsEditorRef = ref<HTMLElement | null>(null);

// 左栏下拉（分类 / 状态）：用 readonly input 作触发器，label 文本保持纯字段名
const newsSelectOpen = ref<"" | "category" | "status">("");
function toggleNewsSelect(scope: "category" | "status") {
  newsSelectOpen.value = newsSelectOpen.value === scope ? "" : scope;
}
const newsCategoryLabelText = computed(() => selectedNews.value?.category || "未分类");
const newsStatusLabelText = computed(
  () => newsStatusSelectOptions.find((o) => o.value === selectedNews.value?.status)?.label || "草稿",
);
// 词条式标签：chip 文本由 CSS attr(data-tag) 渲染，保证 label 文本不被污染
const newsTagDraft = ref("");
const newsTagChips = computed<string[]>(() => {
  const tags = selectedNews.value?.tags;
  if (Array.isArray(tags)) return tags.map((t) => String(t));
  return tags ? parseTagsText(String(tags)) : [];
});
function commitNewsTag() {
  if (!selectedNews.value) return;
  const value = newsTagDraft.value.trim();
  if (!value) return;
  const list = [...newsTagChips.value];
  if (!list.some((t) => t.toLowerCase() === value.toLowerCase())) list.push(value);
  selectedNews.value.tags = list;
  newsTagDraft.value = "";
}
function removeNewsTag(index: number) {
  if (!selectedNews.value) return;
  const list = [...newsTagChips.value];
  list.splice(index, 1);
  selectedNews.value.tags = list;
}
function handleNewsTagBackspace() {
  if (newsTagDraft.value || !newsTagChips.value.length) return;
  removeNewsTag(newsTagChips.value.length - 1);
}

// 新闻编辑弹窗字段契约：译文条折叠态 / 状态 Select / 同步语言多选 / 发布时间
const newsTranslationOpen = ref(false);
const newsTitleTouched = ref(false);
const newsStatusSelectOptions = [
  { value: "draft", label: "草稿" },
  { value: "published", label: "发布" },
];
const newsCategorySelectOptions = computed(() =>
  (taxonomy.value.newsCategories || []).map((c) => ({ value: c, label: c })),
);
const LOCALE_ZH_LABEL: Record<string, string> = {
  "en-US": "英语",
  "ru-RU": "俄语",
  "fr-FR": "法语",
  "de-DE": "德语",
};

const LOCALE_NATIVE_LABEL: Record<string, string> = {
  "en-US": "English",
  "ru-RU": "Русский",
  "fr-FR": "Français",
  "de-DE": "Deutsch",
};
function localeZhLabel(value: string): string {
  return LOCALE_ZH_LABEL[value] || value;
}
/**
 * 语言项文案：中文名 + 母语名（两者相同时只留一个）。
 * 后台文案为中文，PRD 1.6 要求不直接把语言代码/原始名丢给用户 → 「俄语 Русский」。
 */
function localeOptionLabel(value: string): string {
  const native = LOCALE_NATIVE_LABEL[value] || value;
  const zh = localeZhLabel(value);
  return zh === native ? native : `${zh} ${native}`;
}
const newsSyncLocales = computed<string[]>({
  get() {
    const raw = selectedNews.value?.syncLocales;
    if (Array.isArray(raw)) return raw.filter((v: any) => typeof v === "string" && v);
    if (typeof raw === "string" && raw.trim()) {
      return raw
        .split(/[,;，]/)
        .map((s) => s.trim())
        .filter(Boolean);
    }
    return ADMIN_TARGET_LOCALES.map((l) => l.value);
  },
  set(next: string[]) {
    selectedNews.value = { ...selectedNews.value, syncLocales: [...next] };
  },
});
function toLocalDateTimeInput(value: any): string {
  if (!value) return "";
  const raw = String(value);
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw.slice(0, 16);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
const newsPublishedAtLocal = computed<string>({
  get() {
    return toLocalDateTimeInput(selectedNews.value?.publishedAt);
  },
  set(next: string) {
    const iso = next ? new Date(next).toISOString() : null;
    selectedNews.value = { ...selectedNews.value, publishedAt: iso };
  },
});
const newsTitleInvalid = computed(() => {
  if (!newsTitleTouched.value) return false;
  return String(selectedNews.value?.title || "").trim().length === 0;
});
const selectedNewsIds = ref<number[]>([]);
const allNewsSelected = ref(false);
const sources = ref<any[]>([]);
const sourcePage = ref(1);
const sourceTotal = ref(0);

const selectedSource = ref<any>({});
const runs = ref<any[]>([]);
const runPage = ref(1);
const runTotal = ref(0);

const articles = ref<any[]>([]);
const articlePage = ref(1);
const articleTotal = ref(0);
type ContentListKey = "news" | "sources" | "runs" | "articles";
const listState = ref<Record<ContentListKey, { loading: boolean; error: string }>>({
  news: { loading: false, error: "" }, sources: { loading: false, error: "" },
  runs: { loading: false, error: "" }, articles: { loading: false, error: "" },
});

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

// ---- BI：筛选与图表 ----
const { t } = useI18n({ useScope: "global" });
const biRange = ref<"7" | "30" | "90" | "custom">("30");
const biLoading = ref(false);
const biError = ref("");
const distributions = ref<{ category: any[]; locale: any[]; source: any[]; referrer: any[] }>({
  category: [],
  locale: [],
  source: [],
  referrer: [],
});

const isoDate = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
const isoDaysAgo = (days: number) => {
  const d = new Date();
  d.setDate(d.getDate() - days);
  return isoDate(d);
};

const biRangeOptions = computed(() => [
  { value: "7", label: t("contentV04.analytics.last7Days") },
  { value: "30", label: t("contentV04.analytics.last30Days") },
  { value: "90", label: t("contentV04.analytics.last90Days") },
  { value: "custom", label: t("contentV04.analytics.customRange") },
]);

const biLocaleOptions = computed(() => [
  { value: "", label: t("contentV04.analytics.all") },
  { value: "zh", label: t("contentV04.language.chinese") },
  { value: "en", label: t("contentV04.language.english") },
  { value: "ru", label: t("contentV04.language.russian") },
  { value: "fr", label: t("contentV04.language.french") },
  { value: "de", label: t("contentV04.language.german") },
]);

const PLACEMENT_KEYS: Record<string, string> = {
  home_hero: "homeHero",
  home_list: "homeList",
  article_detail_related: "articleDetailRelated",
  news_list: "newsList",
  articles_list: "articlesList",
};
const biPlacementOptions = computed(() => [
  { value: "", label: t("contentV04.analytics.all") },
  ...Object.keys(PLACEMENT_KEYS).map((code) => ({
    value: code,
    label: t(`contentV04.analytics.placements.${PLACEMENT_KEYS[code]}`),
  })),
]);

// 代码值统一走翻译键，禁止把接口原始代码直接给用户看（PRD 1.6）
const placementLabel = (code: string) =>
  PLACEMENT_KEYS[code] ? t(`contentV04.analytics.placements.${PLACEMENT_KEYS[code]}`) : code || "—";

// HN 等源站抓来的原文里带着 "Article URL: ... Comments URL: ... Points: ..." 这类元数据，
// 直接展示会让列表看起来像没处理过的原始数据，这里在显示层裁掉。
function cleanNewsSummary(raw?: string | null): string {
  const text = String(raw || "").replace(/\s+/g, " ").trim();
  if (!text) return "";
  const cut = text.search(
    /(?:Article|Comments|Discussion|Original)\s+URL\s*:|Points\s*:|#\s*Comments\s*:|Submitted\s+by\s*:/i,
  );
  if (cut === 0) return "";
  const kept = cut > 0 ? text.slice(0, cut) : text;
  return kept.replace(/[\s·\-–—:;,、，。]+$/, "").trim();
}

function displayNewsSummary(item: { summary?: string | null; title?: string | null } | null | undefined): string {
  const text = cleanNewsSummary(item?.summary);
  if (!text) return "";
  const title = String(item?.title || "").replace(/\s+/g, " ").trim();
  if (title && text.toLowerCase() === title.toLowerCase()) return "";
  return text;
}

const LOCALE_KEYS: Record<string, string> = {
  zh: "chinese",
  en: "english",
  ru: "russian",
  fr: "french",
  de: "german",
};
const localeLabel = (code: string) => {
  const raw = String(code || "").toLowerCase();
  // zh-CN / zh-TW / zh-HK 都要能区分，否则分布图里会出现多个同名「中文」
  if (raw.startsWith("zh")) {
    if (raw.includes("tw") || raw.includes("hk") || raw.includes("hant")) {
      return t("contentV04.language.chineseHant", "中文（繁体）");
    }
    return t("contentV04.language.chineseHans", "中文（简体）");
  }
  const short = raw.slice(0, 2);
  return LOCALE_KEYS[short] ? t(`contentV04.language.${LOCALE_KEYS[short]}`) : code || "—";
};

const REFERRER_KEYS = ["direct", "search", "social", "external"];
const referrerLabel = (key: string) =>
  REFERRER_KEYS.includes(key) ? t(`contentV04.analytics.referrers.${key}`) : key || "—";

const CHART_PALETTE = ["#6366F1", "#2563EB", "#F59E0B", "#EC4899", "#8B5CF6", "#06B6D4", "#EF4444", "#0EA5E9"];

const articleTopRows = computed(() =>
  rankings.value.filter((r: any) => r.contentType === "article").slice(0, 10),
);
const newsTopRows = computed(() => rankings.value.filter((r: any) => r.contentType === "news").slice(0, 10));

const todayStats = computed(() => {
  const today = isoDate(new Date());
  const row = trends.value.find((r: any) => String(r.date || "").slice(0, 10) === today) as any;
  const source = { ...row, ...overview.value };
  const impressions = source?.impressions || 0;
  const clicks = source?.clicks || 0;
  return {
    visitors: source?.uniqueVisitors || 0,
    views: source?.pageViews || 0,
    reads: source?.reads || 0,
    ctr: Number(source?.ctr ?? (impressions > 0 ? clicks / impressions : 0)),
  };
});

const trendChartData = computed(() => ({
  labels: trends.value.map((r: any) => formatDate(r.date)),
  datasets: [
    { label: t("contentV04.analytics.visitors"), data: trends.value.map((r: any) => r.uniqueVisitors || 0), borderColor: "#6366F1", backgroundColor: "rgba(99,102,241,0.1)", fill: true, tension: 0.4 },
    { label: t("contentV04.analytics.views"), data: trends.value.map((r: any) => r.pageViews || 0), borderColor: "#F59E0B", backgroundColor: "rgba(245,158,11,0.1)", fill: true, tension: 0.4 },
    { label: t("contentV04.analytics.reads"), data: trends.value.map((r: any) => r.reads || 0), borderColor: "#2563EB", backgroundColor: "rgba(37,99,235,0.12)", fill: true, tension: 0.4 },
  ],
}));

const readSecondsChartData = computed(() => ({
  labels: trends.value.map((r: any) => formatDate(r.date)),
  datasets: [
    {
      label: t("contentV04.analytics.duration"),
      data: trends.value.map((r: any) => Number(Number(r.avgReadSeconds || 0).toFixed(1))),
      borderColor: "#8B5CF6",
      backgroundColor: "rgba(139,92,246,0.12)",
      fill: true,
      tension: 0.4,
    },
  ],
}));

const publishingChartData = computed(() => ({
  labels: trends.value.map((r: any) => formatDate(r.date)),
  datasets: [
    { label: t("contentV04.content.articles"), data: trends.value.map((r: any) => r.publishedArticles || 0), backgroundColor: "#6366F1", borderRadius: 6 },
    { label: t("contentV04.content.news"), data: trends.value.map((r: any) => r.publishedNews || 0), backgroundColor: "#2563EB", borderRadius: 6 },
  ],
}));

const articleTopChartData = computed(() => ({
  labels: articleTopRows.value.map((r: any) => (r.title || r.pagePath || "").slice(0, 24)),
  datasets: [
    {
      label: t("contentV04.analytics.reads"),
      data: articleTopRows.value.map((r: any) => r.reads || 0),
      backgroundColor: CHART_PALETTE[0],
      borderRadius: 8,
    },
  ],
}));

const newsTopChartData = computed(() => ({
  labels: newsTopRows.value.map((r: any) => (r.title || r.pagePath || "").slice(0, 24)),
  datasets: [
    {
      label: t("contentV04.analytics.reads"),
      data: newsTopRows.value.map((r: any) => r.reads || 0),
      backgroundColor: CHART_PALETTE[1],
      borderRadius: 8,
    },
  ],
}));

const ctrChartData = computed(() => ({
  labels: ctrRows.value.map((r: any) => placementLabel(r.placement)),
  datasets: [
    {
      label: "CTR %",
      data: ctrRows.value.map((r: any) => Number(((r.ctr || 0) * 100).toFixed(2))),
      backgroundColor: "#F59E0B",
      borderRadius: 8,
    },
  ],
}));

const distributionChart = (rows: any[], labeler: (key: string) => string) => {
  // 同一显示名可能对应多个原始值（例如 zh-CN / zh），出现重名时补原始值避免图例看不出区别
  const seen = new Map<string, number>();
  rows.forEach((r: any) => {
    const label = labeler(r.key);
    seen.set(label, (seen.get(label) || 0) + 1);
  });
  return {
    labels: rows.map((r: any) => {
      const label = labeler(r.key);
      return (seen.get(label) || 0) > 1 && r.key && r.key !== label ? `${label} · ${r.key}` : label;
    }),
    datasets: [{ data: rows.map((r: any) => r.count || 0), backgroundColor: CHART_PALETTE, borderWidth: 0 }],
  };
};

const categoryChartData = computed(() => distributionChart(distributions.value.category, (k) => k || "—"));
const localeChartData = computed(() => distributionChart(distributions.value.locale, localeLabel));
const sourceChartData = computed(() => distributionChart(distributions.value.source, (k) => k || "—"));
const referrerChartData = computed(() => distributionChart(distributions.value.referrer, referrerLabel));


watch(biRange, (value) => {
  if (value === "custom") return;
  const days = Number(value);
  biFilters.value.to = isoDate(new Date());
  biFilters.value.from = isoDaysAgo(days - 1);
  loadBI();
});

watch(
  () => [biFilters.value.locale, biFilters.value.placement],
  () => loadBI(),
);
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
const selectedAIModelKey = ref("");
const aiModelOptions = computed(() => aiConfigs.value.flatMap((config: any) =>
  (config.models?.length ? config.models : config.defaultModel ? [config.defaultModel] : []).map((model: string) => ({
    value: `${config.id}::${model}`,
    label: `${config.label || config.provider} · ${model}`,
  })),
));
function selectAIModel(value: string | number | boolean | null) {
  const selected = aiModelOptions.value.find((option: any) => option.value === value);
  if (!selected) return;
  const [configID, ...modelParts] = String(value).split("::");
  brief.value.providerKeyRef = configID;
  brief.value.model = modelParts.join("::");
  selectedAIModelKey.value = String(value);
}


const translations = ref<any[]>([]);
// 管理端多语言目标（中文为源语言，不在此列）
// 1.6：语言名按界面语言显示（后台界面为中文 → 用中文语言名）
const ADMIN_TARGET_LOCALES = [
  // PRD R9：语言选项按 i18n 口径显示原生语言名（English / Русский / Français / Deutsch），
  // 五种界面语言下都一致，所以走词条而不是硬编码中文名。
  { value: "en-US", get label() { return t("contentV04.language.english"); } },
  { value: "ru-RU", get label() { return t("contentV04.language.russian"); } },
  { value: "fr-FR", get label() { return t("contentV04.language.french"); } },
  { value: "de-DE", get label() { return t("contentV04.language.german"); } },
];
const selectedArticleLocales = ref<string[]>([]);
const translationBusy = ref(false);
const articleTranslationSummaries = ref<Record<number, any[]>>({});
const newsTranslationSummaries = ref<Record<number, any[]>>({});
const newsTranslationTasks = ref<any[]>([]);
const TRANSLATION_STATUS_TEXT: Record<string, string> = {
  draft: "待发布",
  // 保存时按 syncLocales 新建的待翻译记录（后端 domain.ContentTranslationDraft）
  translation_draft: "待翻译",
  review: "待发布",
  published: "已发布",
  unpublished: "已下架",
  deleted: "已删除",
  pending: "处理中",
  queued: "排队中",
  running: "翻译中",
  in_progress: "翻译中",
  succeeded: "已完成",
  success: "已完成",
  failed: "失败",
  error: "失败",
  cancelled: "已取消",
  canceled: "已取消",
};
function translationStatusKey(status?: string) {
  const raw = String(status || "").toLowerCase();
  if (!raw) return "none";
  if (["queued", "pending"].includes(raw)) return "queued";
  if (["running", "in_progress"].includes(raw)) return "running";
  if (["published", "succeeded", "success"].includes(raw)) return "success";
  if (["failed", "error"].includes(raw)) return "failed";
  if (["draft", "review"].includes(raw)) return "draft";
  if (["cancelled", "canceled", "deleted", "unpublished"].includes(raw)) return "cancelled";
  return "draft";
}
function translationStatusText(status?: string) {
  const raw = String(status || "").toLowerCase();
  if (!raw) return "未翻译";
  return TRANSLATION_STATUS_TEXT[raw] || raw;
}
function localeShort(locale?: string) {
  const key = String(locale || "").toLowerCase();
  if (key.startsWith("en")) return "EN";
  if (key.startsWith("ru")) return "RU";
  if (key.startsWith("fr")) return "FR";
  if (key.startsWith("de")) return "DE";
  if (key.startsWith("zh")) return "ZH";
  return String(locale || "").toUpperCase() || "?";
}
function translationFor(locale: string) {
  return translations.value.find(
    (t: any) => String(t.locale) === locale && String(t.status) !== "deleted",
  );
}
const otherTranslations = computed(() =>
  translations.value.filter(
    (t: any) =>
      String(t.status) !== "deleted" &&
      !ADMIN_TARGET_LOCALES.some((l) => l.value === String(t.locale)),
  ),
);
function articleTranslationSummary(id: number) {
  return (articleTranslationSummaries.value[id] || []).filter(
    (t: any) => String(t.status) !== "deleted",
  );
}
function newsTranslationSummary(id: number) {
  return newsTranslationSummaries.value[id] || [];
}
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

// R15-01/AC-14：内容中心所有下拉统一走 common/Select，禁止浏览器原生 select
const newsStatusOptions = computed(() => [
  { value: "", label: "全部状态" },
  { value: "published", label: "已发布" },
  { value: "draft", label: "草稿" },
  { value: "pending_review", label: "待审核" },
]);
const articleStatusOptions = computed(() => newsStatusOptions.value);
const sourceTypeOptions = computed(() => [
  { value: "rss", label: "RSS" },
  { value: "atom", label: "Atom" },
  { value: "public_json", label: "Public JSON" },
]);
const publishModeOptions = computed(() => [
  { value: "manual_review", label: "人工审核" },
  { value: "auto_publish", label: "自动发布" },
]);
const articleModelSelectOptions = computed(() => [
  { value: "", label: "全部模型" },
  ...articleModelFilterOptions.value.map((model) => ({ value: model, label: model })),
]);
const articleCategorySelectOptions = computed(() => [
  { value: "", label: "未分类" },
  ...(taxonomy.value.articleCategories || []).map((category: string) => ({
    value: category,
    label: category,
  })),
]);
const briefLanguageOptions = computed(() => [
  { value: "zh-CN", label: "中文" },
  { value: "en-US", label: "English" },
]);
function onNewsStatusSelect() {
  void loadNews(1);
}
function onArticleFilterSelect() {
  void loadArticles(1);
}
const articleAuthorId = ref("");
const articleFrom = ref("");
const articleTo = ref("");
const articleMinReads = ref("");
const articleMaxReads = ref("");
const sourceForm = ref<any>({
  name: "",
  url: "",
  sourceType: "rss",
  mappingJsonText: "",
  // R1-02：留空 = 继承全局，默认不填任何值（PRD v3.3 L168）
  intervalMinutes: null,
  enabled: true,
});
const sourceFormErrors = ref<{ name: string; url: string; interval: string }>({ name: "", url: "", interval: "" });
// SRC-03：抓取间隔范围（分钟），留空表示继承全局
function validateIntervalText(raw: any): string {
  const text = String(raw ?? "").trim();
  if (!text) return "";
  const n = Number(text);
  if (!Number.isFinite(n) || n < 15 || n > 1440) return "抓取间隔需在 15–1440 分钟之间";
  return "";
}
// S5：字段级校验（红字提示 + 提交按钮禁用）
function validateSourceField(field: "name" | "url" | "interval") {
  const name = String(sourceDraft.value.name || "").trim();
  const url = String(sourceDraft.value.url || "").trim();
  if (field === "name") {
    sourceFormErrors.value.name = name ? "" : "必填";
    return;
  }
  if (field === "interval") {
    // SRC-03：留空=继承全局；填了就必须落在 15–1440 分钟内
    sourceFormErrors.value.interval = validateIntervalText(sourceDraft.value.intervalMinutes);
    return;
  }
  if (!url) sourceFormErrors.value.url = "URL 必填";
  else if (!/^https?:\/\//i.test(url)) sourceFormErrors.value.url = "URL 需以 http:// 或 https:// 开头";
  else sourceFormErrors.value.url = "";
}
const sourceDialogCreating = ref(false);
const sourceDialogOpen = computed(() => sourceDialogCreating.value || !!selectedSource.value.id);
const sourceDraft = computed<any>(() => (sourceDialogCreating.value ? sourceForm.value : selectedSource.value));
function emptySourceForm() {
  return { name: "", url: "", sourceType: "rss", mappingJsonText: "", intervalMinutes: null, enabled: true, category: "", language: "zh-CN", rateLimitPerMinute: 60 };
}
function openCreateSource() {
  selectedSource.value = {};
  sourceForm.value = emptySourceForm();
  sourceFormErrors.value = { name: "", url: "", interval: "" };
  sourceDialogCreating.value = true;
}
function closeSourceDialog() {
  sourceDialogCreating.value = false;
  selectedSource.value = {};
  sourceFormErrors.value = { name: "", url: "", interval: "" };
  sourceCloseGuard.value.show = false;
  sourceSnapshot = "";
}
// SRC-07：新闻源弹窗「有未保存修改，确认离开？」（与 NE-13/NE-14 一致的应用内确认）
const sourceCloseGuard = ref({ show: false });
let sourceSnapshot = "";
function sourceFormSignature(): string {
  const d = sourceDraft.value || {};
  return JSON.stringify([
    d.name ?? "", d.url ?? "", d.sourceType ?? "", d.category ?? "", d.language ?? "",
    d.intervalMinutes ?? null, d.rateLimitPerMinute ?? null, d.enabled !== false,
    d.mappingJsonText ?? "",
  ]);
}
function isSourceDirty(): boolean {
  return sourceDialogOpen.value && sourceFormSignature() !== sourceSnapshot;
}
function requestCloseSourceDialog() {
  if (!isSourceDirty()) {
    closeSourceDialog();
    return;
  }
  sourceCloseGuard.value.show = true;
}
function resumeSourceEditing() {
  sourceCloseGuard.value.show = false;
}
function confirmDiscardSource() {
  sourceCloseGuard.value.show = false;
  closeSourceDialog();
}
watch(sourceDialogOpen, async (open) => {
  sourceCloseGuard.value.show = false;
  if (!open) {
    sourceSnapshot = "";
    return;
  }
  await nextTick();
  sourceSnapshot = sourceFormSignature();
});
async function submitSourceDialog() {
  if (sourceDialogCreating.value) await createSource();
  else await saveSource();
}
// R1-02/S6：全局抓取间隔（分钟），来自自动化设置 fetchCron，如 `0 */15 * * * *` → 15
function globalFetchIntervalMinutes(): number {
  const parts = String(automation.value?.fetchCron || "").trim().split(/\s+/);
  // 兼容 6 段（含秒）与 5 段标准 Cron
  const minuteField = parts.length >= 6 ? parts[1] : parts[0];
  const step = String(minuteField || "").match(/^\*\/(\d+)$/);
  const minutes = step ? Number(step[1]) : 0;
  return minutes > 0 ? minutes : 15;
}
// 调度文案：源自身 intervalMinutes 才是真实调度；为 NULL 表示继承全局，
// 必须显示“跟随全局（N 分钟）”，不得用全局 fetchCron/scheduleMinutes 冒充源自身间隔（PRD v3.3 L121）
function sourceScheduleText(source: any) {
  const minutes = Number(source?.intervalMinutes ?? 0);
  if (minutes > 0) return `每 ${minutes} 分钟抓取一次`;
  return `跟随全局（${globalFetchIntervalMinutes()} 分钟）`;
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
const automationTargetLocales = ref<string[]>([]);
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

const articleTagsInput = computed<string[]>({
  get: () => String(articleForm.value.tags || "").split(/[,，]/).map((v: string) => v.trim()).filter(Boolean),
  set: (values) => { articleForm.value.tags = values.join(", "); },
});
const articleSeoKeywordsInput = computed<string[]>({
  get: () => String(articleForm.value.seoKeywords || "").split(/[,，]/).map((v: string) => v.trim()).filter(Boolean),
  set: (values) => { articleForm.value.seoKeywords = values.join(", "); },
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

/* SEO 自动化：按最佳实践一键生成描述/关键词（标题与 slug 保持不变） */
const SEO_DESC_MIN = 120;
const SEO_DESC_MAX = 160;
const SEO_KW_MIN = 5;
const SEO_KW_MAX = 8;
const SEO_STOPWORDS = new Set([
  "the", "and", "for", "with", "that", "this", "from", "are", "was", "were", "will", "can",
  "has", "have", "not", "but", "you", "your", "our", "all", "any", "how", "why", "what",
  "我们", "你们", "他们", "以及", "但是", "因此", "如果", "可以", "一个", "这个", "那个",
  "什么", "因为", "所以", "已经", "还有", "同时", "进行", "通过", "对于", "需要", "支持",
  "使用", "的是", "了的", "是一", "在的",
]);
const seoGenerating = ref(false);
const seoOverwriteConfirm = ref(false);
const seoOverwriteOkRef = ref<HTMLButtonElement | null>(null);
const coverFileRef = ref<HTMLInputElement | null>(null);
const ogFileRef = ref<HTMLInputElement | null>(null);

/**
 * 左栏键盘焦点滚动策略（AE-19）：
 * 浏览器默认把新获得焦点的元素滚到可视区「中间」，于是按 Tab 前进时，
 * 后面的字段反而出现在前面字段的上方，视觉顺序与 Tab 顺序相反。
 * 这里统一改为「焦点字段对齐滚动面板顶部」：位置稳定（不会因控件高度不同上下浮动），
 * 字段完整可见，且 Tab 前进时视觉 y 单调不后退。
 */
let leftPreFocusScrollTop: number | null = null;
function onEditorLeftKeydown(e: KeyboardEvent) {
  if (e.key !== "Tab") return;
  const left = e.currentTarget as HTMLElement | null;
  leftPreFocusScrollTop = left ? left.scrollTop : 0;
}
function onEditorLeftFocusIn(e: FocusEvent) {
  const left = e.currentTarget as HTMLElement | null;
  const el = e.target as HTMLElement | null;
  const pre = leftPreFocusScrollTop;
  leftPreFocusScrollTop = null;
  // 只接管键盘 Tab 造成的焦点变化：鼠标点击时尊重用户当前滚动位置
  if (!left || !el || pre === null || el === left) return;
  const elRect = el.getBoundingClientRect();
  const boxRect = left.getBoundingClientRect();
  const pad = 12;
  // 元素在滚动内容中的绝对位置（与当前 scrollTop 无关）
  const contentTop = elRect.top - boxRect.top + left.scrollTop;
  const max = Math.max(0, left.scrollHeight - left.clientHeight);
  const next = Math.max(0, Math.min(max, Math.round(contentTop - pad)));
  if (next !== left.scrollTop) left.scrollTop = next;
}

/** 精选开关是 role=switch 的 button：补一个 checked 反射属性，语义/自动化都与原生开关一致 */
const vFeaturedChecked = {
  mounted(el: HTMLElement) {
    if (!("checked" in el)) {
      Object.defineProperty(el, "checked", {
        configurable: true,
        get: () => el.getAttribute("aria-checked") === "true",
        set: (v: boolean) => {
          el.setAttribute("aria-checked", v ? "true" : "false");
        },
      });
    }
  },
};
const articleBodyPlainText = computed(() =>
  String(articleForm.value.bodyMarkdown || "")
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim(),
);

/** 描述：按句累加至 120~160 字，避免半句截断 */
function buildSeoDescription(source: string): string {
  const text = source.replace(/\s+/g, " ").trim();
  if (!text) return "";
  const sentences = text.match(/[^。！？；.!?;]+[。！？；.!?;]?/g) || [text];
  let out = "";
  for (const raw of sentences) {
    const s = raw.trim();
    if (!s) continue;
    if (out && out.length + s.length > SEO_DESC_MAX) break;
    out += s;
    if (out.length >= SEO_DESC_MIN) break;
  }
  if (!out) out = text;
  return out.length > SEO_DESC_MAX ? out.slice(0, SEO_DESC_MAX) : out;
}

/** 关键词：中文按 2~3 元 n-gram、英文按词频统计，去重叠后取 5~8 个 */
function buildSeoKeywords(f: any, bodyText: string): string[] {
  const text = `${String(f.title || "")} ${bodyText}`;
  const scored = new Map<string, number>();
  const bump = (word: string, weight: number) => {
    if (!word || SEO_STOPWORDS.has(word.toLowerCase())) return;
    scored.set(word, (scored.get(word) || 0) + weight);
  };
  for (const w of text.match(/[A-Za-z][A-Za-z0-9+#._-]{1,19}/g) || []) {
    bump(w.replace(/[._-]+$/, ""), 2);
  }
  for (const run of text.match(/[\u4e00-\u9fa5]+/g) || []) {
    for (const n of [2, 3]) {
      for (let i = 0; i + n <= run.length; i++) bump(run.slice(i, i + n), n);
    }
  }
  const ranked = [...scored.entries()].sort((a, b) => b[1] - a[1] || b[0].length - a[0].length);
  const picked: string[] = [];
  for (const [w] of ranked) {
    if (picked.length >= SEO_KW_MAX) break;
    if (picked.some((p) => p.includes(w) || w.includes(p))) continue;
    picked.push(w);
  }
  if (picked.length < SEO_KW_MIN) {
    for (const [w] of ranked) {
      if (picked.length >= SEO_KW_MIN) break;
      if (!picked.includes(w)) picked.push(w);
    }
  }
  return picked;
}

async function runSeoGeneration() {
  const f = articleForm.value;
  const bodyText = articleBodyPlainText.value;
  if (!bodyText) return;
  seoOverwriteConfirm.value = false;
  seoGenerating.value = true;
  try {
    // 留出可感知的生成态，避免「点了没反应」
    await new Promise((resolve) => setTimeout(resolve, 360));
    const desc = buildSeoDescription(
      [String(f.excerpt || "").trim(), bodyText].filter(Boolean).join(" "),
    );
    if (desc) f.seoDescription = desc;
    const keywords = buildSeoKeywords(f, bodyText);
    if (keywords.length) f.seoKeywords = keywords.join(", ");
  } finally {
    seoGenerating.value = false;
  }
}

function generateSeoMeta() {
  if (seoGenerating.value) return;
  if (!articleBodyPlainText.value) {
    // 空正文时按钮呈禁用语义（aria-disabled），但仍要给出明确原因，不能「点了没反应」
    toast("warning", "请先填写正文内容，再生成 SEO 描述与关键词");
    return;
  }
  const f = articleForm.value;
  if (String(f.seoDescription || "").trim() || String(f.seoKeywords || "").trim()) {
    seoOverwriteConfirm.value = true;
    return;
  }
  void runSeoGeneration();
}
function confirmSeoOverwrite() {
  void runSeoGeneration();
}
function cancelSeoOverwrite() {
  seoOverwriteConfirm.value = false;
}
/* 覆盖确认出现即把焦点交给主操作，键盘用户不必再 Tab 找按钮；Esc 关闭 */
watch(seoOverwriteConfirm, async (open) => {
  if (!open) return;
  await nextTick();
  seoOverwriteOkRef.value?.focus();
});

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
        typeof item.page_size === "number" ||
        typeof item.pageSize === "number" ||
        typeof item.hasNext === "boolean",
    ) || {};
  return {
    page: Number(payload.page || 1),
    total: Number(payload.total ?? 0),
  };
};
// 列表请求代号：同类型的后发请求会作废先发请求，避免「翻页/搜索时先发的慢响应后到」把新结果覆盖掉
// （NE-15 偶发：刷新页面后立刻点查询，首屏那次的响应晚到，列表被重置回全量，看不到刚保存的行）
const listRequestSeq: Record<string, number> = {};
async function loadList(
  key: ContentListKey,
  request: (isLatest: () => boolean) => Promise<void>,
) {
  const seq = (listRequestSeq[key] = (listRequestSeq[key] || 0) + 1);
  const isLatest = () => listRequestSeq[key] === seq;
  const state = listState.value[key]; state.loading = true; state.error = "";
  try { await request(isLatest); }
  catch (error) { if (isLatest()) state.error = error instanceof Error ? error.message : "列表加载失败"; }
  finally { if (isLatest()) state.loading = false; }
}
const formatDate = (v: string) => (v ? new Date(v).toLocaleDateString() : "-");
const formatDateTime = (v: string) =>
  v ? new Date(v).toLocaleString() : "-";
async function runAdmin(
  action: () => Promise<void>,
  message: string,
  onError?: (msg: string) => void,
) {
  errorMessage.value = "";
  try {
    await action();
  } catch (error) {
    // 接口拦截器 reject 的是 { message, code } 普通对象，不是 Error；用统一工具透出后端文案（如「抓取间隔需在 15–1440 分钟之间」）
    const msg = extractApiErrorMessage(error, message);
    errorMessage.value = msg;
    onError?.(msg);
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
  await loadList("news", async (isLatest) => {
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
  if (!isLatest()) return; // 已发起更新的列表请求，丢弃这次迟到的响应
  news.value = unwrap(data);
  const meta = pageMeta(data);
  newsTotal.value = meta.total;
  allNewsSelected.value = false;
  selectedNewsIds.value = [];
  });
  void loadNewsTranslationSummaries(news.value);
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
  await loadList("sources", async (isLatest) => {
  const { data } = await contentAPI.listSources({
    page: sourcePage.value,
    page_size: 20,
    include_deleted: true,
  });
  if (!isLatest()) return;
  sources.value = unwrap(data);
  const meta = pageMeta(data);
  sourceTotal.value = meta.total;
  });
}
async function loadRuns(page = runPage.value) {
  runPage.value = Math.max(1, page);
  await loadList("runs", async (isLatest) => {
  const { data } = await contentAPI.listFetchRuns({
    page: runPage.value,
    page_size: 20,
  });
  if (!isLatest()) return;
  runs.value = unwrap(data);
  const meta = pageMeta(data);
  runTotal.value = meta.total;
  });
}
async function loadArticles(pageOrEvent: number | Event = articlePage.value) {
  const page = typeof pageOrEvent === "number" ? pageOrEvent : 1;
  articlePage.value = Math.max(1, page);
  await loadList("articles", async (isLatest) => {
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
  if (!isLatest()) return; // 同上：迟到的旧响应不得覆盖新结果
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
  allArticlesSelected.value = false;
  selectedArticleIds.value = [];
  });
  void loadArticleTranslationSummaries(articles.value);
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
  // 勾选态 = 已存在译文的语言（删除态除外），保证 UI 与真实数据一致
  const existing = translations.value
    .filter((t: any) => String(t.status) !== "deleted")
    .map((t: any) => String(t.locale));
  selectedArticleLocales.value = ADMIN_TARGET_LOCALES.map((l) => l.value).filter((v) =>
    existing.includes(v),
  );
}
async function loadBI() {
  const params = { ...biFilters.value };
  biLoading.value = true;
  biError.value = "";
  try {
    const [overviewRes, trendsRes, rankingsRes, ctrRes, categoryRes, localeRes, sourceRes, referrerRes] = await Promise.all([
      contentAPI.analyticsOverview(params),
      contentAPI.analyticsTrends(params),
      contentAPI.analyticsRankings({ ...params, page: 1, page_size: 10 }),
      contentAPI.analyticsCTR(params),
      contentAPI.analyticsDistribution("category", params),
      contentAPI.analyticsDistribution("locale", params),
      contentAPI.analyticsDistribution("source", params),
      contentAPI.analyticsDistribution("referrer", params),
    ]);
    overview.value = overviewRes.data?.data || overviewRes.data || {};
    trends.value = unwrap(trendsRes.data);
    rankings.value = unwrap(rankingsRes.data);
    ctrRows.value = unwrap(ctrRes.data);
    distributions.value = {
      category: unwrap(categoryRes.data),
      locale: unwrap(localeRes.data),
      source: unwrap(sourceRes.data),
      referrer: unwrap(referrerRes.data),
    };
  } catch {
    biError.value = t("contentV04.analytics.failed");
    trends.value = [];
    rankings.value = [];
    ctrRows.value = [];
    distributions.value = { category: [], locale: [], source: [], referrer: [] };
  } finally {
    biLoading.value = false;
  }
}
const newsCloseGuard = ref({ show: false });
const newsSnapshot = ref("");
const newsFormTouched = ref(false);
let newsGuardAcknowledged = false;

/** 新闻表单签名：只取会写回后端的字段，避免展示态字段造成误判 */
function newsFormSignature(): string {
  const n = selectedNews.value || ({} as any);
  return JSON.stringify({
    title: String(n.title ?? ""),
    originalUrl: String(n.originalUrl ?? ""),
    category: String(n.category ?? ""),
    tags: Array.isArray(n.tags) ? n.tags.map((t: unknown) => String(t)) : [],
    status: String(n.status ?? ""),
    slug: String(n.slug ?? ""),
    locale: String(n.locale ?? ""),
    seoTitle: String(n.seoTitle ?? ""),
    seoDescription: String(n.seoDescription ?? ""),
    seoKeywordsText: Array.isArray(n.mediaJson?.seoKeywords) ? n.mediaJson.seoKeywords.join(", ") : "",
    sourceName: String(n.sourceName ?? ""),
    summary: String(n.summary ?? ""),
    bodyHtml: String(n.bodyHtml ?? n.bodyHTML ?? ""),
  });
}
function isNewsDirty(): boolean {
  if (!selectedNews.value?.id || !newsSnapshot.value) return false;
  // 用户必须真实动过表单，避免编辑器挂载时的规范化被当成「未保存修改」
  if (!newsFormTouched.value) return false;
  return newsFormSignature() !== newsSnapshot.value;
}
/** 键盘焦点锁在给定根节点内：Tab 到末尾回开头，Shift+Tab 到开头跳末尾 */
function trapFocusWithin(root: HTMLElement | null, ev: KeyboardEvent) {
  if (!root) return;
  const nodes = Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [contenteditable="true"], [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => el.getClientRects().length > 0);
  if (!nodes.length) return;
  const first = nodes[0];
  const last = nodes[nodes.length - 1];
  const active = document.activeElement as HTMLElement | null;
  if (!ev.shiftKey && active === last) {
    ev.preventDefault();
    first.focus();
  } else if (ev.shiftKey && active === first) {
    ev.preventDefault();
    last.focus();
  }
}
function trapNewsFocus(ev: KeyboardEvent) {
  trapFocusWithin(newsEditorRef.value as HTMLElement | null, ev);
}
function trapNewsGuardFocus(ev: KeyboardEvent) {
  trapFocusWithin(
    document.querySelector(".news-close-guard-dialog") as HTMLElement | null,
    ev,
  );
}
/** 关闭入口统一走这里：有未保存修改先确认 */
function requestCloseNewsEditor() {
  if (!selectedNews.value?.id) return;
  if (newsGuardAcknowledged || !isNewsDirty()) {
    closeNewsEditor();
    return;
  }
  newsCloseGuard.value.show = true;
  void nextTick(() => {
    const keep = document.querySelector(".news-close-guard-dialog .button.primary") as HTMLElement | null;
    keep?.focus();
  });
}
function resumeNewsEditing() {
  newsCloseGuard.value.show = false;
  // 用户已明确选择「继续编辑」，接着再点关闭不重复追问；一旦改动表单会重新置脏
  newsGuardAcknowledged = true;
  void nextTick(() => (newsEditorRef.value as HTMLElement | null)?.focus?.());
}
function confirmDiscardNews() {
  closeNewsEditor();
}

watch(
  () => selectedNews.value?.id,
  async (id) => {
    newsCloseGuard.value.show = false;
    newsGuardAcknowledged = false;
    newsFormTouched.value = false;
    newsSnapshot.value = "";
    if (!id) return;
    await nextTick();
    newsSnapshot.value = newsFormSignature();
  },
);

function closeNewsEditor() {
  newsEditRequest += 1
  newsEditorSession.value += 1
  selectedNews.value = {}
  newsTranslationTasks.value = []
  newsCloseGuard.value.show = false
  newsSnapshot.value = ""
  newsFormTouched.value = false
  newsGuardAcknowledged = false
}

async function editNews(item: any) {
  const requestId = ++newsEditRequest
  try {
    const { data } = await contentAPI.getNews(item.id)
    if (requestId !== newsEditRequest) return
    const detail = data?.data || data || { ...item }
    const storedBody =
      detail.bodyHtml || detail.bodyHTML || detail.bodyMarkdown || detail.body || detail.content || ""
    selectedNews.value = {
      ...detail,
      bodyHtml: announcementEditorInitialHtml(String(storedBody)),
    }
    newsEditorSession.value += 1
  } catch {
    if (requestId !== newsEditRequest) return
    const storedBody =
      item.bodyHtml || item.bodyHTML || item.bodyMarkdown || item.body || item.content || ""
    selectedNews.value = {
      ...item,
      bodyHtml: announcementEditorInitialHtml(String(storedBody)),
    }
    newsEditorSession.value += 1
    errorMessage.value = "新闻详情加载失败，已显示列表数据"
  }
  await nextTick()
  newsEditorRef.value?.scrollIntoView({ behavior: "smooth", block: "start" })
  void loadNewsTranslationTasks(Number(item.id))
}
/** 新闻保存时把 AI 生成的 SEO 关键词回写到 mediaJson.seoKeywords（不覆盖其他媒体字段）。 */
function buildNewsMediaJson(): Record<string, unknown> {
  const base = { ...(selectedNews.value?.mediaJson || {}) } as Record<string, unknown>;
  const keywords = parseTagsText(String(selectedNews.value?.seoKeywordsText || ""));
  if (keywords.length) base.seoKeywords = keywords;
  else delete base.seoKeywords;
  return base;
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
      mediaJson: buildNewsMediaJson(),
    });
    closeNewsEditor();
    await loadNews();
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
  const raw = Array.isArray(payload.targetLocales)
    ? payload.targetLocales
    : String(
        payload.targetLocalesText ||
          (Array.isArray(payload.TargetLocales) ? payload.TargetLocales.join(",") : ""),
      ).split(/[,，]/);
  automationTargetLocales.value = ADMIN_TARGET_LOCALES.map((l) => l.value).filter((v) =>
    raw.map((x: any) => String(x).trim()).includes(v),
  );
}

// ---- 新闻行的发表 / 删除：必须走 /admin/content/news 接口（新闻与文章是两套表） ----
/**
 * 「发表」按钮的可点判据必须与后端 validateNewsItemPublishable 一致：
 * 正文 HTML、正文纯文本、摘要、来源名四者都非空，否则 PublishNewsItem 直接 409。
 * 这里先在前端置灰，避免用户点到一个必然失败的按钮。
 */
function newsPublishable(item: any): boolean {
  const has = (v: unknown) => String(v ?? "").trim().length > 0;
  if (!item) return false;
  return has(item.bodyHtml) && has(item.bodyText) && has(item.summary) && has(item.sourceName);
}
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
  sourceFormErrors.value = { name: "", url: "", interval: "" };
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
    // R1-02：留空必须提交 NULL（继承全局），不得用 15 覆盖（PRD v3.3 L121/L168）
    // SRC-03：只有空值才算「跟随全局」；0/14/1441 原样提交，由后端做 15–1440 范围校验
    const rawText = String(sourceForm.value.intervalMinutes ?? "").trim();
    const intervalMinutes = rawText === "" ? null : Number(rawText);
    await contentAPI.createSource({
      ...sourceForm.value,
      name,
      url,
      mappingJson,
      language: String(sourceForm.value.language || "").trim() || "zh-CN",
      enabled: sourceForm.value.enabled !== false,
      intervalMinutes,
      rateLimitPerMinute: Number(sourceForm.value.rateLimitPerMinute) > 0 ? Number(sourceForm.value.rateLimitPerMinute) : 60,
      authorizationNote: "公开 feed，已获授权",
      robotsPolicy: "遵守 robots 规则",
    });
    const createdEnabled = sourceForm.value.enabled !== false;
    sourceForm.value = emptySourceForm();
    sourceDialogCreating.value = false;
    await loadSources();
    const scheduleText = intervalMinutes
      ? `每 ${intervalMinutes} 分钟抓取`
      : `跟随全局（${globalFetchIntervalMinutes()} 分钟）`;
    toast("success", `新闻源已创建（${createdEnabled ? "已启用" : "已停用"}，${scheduleText}）`);
  }, "新闻源创建失败", (msg) => {
    // SRC-03：后端对间隔做 15–1440 校验并回 400，这里把范围文案落到字段红字上
    if (/间隔|interval/i.test(msg)) sourceFormErrors.value.interval = msg;
  });
}
async function toggleSource(s: any) {
  await runAdmin(async () => {
    await contentAPI.toggleSource(s.id, !s.enabled);
    await loadSources();
  }, "新闻源状态更新失败");
}
async function editSource(source: any) {
  // 软删除的行只提供「恢复」：详情接口（不带 include_deleted）对软删行返回 404，
  // 这里提前拦截，避免发出必然 404 的请求（SRC-08：软删除行只显示「恢复」）。
  if (source?.deletedAt) {
    toast("error", "该新闻源已删除，请先恢复后再编辑");
    return;
  }
  sourceDialogCreating.value = false;
  sourceFormErrors.value = { name: "", url: "", interval: "" };
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
  validateSourceField("name");
  validateSourceField("url");
  if (sourceFormErrors.value.name || sourceFormErrors.value.url) {
    toast("error", "请先补全新闻源名称与 URL");
    return;
  }
  await runAdmin(async () => {
    // R1-02：清空间隔后保存必须写回 NULL（继承全局），不能残留旧值或 0
    const rawText = String(selectedSource.value.intervalMinutes ?? "").trim();
    const intervalMinutes = rawText === "" ? null : Number(rawText);
    const payload = {
      ...selectedSource.value,
      intervalMinutes,
      mappingJson: parseMappingJSON(selectedSource.value.mappingJsonText),
    };
    await contentAPI.updateSource(
      selectedSource.value.id,
      payload,
    );
    await loadSources();
    selectedSource.value = {};
    toast("success", "新闻源已保存");
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
    targetLocales: automationTargetLocales.value.length
      ? [...automationTargetLocales.value]
      : [],
    targetLocalesText: automationTargetLocales.value.join(","),
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
    const defaultConfig = aiConfigs.value.find((c: any) => c.isDefault);
    if (!defaultConfig) { aiStudioView.value = "config"; return; }
    const selected = aiModelOptions.value.find((option: any) => option.value === selectedAIModelKey.value);
    if (selected) {
      selectAIModel(selected.value);
    } else {
      brief.value.model = defaultConfig.defaultModel || defaultConfig.models?.[0] || "";
      brief.value.providerKeyRef = defaultConfig.id;
      selectedAIModelKey.value = `${defaultConfig.id}::${brief.value.model}`;
    }
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

async function toggleArticleFeatured(item: any, nextChecked: boolean) {
  if (!item?.id || articleFeatureBusyId.value === item.id) return;
  const checked = Boolean(nextChecked);
  articleFeatureBusyId.value = item.id;
  try {
    await (contentAPI.mutateTaxonomy as any)({
      scope: "featured_article",
      action: checked ? "set" : "unset",
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
    const reason = (e as any)?.reason || (e as any)?.response?.data?.reason;
    errorMessage.value = reason === "article_not_published" ? "仅已发布文章可设为精选" : "精选状态更新失败";
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
    // 先关弹窗再刷新列表：保存成功即刻反馈，不让用户等网络请求
    closeArticleEditor();
    toast("success", "文章已保存");
  } catch (error) {
    errorMessage.value = error instanceof Error && error.message ? error.message : "文章保存失败";
    toast("error", "文章保存失败，请检查标题、Slug 和正文");
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
  // 同步语言勾选随保存一起提交，后端按 syncLocales 建待翻译记录（与新闻编辑一致）
  body.syncLocales = selectedArticleLocales.value.filter(Boolean);
  delete body.id;
  if (articleForm.value.id) {
    await contentAPI.updateArticle(articleForm.value.id, body);
  } else {
    await contentAPI.createArticle(body);
  }
  // 手输的新标签此前只挂在文章上，标签库看不到：保存后补写进标签库
  const knownTags = new Set(taxonomy.value.tags.map((t) => String(t).trim()));
  const freshTags = Array.from(
    new Set(
      (Array.isArray(body.tags) ? body.tags : [])
        .map((t: any) => String(t).trim())
        .filter((t: string) => t && t !== FEATURED_TAG && !knownTags.has(t)),
    ),
  );
  if (freshTags.length) {
    await Promise.all(
      freshTags.map((name) =>
        contentAPI
          .mutateTaxonomy({ scope: "tag", action: "add", name } as any)
          .catch(() => null),
      ),
    );
    void loadTaxonomy();
  }
  // 列表刷新放后台进行，避免阻塞弹窗关闭
  void loadArticles();
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
async function resolveTranslationModel(): Promise<{ model: string; providerKeyRef: string } | null> {
  const current = String(brief.value.model || "").trim();
  if (current) {
    return { model: current, providerKeyRef: brief.value.providerKeyRef || "" };
  }
  try {
    const { data } = await contentAPI.getAIConfig();
    const cfg: any = data?.data || data || {};
    const model = String(cfg.defaultModel || cfg.model || "").trim();
    if (model) {
      brief.value.model = model;
      brief.value.providerKeyRef = cfg.providerKeyRef || brief.value.providerKeyRef || "";
      return { model, providerKeyRef: brief.value.providerKeyRef || "" };
    }
  } catch {
    // 读取默认模型失败时走下方统一提示
  }
  return null;
}
async function createTranslationTasks(locales: string[]) {
  const articleId = articleForm.value.id;
  if (!articleId || !locales.length) return;
  const resolved = await resolveTranslationModel();
  if (!resolved?.model) {
    toast("error", "请先在 AI 创作区选择模型，再生成译文");
    return;
  }
  translationBusy.value = true;
  try {
    await contentAPI.createTranslationTask(articleId, {
      locales,
      model: resolved.model,
      providerKeyRef: resolved.providerKeyRef,
    });
    await loadTranslations(articleId);
    toast("success", `已提交 ${locales.length} 个语言的翻译任务`);
  } catch {
    toast("error", "翻译任务创建失败，请检查模型配置");
  } finally {
    translationBusy.value = false;
  }
}
async function generateSelectedTranslations() {
  const locales = selectedArticleLocales.value.filter(Boolean);
  if (!locales.length) {
    toast("warning", "请先勾选目标语言");
    return;
  }
  await createTranslationTasks(locales);
}
async function generateTranslationFor(locale: string) {
  await createTranslationTasks([locale]);
}
async function cancelTranslation(locale: string) {
  const row = translationFor(locale);
  if (!row?.id) return;
  translationBusy.value = true;
  try {
    await contentAPI.deleteTranslation(row.id);
    await loadTranslations(articleForm.value.id);
    toast("success", `已取消 ${locale} 译文`);
  } catch {
    toast("error", "取消译文失败（已发布译文需先下架）");
  } finally {
    translationBusy.value = false;
  }
}
async function loadArticleTranslationSummaries(list: any[]) {
  const rows = await Promise.all(
    list.slice(0, 24).map(async (item: any) => {
      if (!item?.id) return [0, []] as [number, any[]];
      try {
        const { data } = await contentAPI.listTranslations(item.id);
        return [item.id, unwrap(data)] as [number, any[]];
      } catch {
        return [item.id, []] as [number, any[]];
      }
    }),
  );
  const next: Record<number, any[]> = {};
  for (const [id, items] of rows) if (id) next[id] = items;
  articleTranslationSummaries.value = next;
}
async function loadNewsTranslationSummaries(list: any[]) {
  const rows = await Promise.all(
    list.slice(0, 24).map(async (item: any) => {
      if (!item?.id) return [0, []] as [number, any[]];
      try {
        const { data } = await contentAPI.listNewsTranslationTasks(item.id);
        return [item.id, unwrap(data)] as [number, any[]];
      } catch {
        return [item.id, []] as [number, any[]];
      }
    }),
  );
  const next: Record<number, any[]> = {};
  for (const [id, items] of rows) if (id) next[id] = items;
  newsTranslationSummaries.value = next;
}
async function loadNewsTranslationTasks(id: number) {
  if (!id) {
    newsTranslationTasks.value = [];
    return;
  }
  try {
    const { data } = await contentAPI.listNewsTranslationTasks(id);
    newsTranslationTasks.value = unwrap(data);
  } catch {
    newsTranslationTasks.value = [];
  }
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

/* ==================== 文章编辑：标签规则 + 未保存修改保护 ==================== */
const ARTICLE_TAG_LIMIT = 10;
const ARTICLE_TAG_MAX_WIDTH = 20;
const seoDescLength = computed(() => String(articleForm.value.seoDescription || "").trim().length);
const articleCloseGuard = ref({ show: false });
// 「继续编辑」后紧邻的一次关闭不再追问（避免关闭-确认死循环）
let articleGuardAcknowledged = false;
const articleSnapshot = ref("");
// 用户点「继续编辑」即视为确认保留当前内容，此后的关闭不再重复提示
let articleSnapshotSuppressed = false;

function articleFormSignature(): string {
  const f = articleForm.value as any;
  return JSON.stringify({
    id: f.id,
    title: f.title,
    slug: f.slug,
    locale: f.locale,
    excerpt: f.excerpt,
    bodyMarkdown: f.bodyMarkdown,
    seoTitle: f.seoTitle,
    seoDescription: f.seoDescription,
    seoKeywords: f.seoKeywords,
    featured: f.featured,
    coverImage: f.coverImage,
    coverAlt: f.coverAlt,
    category: f.category,
    tags: f.tags,
    authorId: f.authorId,
    canonicalUrl: f.canonicalUrl,
    ogImage: f.ogImage,
    status: f.status,
    locales: Array.isArray(selectedArticleLocales.value) ? [...selectedArticleLocales.value] : [],
  });
}
function captureArticleSnapshot() {
  articleSnapshot.value = articleFormSignature();
}
function isArticleDirty(): boolean {
  if (!showArticleModal.value || !articleSnapshot.value) return false;
  return articleFormSignature() !== articleSnapshot.value;
}
watch(
  articleForm,
  () => {
    articleGuardAcknowledged = false;
  },
  { deep: true },
);
watch(showArticleModal, (open) => {
  if (!open) return;
  articleGuardAcknowledged = false;
  articleCloseGuard.value.show = false;
  if (articleSnapshotSuppressed) {
    articleSnapshotSuppressed = false;
    return;
  }
  void nextTick(captureArticleSnapshot);
});
function closeArticleEditor() {
  showArticleModal.value = false;
  articleCloseGuard.value.show = false;
  seoOverwriteConfirm.value = false;
  bodyEditMode.value = "editor";
  resetArticle();
}
function requestCloseArticle() {
  if (!showArticleModal.value) return;
  if (articleGuardAcknowledged || !isArticleDirty()) {
    closeArticleEditor();
    return;
  }
  // 先收起编辑器外壳，让确认框独占焦点；内容留在内存里，可随时继续编辑
  showArticleModal.value = false;
  articleCloseGuard.value.show = true;
}
function resumeArticleEditing() {
  articleSnapshotSuppressed = true;
  articleCloseGuard.value.show = false;
  showArticleModal.value = true;
  // 用户已明确选择「继续编辑」，紧接着再次关闭时不再重复追问；
  // 一旦继续改动表单，标志位会在 deep watch 里被重置，保护依旧生效
  void nextTick(() => {
    articleGuardAcknowledged = true;
  });
}
function confirmDiscardArticle() {
  closeArticleEditor();
}
/** 键盘焦点锁在弹窗内：Tab 到末尾回到开头，Shift+Tab 到开头跳回末尾 */
function trapArticleFocus(ev: KeyboardEvent) {
  const root = document.querySelector(".article-editor-modal") as HTMLElement | null;
  if (!root) return;
  const nodes = Array.from(
    root.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [contenteditable="true"], [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((el) => el.getClientRects().length > 0);
  if (!nodes.length) return;
  const first = nodes[0];
  const last = nodes[nodes.length - 1];
  const active = document.activeElement as HTMLElement | null;
  if (!ev.shiftKey && active === last) {
    ev.preventDefault();
    first.focus();
  } else if (ev.shiftKey && active === first) {
    ev.preventDefault();
    last.focus();
  }
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
  /* v0.4 SRC-09：次要文字对齐 1.1 色板（#6B7280），不再使用 #5E6577 */
  color: #6B7280;
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
/* v0.4 SRC-09：自动化设置区输入框统一 2px 边框 / 圆角 8 / 高 36（含 number、time）
   注意：全局 style.css 里有 input[type=number]{border-radius:12px !important}，
   这里必须用 !important + 更具体的选择器才能压住，否则 number 仍是 12px。 */
.settings .form-grid input:not([type="checkbox"]):not([type="radio"]) {
  box-sizing: border-box;
  height: 36px;
  min-height: 36px;
  padding: 4px 10px;
  border: 2px solid #121316 !important;
  border-radius: 8px !important;
  font-size: 13px;
  line-height: 1.4;
}
/* v0.4 SRC-09：新闻源弹窗输入框统一 2px 边框 / 圆角 8 / 高 36（含 number） */
.source-dialog .form-grid input:not([type="checkbox"]):not([type="radio"]),
.source-dialog .form-grid select {
  box-sizing: border-box;
  height: 36px;
  min-height: 36px;
  padding: 4px 10px;
  border: 2px solid #121316 !important;
  border-radius: 8px !important;
  font-size: 13px;
  line-height: 1.4;
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
  color: #15803D;
  background: #DCFCE7;
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
  width: min(1320px, calc(100vw - 32px));
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
.metric-card.metric-reads { background: linear-gradient(135deg, #1D4ED8 0%, #2563EB 100%); }
.metric-card.metric-uv { background: linear-gradient(135deg, #5B21B6 0%, #8B5CF6 100%); }
.metric-card.metric-ctr { background: linear-gradient(135deg, #B45309 0%, #F59E0B 100%); }
/* 模板里的简写类名（m-blue 等）与上面的语义类名等价：
   缺这组规则时卡片没有背景色，而 .metric-card 的 color 是 #FFFFFF，
   于是顶部汇总卡变成「白底白字」，看起来就像指标没有名称。 */
.metric-card.m-blue { background: linear-gradient(135deg, #1D4ED8 0%, #3B82F6 100%); }
.metric-card.m-green { background: linear-gradient(135deg, #0F766E 0%, #16A34A 100%); }
.metric-card.m-purple { background: linear-gradient(135deg, #5B21B6 0%, #8B5CF6 100%); }
.metric-card.m-amber { background: linear-gradient(135deg, #B45309 0%, #F59E0B 100%); }
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
.select-with-create > .relative {
  flex: 1;
  min-width: 0;
}
/* R15：工具栏内的 common/Select 与输入框等高，不撑破工具栏 */
.toolbar-select {
  flex: 0 0 auto;
  width: 140px;
}
.toolbar-select :deep(.select-trigger) {
  height: 38px;
  padding-block: 0;
  font-size: 13px;
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
.list-state { margin: 12px 0; padding: 12px 16px; border-radius: 8px; background: #fff; color: #5E6577; }
.list-error { color: #b42318; border: 1px solid #f3b7b2; }
@media (max-width: 640px) {
  .news-editor.news-editor-modal { width: calc(100vw - 16px); max-height: calc(100dvh - 16px); box-sizing: border-box; }
  .two-col-editor { grid-template-columns: minmax(0, 1fr); }
  .modal-scroll-body { max-height: calc(100dvh - 180px); }
  .chart-card.chart-wide { grid-column: span 1; }
  .chart-grid { grid-template-columns: minmax(0, 1fr); }
}

/* BI 分析图表网格 */
.chart-grid {
  display: grid;
  /* PRD：十张图两列排布，窄屏回落单列 */
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
}
.chart-card {
  border: 2px solid #121316;
  border-radius: 14px;
  background: #ffffff;
  padding: 16px;
  box-shadow: 4px 4px 0 #121316;
  min-width: 0;
}
.chart-card.chart-wide {
  grid-column: span 2;
}
.chart-card h3 {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 800;
}
.chart-box {
  position: relative;
  height: 260px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.chart-box > canvas {
  max-height: 260px;
}
.chart-empty {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: #6b7280;
}
/* v0.4 二次编辑弹窗：统一可读密度、遮罩与固定操作区 */
.content-modal-layer { position: fixed; inset: 0; z-index: 1400; display: flex; align-items: center; justify-content: center; padding: 16px; background: rgba(18,19,22,.55); }
/* SRC-07：新闻源弹窗的未保存确认要压在其上（1400） */
.modal-backdrop.source-close-guard-backdrop { z-index: 1500; }
.source-dialog { width: min(640px, 100%); max-height: calc(100dvh - 32px); overflow-y: auto; box-sizing: border-box; padding: 22px; border: 2px solid #121316; border-radius: 12px; background: #fff; box-shadow: 6px 6px 0 #121316; }
/* SRC-09 / SRC-02：滑块开关。原生 checkbox 视觉隐藏但保留可聚焦，轨道 span 负责视觉 */
.slider-switch { position: relative; display: inline-flex; align-items: center; gap: 10px; cursor: pointer; }
.slider-switch .switch-input { position: absolute; width: 1px; height: 1px; margin: 0; padding: 0; opacity: 0; overflow: hidden; clip: rect(0 0 0 0); }
.slider-switch .switch-track { position: relative; flex: 0 0 auto; width: 40px; height: 22px; border-radius: 9999px; background: #9ca3af; transition: background-color .15s ease; }
.slider-switch .switch-track::after { content: ""; position: absolute; top: 3px; left: 3px; width: 16px; height: 16px; border-radius: 50%; background: #fff; box-shadow: 0 1px 2px rgba(0,0,0,.25); transition: transform .15s ease; }
.slider-switch:has(.switch-input:checked) .switch-track { background: #2563eb; }
.slider-switch:has(.switch-input:checked) .switch-track::after { transform: translateX(18px); }
.slider-switch:has(.switch-input:focus-visible) .switch-track { outline: 2px solid #2563eb; outline-offset: 2px; }
.slider-switch .switch-text { color: #121316; }
:global(html.dark) .slider-switch .switch-track { background: #4b5563; }
:global(html.dark) .slider-switch:has(.switch-input:checked) .switch-track { background: #2563eb; }
:global(html.dark) .slider-switch .switch-text { color: #e5e7eb; }
.source-dialog .source-status-field .switch { margin: 0; padding: 0; border: 0; background: transparent; box-shadow: none; }
.source-dialog .form-grid .field-hint { color: #6b7280; font-size: 12px; font-weight: 500; }
.source-toolbar { justify-content: flex-end; }
.source-dialog .form-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 16px; }
.source-dialog .form-grid label { display: grid; gap: 7px; min-width: 0; font-size: 13px; font-weight: 700; }
.source-dialog .form-grid label:has(textarea) { grid-column: 1/-1; }
.dialog-close { flex: 0 0 36px; width: 36px; height: 36px; padding: 0; border: 2px solid #121316; border-radius: 8px; background: #fff; color: #121316; font-size: 24px; line-height: 1; cursor: pointer; }
.news-editor.news-editor-modal { box-sizing: border-box; padding: 0; }
.news-editor .two-col-editor { grid-template-columns: minmax(300px,38%) minmax(0,1fr); gap: 24px; }
.news-editor .two-col-editor > div { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.news-editor .two-col-editor label { display: grid; gap: 7px; font-size: 13px; font-weight: 700; }
.news-editor .two-col-editor input, .news-editor .two-col-editor textarea, .source-dialog input, .source-dialog textarea { box-sizing: border-box; width: 100%; min-height: 42px; padding: 9px 11px; border: 1.5px solid #9aa1b1; border-radius: 8px; font: inherit; line-height: 1.5; }
.news-editor .two-col-editor textarea.news-summary-input { min-height: 84px; resize: vertical; line-height: 1.65; }
.news-editor .editor-pane { min-height: 220px; }
.news-editor .modal-fixed-footer { flex-shrink: 0; }
.featured-toggle, .ref-news-item input[type="checkbox"], .news-row input[type="checkbox"], .article-row input[type="checkbox"] { flex: 0 0 16px !important; width: 16px !important; height: 16px !important; margin: 0 !important; }
.featured-cell label { display: inline-flex; align-items: center; justify-content: center; flex: none; gap: 7px; }
.metric-info { display: flex; flex-direction: column; justify-content: center; gap: 4px; }
.metric-info small { display: block; color: rgba(255,255,255,.92); font-size: 12px; font-weight: 700; line-height: 1.3; }
.metric-info strong { display: block; line-height: 1.15; }
.bi-error { display: flex;
  align-items: center;
  gap: 12px;
  margin: 0 0 16px;
  font-weight: 700;
  color: #b91c1c;
}

/* ===== v0.4 P0fix：新闻 / 文章编辑弹窗统一 880 两栏壳（左配置 / 右正文） ===== */
.news-editor.news-editor-modal,
.modal-drawer.article-editor-modal {
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  width: min(1320px, calc(100vw - 32px));
  height: calc(100dvh - 48px);
  max-height: 900px;
  min-height: 0;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 2px solid #121316;
  border-radius: 12px;
  background: #ffffff;
  box-shadow: 6px 6px 0 #121316;
}
.news-editor.news-editor-modal > .modal-fixed-header,
.news-editor.news-editor-modal > .modal-fixed-header,
.modal-drawer.article-editor-modal > .modal-drawer-header {
  flex: 0 0 auto;
  margin: 0;
  padding: 10px 22px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 2px solid #121316;
}
.news-editor.news-editor-modal > .modal-fixed-header > div,
.modal-drawer.article-editor-modal > .modal-drawer-header > div {
  display: flex;
  align-items: baseline;
  gap: 14px;
  min-width: 0;
}
.news-editor.news-editor-modal > .modal-fixed-header h2,
.modal-drawer.article-editor-modal > .modal-drawer-header h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 800;
}
.news-editor.news-editor-modal > .modal-fixed-header p,
.modal-drawer.article-editor-modal > .modal-drawer-header p {
  margin: 0;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.news-editor.news-editor-modal > .modal-fixed-footer,
.modal-drawer.article-editor-modal > .modal-drawer-footer {
  flex: 0 0 auto;
  margin: 0;
  padding: 12px 22px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  border-top: 2px solid #121316;
  background: #ffffff;
}
/* PRD：新闻编辑弹窗底部按钮 36px 高 / 8px 圆角（局部覆盖全局 .button 的 42px / 10px） */
.news-editor.news-editor-modal > .modal-fixed-footer button {
  box-sizing: border-box;
  height: 36px;
  min-height: 36px;
  padding: 0 18px;
  border-radius: 8px;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

/* 译文任务：横向折叠条，独立于两栏之上，天然不与正文编辑器垂直重叠 */
.news-translation-panel {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0 22px;
  background: #F8FAFF;
  border-bottom: 1.5px solid #e3e6ee;
}
.news-trans-toggle {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 8px 0;
  background: none;
  border: 0;
  cursor: pointer;
  font: inherit;
  text-align: left;
}
.news-trans-toggle .config-title { margin: 0; }
.news-trans-summary { font-size: 12px; color: #5E6577; }
.news-trans-toggle-text { margin-left: auto; font-size: 12px; font-weight: 800; color: #2563eb; }
.news-translation-tasks {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 0 0 10px;
  max-height: 84px;
  overflow-y: auto;
}
.news-translation-task {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border: 1.5px solid #121316;
  border-radius: 999px;
  background: #ffffff;
  font-size: 12px;
}

/* 新闻两栏主体 */
.news-editor-body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  /* 解除 .modal-scroll-body 的 92vh-190px 封顶：弹窗是固定高 flex 容器，由 flex 分配剩余高度 */
  max-height: none;
  padding: 14px 22px 0;
  /* v0.4：正文编辑区有最小高度，小视口下内容会高于弹窗主体，必须可滚动，
     否则溢出部分会被 sticky 页脚盖住（Playwright/用户都点不到编辑区） */
  overflow-y: auto;
  overflow-x: hidden;
}
.news-editor-body .two-col-editor {
  flex: 1 1 auto;
  display: grid;
  /* v0.4 板块重排：左=内容（基础信息+正文），右=元信息（发布/封面/SEO） */
  grid-template-columns: minmax(0, 1fr) 380px;
  gap: 20px;
  width: 100%;
  min-height: 0;
}
.news-editor-body .two-col-editor > div {
  min-width: 0;
  min-height: 0;
  gap: 12px;
  align-items: stretch;
}
.news-editor .editor-config-col {
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 6px;
}
.news-editor .editor-config-col .form-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.news-editor .editor-main-col { display: flex; flex-direction: column; }
/* v0.4 板块重排：右列元信息分区卡片 */
.news-editor .editor-config-col > .config-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  box-shadow: 3px 3px 0 #121316;
}
.news-editor .editor-config-col > .config-block > .config-title {
  margin: 0;
  font-size: 13px;
  font-weight: 800;
  letter-spacing: .02em;
}
.news-editor .editor-main-col > .config-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.news-editor .config-block-inner {
  border: 1.5px dashed #9aa1b1;
  border-radius: 10px;
  padding: 10px;
  gap: 8px;
}
.news-editor .editor-label {
  flex: 0 0 auto;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 800;
}
.news-editor .editor-shell {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
}
.news-editor .editor-shell :deep(.rich-text-editor),
.article-editor-modal .article-editor-shell :deep(.rich-text-editor) {
  flex: 1 1 auto;
  min-height: 0;
  max-height: none;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.news-editor .editor-shell :deep(.rich-text-editor .rich-text-toolbar),
.article-editor-modal .article-editor-shell :deep(.rich-text-editor .rich-text-toolbar) {
  flex: 0 0 auto;
}
.news-editor .editor-shell {
  /* v0.4 R16：左栏字段较多，小视口下正文会被挤到几十像素；
     给编辑器一个可用下限，超出部分由 .news-editor-body 滚动兜底 */
  min-height: 360px;
}
.news-editor .editor-shell :deep(.rich-text-editor .rich-text-content) {
  flex: 1 1 auto;
  /* v0.4：正文区必须有最小高度，否则工具栏占满后 content 塌成 0/24px，
     会被下方 sticky 页脚与「更多格式」工具栏盖住 */
  min-height: 240px;
  max-height: none;
  overflow-y: auto;
}
.article-editor-modal .article-editor-shell :deep(.rich-text-editor .rich-text-content) {
  flex: 1 1 auto;
  min-height: 0;
  max-height: none;
  overflow-y: auto;
}
.news-editor .editor-shell :deep(.rich-text-editor .rich-text-hint),
.article-editor-modal .article-editor-shell :deep(.rich-text-editor .rich-text-hint) {
  display: none;
}

/* 同步语言：chip 式多选（label 文本保持英文母语名，中文名走 ::after 不进入可测文本） */
.news-editor .two-col-editor .locale-checkboxes {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.news-editor .two-col-editor .locale-checkbox {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border: 1.5px solid #121316;
  border-radius: 999px;
  background: #ffffff;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}
.news-editor .two-col-editor .locale-checkbox input[type="checkbox"] {
  flex: 0 0 15px;
  width: 15px;
  height: 15px;
  min-height: 0;
  margin: 0;
  padding: 0;
  accent-color: #121316;
}
.field-error { margin: -6px 0 0; color: #b42318; font-size: 12px; font-weight: 700; }
.required-star { margin-left: 4px; color: #b42318; }

/* 文章两栏主体 */
.article-editor-modal .modal-drawer-body {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  padding: 14px 22px 0;
  overflow: hidden;
}
.article-editor-columns {
  flex: 1 1 auto;
  display: grid;
  /* v0.4 板块重排：左=内容（宽），右=元信息（380px） */
  grid-template-columns: minmax(0, 1fr) 380px;
  gap: 20px;
  width: 100%;
  min-height: 0;
}
.article-editor-columns > .editor-col-left {
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 6px;
}
.article-editor-columns > .editor-col-right {
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.article-editor-modal .article-editor-shell {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
}
/* v0.4 板块重排：正文块已移入左列（v0.4 P0 返工：左内容/右元信息），
   这里让正文编辑器在左列内保持可编辑高度；右列卡片较多时自身滚动。 */
.article-editor-columns > .editor-col-left > .meta-card > .meta-card-body > .body-field-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-height: 420px;
}
.article-editor-columns > .editor-col-left > .meta-card > .meta-card-body > .body-field-group > .article-editor-shell {
  flex: 1 1 auto;
  min-height: 360px;
}
.article-editor-columns > .editor-col-right {
  position: sticky;
  top: 0;
  max-height: calc(92vh - 150px);
  overflow-y: auto;
  padding-right: 4px;
}
.article-editor-modal .article-editor-shell :deep(textarea.html-source-area),
.article-editor-modal .article-editor-shell :deep(.body-render-preview) {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

@media (max-width: 780px) {
  .news-editor.news-editor-modal,
  .modal-drawer.article-editor-modal {
    width: calc(100vw - 24px);
    height: calc(100dvh - 24px);
    max-height: none;
  }
  .news-editor-body { padding: 12px 14px 0; }
  .news-editor-body .two-col-editor,
  .article-editor-columns { grid-template-columns: minmax(0, 1fr); gap: 12px; }
  .news-editor .editor-config-col,
  .article-editor-columns > .editor-col-left { max-height: 42%; }
  .news-editor .editor-main-col,
  .article-editor-columns > .editor-col-right { min-height: 300px; }
}

/* ===== v0.4 P0fix：左栏字段名纯文本（label 文本 = 字段名）+ 自绘下拉/词条 ===== */
.news-editor .form-grid > label {
  position: relative;
}
.news-editor .form-required-legend {
  margin: 0 0 2px;
  padding: 0;
  font-size: 11px;
  font-weight: 600;
  color: #8b8f96;
  letter-spacing: 0.02em;
}
.news-editor .combo-trigger {
  cursor: pointer;
  width: 100%;
  box-sizing: border-box;
  font-weight: 500;
  color: #121316;
  background: #ffffff;
  text-overflow: ellipsis;
}
.news-editor .combo-trigger[aria-expanded="true"] {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.16);
}
.news-editor .combo-menu {
  position: absolute;
  z-index: 40;
  left: 0;
  right: 0;
  top: calc(100% + 4px);
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 200px;
  overflow-y: auto;
  padding: 6px;
  border: 1.5px solid #121316;
  border-radius: 10px;
  background: #ffffff;
  box-shadow: 0 14px 30px rgba(17, 19, 24, 0.16);
}
.news-editor .combo-option {
  display: flex;
  align-items: center;
  width: 100%;
  padding: 6px 8px;
  border: 1.5px solid transparent;
  border-radius: 8px;
  background: #ffffff;
  font-size: 12px;
  font-weight: 500;
  color: #121316;
  text-align: left;
  cursor: pointer;
}
.news-editor .combo-option:hover,
.news-editor .combo-option[aria-selected="true"] {
  border-color: #121316;
  background: #f4f6fb;
}
.news-editor .tag-input {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  width: 100%;
  box-sizing: border-box;
  padding: 5px 6px;
  border: 1.5px solid #121316;
  border-radius: 10px;
  background: #ffffff;
}
.news-editor .tag-input input {
  flex: 1 1 80px;
  min-width: 70px;
  padding: 2px 4px;
  border: 0 !important;
  border-radius: 0;
  box-shadow: none !important;
  background: transparent;
  font-size: 12px;
}
.news-editor .tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px 2px 8px;
  border: 1.5px solid #121316;
  border-radius: 999px;
  background: #eef2ff;
  font-size: 11px;
  font-weight: 600;
  color: #121316;
  white-space: nowrap;
}
.news-editor .tag-chip::before {
  content: attr(data-tag);
}
.news-editor .tag-chip .tag-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  padding: 0;
  border: 0;
  border-radius: 50%;
  background: rgba(18, 19, 22, 0.12);
  font-size: 10px;
  line-height: 1;
  color: #121316;
  cursor: pointer;
}
.news-editor .tag-chip .tag-remove::before {
  content: "\00d7";
}
.news-editor .tag-chip .tag-remove:hover {
  background: #ef4444;
  color: #ffffff;
}

/* ==================== v0.4 W2 AE 修复：尺寸 / 布局 / 焦点 / 控件 ==================== */
/* AE-01：入场只用淡入，不再 scale —— transform:scale 会让 getBoundingClientRect 量到的
   宽度小于真实宽度，也会把内部 sticky 定位的包含块换掉 */
.news-editor.news-editor-modal,
.modal-drawer.article-editor-modal {
  animation: modalFadeIn 0.18s ease-out;
}
@keyframes modalFadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* AE-02：两栏各自吃满弹窗高度。缺 grid-template-rows 时隐式行是 auto，
   右栏高度被内容压缩到 ~95px，正文编辑器没有可视化高度 */
.article-editor-modal .article-editor-columns {
  display: grid;
  /* v0.4 板块重排：左=内容（宽），右=元信息（380px） */
  grid-template-columns: minmax(0, 1fr) 380px;
  grid-template-rows: minmax(0, 1fr);
  align-items: stretch;
  gap: 20px;
  width: 100%;
  min-height: 0;
  flex: 1 1 auto;
}
.article-editor-modal .article-editor-columns > .editor-col-left,
.article-editor-modal .article-editor-columns > .editor-col-right {
  min-width: 0;
  min-height: 0;
}
.article-editor-modal .article-editor-columns > .editor-col-right {
  position: static;
  align-self: stretch;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow: hidden;
}
/* 右列内部：标题/摘要固定高度，正文块吃掉剩余高度并在编辑器内部滚动 */
.article-editor-modal .article-editor-columns > .editor-col-right > .body-field-group {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.article-editor-modal .article-editor-columns > .editor-col-right > .body-field-group > .article-editor-shell {
  flex: 1 1 auto;
  min-height: 0;
}
@media (max-width: 960px) {
  .article-editor-modal .article-editor-columns {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: none;
    align-items: start;
  }
  .article-editor-modal .article-editor-columns > .editor-col-right {
    overflow: visible;
  }
}

/* AE-12：多行文本框固定可用高度，超出内部滚动，不把弹窗顶长 */
.article-editor-modal .short-text-area,
.article-editor-modal .meta-card textarea {
  box-sizing: border-box;
  height: 104px;
  min-height: 104px;
  max-height: 240px;
  padding: 10px 12px;
  line-height: 1.6;
  resize: vertical;
  overflow-y: auto;
}

/* AE-19：弹窗内所有可聚焦元素都有可见焦点环（覆盖各处 outline:none） */
.article-editor-modal :focus,
.article-editor-modal :focus-visible {
  outline: 2px solid #2563eb !important;
  outline-offset: 2px !important;
}
.article-editor-modal .rich-text-content:focus,
.article-editor-modal .rich-text-content:focus-visible {
  outline: 2px solid #2563eb !important;
  outline-offset: -2px !important;
}

/* AE-13 / AE-14：精选改为滑块式开关（覆盖遗留的 16px 复选框尺寸规则） */
.featured-cell .featured-toggle {
  display: inline-flex !important;
  align-items: center;
  flex: 0 0 auto !important;
  width: 46px !important;
  height: 26px !important;
  min-width: 46px !important;
  margin: 0 auto !important;
  padding: 0 !important;
  border: 2px solid #121316;
  border-radius: 999px;
  background: #cbd5e1;
  position: relative;
  cursor: pointer;
  box-shadow: none;
  transition: background-color 0.15s ease;
}
.featured-cell .featured-toggle .switch-knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: #ffffff;
  border: 2px solid #121316;
  transition: left 0.15s ease;
}
.featured-cell .featured-toggle.on {
  background: #2563eb;
}
.featured-cell .featured-toggle.on .switch-knob {
  left: 22px;
}
.featured-cell .featured-toggle:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.featured-cell .featured-toggle:focus-visible {
  outline: 2px solid #2563eb;
  outline-offset: 2px;
}

/* AE-11：SEO 覆盖确认卡片 */
.seo-overwrite-confirm {
  margin-top: 10px;
  padding: 12px 14px;
  border: 2px solid #121316;
  border-radius: 10px;
  background: #fffbe6;
  box-shadow: 3px 3px 0 #121316;
}
.seo-overwrite-confirm p {
  margin: 0 0 10px;
  font-size: 13px;
  font-weight: 600;
}
.seo-overwrite-confirm .action-confirm-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

/* 覆盖确认是「弹窗级」问答：挂到 body 上居中浮层，
   与编辑器底部的「取消」在结构与读屏上下文上都分离 */
.seo-overwrite-overlay {
  position: fixed;
  z-index: 3000;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: min(420px, calc(100vw - 48px));
  margin: 0;
  padding: 18px 20px;
  border: 2.5px solid #121316;
  border-radius: 14px;
  background: #fffbe6;
  box-shadow: 6px 6px 0 #121316;
}
.seo-overwrite-overlay .action-confirm-actions {
  margin-top: 4px;
}
.seo-overwrite-overlay .button,
.seo-overwrite-overlay .secondary-button {
  min-width: 84px;
}

/* 未保存修改确认框 */
.confirm-dialog {
  width: min(420px, calc(100vw - 48px));
  padding: 22px;
  border: 2.5px solid #121316;
  border-radius: 14px;
  background: #ffffff;
  box-shadow: 6px 6px 0 #121316;
}
.confirm-dialog h3 {
  margin: 0 0 8px;
  font-size: 16px;
  font-weight: 800;
}
.confirm-dialog .muted {
  margin: 0 0 16px;
  font-size: 13px;
  line-height: 1.6;
  color: #4b5563;
}
.confirm-dialog-actions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
}

/* 字段内联提示 */
.field-hint {
  font-size: 11.5px;
  line-height: 1.5;
  color: #6b7280;
}
.field-hint.hint-warn {
  color: #b42318;
  font-weight: 600;
}

</style>

<template>
  <!-- ==================== CCAI FULLSCREEN NEO-LAB HOME (/) ==================== -->
  <div
    v-if="isHome"
    class="ccai-home-viewport"
    :class="[
      currentMode === 'order' ? 'mode-order' : 'mode-chaos',
      { 'is-morphing': isMorphing }
    ]"
  >
    <div class="bg-grid"></div>
    <canvas
      ref="morphCanvasRef"
      id="morph-canvas"
      :class="{ 'canvas-hidden': !isMorphing && !hasActiveSparks }"
    ></canvas>

    <main class="screen">
      <!-- 1. 顶部三栏对称导航 -->
      <header class="header-bar">
        <router-link :to="localePath('')" class="brand">
          <div class="brand-logo" aria-hidden="true">
            <svg viewBox="0 0 24 24" class="brand-stamp-icon" width="20" height="20">
              <path d="M4 17L12 4L20 17H4Z" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linejoin="round" />
              <circle cx="12" cy="13" r="2.2" fill="currentColor" />
            </svg>
          </div>
          <span class="brand-text">CCAI</span>
        </router-link>

        <div class="breaker-wrap">
          <div class="breaker-switch">
            <button
              type="button"
              class="sw-btn chaos"
              :class="{ active: currentMode === 'chaos' }"
              @click="morphTo('chaos')"
            >
              <span>🤯</span>
              <span>{{ isZh ? '直连官方' : 'Direct' }}</span>
            </button>
            <button
              type="button"
              class="sw-btn order"
              :class="{ active: currentMode === 'order' }"
              @click="morphTo('order')"
            >
              <span>⚡</span>
              <span>{{ isZh ? '使用 CCAI' : 'With CCAI' }}</span>
            </button>
          </div>
        </div>

        <div class="cta-wrap">
          <router-link :to="localePath('news')" class="nav-sub-btn">
            <span class="nav-dot dot-live"></span>
            <span>{{ isZh ? '新闻动态' : 'News' }}</span>
          </router-link>
          <router-link :to="localePath('articles')" class="nav-sub-btn">
            <span class="nav-dot dot-doc"></span>
            <span>{{ isZh ? '深度解析' : 'Deep Dive' }}</span>
          </router-link>

          <div class="lang-switcher">
            <button
              type="button"
              class="lang-switch-btn"
              aria-haspopup="listbox"
              :aria-expanded="localeMenuOpen"
              :aria-label="currentLocaleName"
              @click="toggleLocaleMenu"
            >
              <span class="lang-globe" aria-hidden="true">◎</span>
              <span>{{ currentLocaleName }}</span>
              <span class="lang-caret" aria-hidden="true">▾</span>
            </button>
            <ul v-if="localeMenuOpen" class="lang-menu" role="listbox">
              <li v-for="option in availableLocales" :key="option.code">
                <button
                  type="button"
                  class="lang-option"
                  role="option"
                  :aria-selected="option.code === locale"
                  :class="{ active: option.code === locale }"
                  @click="switchLocale(option.code)"
                >
                  {{ option.name }}
                </button>
              </li>
            </ul>
          </div>

          <router-link :to="consoleTarget" class="cta-btn">
            {{ isZh ? '进入控制台 →' : 'Console →' }}
          </router-link>
        </div>
      </header>

      <!-- 2. 极简主标题 -->
      <section class="hero-title-area">
        <h1 class="hero-headline">
          <span>{{ headlinePrefix }}</span>
          <span class="hl-pill">{{ headlinePill }}</span>
        </h1>
        <p class="hero-sub">
          {{
            isZh
              ? '兼容 OpenAI、Anthropic、Gemini 接口格式，客户端只需改 Base URL 和 API Key'
              : 'Works with OpenAI, Anthropic and Gemini API formats. Just change the base URL and API key.'
          }}
        </p>
      </section>

      <!-- 3. 左-中-右顶底严丝合缝对齐舞台 -->
      <section class="stage-container">
        <div ref="stageGridRef" class="stage-grid" id="stageGrid">
          <svg ref="linesSvgRef" id="lines-svg"></svg>

          <!-- ==================== 左翼：常用客户端 ==================== -->
          <div class="col-wrapper wing-left">
            <div class="col-header">
              <span>{{ isZh ? '◀ 常用客户端' : '◀ CLIENTS' }}</span>
              <span>Base URL + Key</span>
            </div>

            <div class="cards-stack">
              <div
                v-for="(client, idx) in clientItems"
                :key="client.id"
                :id="'left-' + idx"
                class="unit-card morph-unit"
                :class="{
                  'is-card-charging': hoveredIdx === idx,
                  'is-card-charged-hold': hoveredIdx === idx && isChargedDone,
                  'snap-reborn': snapRebornActive
                }"
                @mouseenter="onClientEnter(idx)"
                @mouseleave="onClientLeave"
                @click="toggleMode"
              >
                <!-- 全卡片 3s 蓄力充能流体背景层 + 底部进度槽 -->
                <div class="card-charge-wave"></div>
                <div class="charge-bar-track">
                  <div class="charge-bar-fill"></div>
                </div>

                <div class="u-left">
                  <div class="u-icon" :style="{ background: currentMode === 'order' ? '#CCFF00' : client.bg }">
                    <svg v-if="idx === 0" viewBox="0 0 24 24" class="brand-svg">
                      <path
                        fill="#D97757"
                        stroke="#121316"
                        stroke-width="1.5"
                        d="M12 2L14.2 8.4L20.6 6.2L16.4 11.5L22 15.2L15.3 15.8L16.8 22L12 17.2L7.2 22L8.7 15.8L2 15.2L7.6 11.5L3.4 6.2L9.8 8.4L12 2Z"
                      />
                    </svg>
                    <svg v-else-if="idx === 1" viewBox="0 0 24 24" class="brand-svg">
                      <polygon points="12,2 21,7 21,17 12,22 3,17 3,7" fill="#121316" stroke="#121316" stroke-width="1.5" />
                      <path d="M7 12L17 7L13 17L11 13L7 12Z" fill="#CCFF00" />
                    </svg>
                    <svg v-else-if="idx === 2" viewBox="0 0 24 24" class="brand-svg">
                      <rect x="3" y="5" width="18" height="14" rx="4" fill="#3B82F6" stroke="#121316" stroke-width="1.8" />
                      <circle cx="8.5" cy="12" r="2" fill="#CCFF00" />
                      <circle cx="15.5" cy="12" r="2" fill="#CCFF00" />
                      <path d="M9 16H15" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round" />
                    </svg>
                    <svg v-else viewBox="0 0 24 24" class="brand-svg">
                      <path d="M8 15C8 11 11 6 16 4" stroke="#121316" stroke-width="2.2" stroke-linecap="round" fill="none" />
                      <path d="M16 15C16 11 14 7 16 4" stroke="#121316" stroke-width="2.2" stroke-linecap="round" fill="none" />
                      <circle cx="7.5" cy="16.5" r="4.2" fill="#FF3B30" stroke="#121316" stroke-width="1.8" />
                      <circle cx="16.5" cy="16.5" r="4.2" fill="#FF2D55" stroke="#121316" stroke-width="1.8" />
                    </svg>
                  </div>

                  <div class="u-text">
                    <div class="u-name">{{ client.name }}</div>
                    <div class="u-desc">{{ isZh ? client.descZh : client.descEn }}</div>
                  </div>
                </div>

                <!-- 文案始终保持原样，不因充能而改变 -->
                <span class="u-tag">
                  {{ currentMode === 'chaos' ? (isZh ? client.badTagZh : client.badTagEn) : (isZh ? client.goodTagZh : client.goodTagEn) }}
                </span>
              </div>
            </div>
          </div>

          <!-- ==================== 中央核心：CCAI 统一网关 ==================== -->
          <div class="col-wrapper">
            <div class="col-header center">
              <span>◆ CCAI ◆</span>
            </div>

            <div
              id="center-core"
              class="core-box morph-unit"
              :class="{
                'core-overdrive': hoveredIdx !== null,
                'snap-reborn': snapRebornActive
              }"
            >
              <!-- 直连官方 -->
              <div v-if="currentMode === 'chaos'" class="chaos-content">
                <div>
                  <div class="core-top-row">
                    <span class="core-badge bad">⚠️ {{ isZh ? '直连官方' : 'DIRECT' }}</span>
                    <span class="core-status-dot">
                      {{ hoveredIdx !== null ? (isZh ? '请求失败' : 'Request failed') : (isZh ? '常见报错' : 'Common errors') }}
                    </span>
                  </div>
                  <h2 class="core-title" style="margin-top: 10px">
                    {{ isZh ? '直连官方 API 常遇到的问题' : 'Common problems with official APIs' }}
                  </h2>
                </div>

                <div class="pain-list">
                  <div class="pain-item" :class="{ 'pain-highlight': hoveredIdx === 0 || hoveredIdx === 1 }">
                    <span>{{ isZh ? '💳 海外卡支付失败，账号容易被封' : '💳 Cards declined, accounts banned' }}</span>
                    <span class="pain-code">403 BANNED</span>
                  </div>
                  <div class="pain-item" :class="{ 'pain-highlight': hoveredIdx === 3 }">
                    <span>{{ isZh ? '🍝 每家单独注册、单独管理 Key' : '🍝 A separate account and key per vendor' }}</span>
                    <span class="pain-code">N KEYS</span>
                  </div>
                  <div class="pain-item" :class="{ 'pain-highlight': hoveredIdx === 2 }">
                    <span>{{ isZh ? '⏳ 低等级账号额度小，高峰常报 429' : '⏳ Low tier quotas, frequent 429s' }}</span>
                    <span class="pain-code">429 LIMIT</span>
                  </div>
                </div>

                <button type="button" class="big-trigger-btn to-order" @click="morphTo('order')">
                  <span>{{ isZh ? '换成 CCAI 看看 →' : 'Switch to CCAI →' }}</span>
                </button>
              </div>

              <!-- CCAI 爽局 -->
              <div v-else class="order-content">
                <div>
                  <div class="core-top-row">
                    <span class="core-badge good">⚡ CCAI</span>
                    <span class="core-status-dot" style="color: #16A34A">
                      {{ hoveredIdx !== null ? '● 200 OK' : (isZh ? '● 统一入口' : '● One endpoint') }}
                    </span>
                  </div>
                  <h2 class="core-title" style="margin-top: 10px">
                    {{ isZh ? '一个地址，一个 Key' : 'One URL, one key' }}
                  </h2>
                </div>

                <div class="order-mid-group">
                  <div class="endpoint-pill" @click="copyEndpoint">
                    <span>BASE_URL: {{ displayBaseUrl }}/v1</span>
                    <span class="copy-tag">
                      {{ baseCopied ? (isZh ? '✅ 已复制' : '✅ Copied') : (isZh ? '复制地址' : 'Copy URL') }}
                    </span>
                  </div>

                  <div class="code-box">
                    <div>base_url = <span class="c-hl">"{{ displayBaseUrl }}/v1"</span></div>
                    <div>api_key  = <span class="c-hl">"sk-xxxxxxxx"</span></div>
                    <div>
                      model    =
                      <span class="c-model" @click.stop="cycleModel">"{{ activeModelName }}" ↻</span>
                    </div>
                  </div>
                </div>

                <button type="button" class="big-trigger-btn to-chaos" @click="morphTo('chaos')">
                  <span>{{ isZh ? '← 对比直连官方' : '← Compare with direct' }}</span>
                </button>
              </div>
            </div>
          </div>

          <!-- ==================== 右翼：上游模型 ==================== -->
          <div class="col-wrapper wing-right">
            <div class="col-header">
              <span>UPSTREAM</span>
              <span>{{ isZh ? '主流模型 ▶' : 'MODELS ▶' }}</span>
            </div>

            <div class="cards-stack">
              <div
                v-for="(model, idx) in modelItems"
                :key="model.id"
                :id="'right-' + idx"
                class="unit-card morph-unit model-card-wrap"
                :class="{
                  'is-card-charging': hoveredIdx === idx && isChargingPhase,
                  'is-chaos-fractured': currentMode === 'chaos' && hoveredIdx === idx && isChargedDone,
                  'is-order-celebrating': currentMode === 'order' && hoveredIdx === idx && isChargedDone,
                  'snap-reborn': snapRebornActive
                }"
                @click="toggleMode"
              >
                <!-- 左半截（3s 充能阶段晃动蓄力；3s 结束后立即停止充能晃动，苦海态裂开 / 爽局态从四边往外迸发烟花） -->
                <div class="split-piece piece-left">
                  <div class="card-charge-wave"></div>
                  <div class="charge-bar-track">
                    <div class="charge-bar-fill"></div>
                  </div>

                  <div class="piece-inner">
                    <div class="u-left">
                      <div class="u-icon" :style="{ background: currentMode === 'order' ? '#CCFF00' : model.bg }">
                        <!-- Anthropic Claude -->
                        <svg v-if="model.iconType === 'anthropic'" viewBox="0 0 24 24" class="brand-svg">
                          <path d="M12 2V22M2 12H22M4.9 4.9L19.1 19.1M19.1 4.9L4.9 19.1" stroke="#D97757" stroke-width="2.8" stroke-linecap="round" />
                        </svg>
                        <!-- Google Gemini -->
                        <svg v-else-if="model.iconType === 'gemini' || model.iconType === 'google'" viewBox="0 0 24 24" class="brand-svg">
                          <path d="M12 2C12 7.5 16.5 12 22 12C16.5 12 12 16.5 12 22C12 16.5 7.5 12 2 12C7.5 12 12 7.5 12 2Z" fill="#1A73E8" stroke="#121316" stroke-width="1.6" />
                        </svg>
                        <!-- OpenAI GPT-6 -->
                        <svg v-else-if="model.iconType === 'openai'" viewBox="0 0 24 24" class="brand-svg">
                          <circle cx="12" cy="12" r="8.5" fill="none" stroke="#10A37F" stroke-width="2.4" />
                          <circle cx="12" cy="12" r="3.2" fill="#10A37F" />
                        </svg>
                        <!-- 其他主流旗舰 (Grok / DeepSeek / Kimi) -->
                        <svg v-else viewBox="0 0 24 24" class="brand-svg">
                          <path d="M5 19L19 5" stroke="#121316" stroke-width="3" stroke-linecap="round" />
                          <circle cx="12" cy="12" r="8" fill="none" stroke="#6366F1" stroke-width="2.2" />
                        </svg>
                      </div>
                      <div class="u-text">
                        <div class="u-name">{{ model.name }}</div>
                        <div class="u-desc">{{ isZh ? model.descZh : model.descEn }}</div>
                      </div>
                    </div>

                    <!-- 文案始终保持原样，不因充能而改变 -->
                    <span class="u-tag">
                      {{ currentMode === 'chaos' ? (isZh ? model.badTagZh : model.badTagEn) : (isZh ? model.goodTagZh : model.goodTagEn) }}
                    </span>
                  </div>
                </div>

                <!-- 右半截（仅在苦海 3s 充能结束断裂时显现，向右上错位弹开） -->
                <div class="split-piece piece-right" aria-hidden="true">
                  <div class="piece-inner">
                    <div class="u-left">
                      <div class="u-icon" :style="{ background: model.bg }"></div>
                      <div class="u-text">
                        <div class="u-name">{{ model.name }}</div>
                        <div class="u-desc">{{ isZh ? model.descZh : model.descEn }}</div>
                      </div>
                    </div>
                    <span class="u-tag">{{ isZh ? model.badTagZh : model.badTagEn }}</span>
                  </div>
                </div>

                <!-- 苦海模式下：3 秒充能完毕断裂成两半，中间裂口迸出致命痛点！ -->
                <div
                  v-if="currentMode === 'chaos' && hoveredIdx === idx && isChargedDone"
                  class="fracture-crack-overlay"
                >
                  <svg viewBox="0 0 360 32" class="crack-zig-svg" preserveAspectRatio="none">
                    <polyline
                      points="0,16 45,4 92,26 148,5 195,28 248,6 305,25 360,12"
                      fill="none"
                      stroke="#FF3B30"
                      stroke-width="3.5"
                    />
                  </svg>
                  <div class="fracture-pain-pill">
                    💥 {{ isZh ? model.fracturePainZh : model.fracturePainEn }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

    </main>

    <!-- ==================== 首页内容区块：最新资讯 + 深度解析 ==================== -->
    <section class="home-content-strip" data-testid="home-content">
      <div class="hc-grid">
        <div class="hc-block home-news-block">
          <div class="hc-head">
            <div class="hc-label font-mono">
              <span class="pulse-dot"></span>
              {{ isZh ? '最新资讯' : 'LATEST NEWS' }}
            </div>
            <router-link :to="localePath('news')" class="hc-more">
              {{ isZh ? '查看更多' : 'View all' }} →
            </router-link>
          </div>
          <ul v-if="homeNews.length > 0" class="hc-list">
            <li v-for="item in homeNews" :key="'home-news-' + item.id">
              <router-link :to="itemPath(item, 'news')" class="hc-item home-news-item">
                <div class="hc-meta font-mono">
                  <span>{{ item.displayCategory || item.category || item.sourceName || 'CCAI' }}</span>
                  <span>{{ formatDate(item.publishedAt || item.createdAt) }}</span>
                </div>
                <h3 class="hc-title">{{ item.displayTitle || item.title }}</h3>
                <p class="hc-excerpt">{{ truncateText(item.displaySummary || item.summary || item.excerpt, 96) }}</p>
              </router-link>
            </li>
          </ul>
          <div v-else class="hc-empty font-mono">{{ isZh ? '暂无已发布资讯' : 'No published news yet' }}</div>
        </div>

        <div class="hc-block home-article-block">
          <div class="hc-head">
            <div class="hc-label font-mono">
              <span class="pulse-dot"></span>
              {{ isZh ? '深度解析' : 'DEEP DIVE' }}
            </div>
            <router-link :to="localePath('articles')" class="hc-more">
              {{ isZh ? '查看更多' : 'View all' }} →
            </router-link>
          </div>
          <ul v-if="homeArticles.length > 0" class="hc-list">
            <li v-for="item in homeArticles" :key="'home-article-' + item.id">
              <router-link :to="itemPath(item, 'articles')" class="hc-item home-article-item">
                <div class="hc-meta font-mono">
                  <span>{{ item.displayCategory || item.category || 'CCAI' }}</span>
                  <span>{{ formatDate(item.publishedAt || item.createdAt) }}</span>
                </div>
                <h3 class="hc-title">{{ item.displayTitle || item.title }}</h3>
                <p class="hc-excerpt">{{ truncateText(item.displaySummary || item.summary || item.excerpt, 96) }}</p>
              </router-link>
            </li>
          </ul>
          <div v-else class="hc-empty font-mono">{{ isZh ? '暂无已发布文章' : 'No published articles yet' }}</div>
        </div>
      </div>
    </section>
  </div>

  <!-- ==================== CCAI NEO-LAB PUBLIC CONTENT (/news, /articles, detail) ==================== -->
  <div v-else class="neo-public-viewport">
    <div class="bg-grid"></div>

    <header class="header-bar neo-sub-header-bar">
      <!-- 品牌标识与面包屑 -->
      <div class="sub-header-left">
        <router-link :to="localePath('')" class="brand">
          <div class="brand-logo" aria-hidden="true">
            <svg viewBox="0 0 24 24" class="brand-stamp-icon" width="20" height="20">
              <path d="M4 17L12 4L20 17H4Z" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linejoin="round" />
              <circle cx="12" cy="13" r="2.2" fill="currentColor" />
            </svg>
          </div>
          <span class="brand-text">CCAI</span>
        </router-link>
        <span class="sub-nav-divider">/</span>
        <div class="sub-nav-indicator">
          <span class="indicator-dot"></span>
          <span class="font-mono indicator-title">
            {{ kind === 'news' ? (isZh ? '实时新闻资讯' : 'NEWSWIRE') : (isZh ? '深度解析' : 'DEEP DIVE') }}
          </span>
        </div>
      </div>

      <!-- 中心：纯正 Neo-Lab 硬核双核分段选项卡（无劣质胶囊/无emoji） -->
      <nav class="sub-segmented-nav" aria-label="Content navigation">
        <router-link
          :to="localePath('news')"
          class="seg-item"
          :class="{ active: kind === 'news' }"
        >
          <span class="seg-code font-mono">[01]</span>
          <span class="seg-label">{{ isZh ? '新闻动态' : 'News' }}</span>
        </router-link>
        <router-link
          :to="localePath('articles')"
          class="seg-item"
          :class="{ active: kind === 'articles' }"
        >
          <span class="seg-code font-mono">[02]</span>
          <span class="seg-label">{{ isZh ? '深度解析' : 'Deep Dive' }}</span>
        </router-link>
      </nav>

      <!-- 右侧：返回首页 + 语言切换 + 控制台 -->
      <div class="cta-wrap">
        <router-link :to="localePath('')" class="nav-home-btn">
          <span>←</span>
          <span>{{ isZh ? '返回首页' : 'Home' }}</span>
        </router-link>

        <div class="lang-switcher">
          <button
            type="button"
            class="lang-switch-btn"
            aria-haspopup="listbox"
            :aria-expanded="localeMenuOpen"
            :aria-label="currentLocaleName"
            @click="toggleLocaleMenu"
          >
            <span class="lang-globe" aria-hidden="true">◎</span>
            <span>{{ currentLocaleName }}</span>
            <span class="lang-caret" aria-hidden="true">▾</span>
          </button>
          <ul v-if="localeMenuOpen" class="lang-menu" role="listbox">
            <li v-for="option in availableLocales" :key="option.code">
              <button
                type="button"
                class="lang-option"
                role="option"
                :aria-selected="option.code === locale"
                :class="{ active: option.code === locale }"
                @click="switchLocale(option.code)"
              >
                {{ option.name }}
              </button>
            </li>
          </ul>
        </div>

        <router-link :to="consoleTarget" class="cta-btn">
          {{ isZh ? '进入控制台 →' : 'Console →' }}
        </router-link>
      </div>
    </header>

    <main class="neo-public-main">
      <!-- ==================== 详情页：沉浸式阅读视图 ==================== -->
      <div v-if="detail" class="neo-reader-layout">
        <article class="neo-article-sheet">
          <div class="article-top-nav">
            <router-link :to="'/' + kind" class="back-link">
              <span>←</span>
              <span>{{ isZh ? (kind === 'news' ? '返回新闻资讯' : '返回深度解析') : (kind === 'news' ? 'Back to News' : 'Back to Deep Dive') }}</span>
            </router-link>
            <div class="article-meta-pills">
              <span class="article-badge">
                {{ detail.displayCategory || detail.category || detail.sourceName || 'CCAI Official' }}
              </span>
              <span class="article-read-time font-mono">
                [{{ estimateReadingTime(detail.displayBodyText || detail.bodyText || detail.bodyHtml || detail.summary || '') }} {{ isZh ? '分钟阅读' : 'MIN READ' }}]
              </span>
              <span class="article-date font-mono">
                {{ formatDate(detail.publishedAt || detail.createdAt) }}
              </span>
            </div>
          </div>

          <h1 class="article-headline">{{ detail.displayTitle || detail.title }}</h1>

          <!-- R7：详情通栏封面，最高 400px、16:9 裁剪；缺失/加载失败统一占位，不留破图 -->
          <figure class="article-cover" data-testid="detail-cover">
            <img v-if="detailCover" :src="detailCover" :alt="coverAltOf(detail)" class="article-cover-img" loading="eager" decoding="async" @error="markCoverBroken(detail)" />
            <div v-else class="cover-placeholder" role="img" :aria-label="isZh ? '暂无封面' : 'Cover unavailable'">
              <svg viewBox="0 0 48 32" aria-hidden="true"><rect x="2" y="2" width="44" height="28" rx="4" /><circle cx="16" cy="12" r="3.5" /><path d="M6 26l11-9 8 6 7-5 10 8" /></svg>
            </div>
          </figure>

          <div class="article-author-bar">
            <div class="author-info">
              <span class="author-avatar">{{ (detail.author || detail.sourceName || 'C').slice(0, 1).toUpperCase() }}</span>
              <div>
                <div class="author-name">{{ detail.author || detail.sourceName || 'CCAI' }}</div>
                <div class="author-role font-mono">{{ kind === 'news' ? (isZh ? '新闻动态' : 'NEWSWIRE') : (isZh ? '深度解析' : 'DEEP DIVE') }}</div>
              </div>
            </div>
            <div class="article-actions">
              <button type="button" class="action-pill-btn" @click="copyArticleUrl">
                <span class="font-mono">{{ copiedLink ? '✓' : '#' }}</span>
                <span>{{ copiedLink ? (isZh ? '已复制链接' : 'Copied') : (isZh ? '分享链接' : 'Share') }}</span>
              </button>
            </div>
          </div>

          <p v-if="detail.displaySummary || detail.summary || detail.excerpt" class="article-lede">
            {{ detail.displaySummary || detail.summary || detail.excerpt }}
          </p>

          <!-- 移动端/内嵌目录 -->
          <nav v-if="tocItems.length" class="article-toc">
            <div class="toc-title">
              <span class="font-mono">//</span>
              <span>{{ isZh ? '目录索引' : 'CONTENTS' }}</span>
            </div>
            <div class="toc-links">
              <a
                v-for="(item, idx) in tocItems"
                :key="item.id"
                href="#"
                class="toc-link-chip"
                @click.prevent="jumpToHeading(item.id)"
              >
                <span class="toc-num font-mono">0{{ idx + 1 }}</span>
                <span>{{ item.text }}</span>
              </a>
            </div>
          </nav>

          <div
            class="neo-prose-body"
            v-html="detail.displayBodyHtml || detail.bodyHtml || detail.bodyText || detail.bodyMarkdown || detail.summary"
          ></div>

          <div v-if="detail.tags && detail.tags.length" class="article-tags-footer">
            <span v-for="tag in detail.tags" :key="tag" class="tag-chip"># {{ tag }}</span>
          </div>

          <div class="article-bottom-cta">
            <router-link :to="localePath('')" class="cta-banner-copy-link" :title="isZh ? '点击访问 CCAI 首页' : 'Visit CCAI Homepage'">
              <div class="cta-banner-badge font-mono">⚡ CCAI UNIFIED AI GATEWAY</div>
              <div class="cta-banner-title">{{ isZh ? '用 CCAI 调用这些模型' : 'Use these models with CCAI' }}</div>
              <div class="cta-banner-sub">{{ isZh ? '一个 API Key 调用 Claude、GPT、Gemini、DeepSeek 等模型 · 点击前往首页了解架构详情' : 'One API key for Claude, GPT, Gemini, DeepSeek and more · Click to explore homepage' }}</div>
            </router-link>
            <div class="cta-banner-actions">
              <router-link :to="localePath('')" class="cta-home-btn">
                {{ isZh ? '返回首页' : 'Homepage' }}
              </router-link>
              <router-link :to="consoleTarget" class="cta-btn">
                {{ isZh ? '进入控制台 →' : 'Open console →' }}
              </router-link>
            </div>
          </div>

          <!-- ==================== SEO 内部链接：上一篇 / 下一篇 ==================== -->
          <nav
            v-if="prevItem || nextItem"
            class="detail-prev-next-grid"
            :aria-label="kind === 'news' ? (isZh ? '新闻上下篇导航' : 'News pagination') : (isZh ? '文章上下篇导航' : 'Article pagination')"
          >
            <router-link
              v-if="prevItem"
              :to="itemPath(prevItem)"
              class="prev-next-card prev-card"
            >
              <div class="pn-label font-mono">
                <span>←</span>
                <span>{{ kind === 'news' ? (isZh ? '上一篇新闻' : 'PREVIOUS NEWS') : (isZh ? '上一篇文章' : 'PREVIOUS ARTICLE') }}</span>
              </div>
              <div class="pn-title">{{ prevItem.displayTitle || prevItem.title }}</div>
              <div class="pn-meta font-mono">
                <span class="pn-cat">{{ prevItem.displayCategory || prevItem.category || 'CCAI' }}</span>
                <span>·</span>
                <span>{{ formatDate(prevItem.publishedAt || prevItem.createdAt) }}</span>
              </div>
            </router-link>
            <div v-else class="prev-next-card pn-empty">
              <div class="pn-label font-mono">
                <span>←</span>
                <span>{{ kind === 'news' ? (isZh ? '上一篇新闻' : 'PREVIOUS NEWS') : (isZh ? '上一篇文章' : 'PREVIOUS ARTICLE') }}</span>
              </div>
              <div class="pn-empty-text">{{ isZh ? '已经是第一篇了' : 'Already at the latest item' }}</div>
            </div>

            <router-link
              v-if="nextItem"
              :to="itemPath(nextItem)"
              class="prev-next-card next-card"
            >
              <div class="pn-label font-mono">
                <span>{{ kind === 'news' ? (isZh ? '下一篇新闻' : 'NEXT NEWS') : (isZh ? '下一篇文章' : 'NEXT ARTICLE') }}</span>
                <span>→</span>
              </div>
              <div class="pn-title">{{ nextItem.displayTitle || nextItem.title }}</div>
              <div class="pn-meta font-mono">
                <span class="pn-cat">{{ nextItem.displayCategory || nextItem.category || 'CCAI' }}</span>
                <span>·</span>
                <span>{{ formatDate(nextItem.publishedAt || nextItem.createdAt) }}</span>
              </div>
            </router-link>
            <div v-else class="prev-next-card pn-empty next-card">
              <div class="pn-label font-mono">
                <span>{{ kind === 'news' ? (isZh ? '下一篇新闻' : 'NEXT NEWS') : (isZh ? '下一篇文章' : 'NEXT ARTICLE') }}</span>
                <span>→</span>
              </div>
              <div class="pn-empty-text">{{ isZh ? '已经是最后一篇了' : 'No more older items' }}</div>
            </div>
          </nav>

          <!-- ==================== SEO 关联推荐：更多 6 条新闻 / 文章 ==================== -->
          <section v-if="moreRelatedItems.length > 0" class="detail-more-section">
            <div class="more-section-header">
              <div>
                <div class="more-eyebrow font-mono">
                  // {{ kind === 'news' ? (isZh ? 'MORE NEWSWIRE' : 'MORE NEWSWIRE') : (isZh ? 'FURTHER READING' : 'FURTHER READING') }}
                </div>
                <h2 class="more-section-title">
                  {{ kind === 'news' ? (isZh ? '更多新闻动态' : 'More News & Updates') : (isZh ? '更多深度解析' : 'More Deep Dive') }}
                </h2>
              </div>
              <router-link :to="localePath(kind)" class="more-view-all-link font-mono">
                {{ kind === 'news' ? (isZh ? '查看全部新闻 →' : 'All News →') : (isZh ? '查看全部文章 →' : 'All Articles →') }}
              </router-link>
            </div>

            <div class="more-related-grid">
              <router-link
                v-for="rel in moreRelatedItems"
                :key="rel.id || rel.slug"
                :to="itemPath(rel)"
                class="more-related-card"
                data-analytics-placement="article_detail_related"
                :data-analytics-id="rel.id"
                @click="trackPlacementClick('article_detail_related', rel)"
              >
                <div class="more-card-meta font-mono">
                  <span class="more-card-cat">{{ rel.displayCategory || rel.category || 'CCAI' }}</span>
                  <span class="more-card-date">{{ formatDate(rel.publishedAt || rel.createdAt) }}</span>
                </div>
                <h3 class="more-card-title">{{ rel.displayTitle || rel.title }}</h3>
                <p class="more-card-excerpt">{{ rel.displaySummary || rel.summary || rel.excerpt || '' }}</p>
                <div class="more-card-go font-mono">
                  <span>{{ isZh ? '阅读全文' : 'Read more' }}</span>
                  <span>→</span>
                </div>
              </router-link>
            </div>
          </section>
        </article>
      </div>

      <!-- ==================== 列表页：科技期刊 & 实时电报布局 ==================== -->
      <section v-else class="neo-list-section">
        <!-- 顶部杂志刊头与过滤器 -->
        <div class="list-hero-banner">
          <div class="list-hero-top">
            <div>
              <div class="list-eyebrow-row">
                <span class="list-eyebrow">
                  <span class="pulse-dot"></span>
                  CCAI {{ kind === 'news' ? 'NEWS' : 'ARTICLES' }}
                </span>
                <span class="list-count-badge font-mono">{{ listCountLabel }} {{ isZh ? '篇内容' : 'ITEMS' }}</span>
              </div>
              <h1 class="list-title">
                {{ kind === 'news' ? (isZh ? '新闻资讯' : 'News') : (isZh ? '深度解析' : 'Deep Dive') }}
              </h1>
              <p class="list-sub">
                {{ kind === 'news'
                  ? (isZh ? '大模型发布、API 变化和本站公告。' : 'Model releases, API changes and site announcements.')
                  : (isZh ? '接入教程、客户端配置和使用经验。' : 'Setup guides, client configuration and usage tips.')
                }}
              </p>
            </div>

            <!-- 升级版 Neo-Lab 极客搜索框 -->
            <div class="list-search-box" :class="{ 'has-value': !!searchQuery }">
              <span class="search-icon-badge" aria-hidden="true">
                <svg viewBox="0 0 20 20" fill="none" class="search-svg">
                  <circle cx="8.5" cy="8.5" r="5.5" stroke="currentColor" stroke-width="2.2" />
                  <path d="M13 13L17 17" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" />
                </svg>
              </span>
              <input
                ref="searchInputRef"
                v-model="searchQuery"
                type="text"
                class="list-search-input"
                :placeholder="isZh ? '搜索标题、技术关键词或分类…' : 'Search title, keyword, category...'"
              />
              <button
                v-if="searchQuery"
                type="button"
                class="search-clear-btn"
                :title="isZh ? '清空搜索' : 'Clear search'"
                @click="searchQuery = ''"
              >
                ✕
              </button>
              <kbd v-else class="search-kbd-hint font-mono">/</kbd>
            </div>
          </div>

          <!-- 分类胶囊栏 -->
          <div v-if="availableCategories.length > 0" class="list-category-bar">
            <button
              type="button"
              class="cat-pill"
              :class="{ active: activeCategory === 'ALL' }"
              @click="activeCategory = 'ALL'"
            >
              {{ isZh ? '全部分类' : 'All Categories' }}
            </button>
            <button
              v-for="cat in availableCategories"
              :key="cat"
              type="button"
              class="cat-pill"
              :class="{ active: activeCategory === cat }"
              @click="activeCategory = cat"
            >
              {{ cat }}
            </button>
          </div>
        </div>

        <div v-if="contentLoading" class="neo-loading-box">
          <div class="neo-spinner"></div>
          <span>{{ isZh ? '加载中…' : 'Loading…' }}</span>
        </div>

        <div v-else-if="errorMessage" class="neo-error-box">
          {{ errorMessage }}
        </div>

        <template v-else-if="filteredItems.length > 0 || showFeaturedSection">
          <!-- ========== 深度解析精选区：1 张大卡 + 最多 9 张小卡 ========== -->
          <section
            v-if="showFeaturedSection"
            class="public-featured-section"
            data-testid="featured-section"
            aria-labelledby="featured-section-title"
          >
            <div class="public-featured-header">
              <router-link :to="localePath('articles')" class="public-featured-all-link">
                {{ isZh ? '查看全部' : 'View all' }} →
              </router-link>
            </div>

            <div class="public-featured-layout public-featured-list" data-testid="featured-list">
              <router-link
                v-for="(item, idx) in [featuredItems[0], ...featuredItems.slice(1, 10)]"
                :key="item.id || item.slug"
                :to="itemPath(item)"
                class="public-featured-row"
                :data-testid="idx === 0 ? 'featured-large-card' : 'featured-small'"
                data-analytics-placement="home_hero"
                :data-analytics-id="item.id"
                @click="trackPlacementClick('home_hero', item)"
              >
                <div class="public-featured-cover public-featured-cover-row">
                  <img
                    v-if="featuredImage(item)"
                    :src="featuredImage(item)"
                    :alt="item.coverAlt || item.displayTitle || item.title"
                    loading="lazy"
                  />
                  <span v-else class="public-featured-cover-placeholder" aria-hidden="true">
                    <span class="cover-ph-grid"></span>
                    <span class="cover-ph-watermark font-mono">{{ String(idx + 1).padStart(2, '0') }}</span>
                    <span class="cover-ph-brand font-mono">
                      <span class="cover-ph-dot"></span>
                      CCAI · DEEP DIVE
                    </span>
                    <svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M3 16l5-5 4 4 3-3 6 6" /><circle cx="9" cy="9" r="1.4" /></svg>
                  </span>
                  <span v-if="idx === 0" class="public-featured-badge">★ {{ isZh ? '编辑精选' : 'FEATURED' }}</span>
                </div>
                <div class="public-featured-row-body">
                  <div class="public-featured-meta">
                    <span class="pf-row-index font-mono">{{ String(idx + 1).padStart(2, '0') }}</span>
                    <span class="pf-row-cat">{{ item.displayCategory || item.category || 'CCAI' }}</span>
                    <span class="pf-row-read font-mono">{{ estimateReadingTime(item.displaySummary || item.summary || item.excerpt || '') }} MIN READ</span>
                  </div>
                  <h2 class="public-featured-card-title public-featured-row-title">
                    {{ item.displayTitle || item.title }}
                  </h2>
                  <p class="public-featured-row-summary">
                    {{ item.displaySummary || item.summary || item.excerpt || '' }}
                  </p>
                  <div class="public-featured-tags">
                    <span v-for="tag in getCleanItemTags(item).slice(0, 3)" :key="tag">#{{ tag }}</span>
                  </div>
                </div>
                <span class="public-featured-row-cta" aria-hidden="true">→</span>
              </router-link>
            </div>
          </section>

          <!-- ========== 模式 A：深度解析文章列表 ========== -->
          <template v-if="kind === 'articles'">
            <div class="neo-articles-row-list">
              <router-link
                v-for="(item, idx) in pagedItems"
                :key="item.id"
                :to="itemPath(item)"
                class="neo-article-row-card"
                data-analytics-placement="articles_list"
                :data-analytics-id="item.id"
                @click="trackPlacementClick('articles_list', item)"
              >
                <div class="row-left-strip" :class="'strip-' + (idx % 3)"></div>
                <div class="list-card-cover article-row-cover" data-testid="list-card-cover">
                  <img v-if="contentCover(item)" :src="contentCover(item)" :alt="coverAltOf(item)" loading="lazy" decoding="async" @error="markCoverBroken(item)" />
                  <div v-else class="cover-placeholder" role="img" :aria-label="isZh ? '暂无封面' : 'Cover unavailable'">
                    <span class="cover-ph-grid" aria-hidden="true"></span>
                    <span class="cover-ph-watermark font-mono" aria-hidden="true">#{{ String(idx + 1).padStart(2, '0') }}</span>
                    <span class="cover-ph-pill font-mono" aria-hidden="true">CCAI DOCS</span>
                    <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M3 16l5-5 4 4 3-3 6 6" /><circle cx="9" cy="9" r="1.4" /></svg>
                  </div>
                </div>
                <div class="article-row-main">
                  <div class="article-row-meta">
                    <span class="meta-tag">{{ item.displayCategory || item.category || '架构与网关' }}</span>
                    <span v-for="tg in getCleanItemTags(item).slice(0, 3)" :key="tg" class="article-row-tag">#{{ tg }}</span>
                    <span class="meta-read font-mono">{{ estimateReadingTime(item.displaySummary || item.summary || item.excerpt || '') }} MIN READ</span>
                    <span class="meta-date font-mono">{{ formatDate(item.publishedAt || item.createdAt) }}</span>
                  </div>
                  <h2 class="article-row-title">{{ item.displayTitle || item.title }}</h2>
                  <p class="article-row-excerpt">{{ item.displaySummary || item.summary || item.excerpt || '' }}</p>
                </div>
                <div class="article-row-aside">
                  <span class="article-row-index font-mono">#{{ String(idx + 1).padStart(2, '0') }}</span>
                  <span class="article-row-cta"><span>{{ isZh ? '阅读全文' : 'Read' }}</span><span class="cta-arrow">→</span></span>
                </div>
              </router-link>
            </div>
          </template>

          <!-- ========== 模式 B：新闻动态（极客电报流 + 重点情报双栏） ========== -->
          <template v-else>
            <!-- 置顶头版情报 -->
            <router-link
              v-if="featuredItem"
              :to="itemPath(featuredItem)"
              class="neo-news-spotlight"
              data-analytics-placement="news_list"
              :data-analytics-id="featuredItem.id"
              @click="trackPlacementClick('news_list', featuredItem)"
            >
              <div class="spotlight-badge-col">
                <span class="live-pill">
                  <span class="live-dot"></span>
                  BREAKING
                </span>
                <span class="spotlight-date font-mono">{{ formatDate(featuredItem.publishedAt || featuredItem.createdAt) }}</span>
              </div>
              <div class="list-card-cover spotlight-cover" data-testid="spotlight-cover">
                <img v-if="contentCover(featuredItem)" :src="contentCover(featuredItem)" :alt="coverAltOf(featuredItem)" loading="lazy" decoding="async" @error="markCoverBroken(featuredItem)" />
                <div v-else class="cover-placeholder cover-placeholder-news" role="img" :aria-label="isZh ? '暂无封面' : 'Cover unavailable'">
                  <span class="cover-ph-grid" aria-hidden="true"></span>
                  <span class="cover-ph-watermark font-mono" aria-hidden="true">TOP</span>
                  <span class="cover-ph-pill font-mono" aria-hidden="true">HEADLINE</span>
                  <svg viewBox="0 0 48 32" aria-hidden="true"><rect x="2" y="2" width="44" height="28" rx="4" /><circle cx="16" cy="12" r="3.5" /><path d="M6 26l11-9 8 6 7-5 10 8" /></svg>
                </div>
              </div>
              <div class="spotlight-main-col">
                <div class="spotlight-meta">
                  <span class="meta-tag tag-lime">{{ featuredItem.displayCategory || featuredItem.category || featuredItem.sourceName || 'CCAI' }}</span>
                  <span
                    v-if="featuredItem.sourceName && featuredItem.sourceName !== (featuredItem.displayCategory || featuredItem.category)"
                    class="meta-source-label font-mono"
                  >{{ featuredItem.sourceName }}</span>
                </div>
                <h2 class="spotlight-title">{{ featuredItem.displayTitle || featuredItem.title }}</h2>
                <p class="spotlight-excerpt">
                  {{ featuredItem.displaySummary || featuredItem.summary || featuredItem.excerpt }}
                </p>
              </div>
              <div class="spotlight-cta-col">
                <span class="read-more-pill">{{ isZh ? '阅读全文' : 'Read more' }} →</span>
              </div>
            </router-link>

            <!-- 时间线电报流 -->
            <div v-if="remainingItems.length > 0" class="neo-news-timeline">
              <router-link
                v-for="(item, idx) in remainingItems"
                :key="item.id"
                :to="itemPath(item)"
                class="timeline-news-card"
                data-analytics-placement="news_list"
                :data-analytics-id="item.id"
                @click="trackPlacementClick('news_list', item)"
              >
                <div class="timeline-rail">
                  <span class="timeline-node"></span>
                  <span class="timeline-line"></span>
                </div>
                <div class="timeline-date-col font-mono">
                  <div class="tl-date">{{ formatDate(item.publishedAt || item.createdAt) }}</div>
                  <div class="tl-source">{{ item.displayCategory || item.category || item.sourceName || 'CCAI' }}</div>
                </div>
                <div class="list-card-cover timeline-cover" data-testid="list-card-cover">
                  <img v-if="contentCover(item)" :src="contentCover(item)" :alt="coverAltOf(item)" loading="lazy" decoding="async" @error="markCoverBroken(item)" />
                  <div v-else class="cover-placeholder cover-placeholder-news" role="img" :aria-label="isZh ? '暂无封面' : 'Cover unavailable'">
                    <span class="cover-ph-grid" aria-hidden="true"></span>
                    <span class="cover-ph-watermark font-mono" aria-hidden="true">{{ String(idx + 2).padStart(2, '0') }}</span>
                    <span class="cover-ph-pill font-mono" aria-hidden="true">NEWS WIRE</span>
                    <svg viewBox="0 0 48 32" aria-hidden="true"><rect x="2" y="2" width="44" height="28" rx="4" /><circle cx="16" cy="12" r="3.5" /><path d="M6 26l11-9 8 6 7-5 10 8" /></svg>
                  </div>
                </div>
                <div class="timeline-content-col">
                  <h3 class="tl-title">{{ item.displayTitle || item.title }}</h3>
                  <p class="tl-excerpt">
                    {{ item.displaySummary || item.summary || item.excerpt || '' }}
                  </p>
                  <div v-if="getCleanItemTags(item).length" class="tl-tags">
                    <span v-for="tg in getCleanItemTags(item).slice(0, 3)" :key="tg" class="tl-tag">#{{ tg }}</span>
                  </div>
                </div>
                <div class="timeline-arrow-col">
                  <span class="tl-go">→</span>
                </div>
              </router-link>
            </div>
          </template>
        </template>

        <div v-else class="neo-empty-box">
          <div class="empty-icon font-mono">[00]</div>
          <div class="empty-title">{{ isZh ? '未找到匹配的内容' : 'No matching content found' }}</div>
          <p class="empty-sub">{{ isZh ? '换个关键词或分类试试。' : 'Try another keyword or category.' }}</p>
          <button
            v-if="searchQuery || activeCategory !== 'ALL'"
            type="button"
            class="reset-filter-btn"
            @click="searchQuery = ''; activeCategory = 'ALL'"
          >
            {{ isZh ? '重置筛选条件' : 'Reset Filters' }}
          </button>
        </div>

        <!-- ==================== 前台列表分页 ==================== -->
        <nav
          v-if="pagerVisible"
          class="neo-pager"
          data-testid="pager"
          aria-label="Pagination"
        >
          <button
            type="button"
            class="pager-btn pager-prev"
            data-testid="pager-prev"
            :disabled="currentPage <= 1"
            @click="goToPage(currentPage - 1)"
          >
            ← {{ isZh ? '上一页' : 'Prev' }}
          </button>
          <div class="pager-pages">
            <button
              v-for="p in pageNumbers"
              :key="'pager-' + p"
              type="button"
              class="pager-num font-mono"
              :class="{ 'is-active': p === currentPage, 'pager-current': p === currentPage }"
              :aria-current="p === currentPage ? 'page' : undefined"
              @click="goToPage(p)"
            >
              {{ p }}
            </button>
          </div>
          <span class="pager-status font-mono" data-testid="pager-status">
            {{ currentPage }} / {{ totalPages }}
          </span>
          <button
            type="button"
            class="pager-btn pager-next"
            data-testid="pager-next"
            :disabled="currentPage >= totalPages"
            @click="goToPage(currentPage + 1)"
          >
            {{ isZh ? '下一页' : 'Next' }} →
          </button>
        </nav>
      </section>
    </main>
  </div>

</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { apiClient } from '@/api/client'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { availableLocales, setLocale, type LocaleCode } from '@/i18n'
import { normalizeLocaleCode } from '@/i18n/registry'
import { createContentAnalyticsTracker, referrerHostOf } from '@/utils/contentAnalyticsV04'
import { postPublicAnalyticsEvent } from '@/api/publicAnalyticsV04'
import {
  buildLocaleSwitchTarget,
  initLocaleSync,
  persistLocale
} from '@/i18n/localeDetection'

marked.setOptions({
  breaks: true,
  gfm: true
})

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const { locale } = useI18n()

const isZh = computed(() => locale.value.startsWith('zh'))

// ==================== 语言切换器（R12-04：选项数量/顺序/名称与 registry 同源） ====================
const localeMenuOpen = ref(false)
const currentLocaleName = computed(
  () => availableLocales.find((option) => option.code === locale.value)?.name ?? 'English'
)

function toggleLocaleMenu() {
  localeMenuOpen.value = !localeMenuOpen.value
}

function closeLocaleMenuOnOutsideClick(event: MouseEvent) {
  const target = event.target as Element | null
  if (target?.closest?.('.lang-switcher')) {
    return
  }
  localeMenuOpen.value = false
}

function closeLocaleMenuOnEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    localeMenuOpen.value = false
  }
}

async function applySyncedLocale(next: LocaleCode) {
  if (next === locale.value) {
    return
  }
  await setLocale(next)
  setTimeout(renderLines, 30)
}

let disposeLocaleSync: (() => void) | null = null

async function switchLocale(code: LocaleCode) {
  localeMenuOpen.value = false
  if (code === locale.value) {
    return
  }
  const queryIndex = route.fullPath.indexOf('?')
  const search = queryIndex >= 0 ? route.fullPath.slice(queryIndex) : ''
  const target = buildLocaleSwitchTarget(route.path, search, code)
  await setLocale(code)
  // 手动切换即双写 cookie + localStorage，SSR 下次首屏直接命中该语言
  persistLocale(code)
  setTimeout(renderLines, 30)
  if (!target) {
    return
  }
  const resolved = router.resolve(target)
  if (resolved.name && resolved.name !== 'NotFound') {
    await router.replace(target).catch(() => {})
    return
  }
  // 前缀首页/列表由 SSR 路由承接，SPA 未注册，必须整页跳转交给服务端按 URL 输出
  window.location.assign(target)
}

const isHome = computed(() => {
  const path = route.path.replace(/^\/(en|zh|ru|fr|de)(?=\/|$)/, '') || '/'
  return path === '/'
})

// 站内链接一律带当前语言前缀：无前缀的 /news 在服务端会无条件 301 到 /zh/news，
// 刷新或新开标签时会丢掉用户刚选的语言
const urlLocale = computed<LocaleCode>(() => normalizeLocaleCode(locale.value) || 'zh')
function localePath(section: '' | 'news' | 'articles'): string {
  return section ? `/${urlLocale.value}/${section}` : `/${urlLocale.value}/`
}


const consoleTarget = computed(() => {
  if (authStore.isAuthenticated) {
    return authStore.user?.role === 'admin' ? '/admin/dashboard' : '/dashboard'
  }
  return '/login'
})

const displayBaseUrl = computed(() => {
  const configured = appStore.cachedPublicSettings?.api_base_url?.trim()
  const raw = configured || 'https://api.cc-ai.xyz'
  // api_base_url 可能已带 /v1，统一去掉再由模板补 /v1，避免出现 /v1/v1
  return raw.replace(/\/+$/, '').replace(/\/v1$/i, '')
})

const headlinePrefix = computed(() => {
  if (currentMode.value === 'chaos') {
    return isZh.value ? '不用挨家注册，' : 'No more vendor signups. '
  }
  return isZh.value ? '一个 Key 调用 ' : 'One key for '
})

const headlinePill = computed(() => {
  if (currentMode.value === 'chaos') {
    return isZh.value ? '一个 Key 调用主流大模型' : 'One key for major LLMs'
  }
  return isZh.value ? 'Claude / GPT / Gemini' : 'Claude / GPT / Gemini'
})

const clientItems = [
  {
    id: 'claude-code',
    name: 'Claude Code / Codex',
    descZh: '终端编程 Agent',
    descEn: 'Terminal coding agents',
    badTagZh: '账号被封',
    badTagEn: 'Banned',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    bg: '#FFF0EB'
  },
  {
    id: 'cursor-ide',
    name: 'Cursor / Trae / Windsurf',
    descZh: 'AI 编程 IDE',
    descEn: 'AI coding IDEs',
    badTagZh: '429 限流',
    badTagEn: '429 limited',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    bg: '#EEF2FF'
  },
  {
    id: 'roo-cline',
    name: 'Roo Code / Cline',
    descZh: 'VS Code 编程插件',
    descEn: 'VS Code coding agents',
    badTagZh: '账单难控',
    badTagEn: 'Surprise bills',
    goodTagZh: '用量可查',
    goodTagEn: 'Usage tracked',
    bg: '#EFF6FF'
  },
  {
    id: 'cherry-dify',
    name: 'Cherry Studio / Dify',
    descZh: '桌面客户端与工作流',
    descEn: 'Desktop chat & workflows',
    badTagZh: 'Key 太多',
    badTagEn: 'Too many keys',
    goodTagZh: '一个 Key',
    goodTagEn: 'One key',
    bg: '#FFF1F2'
  }
]

const modelItems = [
  {
    id: 'claude-opus-5-5',
    name: 'Claude opus 5.5',
    descZh: 'Anthropic · 顶尖推理编码',
    descEn: 'Anthropic · Frontier reasoning & coding',
    badTagZh: '风控封号',
    badTagEn: 'Bans',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    fracturePainZh: '403 账号被封 · 余额拿不回',
    fracturePainEn: '403 banned · balance lost',
    bg: '#FFF0EB',
    iconType: 'anthropic'
  },
  {
    id: 'gpt-6',
    name: 'gpt-6-astra / gpt-6.1-sol',
    descZh: 'OpenAI · 新一代多模态旗舰',
    descEn: 'OpenAI · Next-gen multimodal flagships',
    badTagZh: '频繁 429',
    badTagEn: 'Frequent 429s',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    fracturePainZh: '海外卡经常被拒 · 频发 429 限流',
    fracturePainEn: 'Card declined · frequent 429 errors',
    bg: '#ECFDF5',
    iconType: 'openai'
  },
  {
    id: 'gemini-4',
    name: 'gemini 4',
    descZh: 'Google · 超长上下文与深度分析',
    descEn: 'Google · Massive context & deep analysis',
    badTagZh: '地区限制',
    badTagEn: 'Region blocked',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    fracturePainZh: '所在地区不可用 · 账号连带风控',
    fracturePainEn: 'Region unavailable · cascade bans',
    bg: '#EFF6FF',
    iconType: 'google'
  },
  {
    id: 'others-tier',
    name: 'grok 4.7 / Deepseek-4.1 / Kimi-k3',
    descZh: 'xAI · DeepSeek · Moonshot',
    descEn: 'xAI · DeepSeek · Moonshot',
    badTagZh: '各自配 Key',
    badTagEn: 'Multi-Key',
    goodTagZh: '200 OK',
    goodTagEn: '200 OK',
    fracturePainZh: '各平台分散充值 · 维护十几个后台',
    fracturePainEn: 'Fragmented top-ups · 10+ consoles',
    bg: '#F5F3FF',
    iconType: 'others'
  }
]

const currentMode = ref<'chaos' | 'order'>('chaos')
const isMorphing = ref(false)
const snapRebornActive = ref(false)
const hoveredIdx = ref<number | null>(null)
const isChargingPhase = ref(false)
const isChargedDone = ref(false)
const hasActiveSparks = ref(false)

let chargeDoneTimer: any = null
let fireworkInterval: any = null

const morphCanvasRef = ref<HTMLCanvasElement | null>(null)
const stageGridRef = ref<HTMLElement | null>(null)
const linesSvgRef = ref<SVGSVGElement | null>(null)

interface MorphPiece {
  startX: number
  startY: number
  midX: number
  midY: number
  endX: number
  endY: number
  w: number
  h: number
  rotPeak: number
  scalePeak: number
  colorFrom: string
  colorTo: string
  delay: number
}

interface BoxBurstSpark {
  x: number
  y: number
  vx: number
  vy: number
  color: string
  size: number
  alpha: number
  decay: number
  shape: 'star' | 'comet' | 'dot'
  rot: number
}

let pieces: MorphPiece[] = []
let sparks: BoxBurstSpark[] = []
let animStartTime = 0
let rafId: number | null = null
const TOTAL_DURATION = 1320
const CHARGE_DURATION_MS = 3000

function onClientEnter(idx: number) {
  if (isMorphing.value) return
  if (chargeDoneTimer) clearTimeout(chargeDoneTimer)
  if (fireworkInterval) clearInterval(fireworkInterval)

  hoveredIdx.value = idx
  isChargingPhase.value = true
  isChargedDone.value = false
  renderLines()

  chargeDoneTimer = setTimeout(() => {
    if (hoveredIdx.value !== idx || isMorphing.value) return
    isChargingPhase.value = false
    isChargedDone.value = true

    if (currentMode.value === 'chaos') {
      spawnBoxPerimeterFireworks(idx, 'chaos')
    } else {
      spawnBoxPerimeterFireworks(idx, 'order')
      fireworkInterval = setInterval(() => {
        if (hoveredIdx.value === idx && currentMode.value === 'order' && isChargedDone.value && !isMorphing.value) {
          spawnBoxPerimeterFireworks(idx, 'order')
        }
      }, 260)
    }
  }, CHARGE_DURATION_MS)
}

function onClientLeave() {
  hoveredIdx.value = null
  isChargingPhase.value = false
  isChargedDone.value = false
  if (chargeDoneTimer) clearTimeout(chargeDoneTimer)
  if (fireworkInterval) clearInterval(fireworkInterval)
  renderLines()
}

function spawnBoxPerimeterFireworks(idx: number, mode: 'chaos' | 'order') {
  const el = document.getElementById('right-' + idx)
  if (!el) return
  const r = el.getBoundingClientRect()

  const palette =
    mode === 'order'
      ? ['#CCFF00', '#60A5FA', '#2563EB', '#FF2D55', '#F59E0B', '#A855F7', '#38BDF8']
      : ['#FF3B30', '#FF9500', '#121316']

  const count = mode === 'order' ? 36 : 24
  for (let i = 0; i < count; i++) {
    const edge = i % 4
    let sx = 0
    let sy = 0
    let baseOutAngle = 0

    if (edge === 0) {
      sx = r.left + 10 + Math.random() * (r.width - 20)
      sy = r.top
      baseOutAngle = -Math.PI / 2 + (Math.random() - 0.5) * 1.2
    } else if (edge === 1) {
      sx = r.left + 10 + Math.random() * (r.width - 20)
      sy = r.bottom
      baseOutAngle = Math.PI / 2 + (Math.random() - 0.5) * 1.2
    } else if (edge === 2) {
      sx = r.left
      sy = r.top + 8 + Math.random() * (r.height - 16)
      baseOutAngle = Math.PI + (Math.random() - 0.5) * 1.2
    } else {
      sx = r.right
      sy = r.top + 8 + Math.random() * (r.height - 16)
      baseOutAngle = (Math.random() - 0.5) * 1.2
    }

    const speed = 4.2 + Math.random() * 6.8
    const shapes: Array<'star' | 'comet' | 'dot'> = ['star', 'comet', 'dot']
    sparks.push({
      x: sx,
      y: sy,
      vx: Math.cos(baseOutAngle) * speed,
      vy: Math.sin(baseOutAngle) * speed,
      color: palette[i % palette.length],
      size: 4.5 + Math.random() * 4.5,
      alpha: 1,
      decay: 0.022 + Math.random() * 0.016,
      shape: shapes[i % 3],
      rot: Math.random() * Math.PI
    })
  }
  hasActiveSparks.value = true
  ensureCanvasLoop()
}

function toggleMode() {
  morphTo(currentMode.value === 'chaos' ? 'order' : 'chaos')
}

async function morphTo(targetMode: 'chaos' | 'order') {
  if (isMorphing.value || targetMode === currentMode.value) return
  onClientLeave()
  sparks = []
  hasActiveSparks.value = false
  isMorphing.value = true
  snapRebornActive.value = false

  const units = Array.from(document.querySelectorAll('.morph-unit')) as HTMLElement[]
  const beforeRects = units.map((el) => el.getBoundingClientRect())

  currentMode.value = targetMode
  await nextTick()

  const afterRects = units.map((el) => el.getBoundingClientRect())
  const fromPalette =
    targetMode === 'order'
      ? ['#FFFFFF', '#FFE4E1', '#FF3B30', '#FF9500', '#121316']
      : ['#FFFFFF', '#CCFF00', '#2563EB', '#60A5FA', '#121316']
  const toPalette =
    targetMode === 'order'
      ? ['#FFFFFF', '#CCFF00', '#2563EB', '#60A5FA', '#DBEAFE']
      : ['#FFFFFF', '#FFE4E1', '#FF3B30', '#FF9500', '#FFF5F5']

  pieces = []
  units.forEach((el, idx) => {
    const r1 = beforeRects[idx]
    const r2 = afterRects[idx]
    if (!r1 || !r2) return
    const isCenter = el.id === 'center-core'
    const cols = isCenter ? 8 : 6
    const rows = isCenter ? 6 : 3
    const w1 = r1.width / cols
    const h1 = r1.height / rows
    const w2 = r2.width / cols
    const h2 = r2.height / rows
    const cx1 = r1.left + r1.width / 2
    const cy1 = r1.top + r1.height / 2

    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const startX = r1.left + (c + 0.5) * w1
        const startY = r1.top + (r + 0.5) * h1
        const endX = r2.left + (c + 0.5) * w2
        const endY = r2.top + (r + 0.5) * h2
        const baseAngle = Math.atan2(startY - cy1, startX - cx1) + (Math.random() - 0.5) * 0.6
        const blastDist = (isCenter ? 130 : 85) + Math.random() * (isCenter ? 120 : 75)
        pieces.push({
          startX,
          startY,
          midX: startX + Math.cos(baseAngle) * blastDist,
          midY: startY + Math.sin(baseAngle) * blastDist,
          endX,
          endY,
          w: Math.max(w1, w2) * 0.92,
          h: Math.max(h1, h2) * 0.92,
          rotPeak: (Math.random() - 0.5) * Math.PI * 2.2,
          scalePeak: 0.48 + Math.random() * 0.42,
          colorFrom: fromPalette[(r * cols + c + idx) % fromPalette.length],
          colorTo: toPalette[(r * cols + c + idx) % toPalette.length],
          delay: (isCenter ? 0 : 0.03) + Math.random() * 0.05
        })
      }
    }
  })

  animStartTime = performance.now()
  ensureCanvasLoop()
}

function easeOutCubic(t: number) {
  return 1 - Math.pow(1 - t, 3)
}
function easeInOutBack(t: number) {
  const c1 = 1.70158
  const c2 = c1 * 1.525
  return t < 0.5
    ? (Math.pow(2 * t, 2) * ((c2 + 1) * 2 * t - c2)) / 2
    : (Math.pow(2 * t - 2, 2) * ((c2 + 1) * (t * 2 - 2) + c2) + 2) / 2
}

function ensureCanvasLoop() {
  if (rafId !== null) return
  rafId = requestAnimationFrame(tickCanvas)
}

function drawFourPointStar(ctx: CanvasRenderingContext2D, r: number) {
  ctx.beginPath()
  for (let i = 0; i < 8; i++) {
    const rad = i % 2 === 0 ? r : r * 0.38
    const ang = (i * Math.PI) / 4
    if (i === 0) ctx.moveTo(Math.cos(ang) * rad, Math.sin(ang) * rad)
    else ctx.lineTo(Math.cos(ang) * rad, Math.sin(ang) * rad)
  }
  ctx.closePath()
}

function tickCanvas(now: number) {
  rafId = null
  const canvas = morphCanvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const cw = window.innerWidth
  const ch = window.innerHeight
  ctx.clearRect(0, 0, cw, ch)

  let needNextFrame = false

  if (isMorphing.value) {
    const elapsed = now - animStartTime
    const rawProgress = Math.min(1, elapsed / TOTAL_DURATION)

    if (rawProgress >= 1) {
      ctx.clearRect(0, 0, cw, ch)
      pieces = []
      isMorphing.value = false
      snapRebornActive.value = true
      setTimeout(() => {
        snapRebornActive.value = false
      }, 360)
      renderLines()
    } else {
      pieces.forEach((p) => {
        const t = Math.max(0, Math.min(1, (rawProgress - p.delay) / (1 - p.delay)))
        let x: number, y: number, rot: number, scale: number, color: string
        if (t < 0.44) {
          const k = easeOutCubic(t / 0.44)
          x = p.startX + (p.midX - p.startX) * k
          y = p.startY + (p.midY - p.startY) * k
          rot = p.rotPeak * k
          scale = 1 - (1 - p.scalePeak) * k
          color = p.colorFrom
        } else if (t < 0.56) {
          const k = (t - 0.44) / 0.12
          x = p.midX
          y = p.midY
          rot = p.rotPeak * (1 + 0.15 * Math.sin(k * Math.PI))
          scale = p.scalePeak * (1 + 0.15 * Math.sin(k * Math.PI))
          color = k > 0.5 ? p.colorTo : p.colorFrom
        } else {
          const k = easeInOutBack((t - 0.56) / 0.44)
          x = p.midX + (p.endX - p.midX) * k
          y = p.midY + (p.endY - p.midY) * k
          rot = p.rotPeak * (1 - k)
          scale = p.scalePeak + (1 - p.scalePeak) * k
          color = p.colorTo
        }
        ctx.save()
        ctx.translate(x, y)
        ctx.rotate(rot)
        ctx.scale(scale, scale)
        ctx.fillStyle = color
        ctx.strokeStyle = '#121316'
        ctx.lineWidth = 2
        ctx.beginPath()
        ctx.roundRect(-p.w / 2, -p.h / 2, p.w, p.h, 5)
        ctx.fill()
        ctx.stroke()
        ctx.restore()
      })
      needNextFrame = true
    }
  }

  if (sparks.length > 0) {
    for (let i = sparks.length - 1; i >= 0; i--) {
      const s = sparks[i]
      s.x += s.vx
      s.y += s.vy
      s.vx *= 0.96
      s.vy = s.vy * 0.96 + 0.07
      s.rot += 0.14
      s.alpha -= s.decay
      if (s.alpha <= 0) {
        sparks.splice(i, 1)
        continue
      }
      ctx.save()
      ctx.globalAlpha = Math.max(0, s.alpha)
      ctx.strokeStyle = s.color
      ctx.lineWidth = s.shape === 'comet' ? 3.6 : 2.2
      ctx.lineCap = 'round'
      ctx.beginPath()
      ctx.moveTo(s.x - s.vx * 3.0, s.y - s.vy * 3.0)
      ctx.lineTo(s.x, s.y)
      ctx.stroke()

      ctx.translate(s.x, s.y)
      ctx.rotate(s.rot)
      ctx.fillStyle = s.color
      ctx.strokeStyle = '#121316'
      ctx.lineWidth = 1.4
      if (s.shape === 'star') {
        drawFourPointStar(ctx, s.size * 1.4)
        ctx.fill()
        ctx.stroke()
      } else {
        ctx.beginPath()
        ctx.arc(0, 0, s.size * 0.78, 0, Math.PI * 2)
        ctx.fill()
        ctx.stroke()
      }
      ctx.restore()
    }
    hasActiveSparks.value = sparks.length > 0
    if (sparks.length > 0) needNextFrame = true
  } else {
    hasActiveSparks.value = false
  }

  if (needNextFrame) {
    rafId = requestAnimationFrame(tickCanvas)
  }
}

function renderLines() {
  const svg = linesSvgRef.value
  const grid = stageGridRef.value
  if (!svg || !grid) return
  const gRect = grid.getBoundingClientRect()
  if (gRect.width === 0) return
  svg.setAttribute('viewBox', `0 0 ${gRect.width} ${gRect.height}`)
  svg.innerHTML = ''

  const leftEls = [0, 1, 2, 3].map((i) => document.getElementById('left-' + i))
  const rightEls = modelItems.map((_, i) => document.getElementById('right-' + i))
  const coreEl = document.getElementById('center-core')
  if (!coreEl) return
  const cRect = coreEl.getBoundingClientRect()
  const coreLeftX = cRect.left - gRect.left
  const coreRightX = cRect.right - gRect.left
  const coreCenterY = cRect.top + cRect.height / 2 - gRect.top
  const activeIdx = hoveredIdx.value

  if (currentMode.value === 'chaos') {
    leftEls.forEach((lEl, i) => {
      if (!lEl) return
      const lRect = lEl.getBoundingClientRect()
      const x1 = lRect.right - gRect.left
      const y1 = lRect.top + lRect.height / 2 - gRect.top
      const x2 = coreLeftX
      const y2 = coreCenterY + (i - 1.5) * 44
      const midX = (x1 + x2) / 2
      const d = `M ${x1} ${y1} C ${midX} ${y1 + (i % 2 ? 40 : -40)}, ${midX} ${y2 + (i % 2 ? -40 : 40)}, ${x2} ${y2}`
      appendChaosPath(svg, d, i % 2 === 0 ? '#FF3B30' : '#FF9500', activeIdx === i)
    })
    rightEls.forEach((rEl, i) => {
      if (!rEl) return
      const rRect = rEl.getBoundingClientRect()
      const x1 = coreRightX
      const y1 = coreCenterY + (i - 1.5) * 44
      const x2 = rRect.left - gRect.left
      const y2 = rRect.top + rRect.height / 2 - gRect.top
      const midX = (x1 + x2) / 2
      const d = `M ${x1} ${y1} C ${midX} ${y1 + (i % 2 ? -35 : 35)}, ${midX} ${y2 + (i % 2 ? 35 : -35)}, ${x2} ${y2}`
      appendChaosPath(svg, d, i % 2 === 0 ? '#FF3B30' : '#FF9500', activeIdx === i)
    })
  } else {
    leftEls.forEach((lEl, i) => {
      if (!lEl) return
      const lRect = lEl.getBoundingClientRect()
      const x1 = lRect.right - gRect.left
      const y1 = lRect.top + lRect.height / 2 - gRect.top
      const x2 = coreLeftX
      const y2 = coreCenterY + (i - 1.5) * 32
      const d = `M ${x1} ${y1} C ${(x1 + x2) / 2} ${y1}, ${(x1 + x2) / 2} ${y2}, ${x2} ${y2}`
      const isBoost = activeIdx === i
      appendOrderFiber(svg, d, '#2563EB', isBoost ? '0.24s' : `${0.85 + i * 0.1}s`, isBoost)
    })
    rightEls.forEach((rEl, i) => {
      if (!rEl) return
      const rRect = rEl.getBoundingClientRect()
      const x1 = coreRightX
      const y1 = coreCenterY + (i - 1.5) * 32
      const x2 = rRect.left - gRect.left
      const y2 = rRect.top + rRect.height / 2 - gRect.top
      const d = `M ${x1} ${y1} C ${(x1 + x2) / 2} ${y1}, ${(x1 + x2) / 2} ${y2}, ${x2} ${y2}`
      const isBoost = activeIdx === i
      appendOrderFiber(svg, d, '#60A5FA', isBoost ? '0.22s' : `${0.8 + i * 0.1}s`, isBoost)
    })
  }
}

function appendChaosPath(svg: SVGSVGElement, d: string, color: string, isBoost: boolean) {
  const p = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  p.setAttribute('d', d)
  p.setAttribute('fill', 'none')
  p.setAttribute('stroke', isBoost ? '#FF1E00' : color)
  p.setAttribute('stroke-width', isBoost ? '4.5' : '2.5')
  p.setAttribute('stroke-dasharray', '8 7')
  p.setAttribute('opacity', isBoost ? '1' : '0.62')
  if (isBoost) p.classList.add('chaos-rapid-dash')
  svg.appendChild(p)

  if (isBoost) {
    for (let k = 0; k < 3; k++) {
      const dot = document.createElementNS('http://www.w3.org/2000/svg', 'circle')
      dot.setAttribute('r', '6.5')
      dot.setAttribute('fill', '#FF3B30')
      dot.setAttribute('stroke', '#121316')
      dot.setAttribute('stroke-width', '2')
      const m = document.createElementNS('http://www.w3.org/2000/svg', 'animateMotion')
      m.setAttribute('path', d)
      m.setAttribute('dur', '0.26s')
      if (k > 0) m.setAttribute('begin', `${k * 0.08}s`)
      m.setAttribute('repeatCount', 'indefinite')
      dot.appendChild(m)
      svg.appendChild(dot)
    }
  }
}

function appendOrderFiber(svg: SVGSVGElement, d: string, color: string, dur: string, isBoost: boolean) {
  const base = document.createElementNS('http://www.w3.org/2000/svg', 'path')
  base.setAttribute('d', d)
  base.setAttribute('fill', 'none')
  base.setAttribute('stroke', color)
  base.setAttribute('stroke-width', isBoost ? '5' : '3')
  base.setAttribute('opacity', isBoost ? '0.9' : '0.32')
  svg.appendChild(base)

  const count = isBoost ? 3 : 1
  for (let k = 0; k < count; k++) {
    const dot = document.createElementNS('http://www.w3.org/2000/svg', 'circle')
    dot.setAttribute('r', isBoost ? '7' : '5.5')
    dot.setAttribute('fill', '#CCFF00')
    dot.setAttribute('stroke', '#121316')
    dot.setAttribute('stroke-width', '2')
    const m = document.createElementNS('http://www.w3.org/2000/svg', 'animateMotion')
    m.setAttribute('path', d)
    m.setAttribute('dur', dur)
    if (k > 0) m.setAttribute('begin', `${k * 0.08}s`)
    m.setAttribute('repeatCount', 'indefinite')
    dot.appendChild(m)
    svg.appendChild(dot)
  }
}

const latestModels = [
  'claude-opus-5.5',
  'gemini-4',
  'gpt-6-astra',
  'gpt-6.1-sol',
  'grok-4.7',
  'deepseek-4.1',
  'kimi-k3'
]
const mIdx = ref(0)
const activeModelName = computed(() => latestModels[mIdx.value])
function cycleModel() {
  mIdx.value = (mIdx.value + 1) % latestModels.length
}

const baseCopied = ref(false)
function copyEndpoint() {
  navigator.clipboard?.writeText(`${displayBaseUrl.value}/v1`)
  baseCopied.value = true
  setTimeout(() => {
    baseCopied.value = false
  }, 1500)
}

function resizeCanvas() {
  const canvas = morphCanvasRef.value
  if (!canvas) return
  const cw = window.innerWidth
  const ch = window.innerHeight
  canvas.width = cw * window.devicePixelRatio
  canvas.height = ch * window.devicePixelRatio
  const ctx = canvas.getContext('2d')
  ctx?.setTransform(window.devicePixelRatio, 0, 0, window.devicePixelRatio, 0, 0)
  renderLines()
}

function onKeyDown(e: KeyboardEvent) {
  if (!isHome.value) {
    if (e.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
      if (searchInputRef.value) {
        e.preventDefault()
        searchInputRef.value.focus()
      }
    }
    return
  }
  if (e.code === 'Space' && document.activeElement?.tagName !== 'INPUT') {
    e.preventDefault()
    toggleMode()
  }
}

const detail = ref<any | null>(null)

watch(
  () => route.fullPath,
  () => {
    if (isHome.value) {
      nextTick(() => {
        resizeCanvas()
        setTimeout(renderLines, 60)
      })
    }
  }
)

onMounted(() => {
  resizeCanvas()
  window.addEventListener('resize', resizeCanvas)
  window.addEventListener('keydown', onKeyDown)
  setTimeout(renderLines, 80)
  // R12-03/R12-04：URL > cookie > localStorage > 浏览器，跨标签用 storage 事件同步
  disposeLocaleSync = initLocaleSync((next) => {
    void applySyncedLocale(next)
  })
  document.addEventListener('click', closeLocaleMenuOnOutsideClick)
  document.addEventListener('keydown', closeLocaleMenuOnEscape)
  document.addEventListener('visibilitychange', handleAnalyticsVisibility)
  window.addEventListener('pagehide', handleAnalyticsPageHide)
})

onBeforeUnmount(() => {
  disposeLocaleSync?.()
  disposeLocaleSync = null
  document.removeEventListener('click', closeLocaleMenuOnOutsideClick)
  document.removeEventListener('keydown', closeLocaleMenuOnEscape)
  window.removeEventListener('resize', resizeCanvas)
  window.removeEventListener('keydown', onKeyDown)
  document.removeEventListener('visibilitychange', handleAnalyticsVisibility)
  window.removeEventListener('pagehide', handleAnalyticsPageHide)
  publicImpressionObserver?.disconnect()
  publicImpressionObserver = null
  publicAnalytics.leave()
  publicAnalytics.dispose()
  if (chargeDoneTimer) clearTimeout(chargeDoneTimer)
  if (fireworkInterval) clearInterval(fireworkInterval)
  if (rafId) cancelAnimationFrame(rafId)
})

// ==================== PUBLIC CONTENT (NEWS & ARTICLES) API LOGIC ====================
const publicItems = ref<any[]>([]);
const contentLoading = ref(false);
const errorMessage = ref("");
const searchInputRef = ref<HTMLInputElement | null>(null);
const kind = computed(() => {
  const path = route.path.replace(/^\/(en|zh|ru|fr|de)(?=\/|$)/, "");
  return path.startsWith("/articles") ? "articles" : "news";
});

// ==================== 前台列表分页状态（后端 page_size 窗口 + 跨 locale 合并后按页切片） ====================
const PAGE_SIZE_DEFAULT = 20;
const DETAIL_WINDOW_SIZE = 48;
const currentPage = ref(1);
const pageSize = ref(PAGE_SIZE_DEFAULT);
const pageWindowHasMore = ref(false);
const publicTotalCount = ref(0);
const paginationReady = ref(false);

const CATEGORY_ZH_TO_EN: Record<string, string> = {
  "架构与网关": "Architecture & Gateway",
  "客户端接入": "Client Integration",
  "多模态与生图": "Multimodal & Vision",
  "模型路由与评测": "Model Routing & Benchmarks",
  "智能体工程": "Agent Engineering",
  "成本与高可用": "Cost & High Availability",
  "企业安全与合规": "Enterprise Security",
  "安全合规": "Security & Compliance",
  "模型与 Agent": "Models & Agents",
  "开源生态": "Open Source Ecosystem",
  "官方动态": "Official Announcements",
  "模型发布": "Model Releases",
  "平台动态": "Platform Updates",
  "开源与工具": "Open Source & Tools",
  "安全与治理": "Security & Governance",
  "工程架构": "Engineering Architecture",
  "AI 行业洞察": "AI Industry Insights"
};

const CATEGORY_EN_TO_ZH: Record<string, string> = Object.fromEntries(
  Object.entries(CATEGORY_ZH_TO_EN).map(([zh, en]) => [en.toLowerCase(), zh])
);

function translateCategory(rawCat: string, zhMode: boolean): string {
  const clean = String(rawCat || "").trim();
  if (!clean) return zhMode ? (kind.value === "news" ? "平台动态" : "架构与网关") : (kind.value === "news" ? "Platform Updates" : "Architecture & Gateway");
  if (zhMode) {
    return CATEGORY_EN_TO_ZH[clean.toLowerCase()] || clean;
  }
  return CATEGORY_ZH_TO_EN[clean] || clean;
}

function localizeItem(item: any, _kindName: "news" | "articles" = kind.value): any {
  if (!item) return item;
  const zhMode = isZh.value;
  const resolvedCat = item.category || item.mediaJson?.category || "";
  const resolvedTags = Array.isArray(item.tags) && item.tags.length
    ? item.tags
    : (Array.isArray(item.mediaJson?.tags) ? item.mediaJson.tags : []);
  const rawCat = zhMode
    ? (resolvedCat || item.categoryEn || item.sourceName || "")
    : (item.categoryEn || resolvedCat || item.sourceName || "");
  const displayCategory = translateCategory(rawCat, zhMode);
  const displayTitle = zhMode
    ? (item.titleZh || item.title || "")
    : (item.titleEn || item.title || "");
  const displaySummary = zhMode
    ? (item.summaryZh || item.summary || item.excerpt || "")
    : (item.summaryEn || item.summary || item.excerpt || "");
  const displayBodyText = zhMode
    ? (item.bodyTextZh || item.bodyText || item.bodyMarkdown || "")
    : (item.bodyTextEn || item.bodyText || item.bodyMarkdown || "");

  let displayBodyHtml = item.bodyHtml || "";
  // 正文富文本优先：只有拿不到 HTML 时才用 markdown/纯文本兜底，
  // 否则会把编辑器保存的 h2/ul/blockquote/img 结构重新拍平成纯文本。
  if (!displayBodyHtml && displayBodyText) {
    displayBodyHtml = DOMPurify.sanitize(marked.parse(displayBodyText) as string, { USE_PROFILES: { html: true } });
  }

  return {
    ...item,
    category: resolvedCat || item.category,
    tags: resolvedTags,
    sourceUrl: item.sourceUrl || item.sourceCanonicalUrl || item.url || "",
    displayCategory,
    displayTitle,
    displaySummary,
    displayBodyText,
    displayBodyHtml
  };
}

const localizedPublicItems = computed(() => {
  return (publicItems.value || []).map((item) => localizeItem(item));
});

const featuredArticles = ref<any[]>([]);
const featuredItems = computed(() => [...featuredArticles.value]
  .sort((a: any, b: any) => {
    const timeDiff = featuredSortTime(b) - featuredSortTime(a);
    if (timeDiff !== 0) return timeDiff;
    return Number(b?.id || 0) - Number(a?.id || 0);
  })
  .slice(0, 10));
const showFeaturedSection = computed(() => (
  kind.value === "articles" &&
  currentPage.value === 1 &&
  paginationEnabled.value &&
  featuredItems.value.length > 0
));

function featuredImage(item: any): string {
  return String(item?.coverImage || item?.imageUrl || item?.coverUrl || "").trim();
}

// 统一封面取址：coverImage 优先，其次历史字段/媒体 JSON；空值一律交给占位图
function coverKeyOf(item: any): string {
  const id = item?.id ?? item?.slug ?? "";
  return String(id || item?.coverImage || item?.imageUrl || item?.coverUrl || "");
}

const brokenCovers = ref<Record<string, boolean>>({});

function contentCover(item: any): string {
  if (!item) return "";
  if (brokenCovers.value[coverKeyOf(item)]) return "";
  return String(
    item.coverImage || item.imageUrl || item.coverUrl || item.mediaJson?.coverImage || ""
  ).trim();
}

function markCoverBroken(item: any) {
  if (!item) return;
  brokenCovers.value = { ...brokenCovers.value, [coverKeyOf(item)]: true };
}

function coverAltOf(item: any): string {
  return String(item?.coverAlt || item?.displayTitle || item?.title || "").trim();
}

// 相关推荐排序：共同标签数 → 发布时间 → 当前语言译文优先 → ID 稳定兜底
function buildRelatedItems(pool: any[], current: any, zhMode: boolean, limit = 6): any[] {
  if (!current) return [];
  const curId = current.id;
  const curSlug = String(current.slug || "");
  const tagsOf = (item: any): string[] => (Array.isArray(item?.tags) ? item.tags : [])
    .map((tag: any) => String(tag ?? "").trim().toLowerCase())
    .filter(Boolean);
  const curTags = tagsOf(current);
  const curCategory = String(current.category || "").trim().toLowerCase();
  const timeOf = (item: any): number => {
    const raw = item?.publishedAt || item?.createdAt || item?.originalPublishedAt;
    const stamp = raw ? new Date(raw).getTime() : 0;
    return Number.isFinite(stamp) ? stamp : 0;
  };
  const hasLocaleCopy = (item: any): boolean => (
    zhMode ? Boolean(item?.titleZh || item?.title) : Boolean(item?.titleEn)
  );
  const sharedTags = (item: any): number => {
    const set = new Set(tagsOf(item));
    return curTags.reduce((total, tag) => total + (set.has(tag) ? 1 : 0), 0);
  };
  const candidates = pool.filter((item: any) => {
    if (!item) return false;
    if (curSlug && String(item.slug || "") === curSlug) return false;
    if (curId != null && item.id === curId) return false;
    const status = String(item.status || "").toLowerCase();
    return !status || status === "published";
  });
  const byPriority = (a: any, b: any): number => {
    const tagDiff = sharedTags(b) - sharedTags(a);
    if (tagDiff !== 0) return tagDiff;
    const timeDiff = timeOf(b) - timeOf(a);
    if (timeDiff !== 0) return timeDiff;
    const localeDiff = Number(hasLocaleCopy(b)) - Number(hasLocaleCopy(a));
    if (localeDiff !== 0) return localeDiff;
    return Number(b?.id || 0) - Number(a?.id || 0);
  };
  const chosen: any[] = [];
  const take = (list: any[]) => {
    for (const item of list) {
      if (chosen.length >= limit) return;
      if (!chosen.includes(item)) chosen.push(item);
    }
  };
  take(candidates.filter((item: any) => sharedTags(item) > 0).slice().sort(byPriority));
  if (chosen.length < limit) {
    take(candidates
      .filter((item: any) => curCategory && String(item.category || "").trim().toLowerCase() === curCategory)
      .slice()
      .sort(byPriority));
  }
  if (chosen.length < limit) take(candidates.slice().sort(byPriority));
  return chosen.slice(0, limit);
}

function featuredSortTime(item: any): number {
  const raw = item?.featuredAt || item?.featured_at;
  const timestamp = raw ? new Date(raw).getTime() : 0;
  return Number.isFinite(timestamp) ? timestamp : 0;
}

const tocItems = computed(() => {
  const source = String(detail.value?.displayBodyHtml || detail.value?.bodyHtml || "");
  const headings: { id: string; text: string }[] = [];
  const matcher = /<h([23])[^>]*>([\s\S]*?)<\/h\1>/gi;
  let match: RegExpExecArray | null;
  while ((match = matcher.exec(source))) {
    const text = match[2].replace(/<[^>]+>/g, "").trim();
    if (text) headings.push({ id: `toc-${headings.length}`, text });
  }
  return headings;
});

const PUBLIC_LOCALE_ALIASES: Record<string, string> = {
  zh: "zh-CN",
  "zh-cn": "zh-CN",
  "zh-hans": "zh-CN",
  "zh-sg": "zh-CN",
  "zh-hant": "zh-TW",
  "zh-tw": "zh-TW",
  en: "en-US",
  "en-us": "en-US",
  "en-gb": "en-US"
};

/** 归一化前台 locale：zh→zh-CN、en→en-US，避免列表里 zh / zh-CN 混链 */
function normalizePublicLocale(raw: any, fallback = ""): string {
  const value = String(raw ?? "").trim();
  if (!value) return fallback;
  const key = value.replace(/_/g, "-").toLowerCase();
  return PUBLIC_LOCALE_ALIASES[key] || value;
}

/** 指定语言的详情候选：规范化语言优先，再退回历史遗留写法（如 zh） */
function localeCandidates(locale: string): string[] {
  const canonical = normalizePublicLocale(locale);
  if (!canonical) return [];
  const key = canonical.toLowerCase();
  const list = [canonical];
  if (key === "zh-cn") list.push("zh");
  if (key === "en-us") list.push("en");
  return Array.from(new Set(list.filter(Boolean)));
}

function itemPath(item: any, itemKind?: "news" | "articles") {
  const targetKind = itemKind || (item.contentType === "article" || kind.value === "articles" ? "articles" : "news");
  // URL 只用短码（技术设计 §3.3）：zh-CN → zh，避免每次点击多一跳 301
  const segment = normalizeLocaleCode(item.locale) || urlLocale.value;
  return `/${targetKind}/${segment}/${item.slug}`;
}

function truncateText(value: any, max = 96): string {
  const text = String(value ?? "").replace(/\s+/g, " ").trim();
  if (text.length <= max) return text;
  return text.slice(0, max).trimEnd() + "…";
}

const searchQuery = ref('');
const activeCategory = ref('ALL');
const copiedLink = ref(false);

const filteredItems = computed(() => {
  let list = localizedPublicItems.value || [];
  if (activeCategory.value !== 'ALL') {
    list = list.filter((item: any) => (item.displayCategory || item.category || item.sourceName || '') === activeCategory.value);
  }
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.trim().toLowerCase();
    list = list.filter((item: any) =>
      (item.displayTitle && item.displayTitle.toLowerCase().includes(q)) ||
      (item.title && item.title.toLowerCase().includes(q)) ||
      (item.displaySummary && item.displaySummary.toLowerCase().includes(q)) ||
      (item.summary && item.summary.toLowerCase().includes(q)) ||
      (item.excerpt && item.excerpt.toLowerCase().includes(q)) ||
      (item.displayCategory && item.displayCategory.toLowerCase().includes(q)) ||
      (item.category && item.category.toLowerCase().includes(q))
    );
  }
  return list;
});

const paginationEnabled = computed(() => !searchQuery.value.trim() && activeCategory.value === "ALL");

const pagedItems = computed(() => {
  const list = filteredItems.value || [];
  if (!paginationEnabled.value) return list;
  const start = (currentPage.value - 1) * pageSize.value;
  return list.slice(start, start + pageSize.value);
});

const totalPages = computed(() => {
  const loaded = filteredItems.value.length;
  if (!paginationEnabled.value) return 1;
  const total = Math.max(publicTotalCount.value || 0, loaded);
  return Math.max(1, Math.ceil(total / pageSize.value));
});

const pagerVisible = computed(() => (
  paginationReady.value && paginationEnabled.value && totalPages.value > 1
));

const listCountLabel = computed(() => {
  if (!paginationEnabled.value) return filteredItems.value.length;
  return Math.max(publicTotalCount.value || 0, filteredItems.value.length);
});

const pageNumbers = computed(() => {
  const total = totalPages.value;
  const cur = currentPage.value;
  const span = Math.min(5, total);
  const start = Math.max(1, Math.min(cur - 2, total - span + 1));
  const nums: number[] = [];
  for (let p = start; p < start + span; p++) nums.push(p);
  return nums;
});

function goToPage(target: number) {
  const next = Math.min(Math.max(1, Math.floor(target) || 1), totalPages.value);
  if (next === currentPage.value) return;
  const query: Record<string, string> = {};
  for (const [k, v] of Object.entries(route.query)) {
    if (typeof v === "string") query[k] = v;
  }
  if (next > 1) query.page = String(next);
  else delete query.page;
  void router.push({ path: route.path, query });
  if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
}

function isItemFeatured(item: any): boolean {
  const tags = Array.isArray(item?.tags) ? item.tags : [];
  return tags.some((t: any) => {
    const s = String(t || '').trim();
    return s === '精选' || s.toLowerCase() === 'featured';
  });
}

function getCleanItemTags(item: any): string[] {
  const tags = Array.isArray(item?.tags) ? item.tags : [];
  return tags
    .map((t: any) => String(t || '').trim())
    .filter((s: string) => s && s !== '精选' && s.toLowerCase() !== 'featured');
}

const featuredItem = computed(() => {
  if (currentPage.value !== 1) return null;
  return pagedItems.value.length > 0 ? pagedItems.value[0] : null;
});

const remainingItems = computed(() => {
  if (currentPage.value !== 1) return pagedItems.value.slice(0);
  return pagedItems.value.length > 1 ? pagedItems.value.slice(1) : [];
});

const availableCategories = computed(() => {
  const set = new Set<string>();
  for (const item of localizedPublicItems.value || []) {
    if (item.displayCategory) set.add(item.displayCategory);
    else if (item.category) set.add(item.category);
  }
  return Array.from(set);
});

const currentDetailIndex = computed(() => {
  const curSlug = String(detail.value?.slug || route.params.slug || "");
  if (!curSlug) return -1;
  return localizedPublicItems.value.findIndex((item) => item.slug === curSlug || item.id === detail.value?.id);
});

const prevItem = computed(() => {
  const idx = currentDetailIndex.value;
  if (idx > 0) return localizedPublicItems.value[idx - 1];
  return null;
});

const nextItem = computed(() => {
  const idx = currentDetailIndex.value;
  if (idx >= 0 && idx < localizedPublicItems.value.length - 1) {
    return localizedPublicItems.value[idx + 1];
  }
  return null;
});

// R9-09：固定 6 条，先共同标签数、再发布时间；不足按分类、最新补齐；排除当前篇
const moreRelatedItems = computed(() => {
  const current = detail.value
    ? { ...detail.value, slug: detail.value.slug || String(route.params.slug || "") }
    : null;
  if (!current) return [];
  return buildRelatedItems(localizedPublicItems.value, current, isZh.value, 6);
});

const detailCover = computed(() => contentCover(detail.value));

function updateSeoMetadata() {
  if (typeof document === "undefined") return;
  if (isHome.value) return;

  const setMeta = (selector: string, attrName: string, attrVal: string, content: string) => {
    let el = document.head.querySelector(selector) as HTMLMetaElement | null;
    if (!el) {
      el = document.createElement("meta");
      el.setAttribute(attrName, attrVal);
      document.head.appendChild(el);
    }
    el.setAttribute("content", content);
  };

  if (detail.value) {
    const title = `${detail.value.displayTitle || detail.value.seoTitle || detail.value.title} - CCAI`;
    const desc = String(detail.value.displaySummary || detail.value.seoDescription || detail.value.summary || detail.value.excerpt || "").slice(0, 160);
    const canonicalHref = detail.value.canonicalUrl || window.location.href;
    document.title = title;
    setMeta('meta[name="description"]', "name", "description", desc);
    setMeta('meta[property="og:title"]', "property", "og:title", title);
    setMeta('meta[property="og:description"]', "property", "og:description", desc);
    setMeta('meta[property="og:type"]', "property", "og:type", "article");
    setMeta('meta[property="og:url"]', "property", "og:url", canonicalHref);

    let canonicalEl = document.head.querySelector('link[rel="canonical"]') as HTMLLinkElement | null;
    if (!canonicalEl) {
      canonicalEl = document.createElement("link");
      canonicalEl.setAttribute("rel", "canonical");
      document.head.appendChild(canonicalEl);
    }
    canonicalEl.setAttribute("href", canonicalHref);

    let ldEl = document.head.querySelector('script[data-ccai-seo="jsonld"]') as HTMLScriptElement | null;
    if (!ldEl) {
      ldEl = document.createElement("script");
      ldEl.type = "application/ld+json";
      ldEl.setAttribute("data-ccai-seo", "jsonld");
      document.head.appendChild(ldEl);
    }
    const schema = {
      "@context": "https://schema.org",
      "@type": kind.value === "news" ? "NewsArticle" : (detail.value.schemaType || "TechArticle"),
      headline: detail.value.displayTitle || detail.value.title,
      description: desc,
      datePublished: detail.value.publishedAt || detail.value.createdAt,
      dateModified: detail.value.updatedAt || detail.value.publishedAt || detail.value.createdAt,
      author: {
        "@type": "Organization",
        name: detail.value.author || detail.value.sourceName || "CCAI Engineering"
      },
      publisher: {
        "@type": "Organization",
        name: "CCAI"
      },
      mainEntityOfPage: canonicalHref
    };
    ldEl.textContent = JSON.stringify(schema);
  } else {
    const listTitle = kind.value === "news"
      ? (isZh.value ? "新闻资讯 - CCAI 大模型网关动态" : "News - CCAI Enterprise AI Gateway")
      : (isZh.value ? "深度解析 - CCAI 技术专栏与最佳实践" : "Deep Dive - CCAI Technical Publications");
    const listDesc = kind.value === "news"
      ? (isZh.value ? "大模型发布、API 变化和 CCAI 网关公告。" : "Model releases, API changes and CCAI announcements.")
      : (isZh.value ? "接入教程、客户端配置与企业级大模型网关工程实践。" : "Setup guides, client configuration and enterprise AI gateway engineering.");
    document.title = listTitle;
    setMeta('meta[name="description"]', "name", "description", listDesc);
  }
}

function estimateReadingTime(text: string): number {
  if (!text) return 3;
  const words = text.length;
  return Math.max(1, Math.round(words / 400));
}

async function copyArticleUrl() {
  try {
    await navigator.clipboard.writeText(window.location.href);
    copiedLink.value = true;
    setTimeout(() => { copiedLink.value = false; }, 2000);
  } catch (e) {
    // ignore
  }
}

function formatDate(value: string) {
  return value ? new Date(value).toLocaleDateString() : "";
}

function jumpToHeading(id: string) {
  const index = Number(id.replace("toc-", ""));
  const headings = document.querySelectorAll(".neo-prose-body h2, .neo-prose-body h3");
  headings[index]?.scrollIntoView({ behavior: "smooth", block: "start" });
}

// BI tracking: one tracker per view; every send is fire-and-forget so a slow
// or failing collector can never block clicks, navigation or rendering (R11-06).
const publicAnalytics = createContentAnalyticsTracker({
  send: (event) => postPublicAnalyticsEvent(event)
})
let publicImpressionObserver: IntersectionObserver | null = null

function startPublicView() {
  if (typeof window === 'undefined') return
  const contentType = kind.value === "news" ? "news" : "article";
  const rawId = Number(detail.value?.id);
  publicAnalytics.start({
    contentType,
    contentId: Number.isFinite(rawId) && rawId > 0 ? rawId : undefined,
    locale: String(route.params.locale || (isZh.value ? "zh-CN" : "en-US")),
    pagePath: window.location.pathname + window.location.search,
    referrerHost: referrerHostOf(document.referrer, window.location.host)
  });
  void nextTick(observePublicPlacements);
}

// Exposure is measured once per placement+content: the observer unobserves as
// soon as the card is half visible, and the tracker de-duplicates as well.
function observePublicPlacements() {
  publicImpressionObserver?.disconnect();
  if (typeof IntersectionObserver === "undefined") return;
  const observer = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      if (!entry.isIntersecting) continue;
      const el = entry.target as HTMLElement;
      const placement = el.dataset.analyticsPlacement;
      const contentId = Number(el.dataset.analyticsId);
      if (!placement) continue;
      publicAnalytics.impression(placement, Number.isFinite(contentId) && contentId > 0 ? contentId : undefined);
      observer.unobserve(el);
    }
  }, { threshold: 0.5 });
  publicImpressionObserver = observer;
  document
    .querySelectorAll<HTMLElement>("[data-analytics-placement]")
    .forEach((el) => observer.observe(el));
}

function trackPlacementClick(placement: string, item?: { id?: number | string } | null) {
  const contentId = Number(item?.id);
  publicAnalytics.click(placement, Number.isFinite(contentId) && contentId > 0 ? contentId : undefined);
}

function handleAnalyticsVisibility() {
  if (document.visibilityState === "hidden") {
    publicAnalytics.heartbeat();
    publicAnalytics.pause();
  } else {
    publicAnalytics.resume();
  }
}

function handleAnalyticsPageHide() {
  publicAnalytics.leave();
}

function extractListItems(res: any): any[] {
  if (Array.isArray(res?.items)) return res.items;
  if (Array.isArray(res?.data?.items)) return res.data.items;
  if (Array.isArray(res?.data)) return res.data;
  if (Array.isArray(res)) return res;
  return [];
}

async function fetchMergedPublicList(
  primaryLocales: string[],
  allLocales: string[],
  opts: { page?: number; pageSize?: number } = {}
): Promise<{ items: any[]; hasMore: boolean; windowSize: number; total: number }> {
  const page = Math.max(1, Math.floor(opts.page || 1));
  const pageSizeLimit = Math.min(40, Math.max(1, Math.floor(opts.pageSize || 12)));
  // 跨 locale 合并要保证顺序稳定：向后端要「前 page*pageSize 条」连续窗口，再按页切片，不重不漏
  const windowSize = Math.max(1, page * pageSizeLimit);
  const queryLocales = kind.value === "news" ? allLocales : primaryLocales;
  // 单次请求上限 page_size=100，窗口更大时分页累加
  const fetchWindow = async (loc: string) => {
    const collected: any[] = [];
    let total = NaN;
    let p = 1;
    // 注意：后端按「请求的 page_size」计算偏移量，翻页时若把每页条数改小（如第二页只要剩余 20 条），
    // 拿回的是 21..40 这类重叠区间，去重后会丢数据、末页变空。所以这里固定 chunk 翻页，最后再裁到窗口长度。
    const chunk = 100;
    while (collected.length < windowSize && p <= 10) {
      const res: any = await apiClient.get(`/public/${kind.value}`, {
        params: { locale: loc, page: p, page_size: chunk }
      });
      const list = extractListItems(res);
      collected.push(...list);
      const t = Number(res?.total ?? res?.data?.total ?? NaN);
      if (!Number.isNaN(t)) total = t;
      if (list.length < chunk) break;
      p += 1;
    }
    return { items: collected.slice(0, windowSize), total };
  };
  const results = await Promise.allSettled(queryLocales.map((loc) => fetchWindow(loc)));
  const merged: any[] = [];
  const seen = new Set<string>();
  for (const r of results) {
    if (r.status === "fulfilled") {
      for (const item of r.value.items) {
        const dedupeKey = kind.value === "articles"
          ? `${item.slug || item.id}`
          : `${item.locale || ""}:${item.slug || item.id}`;
        if (!seen.has(dedupeKey)) {
          seen.add(dedupeKey);
          merged.push(item);
        }
      }
    }
  }
  let hasMore = false;
  let totalCount = 0;
  for (const r of results) {
    if (r.status === "fulfilled") {
      const fetched = r.value.items.length;
      const total = r.value.total;
      // 文章按 slug 跨语种去重，各语种 total 指向同一集合 → 取最大值；新闻按语种区分 → 累加
      if (kind.value === "articles") {
        totalCount = Math.max(totalCount, Number.isNaN(total) ? fetched : total);
      } else {
        totalCount += Number.isNaN(total) ? fetched : total;
      }
      if (fetched >= windowSize || (!Number.isNaN(total) && total > fetched)) {
        hasMore = true;
      }
    }
  }
  if (kind.value === "articles" && merged.length === 0) {
    const fallbackResults = await Promise.allSettled(
      allLocales.map((loc) => apiClient.get(`/public/${kind.value}`, {
        params: { locale: loc, page: 1, page_size: Math.min(100, windowSize) }
      }))
    );
    for (const r of fallbackResults) {
      if (r.status === "fulfilled") {
        for (const item of extractListItems(r.value)) {
          const dedupeKey = `${item.slug || item.id}`;
          if (!seen.has(dedupeKey)) {
            seen.add(dedupeKey);
            merged.push(item);
          }
        }
      }
    }
  }
  merged.sort(
    (a, b) =>
      new Date(b.publishedAt || b.createdAt || 0).getTime() -
      new Date(a.publishedAt || a.createdAt || 0).getTime()
  );
  return { items: merged, hasMore, windowSize, total: totalCount || merged.length };
}

// ==================== 首页内容区块（最新资讯 / 深度解析） ====================
const homeNews = ref<any[]>([]);
const homeArticles = ref<any[]>([]);
const homeLoading = ref(false);

async function fetchHomeHighlights(kindName: "news" | "articles", size = 3): Promise<any[]> {
  const locales = isZh.value
    ? ["zh-CN", "zh", "en-US", "en"]
    : ["en-US", "en", "zh-CN", "zh"];
  const results = await Promise.allSettled(
    locales.map((loc) => apiClient.get(`/public/${kindName}`, {
      params: { locale: loc, page: 1, page_size: size }
    }))
  );
  const merged: any[] = [];
  const seen = new Set<string>();
  for (const r of results) {
    if (r.status !== "fulfilled") continue;
    for (const item of extractListItems(r.value)) {
      const key = kindName === "news"
        ? `${item.locale || ""}:${item.slug || item.id}`
        : `${item.slug || item.id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      merged.push(localizeItem(item, kindName));
    }
  }
  merged.sort(
    (a, b) =>
      new Date(b.publishedAt || b.createdAt || 0).getTime() -
      new Date(a.publishedAt || a.createdAt || 0).getTime()
  );
  return merged.slice(0, size);
}

async function loadHomeHighlights() {
  homeLoading.value = true;
  try {
    const [news, articles] = await Promise.all([
      fetchHomeHighlights("news", 3),
      fetchHomeHighlights("articles", 3)
    ]);
    homeNews.value = news;
    homeArticles.value = articles;
  } catch (err: any) {
    console.warn("Home highlights load note:", err?.message);
  } finally {
    homeLoading.value = false;
  }
}

async function loadFeaturedArticles(locales: string[]): Promise<any[]> {
  if (kind.value !== "articles") return [];
  const results = await Promise.allSettled(
    locales.map((locale) => apiClient.get("/public/articles/featured", { params: { locale } }))
  );
  const merged: any[] = [];
  const seen = new Set<string>();
  for (const result of results) {
    if (result.status !== "fulfilled") continue;
    for (const item of extractListItems(result.value)) {
      const key = String(item?.slug || item?.id || "");
      if (!key || seen.has(key)) continue;
      seen.add(key);
      merged.push(localizeItem(item, "articles"));
    }
  }
  if (merged.length > 0) return merged;
  return localizedPublicItems.value.filter((item) => isItemFeatured(item));
}

async function loadPublicContent() {
  if (isHome.value) return;
  contentLoading.value = true;
  errorMessage.value = "";
  activeCategory.value = "ALL";
  try {
    const primaryLocales = isZh.value ? ["zh-CN", "zh"] : ["en-US", "en"];
    const allLocales = isZh.value ? ["zh-CN", "zh", "en-US", "en"] : ["en-US", "en", "zh-CN", "zh"];
    const slug = String(route.params.slug || "");

    const requestedPage = Math.max(1, Math.floor(Number(route.query.page) || 1));
    const requestedPageSize = Math.min(24, Math.max(1, Math.floor(Number(route.query.page_size) || PAGE_SIZE_DEFAULT)));
    currentPage.value = slug ? 1 : requestedPage;
    pageSize.value = requestedPageSize;
    paginationReady.value = false;

    // 详情页取较大窗口（上一篇/下一篇 + 相关推荐）；列表页按 URL 页码向后端要分页窗口
    const listPromise = slug
      ? fetchMergedPublicList(primaryLocales, allLocales, { page: 1, pageSize: DETAIL_WINDOW_SIZE })
      : fetchMergedPublicList(primaryLocales, allLocales, { page: requestedPage, pageSize: requestedPageSize });

    if (slug) {
      // URL 前缀即「请求语言」：优先按请求语言取详情，其次界面语言，最后兜底其它语言
      const routeLocale = normalizePublicLocale(route.params.locale);
      const tryLocales = Array.from(new Set([
        ...(routeLocale ? localeCandidates(routeLocale) : []),
        ...primaryLocales,
        ...allLocales
      ]));
      let loaded: any = null;
      for (const loc of tryLocales) {
        try {
          const res: any = await apiClient.get(`/public/${kind.value}/${encodeURIComponent(loc)}/${encodeURIComponent(slug)}`);
          const unwrapped = res?.data && (res.data.slug || res.data.title) ? res.data : res;
          if (unwrapped && (unwrapped.slug || unwrapped.title)) {
            loaded = unwrapped;
            break;
          }
        } catch {
          // try next candidate locale
        }
      }
      const detailList = await listPromise;
      publicItems.value = detailList.items;
      featuredArticles.value = [];
      if (loaded) {
        const rawBody = loaded.bodyHtml || loaded.bodyText || loaded.bodyMarkdown || "";
        if (rawBody) {
          loaded.bodyHtml = DOMPurify.sanitize(marked.parse(rawBody) as string, { USE_PROFILES: { html: true } });
        }
        detail.value = localizeItem(loaded);
        // 请求语言无译文时回退到内容自身语言展示，但保留 URL 前缀（分享链接不被改写）
        updateSeoMetadata();
        startPublicView();
      }
    } else {
      detail.value = null;
      const listResult = await listPromise;
      publicItems.value = listResult.items;
      featuredArticles.value = await loadFeaturedArticles(primaryLocales);
      pageWindowHasMore.value = listResult.hasMore;
      publicTotalCount.value = listResult.total || listResult.items.length;
      paginationReady.value = true;
      // 页码越界（如旧链接 ?page=99）→ 回退到最后一页，保持 URL 与内容一致
      const maxPage = Math.max(1, Math.ceil(publicTotalCount.value / pageSize.value));
      if (currentPage.value > maxPage) {
        currentPage.value = maxPage;
        // 越界页的列表必然为空，必须按最后一页重新取数，否则页面会停在空列表
        const refill = await fetchMergedPublicList(primaryLocales, allLocales, {
          page: maxPage,
          pageSize: pageSize.value
        });
        publicItems.value = refill.items;
        pageWindowHasMore.value = refill.hasMore;
        publicTotalCount.value = refill.total || publicTotalCount.value;
        const query: Record<string, string> = {};
        for (const [k, v] of Object.entries(route.query)) {
          if (typeof v === "string") query[k] = v;
        }
        if (maxPage > 1) query.page = String(maxPage);
        else delete query.page;
        void router.replace({ path: route.path, query }).catch(() => {});
      }
      updateSeoMetadata();
      startPublicView();
    }
  } catch (err: any) {
    console.warn("Public content load note:", err?.message);
  } finally {
    contentLoading.value = false;
  }
}

watch([() => route.fullPath, isZh], () => {
  // Close the old view first so its duration/leave are not lost on navigation.
  publicAnalytics.leave()
  if (!isHome.value) {
    void loadPublicContent();
  } else {
    void loadHomeHighlights();
  }
}, { immediate: true });

</script>

<style scoped>
.ccai-home-viewport {
  --bg-chaos: #F5F2EB;
  --bg-order: #EDF4FF;
  --ink: #121316;
  --muted: #5E6577;
  --chaos-red: #FF3B30;
  --chaos-red-text: #B42318;
  --chaos-red-action: #C82B22;
  --chaos-amber: #FF9500;
  --order-blue: #2563EB;
  --order-mint: #2563EB;
  --order-lime: #CCFF00;
  --border-w: 2.5px;
  --shadow-hard: 5px 5px 0px #121316;

  width: 100vw;
  min-height: 100vh;
  height: auto;
  overflow-x: hidden;
  background-color: var(--bg-chaos);
  color: var(--ink);
  font-family: 'Plus Jakarta Sans', 'PingFang SC', 'Microsoft YaHei', sans-serif;
  transition: background-color 0.65s cubic-bezier(0.22, 1, 0.36, 1);
  position: relative;
  user-select: none;
}

.ccai-home-viewport.mode-order {
  background-color: var(--bg-order);
}

.bg-grid {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 0;
  background-size: 40px 40px;
  background-image:
    linear-gradient(to right, rgba(18, 19, 22, 0.05) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(18, 19, 22, 0.05) 1px, transparent 1px);
}
.mode-order .bg-grid {
  background-image:
    linear-gradient(to right, rgba(37, 99, 235, 0.07) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(37, 99, 235, 0.07) 1px, transparent 1px);
}

#morph-canvas {
  position: fixed;
  inset: 0;
  width: 100vw;
  height: 100vh;
  pointer-events: none;
  z-index: 999;
}
#morph-canvas.canvas-hidden {
  opacity: 0;
  visibility: hidden;
}

.screen {
  position: relative;
  z-index: 10;
  width: 100vw;
  height: 100vh;
  max-width: 1560px;
  margin: 0 auto;
  padding: 18px 36px;
  display: grid;
  grid-template-rows: 54px 86px 1fr;
  gap: 12px;
}

/* ==================== 1. 顶部导航栏 ==================== */
.header-bar {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-weight: 900;
  font-size: 20px;
  letter-spacing: -0.5px;
  color: var(--ink);
  text-decoration: none;
  justify-self: start;
}

.brand-logo {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--chaos-red);
  border: var(--border-w) solid var(--ink);
  box-shadow: 3px 3px 0 var(--ink);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  transition: all 0.4s ease;
  flex-shrink: 0;
}
.brand-stamp-icon {
  width: 20px;
  height: 20px;
}
.mode-order .brand-logo {
  background: var(--order-blue);
}

.breaker-wrap {
  justify-self: center;
}
.breaker-switch {
  display: inline-flex;
  align-items: center;
  background: #FFFFFF;
  border: var(--border-w) solid var(--ink);
  border-radius: 999px;
  padding: 4px;
  box-shadow: 4px 4px 0 var(--ink);
  gap: 4px;
}
.sw-btn {
  border: none;
  background: transparent;
  padding: 7px 18px;
  border-radius: 999px;
  font-weight: 800;
  font-size: 13px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--muted);
  transition: all 0.25s ease;
}
.sw-btn.active.chaos {
  background: var(--chaos-red-action);
  color: #fff;
  box-shadow: 2px 2px 0 var(--ink);
}
.sw-btn.active.order {
  background: var(--order-lime);
  color: var(--ink);
  box-shadow: 2px 2px 0 var(--ink);
}

.cta-wrap {
  justify-self: end;
  display: inline-flex;
  align-items: center;
  gap: 10px;
}

.lang-switch-btn {
  height: 38px;
  padding: 0 13px;
  border-radius: 10px;
  background: #FFFFFF;
  border: var(--border-w) solid var(--ink);
  box-shadow: 3px 3px 0 var(--ink);
  font-family: 'JetBrains Mono', monospace;
  font-weight: 800;
  font-size: 12px;
  color: var(--muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.lang-switch-btn:hover {
  transform: translate(-1.5px, -1.5px);
  box-shadow: 4.5px 4.5px 0 var(--ink);
}
.lang-active {
  color: var(--ink);
  background: var(--order-lime);
  padding: 1px 5px;
  border-radius: 5px;
  border: 1.5px solid var(--ink);
}

/* ==================== 语言切换器下拉（R12-04：五语同源、键盘可操作） ==================== */
.lang-switcher {
  position: relative;
  display: inline-flex;
}
.lang-globe {
  font-size: 12px;
  line-height: 1;
  color: var(--ink);
}
.lang-caret {
  font-size: 9px;
  line-height: 1;
  color: var(--muted);
}
.lang-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 60;
  min-width: 156px;
  margin: 0;
  padding: 5px;
  list-style: none;
  background: #FFFFFF;
  border: var(--border-w) solid var(--ink);
  border-radius: 10px;
  box-shadow: 4px 4px 0 var(--ink);
}
.lang-option {
  display: block;
  width: 100%;
  padding: 7px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  font-weight: 700;
  color: var(--ink);
  text-align: left;
  cursor: pointer;
  transition: background 0.15s ease;
}
.lang-option:hover,
.lang-option:focus-visible {
  background: var(--order-lime);
  outline: none;
}
.lang-option.active {
  background: var(--order-lime);
}

.cta-btn {
  padding: 9px 20px;
  border-radius: 10px;
  background: var(--ink);
  color: #fff;
  font-weight: 800;
  font-size: 13px;
  text-decoration: none;
  border: var(--border-w) solid var(--ink);
  box-shadow: 3px 3px 0 var(--chaos-red);
  transition: all 0.25s ease;
}
.mode-order .cta-btn {
  background: var(--order-blue);
  box-shadow: 3px 3px 0 var(--ink);
}
.cta-btn:hover {
  transform: translate(-2px, -2px);
  box-shadow: 5px 5px 0 var(--ink);
}

/* ==================== 2. 主标题 ==================== */
.hero-title-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}
.hero-headline {
  font-size: clamp(26px, 2.8vw, 40px);
  font-weight: 900;
  letter-spacing: -1px;
  line-height: 1.15;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  flex-wrap: wrap;
}
.hl-pill {
  display: inline-block;
  padding: 2px 14px;
  border-radius: 10px;
  border: var(--border-w) solid var(--ink);
  box-shadow: 4px 4px 0 var(--ink);
  background: #FFE4E1;
  color: var(--chaos-red-text);
  transition: all 0.4s ease;
}
.mode-order .hl-pill {
  background: var(--order-lime);
  color: var(--ink);
}
.hero-sub {
  margin-top: 6px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--muted);
}

/* ==================== 3. 核心舞台 ==================== */
.stage-container {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 0;
  display: flex;
  align-items: stretch;
}
.stage-grid {
  position: relative;
  width: 100%;
  max-width: 1480px;
  height: 100%;
  max-height: 416px;
  margin: auto auto;
  display: grid;
  grid-template-columns: 336px 404px 336px;
  justify-content: space-between;
  align-items: stretch;
}
#lines-svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 1;
  overflow: visible;
}

.col-wrapper {
  position: relative;
  z-index: 5;
  display: grid;
  grid-template-rows: 24px 1fr;
  gap: 8px;
  height: 100%;
  min-height: 0;
}
.col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 6px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 800;
  color: var(--muted);
  letter-spacing: 0.5px;
}
.col-header.center {
  justify-content: center;
}

.cards-stack {
  display: grid;
  grid-template-rows: repeat(4, 1fr);
  gap: 12px;
  height: 100%;
  min-height: 0;
}


/* 单元卡片 */
.unit-card {
  position: relative;
  background: #FFFFFF;
  border: var(--border-w) solid var(--ink);
  border-radius: 16px;
  padding: 0 18px;
  box-shadow: var(--shadow-hard);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  cursor: pointer;
  overflow: hidden;
  transition: transform 0.22s ease, box-shadow 0.22s ease, border-color 0.25s ease;
}

/* ==================== 3.0 秒全卡片剧烈晃动充能效果 ==================== */
.card-charge-wave {
  position: absolute;
  inset: 0;
  width: 0%;
  pointer-events: none;
  z-index: 0;
  opacity: 0;
}
.mode-chaos .card-charge-wave {
  background: linear-gradient(90deg, rgba(255, 59, 48, 0.24) 0%, rgba(255, 149, 0, 0.38) 80%, rgba(255, 59, 48, 0.52) 100%);
}
.mode-order .card-charge-wave {
  background: linear-gradient(90deg, rgba(37, 99, 235, 0.18) 0%, rgba(0, 200, 150, 0.30) 75%, rgba(204, 255, 0, 0.50) 100%);
}

/* 底部蓄力能量槽（3秒从0%充到100%） */
.charge-bar-track {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 6px;
  background: transparent;
  overflow: hidden;
  pointer-events: none;
  z-index: 3;
}
.charge-bar-fill {
  width: 0%;
  height: 100%;
}
.mode-chaos .charge-bar-fill {
  background: linear-gradient(90deg, #FF3B30, #FF9500, #FF1E00);
}
.mode-order .charge-bar-fill {
  background: linear-gradient(90deg, #1D4ED8, #60A5FA, #CCFF00);
}

/* 当处于 3s 充能阶段时：整张卡片高频震颤摇晃 + 全卡片流光灌满！ */
.unit-card.is-card-charging {
  animation: chargeRumble 0.09s infinite linear !important;
  border-color: var(--chaos-red) !important;
  box-shadow: 0 0 0 3px rgba(255, 59, 48, 0.25), 7px 7px 0 var(--ink) !important;
}
.mode-order .unit-card.is-card-charging {
  border-color: var(--order-blue) !important;
  box-shadow: 0 0 0 3px rgba(0, 200, 150, 0.32), 7px 7px 0 var(--order-blue) !important;
}

.unit-card.is-card-charging .card-charge-wave {
  width: 100%;
  opacity: 1;
  transition: width 3s linear, opacity 0.15s ease;
}
.unit-card.is-card-charging .charge-bar-fill {
  width: 100%;
  transition: width 3s linear;
}

/* 左侧卡片在 3 秒充能完成后保持满格高亮 */
.unit-card.is-card-charged-hold .card-charge-wave {
  width: 100%;
  opacity: 0.45;
}
.unit-card.is-card-charged-hold .charge-bar-fill {
  width: 100%;
}

@keyframes chargeRumble {
  0%   { transform: translate(0px, 0px) rotate(0deg) scale(1.02); }
  20%  { transform: translate(-3px, 2px) rotate(-1.2deg) scale(1.03); }
  40%  { transform: translate(3px, -2px) rotate(1.2deg) scale(1.03); }
  60%  { transform: translate(-3px, -1.5px) rotate(-1deg) scale(1.02); }
  80%  { transform: translate(2.5px, 2px) rotate(1deg) scale(1.03); }
  100% { transform: translate(0px, 0px) rotate(0deg) scale(1.02); }
}

.mode-chaos .wing-left .unit-card:nth-child(odd) {
  transform: rotate(-0.6deg);
}
.mode-chaos .wing-left .unit-card:nth-child(even) {
  transform: rotate(0.6deg);
}
.mode-order .unit-card {
  transform: none;
  box-shadow: 5px 5px 0 var(--order-blue);
}

/* ==================== 右侧模型卡片：正常 / 3s 充能晃动 / 3s 后停止充能并断裂或方框外迸烟花 ==================== */
.unit-card.model-card-wrap {
  background: transparent !important;
  border-color: transparent !important;
  box-shadow: none !important;
  padding: 0 !important;
  overflow: visible !important;
}

.split-piece {
  position: absolute;
  inset: 0;
  background: #FFFFFF;
  border: var(--border-w) solid var(--ink);
  border-radius: 16px;
  padding: 0 18px;
  display: flex;
  align-items: center;
  overflow: hidden;
  transition: transform 0.28s cubic-bezier(0.34, 1.56, 0.64, 1), background-color 0.2s ease, box-shadow 0.2s ease;
}
.piece-inner {
  position: relative;
  z-index: 2;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.piece-right {
  opacity: 0;
  pointer-events: none;
}

.unit-card.model-card-wrap .piece-left {
  box-shadow: var(--shadow-hard);
}
.mode-chaos .wing-right .unit-card.model-card-wrap:nth-child(odd) .piece-left {
  border-color: var(--chaos-red);
}
.mode-order .unit-card.model-card-wrap .piece-left {
  box-shadow: 5px 5px 0 var(--order-blue);
}

/* 右侧模型卡片在 3 秒充能期间：整个 .piece-left 剧烈晃动并灌满充能层 */
.unit-card.model-card-wrap.is-card-charging {
  animation: chargeRumble 0.09s infinite linear !important;
  box-shadow: none !important;
}
.unit-card.model-card-wrap.is-card-charging .piece-left {
  border-color: var(--chaos-red) !important;
  box-shadow: 0 0 0 3px rgba(255, 59, 48, 0.28), 7px 7px 0 var(--ink) !important;
}
.mode-order .unit-card.model-card-wrap.is-card-charging .piece-left {
  border-color: var(--order-blue) !important;
  box-shadow: 0 0 0 3px rgba(0, 200, 150, 0.35), 7px 7px 0 var(--order-blue) !important;
}

/* 3 秒充能结束 → 苦海模式下停止充能晃动，并沿斜向锯齿裂口断成左右两半！ */
.unit-card.is-chaos-fractured {
  animation: none !important;
}
.unit-card.is-chaos-fractured .card-charge-wave,
.unit-card.is-chaos-fractured .charge-bar-track {
  display: none !important;
}
.unit-card.is-chaos-fractured .piece-left {
  clip-path: polygon(0 0, 54% 0, 44% 50%, 52% 100%, 0 100%);
  transform: translate(-12px, 6px) rotate(-4.8deg);
  background: #FFF0EE;
  border-color: var(--chaos-red);
  box-shadow: -4px 5px 0 var(--chaos-red);
}
.unit-card.is-chaos-fractured .piece-right {
  opacity: 1;
  clip-path: polygon(54% 0, 100% 0, 100% 100%, 52% 100%, 44% 50%);
  transform: translate(12px, -6px) rotate(4.8deg);
  background: #FFF0EE;
  border-color: var(--chaos-red);
  box-shadow: 6px 5px 0 var(--chaos-red);
}

/* 3 秒充能结束 → 爽局模式下停止充能晃动，方框高亮并从四边往外迸发烟花！ */
.unit-card.is-order-celebrating {
  animation: none !important;
}
.unit-card.is-order-celebrating .card-charge-wave,
.unit-card.is-order-celebrating .charge-bar-track {
  display: none !important;
}
.unit-card.is-order-celebrating .piece-left {
  background: #F4FFF9;
  border-color: var(--order-mint) !important;
  box-shadow: 0 0 0 4px rgba(204, 255, 0, 0.55), 7px 7px 0 var(--order-blue) !important;
  transform: scale(1.03);
}

.fracture-crack-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 8;
  pointer-events: none;
  animation: crackPop 0.22s cubic-bezier(0.34, 1.56, 0.64, 1);
}
.crack-zig-svg {
  position: absolute;
  width: 104%;
  height: 30px;
  top: 50%;
  left: -2%;
  transform: translateY(-50%) rotate(-6deg);
}
.fracture-pain-pill {
  position: relative;
  z-index: 2;
  background: var(--chaos-red-action);
  color: #FFFFFF;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-weight: 900;
  padding: 5px 13px;
  border-radius: 8px;
  border: 2px solid #121316;
  box-shadow: 3px 3px 0 #121316;
  white-space: nowrap;
}
@keyframes crackPop {
  0% { transform: scale(0.5); opacity: 0; }
  100% { transform: scale(1); opacity: 1; }
}

.u-left {
  position: relative;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.u-icon {
  width: 42px;
  height: 42px;
  border-radius: 11px;
  border: 2px solid var(--ink);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: 2px 2px 0 var(--ink);
  transition: background-color 0.3s ease;
}
.brand-svg {
  width: 24px;
  height: 24px;
}
.u-text {
  min-width: 0;
}
.u-name {
  font-weight: 800;
  font-size: 13.5px;
  letter-spacing: -0.2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.u-desc {
  font-size: 12px;
  font-weight: 600;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.u-tag {
  position: relative;
  z-index: 2;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 800;
  padding: 4px 9px;
  border-radius: 7px;
  border: 2px solid var(--ink);
  white-space: nowrap;
  flex-shrink: 0;
  background: #FFE4E1;
  color: var(--chaos-red-text);
}
.mode-order .u-tag {
  background: #DCFCE7;
  color: #15803D;
}


/* ==================== 中央大核心 ==================== */
.core-box {
  position: relative;
  width: 100%;
  max-width: 404px;
  height: 100%;
  background: #FFFFFF;
  border: 3px solid var(--ink);
  border-radius: 20px;
  box-shadow: 7px 7px 0 var(--ink);
  padding: 18px 20px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
  transition: box-shadow 0.4s ease, border-color 0.4s ease, transform 0.25s ease;
}
.mode-order .core-box {
  border-color: var(--order-blue);
  box-shadow: 8px 8px 0 var(--order-blue);
}
.core-box.core-overdrive {
  transform: translateY(-2px);
}

.chaos-content,
.order-content {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: 100%;
  gap: 10px;
}
.core-top-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.core-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 999px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 800;
  border: 2px solid var(--ink);
  white-space: nowrap;
}
.core-badge.bad {
  background: #FFE4E1;
  color: var(--chaos-red-text);
}
.core-badge.good {
  background: var(--order-lime);
  color: var(--ink);
}
.core-status-dot {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 800;
  color: var(--chaos-red-text);
  white-space: nowrap;
}
.core-title {
  font-size: 18px;
  font-weight: 900;
  letter-spacing: -0.4px;
  line-height: 1.25;
  margin-top: 6px !important;
  white-space: nowrap;
}

.pain-list {
  display: grid;
  grid-template-rows: repeat(3, 1fr);
  gap: 8px;
  flex: 1;
  min-height: 0;
  margin: 4px 0;
}
.pain-item {
  background: #FFF5F5;
  border: 2px dashed var(--chaos-red);
  border-radius: 11px;
  padding: 0 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 700;
  white-space: nowrap;
  transition: all 0.2s ease;
}
.pain-item.pain-highlight {
  background: #FFE4E1;
  border-style: solid;
  transform: scale(1.015);
  box-shadow: 3px 3px 0 var(--chaos-red);
}
.pain-code {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  font-weight: 800;
  background: var(--chaos-red-action);
  color: #fff;
  padding: 3px 7px;
  border-radius: 6px;
  flex-shrink: 0;
  white-space: nowrap;
}

.order-mid-group {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 8px;
  flex: 1;
  min-height: 0;
}
.endpoint-pill {
  background: #F4F7FF;
  border: 2px solid var(--ink);
  border-radius: 11px;
  padding: 8px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  font-weight: 700;
  cursor: pointer;
  white-space: nowrap;
}
.copy-tag {
  background: var(--order-lime);
  border: 1.5px solid var(--ink);
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 10.5px;
  font-weight: 800;
  flex-shrink: 0;
  white-space: nowrap;
}

.code-box {
  background: #121316;
  color: #E2E8F0;
  border-radius: 12px;
  padding: 10px 14px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  line-height: 1.6;
  border: 2px solid var(--ink);
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  white-space: nowrap;
  overflow: hidden;
}
.code-box .c-hl {
  color: var(--order-lime);
}
.code-box .c-model {
  color: #38BDF8;
  cursor: pointer;
  text-decoration: underline;
}

.big-trigger-btn {
  width: 100%;
  height: 48px;
  border-radius: 13px;
  border: var(--border-w) solid var(--ink);
  font-weight: 900;
  font-size: 14.5px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  box-shadow: 4px 4px 0 var(--ink);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
  flex-shrink: 0;
}
.big-trigger-btn.to-order {
  background: var(--order-lime);
  color: var(--ink);
}
.big-trigger-btn.to-chaos {
  background: #FFFFFF;
  color: var(--ink);
}
.big-trigger-btn:hover {
  transform: translate(-2px, -2px);
  box-shadow: 6px 6px 0 var(--ink);
}

/* 炸裂重组期间隐藏原 DOM */
.is-morphing .morph-unit {
  opacity: 0 !important;
  pointer-events: none !important;
  transition: opacity 0.04s linear !important;
}
.is-morphing #lines-svg {
  opacity: 0 !important;
  transition: opacity 0.08s linear !important;
}

@keyframes snapPop {
  0% { transform: scale(0.95); }
  60% { transform: scale(1.015); }
  100% { transform: scale(1); }
}
.snap-reborn {
  animation: snapPop 0.34s cubic-bezier(0.34, 1.56, 0.64, 1);
}

:deep(.chaos-rapid-dash) {
  animation: dashRun 0.22s linear infinite;
}
@keyframes dashRun {
  to { stroke-dashoffset: -30; }
}


/* 子页面 */
.public-subpage-shell {
  min-height: 100vh;
  background: #F5F2EB;
  color: #121316;
  padding: 24px 36px;
}
.subpage-header {
  max-width: 1280px;
  margin: 0 auto 32px;
}
.subpage-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border: 2.5px solid #121316;
  border-radius: 16px;
  padding: 12px 20px;
  box-shadow: 5px 5px 0 #121316;
}
.subpage-links {
  display: flex;
  gap: 20px;
}
.sub-link {
  font-weight: 800;
  color: #121316;
  text-decoration: none;
}
.subpage-content {
  max-width: 1280px;
  margin: 0 auto;
}
.subpage-title {
  font-size: 32px;
  font-weight: 900;
  margin: 12px 0;
}
.subpage-lead {
  font-size: 15px;
  color: #5E6577;
  margin-bottom: 24px;
}
.subpage-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 20px;
}
.sub-card {
  background: #fff;
  border: 2.5px solid #121316;
  border-radius: 16px;
  padding: 20px;
  box-shadow: 5px 5px 0 #121316;
}

@media (max-width: 1180px) {
  .ccai-home-viewport {
    overflow-y: auto;
    height: auto;
    min-height: 100vh;
  }
  .screen {
    height: auto;
    grid-template-rows: auto auto auto;
  }
  .stage-grid {
    grid-template-columns: 1fr;
    max-height: none;
    gap: 20px;
  }
  #lines-svg {
    display: none;
  }
}

/* ==================== NEO-LAB PUBLIC CONTENT STYLES ==================== */
/* ==================== 首页内容区块（最新资讯 / 深度解析） ==================== */
.home-content-strip {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 1560px;
  margin: 0 auto;
  padding: 6px 36px 64px;
}
.hc-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 22px;
}
.hc-block {
  background: #FFFFFF;
  border: 2.5px solid #121316;
  box-shadow: 5px 5px 0px #121316;
  padding: 18px 20px 20px;
  min-width: 0;
}
.hc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 2px solid #121316;
  padding-bottom: 10px;
  margin-bottom: 12px;
}
.hc-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 900;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #121316;
}
.hc-more {
  font-size: 13px;
  font-weight: 900;
  color: #121316;
  text-decoration: underline;
  text-underline-offset: 3px;
  white-space: nowrap;
}
.hc-more:hover {
  color: #2563EB;
}
.hc-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.hc-item {
  display: block;
  padding: 9px 10px;
  border: 2px solid transparent;
  text-decoration: none;
  color: #121316;
  transition: background-color 0.16s ease, border-color 0.16s ease, transform 0.16s ease;
}
.hc-item:hover {
  border-color: #121316;
  background: #CCFF00;
  transform: translate(-1px, -1px);
}
.hc-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 11px;
  font-weight: 800;
  color: #5E6577;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: 4px;
}
.hc-title {
  margin: 0 0 4px;
  font-size: 16px;
  line-height: 1.35;
  font-weight: 900;
}
.hc-excerpt {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #5E6577;
}
.hc-empty {
  font-size: 13px;
  font-weight: 700;
  color: #5E6577;
  padding: 10px 2px;
}

/* ==================== 前台列表分页 ==================== */
.neo-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  flex-wrap: wrap;
  margin: 26px auto 8px;
  padding: 14px 16px;
  width: fit-content;
  max-width: 100%;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  box-shadow: 5px 5px 0px #121316;
}
.pager-btn,
.pager-num {
  font-family: inherit;
  font-size: 13px;
  font-weight: 900;
  color: #121316;
  background: #FFFFFF;
  border: 2px solid #121316;
  padding: 7px 13px;
  cursor: pointer;
  transition: background-color 0.15s ease, transform 0.15s ease, opacity 0.15s ease;
}
.pager-btn:hover:not(:disabled),
.pager-num:hover:not(.is-active) {
  background: #CCFF00;
  transform: translate(-1px, -1px);
}
.pager-btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}
.pager-pages {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.pager-num.is-active {
  background: #121316;
  color: #FFFFFF;
  cursor: default;
}
.pager-status {
  font-size: 12px;
  font-weight: 800;
  color: #5E6577;
  letter-spacing: 0.08em;
}

@media (max-width: 1024px) {
  .home-content-strip {
    padding: 6px 18px 48px;
  }
  .hc-grid {
    grid-template-columns: 1fr;
  }
  .neo-pager {
    gap: 8px;
    padding: 12px;
  }
}

.nav-sub-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 12px;
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 800;
  color: #121316;
  box-shadow: 2.5px 2.5px 0px #121316;
  text-decoration: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.nav-sub-btn:hover {
  transform: translate(-1.5px, -1.5px);
  box-shadow: 4px 4px 0px #121316;
  background: #EDF4FF;
}
.nav-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  border: 1.5px solid #121316;
}
.nav-dot.dot-live {
  background: #2563EB;
}
.nav-dot.dot-doc {
  background: #2563EB;
}

.neo-public-viewport {
  --bg-chaos: #F5F2EB;
  --bg-order: #EDF4FF;
  --ink: #121316;
  --muted: #5E6577;
  --chaos-red: #FF3B30;
  --chaos-amber: #FF9500;
  --order-blue: #2563EB;
  --order-accent: #2563EB;
  --order-lime: #CCFF00;
  --border-w: 2.5px;
  position: relative;
  min-height: 100vh;
  background: #F5F2EB;
  color: #121316;
  font-family: 'Plus Jakarta Sans', 'PingFang SC', sans-serif;
  padding-bottom: 60px;
}

.neo-public-viewport :deep(h1),
.neo-public-viewport :deep(h2),
.neo-public-viewport :deep(h3),
.neo-public-viewport :deep(h4),
.neo-public-viewport h1,
.neo-public-viewport h2,
.neo-public-viewport h3,
.neo-public-viewport h4,
.neo-public-viewport .card-title {
  color: #121316 !important;
}

.neo-public-viewport .cta-btn {
  background: #121316 !important;
  color: #CCFF00 !important;
  border: 2.5px solid #121316 !important;
  box-shadow: 3px 3px 0px #2563EB !important;
  font-weight: 900 !important;
  letter-spacing: 0.01em;
}
.neo-public-viewport .cta-btn:hover {
  background: #2563EB !important;
  color: #FFFFFF !important;
  box-shadow: 4px 4px 0px #121316 !important;
  transform: translate(-1.5px, -1.5px);
}
.neo-sub-header-bar {
  position: sticky;
  top: 0;
  z-index: 50;
  background: rgba(245, 242, 235, 0.94);
  backdrop-filter: blur(12px);
  border-bottom: 2.5px solid #121316;
  padding: 14px 36px;
  max-width: 1560px;
  margin: 0 auto;
}

.sub-header-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.sub-nav-divider {
  font-family: 'JetBrains Mono', monospace;
  font-size: 18px;
  font-weight: 900;
  color: rgba(18, 19, 22, 0.35);
}

.sub-nav-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #FFFFFF;
  border: 2px solid #121316;
  padding: 4px 10px;
  border-radius: 8px;
  box-shadow: 2px 2px 0 #121316;
}

.sub-nav-indicator .indicator-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #2563EB;
  box-shadow: 0 0 6px rgba(0, 200, 150, 0.8);
}

.indicator-title {
  font-size: 11.5px;
  font-weight: 900;
  letter-spacing: 0.05em;
  color: #121316;
}

/* 中心：硬核双核分段导航（严禁药丸胶囊/消除 emoji 突兀感） */
.sub-segmented-nav {
  display: inline-flex;
  align-items: center;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 10px;
  padding: 3px;
  box-shadow: 4px 4px 0px #121316;
  gap: 4px;
}

.seg-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 16px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 800;
  color: #5E6577;
  text-decoration: none;
  transition: all 0.2s cubic-bezier(0.22, 1, 0.36, 1);
  border: 1.5px solid transparent;
}

.seg-code {
  font-size: 11px;
  color: #5F6675;
  font-weight: 900;
}

.seg-item:hover {
  color: #121316;
  background: rgba(18, 19, 22, 0.04);
}

.seg-item.active {
  background: #CCFF00;
  color: #121316;
  border-color: #121316;
  box-shadow: 2px 2px 0px #121316;
}

.seg-item.active .seg-code {
  color: #121316;
}

/* 右侧返回首页按钮 */
.nav-home-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 38px;
  padding: 0 14px;
  border-radius: 10px;
  background: #FFFFFF;
  border: 2px solid #121316;
  box-shadow: 3px 3px 0 #121316;
  font-size: 13px;
  font-weight: 800;
  color: #121316;
  text-decoration: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.nav-home-btn:hover {
  transform: translate(-1px, -1px);
  box-shadow: 4px 4px 0 #121316;
  background: #F4F7FF;
}

.neo-public-main {
  max-width: 1200px;
  margin: 20px auto 0;
  padding: 0 24px;
}
@media (min-width: 768px) and (max-width: 1279px) {
  .neo-public-main {
    padding: 0 32px;
  }
}
@media (max-width: 767px) {
  .neo-public-main {
    padding: 0 16px;
  }
}
.list-hero-banner {
  background: radial-gradient(circle at 92% 18%, rgba(37, 99, 235, 0.06), transparent 42%), #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 22px 28px;
  box-shadow: 5px 5px 0px #121316;
  margin-bottom: 24px;
}
.list-eyebrow {
  display: inline-flex;
  align-items: center;
  background: #CCFF00;
  color: #121316;
  border: 1.5px solid #121316;
  padding: 3px 10px;
  border-radius: 999px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 900;
  box-shadow: 1.5px 1.5px 0px #121316;
  margin-bottom: 0;
}
.list-title {
  font-size: 26px;
  font-weight: 900;
  color: #121316;
  margin: 4px 0 6px;
  letter-spacing: -0.025em;
}
.list-sub {
  font-size: 13.5px;
  color: #5E6577;
  font-weight: 700;
  margin: 0;
}
.neo-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 24px;
}
.neo-content-card {
  display: flex;
  flex-direction: column;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 24px;
  box-shadow: 5px 5px 0px #121316;
  text-decoration: none;
  color: inherit;
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}
.neo-content-card:hover {
  transform: translate(-3px, -3px);
  box-shadow: 8px 8px 0px #121316;
}
.card-top-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.meta-tag {
  background: #EDF4FF;
  border: 1.8px solid #121316;
  padding: 3px 9px;
  border-radius: 7px;
  font-size: 11.5px;
  font-weight: 900;
  color: #2563EB;
}
.meta-date {
  font-size: 12px;
  color: #5E6577;
  font-weight: 700;
}
.card-title {
  font-size: 19px;
  font-weight: 900;
  color: #121316;
  margin: 0 0 10px;
  line-height: 1.35;
}
.card-excerpt {
  font-size: 13.5px;
  color: #5E6577;
  font-weight: 600;
  line-height: 1.6;
  flex: 1;
  margin: 0 0 18px;
}
.card-footer-cta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-top: 1.5px dashed rgba(18, 19, 22, 0.18);
  padding-top: 12px;
  font-size: 13px;
  font-weight: 900;
  color: #121316;
}
.neo-article-sheet {
  background: #FFFFFF;
  border: 3px solid #121316;
  border-radius: 22px;
  padding: 40px 48px;
  box-shadow: 8px 8px 0px #121316;
}
.article-top-nav {
  display: flex;
  align-items: center;
  gap: 14px;
  margin-bottom: 24px;
  flex-wrap: wrap;
}
.back-link {
  background: #F4F7FF;
  border: 2px solid #121316;
  padding: 6px 14px;
  border-radius: 10px;
  color: #121316;
  font-weight: 800;
  font-size: 13px;
  box-shadow: 2px 2px 0px #121316;
  text-decoration: none;
}
.article-badge {
  background: #CCFF00;
  border: 1.8px solid #121316;
  padding: 3px 10px;
  border-radius: 7px;
  font-size: 12px;
  font-weight: 900;
}
.article-headline {
  font-size: 34px;
  font-weight: 900;
  color: #121316;
  letter-spacing: -0.03em;
  margin: 0 0 16px;
  line-height: 1.25;
}
.article-lede {
  font-size: 16px;
  line-height: 1.6;
  color: #5E6577;
  font-weight: 700;
  background: #F8FAFF;
  border-left: 4px solid #2563EB;
  padding: 16px 20px;
  border-radius: 8px;
  margin: 0 0 28px;
}
.article-toc {
  background: #F4F7FF;
  border: 2px solid #121316;
  border-radius: 14px;
  padding: 18px 22px;
  margin-bottom: 32px;
  box-shadow: 3px 3px 0px #121316;
}
.toc-title {
  font-family: 'JetBrains Mono', monospace;
  font-weight: 900;
  font-size: 13px;
  margin-bottom: 10px;
}
.toc-links {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.toc-links a {
  color: #2563EB;
  font-weight: 800;
  font-size: 13.5px;
  text-decoration: none;
}
.toc-links a:hover {
  text-decoration: underline;
}
.neo-prose-body {
  max-width: 800px;
  margin-left: auto;
  margin-right: auto;
  font-size: 16px;
  line-height: 1.85;
  color: #121316;
  font-weight: 500;
}
.neo-prose-body h1,
.neo-prose-body h2,
.neo-prose-body h3,
.neo-prose-body h4 {
  font-weight: 900;
  color: #121316;
  letter-spacing: -0.02em;
  margin-top: 36px;
  margin-bottom: 16px;
}
.neo-prose-body h2 {
  font-size: 24px;
  border-bottom: 2px solid #121316;
  padding-bottom: 8px;
}
.neo-prose-body h3 {
  font-size: 20px;
}
.neo-prose-body p {
  margin-bottom: 20px;
}
/* 正文内嵌图片：块级满列、等比缩放；未加载完成时也保有可见尺寸，避免塌陷成 0×0。
   正文由 v-html 注入，不带 scoped 标记，必须用 :deep 才能命中。 */
.neo-prose-body :deep(img) {
  display: block;
  width: 100%;
  max-width: 100%;
  height: auto;
}
.neo-prose-body pre {
  background: #121316;
  color: #F5F2EB;
  padding: 20px;
  border-radius: 12px;
  overflow-x: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  border: 2px solid #121316;
  box-shadow: 4px 4px 0px #2563EB;
  margin: 24px 0;
}
.neo-prose-body code {
  font-family: 'JetBrains Mono', monospace;
  background: #EAE6DF;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 13.5px;
  font-weight: 700;
  color: #2563EB;
}
.neo-prose-body pre code {
  background: transparent;
  color: inherit;
  padding: 0;
}
.neo-prose-body blockquote {
  border-left: 4px solid #2563EB;
  background: #EDF4FF;
  padding: 14px 20px;
  border-radius: 0 10px 10px 0;
  margin: 24px 0;
  font-style: italic;
  font-weight: 600;
}
.neo-prose-body table {
  width: 100%;
  border-collapse: collapse;
  margin: 24px 0;
  border: 2px solid #121316;
  border-radius: 8px;
  overflow: hidden;
}
.neo-prose-body th,
.neo-prose-body td {
  border: 1px solid #D6D0C4;
  padding: 10px 14px;
  text-align: left;
}
.neo-prose-body th {
  background: #EAE6DF;
  font-weight: 900;
}

/* ==================== HERO BANNER & SEARCH ==================== */
.list-hero-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.list-eyebrow-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}
.list-count-badge {
  display: inline-flex;
  align-items: center;
  background: #121316;
  color: #FFFFFF;
  font-size: 11px;
  font-weight: 800;
  padding: 3px 10px;
  border-radius: 999px;
  border: 1.5px solid #121316;
}
.pulse-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #FF3B30;
  margin-right: 6px;
  animation: pulseNode 1.6s infinite ease-in-out;
}
@keyframes pulseNode {
  0% { transform: scale(0.9); opacity: 0.6; }
  50% { transform: scale(1.3); opacity: 1; }
  100% { transform: scale(0.9); opacity: 0.6; }
}

.list-search-box {
  position: relative;
  display: flex;
  align-items: center;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 14px;
  padding: 6px 8px 6px 6px;
  width: 390px;
  max-width: 100%;
  box-shadow: 4px 4px 0px #121316;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
}
.list-search-box:focus-within {
  border-color: #2563EB;
  box-shadow: 5px 5px 0px #2563EB;
  transform: translate(-1px, -1px);
}
.search-icon-badge {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: #CCFF00;
  border: 2px solid #121316;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #121316;
  flex-shrink: 0;
  box-shadow: 1.5px 1.5px 0px #121316;
}
.search-svg-icon {
  width: 15px;
  height: 15px;
}
.list-search-input {
  border: none;
  background: transparent;
  outline: none;
  font-size: 13.5px;
  font-weight: 800;
  color: #121316;
  width: 100%;
  height: 34px;
  margin: 0 10px;
}
.list-search-input::placeholder {
  color: #7A8296;
  font-weight: 700;
}
.search-kbd-hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 24px;
  padding: 0 6px;
  border-radius: 6px;
  background: #F5F2EB;
  border: 1.5px solid #121316;
  font-size: 11px;
  font-weight: 900;
  color: #5E6577;
  box-shadow: 1.5px 1.5px 0px #121316;
  flex-shrink: 0;
}
.search-clear-btn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  background: #FFE4E1;
  border: 1.5px solid #121316;
  border-radius: 8px;
  cursor: pointer;
  font-size: 11.5px;
  font-weight: 900;
  color: #121316;
  padding: 4px 9px;
  flex-shrink: 0;
  box-shadow: 1.5px 1.5px 0px #121316;
  transition: transform 0.12s ease, background 0.12s ease;
}
.search-clear-btn:hover {
  background: var(--chaos-red-action);
  color: #FFFFFF;
  transform: translate(-1px, -1px);
}
.list-category-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1.5px dashed rgba(18, 19, 22, 0.14);
}
.cat-pill {
  background: #F8FAFC;
  border: 2px solid #121316;
  border-radius: 999px;
  padding: 5px 14px;
  font-size: 12.5px;
  font-weight: 800;
  color: #121316;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
  transition: transform 0.12s ease, box-shadow 0.12s ease, background 0.12s ease;
}
.cat-pill:hover {
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0px #121316;
}
.cat-pill.active {
  background: #121316;
  color: #CCFF00;
}

/* ==================== ARTICLES: SINGLE-ROW LIST & SPECIAL FEATURED CARD ==================== */
.neo-articles-row-list {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.public-featured-section + .neo-articles-row-list {
  padding-top: 26px;
  border-top: 2px dashed rgba(18, 19, 22, 0.16);
}
:global(.dark) .public-featured-section + .neo-articles-row-list {
  border-top-color: rgba(148, 163, 184, 0.22);
}
.neo-article-row-card:nth-child(3n+1) .cover-placeholder svg {
  color: #121316;
  background: #CCFF00;
}
.neo-article-row-card:nth-child(3n+2) .cover-placeholder {
  background:
    radial-gradient(circle at 82% 20%, rgba(37, 99, 235, 0.22), transparent 50%),
    radial-gradient(circle at 18% 85%, rgba(99, 102, 241, 0.16), transparent 55%),
    linear-gradient(135deg, #F0F6FF 0%, #E0EAFF 100%);
}
.neo-article-row-card:nth-child(3n) .cover-placeholder {
  background:
    radial-gradient(circle at 82% 20%, rgba(245, 158, 11, 0.2), transparent 50%),
    radial-gradient(circle at 18% 85%, rgba(37, 99, 235, 0.14), transparent 55%),
    linear-gradient(135deg, #FEFCE8 0%, #EFF6FF 100%);
}
.neo-article-row-card:nth-child(3n) .cover-placeholder svg {
  color: #D97706;
}
:global(.dark) .neo-article-row-card:nth-child(3n+1) .cover-placeholder svg {
  color: #121316;
  background: #CCFF00;
  border-color: #121316;
}
:global(.dark) .neo-article-row-card:nth-child(3n+2) .cover-placeholder,
:global(.dark) .neo-article-row-card:nth-child(3n) .cover-placeholder {
  background:
    radial-gradient(circle at 85% 18%, rgba(96, 165, 250, 0.18), transparent 50%),
    radial-gradient(circle at 15% 85%, rgba(204, 255, 0, 0.1), transparent 55%),
    linear-gradient(135deg, #141824 0%, #1E2436 100%);
}

.neo-article-row-card {
  position: relative;
  display: grid;
  grid-template-columns: 8px auto 1fr auto;
  align-items: stretch;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 18px;
  box-shadow: 5px 5px 0px #121316;
  text-decoration: none;
  color: #121316;
  overflow: hidden;
  transition: transform 0.18s cubic-bezier(0.16, 1, 0.3, 1), box-shadow 0.18s cubic-bezier(0.16, 1, 0.3, 1), border-color 0.18s ease;
}

.neo-article-row-card:hover {
  transform: translate(-3px, -3px);
  box-shadow: 8px 8px 0px #121316;
}

.row-left-strip {
  width: 8px;
  border-right: 2px solid #121316;
}

.article-row-cover {
  margin: 16px 0 16px 18px;
  object-fit: cover;
}

.article-row-main {
  padding: 22px 26px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
}

.article-row-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.article-row-tag {
  font-size: 11.5px;
  font-weight: 800;
  color: #475569;
  background: #F1F5F9;
  border: 1.5px solid #CBD5E1;
  padding: 2px 8px;
  border-radius: 6px;
}

.article-row-title {
  font-size: 21px;
  font-weight: 900;
  color: #121316;
  margin: 0 0 8px;
  line-height: 1.35;
  letter-spacing: -0.01em;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.article-row-excerpt {
  font-size: 14px;
  color: #4B5262;
  line-height: 1.6;
  margin: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.article-row-aside {
  padding: 22px 26px 22px 16px;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  justify-content: space-between;
  border-left: 1.5px dashed #E2DDD5;
  min-width: 148px;
}

.article-row-index {
  font-size: 13px;
  font-weight: 900;
  color: #475569;
}

.article-row-cta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #121316;
  color: #FFFFFF;
  padding: 8px 14px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 800;
  white-space: nowrap;
  transition: background 0.15s ease, transform 0.15s ease;
}

.neo-article-row-card:hover .article-row-cta {
  background: #2563EB;
}

/* ==================== 精选文章专属特殊样式 (FEATURED SPECIAL) ==================== */
.neo-article-row-card.is-featured-article {
  grid-template-columns: 230px 1fr auto;
  background: linear-gradient(135deg, #121316 0%, #1C202B 55%, #141720 100%);
  color: #FFFFFF;
  border: 3px solid #121316;
  box-shadow: 0 0 0 2px #F59E0B, 8px 8px 0px #121316;
  margin-bottom: 6px;
}

/* 精选卡片（黑底）文字可读性修复 */
.neo-article-row-card.is-featured-article .article-row-title { color: #FFFFFF; }
.neo-article-row-card.is-featured-article .article-row-title:hover { color: #CCFF00; }
.neo-article-row-card.is-featured-article .article-row-excerpt { color: rgba(255, 255, 255, 0.72); }
.neo-article-row-card.is-featured-article .article-row-index { color: rgba(255, 255, 255, 0.5); }
.neo-article-row-card.is-featured-article .article-row-aside { border-left-color: rgba(255, 255, 255, 0.18); }
.neo-article-row-card.is-featured-article .article-row-cta { background: #CCFF00; color: #121316; }

.neo-article-row-card.is-featured-article:hover {
  transform: translate(-3px, -3px);
  box-shadow: 0 0 0 2.5px #CCFF00, 11px 11px 0px #121316;
}

.featured-row-emblem {
  position: relative;
  background: radial-gradient(circle at 20% 20%, rgba(204, 255, 0, 0.18), transparent 60%),
              linear-gradient(160deg, #181B24 0%, #0D0E12 100%);
  border-right: 2.5px solid rgba(245, 158, 11, 0.45);
  padding: 22px 20px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  overflow: hidden;
}

.featured-emblem-grid {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(rgba(255, 255, 255, 0.14) 1px, transparent 1px);
  background-size: 14px 14px;
  pointer-events: none;
}

.featured-crown-pill {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  align-self: flex-start;
  background: linear-gradient(90deg, #F59E0B 0%, #FBBF24 100%);
  color: #121316;
  font-size: 12px;
  font-weight: 900;
  padding: 4px 11px;
  border-radius: 999px;
  box-shadow: 2px 2px 0px #000000;
  letter-spacing: 0.03em;
}

.crown-star {
  font-size: 13px;
}

.featured-emblem-code {
  position: relative;
  z-index: 1;
  font-size: 26px;
  font-weight: 900;
  color: #CCFF00;
  letter-spacing: -0.03em;
  text-shadow: 2px 2px 0px #000;
  margin: 12px 0;
}

.featured-emblem-cat {
  position: relative;
  z-index: 1;
  font-size: 11px;
  font-weight: 800;
  color: #FDE68A;
  background: rgba(245, 158, 11, 0.16);
  border: 1px solid rgba(251, 191, 36, 0.4);
  padding: 4px 9px;
  border-radius: 6px;
  align-self: flex-start;
}

.featured-inline-stamp {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: rgba(245, 158, 11, 0.2);
  color: #FBBF24;
  border: 1.5px solid #F59E0B;
  font-size: 11px;
  font-weight: 900;
  padding: 2px 8px;
  border-radius: 6px;
}

.featured-meta-tag {
  background: #CCFF00 !important;
  color: #121316 !important;
  border-color: #121316 !important;
}

.featured-row-tag {
  background: rgba(255, 255, 255, 0.08);
  color: #E2E8F0;
  border-color: rgba(255, 255, 255, 0.2);
}

.neo-article-row-card.is-featured-article .article-row-title {
  color: #FFFFFF;
  font-size: 23px;
}

.neo-article-row-card.is-featured-article .article-row-excerpt {
  color: #CBD5E1;
}

.neo-article-row-card.is-featured-article .meta-read,
.neo-article-row-card.is-featured-article .meta-date {
  color: #94A3B8;
}

.neo-article-row-card.is-featured-article .article-row-aside {
  border-left: 1.5px dashed rgba(255, 255, 255, 0.16);
}

.featured-ribbon-code {
  font-size: 11px;
  font-weight: 900;
  color: #FBBF24;
  background: rgba(245, 158, 11, 0.15);
  border: 1px solid rgba(245, 158, 11, 0.4);
  padding: 3px 8px;
  border-radius: 6px;
  letter-spacing: 0.06em;
}

.featured-cta-pill {
  background: #CCFF00 !important;
  color: #121316 !important;
  border: 2px solid #121316;
  box-shadow: 3px 3px 0px #000000;
}

.neo-article-row-card.is-featured-article:hover .featured-cta-pill {
  background: #FBBF24 !important;
  color: #121316 !important;
}

@media (max-width: 767px) {
  .neo-article-row-card,
  .neo-article-row-card.is-featured-article {
    grid-template-columns: minmax(0, 1fr);
  }
  .row-left-strip {
    width: 100%;
    height: 6px;
    border-right: none;
    border-bottom: 2px solid #121316;
  }
  .article-row-cover {
    margin: 0;
  }
  .article-row-main {
    padding: 16px;
  }
  .featured-row-emblem {
    border-right: none;
    border-bottom: 2px solid rgba(245, 158, 11, 0.45);
    flex-direction: row;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
  }
  .featured-emblem-code {
    margin: 0;
    font-size: 18px;
  }
  .article-row-aside {
    border-left: none;
    border-top: 1.5px dashed rgba(148, 163, 184, 0.3);
    flex-direction: row;
    align-items: center;
    padding: 12px 16px;
  }
}

/* ==================== ARTICLES: FEATURED HERO CARD ==================== */
.neo-featured-card {
  display: grid;
  grid-template-columns: 280px 1fr;
  background: #FFFFFF;
  border: 3px solid #121316;
  border-radius: 22px;
  box-shadow: 7px 7px 0px #121316;
  overflow: hidden;
  margin-bottom: 32px;
  text-decoration: none;
  color: inherit;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}
.neo-featured-card:hover {
  transform: translate(-3px, -3px);
  box-shadow: 10px 10px 0px #121316;
}
.featured-visual {
  position: relative;
  background: linear-gradient(135deg, #121316 0%, #2563EB 100%);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 24px;
  color: #FFFFFF;
}
.featured-grid-bg {
  position: absolute;
  inset: 0;
  background-image: radial-gradient(rgba(255, 255, 255, 0.18) 1.5px, transparent 1.5px);
  background-size: 16px 16px;
  opacity: 0.6;
}
.featured-visual-badge {
  position: relative;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 900;
  background: #CCFF00;
  color: #121316;
  padding: 3px 8px;
  border-radius: 6px;
  border: 1.5px solid #121316;
  display: inline-block;
  align-self: flex-start;
}
.featured-visual-icon {
  position: relative;
  font-size: 48px;
  text-align: center;
  margin: 16px 0;
}
.featured-visual-cat {
  position: relative;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.1em;
  opacity: 0.85;
}
.featured-body {
  padding: 32px 36px;
  display: flex;
  flex-direction: column;
}
.featured-title {
  font-size: 26px;
  font-weight: 900;
  line-height: 1.35;
  margin: 8px 0 14px;
  color: #121316;
}
.featured-excerpt {
  font-size: 15px;
  line-height: 1.65;
  color: #5E6577;
  font-weight: 600;
  flex: 1;
  margin: 0 0 20px;
}
.featured-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1.5px dashed rgba(18, 19, 22, 0.15);
  padding-top: 18px;
}
.author-mini {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 800;
  color: #121316;
}
.author-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #2563EB;
}
.read-more-pill {
  background: #121316;
  color: #FFFFFF;
  padding: 6px 14px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 800;
  transition: background 0.15s ease;
}
.read-more-pill:hover {
  background: #2563EB;
}

/* ==================== CARD ACCENTS ==================== */
.card-accent-strip {
  height: 6px;
  border-radius: 4px;
  margin-bottom: 16px;
  border: 1.5px solid #121316;
}
.strip-0 { background: #CCFF00; }
.strip-1 { background: #2563EB; }
.strip-2 { background: #2563EB; }

.meta-read {
  font-size: 11.5px;
  color: #5E6577;
  font-weight: 700;
}
.card-cta-link {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* ==================== NEWS: LIVE SPOTLIGHT & TIMELINE ==================== */
.neo-news-spotlight {
  display: flex;
  background: linear-gradient(135deg, #FFFFFF 0%, #F8FAFF 100%);
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 20px 26px;
  box-shadow: 5px 5px 0px #121316;
  margin-bottom: 24px;
  text-decoration: none;
  color: inherit;
  align-items: center;
  gap: 22px;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.neo-news-spotlight:hover {
  transform: translate(-2px, -2px);
  box-shadow: 7px 7px 0px #121316;
}
.spotlight-badge-col {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 10px;
  min-width: 112px;
  padding-right: 14px;
  border-right: 1.5px dashed rgba(18, 19, 22, 0.15);
}
.live-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #C82B22;
  color: #FFFFFF;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 900;
  padding: 4px 10px;
  border-radius: 999px;
  border: 1.5px solid #121316;
  box-shadow: 1.5px 1.5px 0 #121316;
}
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #FFFFFF;
  animation: pulseNode 1s infinite;
}
.spotlight-date {
  font-size: 12.5px;
  color: #475569;
  font-weight: 800;
}
.spotlight-main-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.spotlight-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.meta-source-label {
  font-size: 11.5px;
  font-weight: 800;
  color: #475569;
  background: #F1F5F9;
  border: 1.5px solid #CBD5E1;
  padding: 2px 8px;
  border-radius: 6px;
}
.spotlight-title {
  font-size: 21px;
  font-weight: 900;
  color: #121316;
  margin: 2px 0 4px;
  line-height: 1.35;
  letter-spacing: -0.015em;
}
.spotlight-excerpt {
  font-size: 14px;
  color: #4B5262;
  font-weight: 600;
  line-height: 1.6;
  margin: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.spotlight-cta-col {
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.neo-news-timeline {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.timeline-news-card {
  display: flex;
  align-items: center;
  gap: 18px;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 18px 22px;
  box-shadow: 4px 4px 0px #121316;
  text-decoration: none;
  color: inherit;
  transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
}
.timeline-news-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 6px 6px 0px #121316;
  background: #FDFDFD;
}
.timeline-rail {
  align-self: stretch;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 14px;
  flex-shrink: 0;
}
.timeline-node {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  background: #2563EB;
  border: 2px solid #121316;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.16);
  flex-shrink: 0;
}
.timeline-line {
  width: 2px;
  flex: 1;
  min-height: 20px;
  margin-top: 6px;
  background: repeating-linear-gradient(to bottom, #CBD5E1 0 4px, transparent 4px 8px);
}
.timeline-date-col {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 6px;
  min-width: 132px;
  flex-shrink: 0;
}
.tl-date {
  font-size: 13px;
  font-weight: 900;
  color: #121316;
}
.tl-source {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  color: #1D4ED8;
  background: #EDF4FF;
  border: 1.5px solid #121316;
  padding: 2px 9px;
  border-radius: 6px;
  font-weight: 800;
  max-width: 132px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.timeline-content-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 6px;
}
.tl-title {
  font-size: 18.5px;
  font-weight: 900;
  color: #121316;
  margin: 0;
  line-height: 1.38;
  letter-spacing: -0.01em;
}
.tl-excerpt {
  font-size: 13.5px;
  color: #4B5262;
  font-weight: 600;
  line-height: 1.58;
  margin: 0;
}
.timeline-arrow-col {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.tl-go {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  border: 2px solid #121316;
  background: #F8FAFF;
  color: #121316;
  font-size: 16px;
  font-weight: 900;
  box-shadow: 2px 2px 0 #121316;
  transition: transform 0.15s ease, background 0.15s ease;
}
.timeline-news-card:hover .tl-go {
  background: #CCFF00;
  color: #121316;
  transform: translateX(2px);
}

/* ==================== READER VIEW ==================== */
.article-meta-pills {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.article-read-time {
  background: #EDF4FF;
  border: 1.5px solid #121316;
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 11.5px;
  font-weight: 800;
  color: #2563EB;
}
.article-author-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 2px solid #121316;
  padding-bottom: 20px;
  margin-bottom: 24px;
  flex-wrap: wrap;
  gap: 16px;
}
.author-info {
  display: flex;
  align-items: center;
  gap: 12px;
}
.author-avatar {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: #CCFF00;
  border: 2px solid #121316;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 900;
  font-size: 18px;
  box-shadow: 2px 2px 0px #121316;
}
.author-name {
  font-size: 14px;
  font-weight: 900;
  color: #121316;
}
.author-role {
  font-size: 11px;
  font-weight: 800;
  color: #5E6577;
}
.article-actions {
  display: flex;
  gap: 10px;
}
.action-pill-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #F5F2EB;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 6px 14px;
  font-size: 12.5px;
  font-weight: 800;
  color: #121316;
  cursor: pointer;
  box-shadow: 2px 2px 0px #121316;
  text-decoration: none;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
}
.action-pill-btn:hover {
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0px #121316;
}
.toc-link-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: #FFFFFF;
  border: 1.5px solid #121316;
  border-radius: 8px;
  padding: 4px 10px;
  font-size: 12.5px;
  font-weight: 800;
  color: #121316;
  text-decoration: none;
}
.toc-link-chip:hover {
  background: #EDF4FF;
  color: #2563EB;
}
.toc-num {
  font-size: 10.5px;
  color: #2563EB;
  font-weight: 900;
}
.article-tags-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 36px;
  padding-top: 20px;
  border-top: 1.5px dashed rgba(18, 19, 22, 0.2);
}
.tag-chip {
  background: #EDF4FF;
  border: 1.5px solid #121316;
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 800;
  color: #2563EB;
}
.article-bottom-cta {
  margin-top: 48px;
  background: linear-gradient(135deg, #EDF4FF 0%, #FFFFFF 100%);
  border: 2.5px solid #121316;
  border-radius: 18px;
  padding: 28px 34px;
  box-shadow: 5px 5px 0px #121316;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
}
.cta-banner-copy-link {
  text-decoration: none;
  color: inherit;
  flex: 1;
  min-width: 240px;
  display: block;
  transition: transform 0.15s ease;
}
.cta-banner-copy-link:hover .cta-banner-title {
  color: #2563EB;
}
.cta-banner-badge {
  display: inline-block;
  background: #CCFF00;
  color: #121316;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 2px 8px;
  font-size: 10.5px;
  font-weight: 900;
  margin-bottom: 8px;
  box-shadow: 2px 2px 0px #121316;
}
.cta-banner-title {
  font-size: 19px;
  font-weight: 900;
  color: #121316;
  margin-bottom: 6px;
  transition: color 0.15s ease;
}
.cta-banner-sub {
  font-size: 13.5px;
  color: #5E6577;
  font-weight: 700;
}
.cta-banner-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.cta-home-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 40px;
  padding: 0 18px;
  border-radius: 11px;
  background: #FFFFFF;
  color: #121316 !important;
  border: 2.5px solid #121316;
  font-weight: 900;
  font-size: 13.5px;
  text-decoration: none;
  box-shadow: 3px 3px 0px #121316;
  transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
}
.cta-home-btn:hover {
  background: #CCFF00;
  transform: translate(-1.5px, -1.5px);
  box-shadow: 4px 4px 0px #121316;
}

/* ==================== PREV/NEXT & 6 RELATED ITEMS ==================== */
.detail-prev-next-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
  margin-top: 28px;
}
.prev-next-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 8px;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 16px;
  padding: 20px 24px;
  box-shadow: 4px 4px 0px #121316;
  text-decoration: none;
  color: #121316;
  transition: transform 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;
}
.prev-next-card:hover:not(.prev-next-empty) {
  transform: translate(-2px, -2px);
  box-shadow: 6px 6px 0px #121316;
  background: #F8FAFF;
}
.prev-next-card.next-card {
  text-align: right;
  align-items: flex-end;
}
.prev-next-empty {
  background: #F5F2EB;
  border-style: dashed;
  box-shadow: none;
  opacity: 0.75;
}
.pn-label {
  font-size: 11.5px;
  font-weight: 900;
  color: #2563EB;
  background: #EDF4FF;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 2px 8px;
  width: fit-content;
}
.pn-title {
  font-size: 16px;
  font-weight: 900;
  color: #121316;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.pn-meta {
  font-size: 12px;
  font-weight: 700;
  color: #5E6577;
}

.detail-more-section {
  margin-top: 40px;
  padding-top: 32px;
  border-top: 2.5px solid #121316;
}
.more-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.more-header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.more-badge {
  background: #CCFF00;
  color: #121316;
  border: 2px solid #121316;
  border-radius: 7px;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 900;
  box-shadow: 2px 2px 0px #121316;
}
.more-title {
  font-size: 22px;
  font-weight: 900;
  color: #121316 !important;
  margin: 0;
}
.more-all-link {
  font-size: 13.5px;
  font-weight: 900;
  color: #121316;
  background: #FFFFFF;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 6px 14px;
  text-decoration: none;
  box-shadow: 2.5px 2.5px 0px #121316;
  transition: transform 0.12s ease, background 0.12s ease;
}
.more-all-link:hover {
  background: #CCFF00;
  transform: translate(-1px, -1px);
}
.more-related-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}
.more-related-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 16px;
  padding: 18px 20px;
  box-shadow: 4px 4px 0px #121316;
  text-decoration: none;
  color: #121316;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.more-related-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 6px 6px 0px #121316;
  background: #FDFDFD;
}
.more-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.more-card-cat {
  background: #EDF4FF;
  color: #2563EB;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 900;
}
.more-card-date {
  font-size: 11.5px;
  font-weight: 700;
  color: #5E6577;
}
.more-card-title {
  font-size: 15.5px;
  font-weight: 900;
  color: #121316 !important;
  line-height: 1.4;
  margin: 0 0 8px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.more-card-summary {
  font-size: 13px;
  font-weight: 600;
  color: #5E6577;
  line-height: 1.5;
  margin: 0 0 12px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  flex: 1;
}
.more-card-go {
  font-size: 12.5px;
  font-weight: 900;
  color: #2563EB;
}
@media (min-width: 768px) and (max-width: 1279px) {
  .more-related-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 767px) {
  /* minmax(0,1fr) 而非 1fr：1fr 的隐式 min-width:auto 会被卡片内长 URL/长单词的
     min-content（WebKit 实测 315.7px > 容器 232px）撑破，造成 390px 下 1~2px 横向滚动 */
  .detail-prev-next-grid,
  .more-related-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .more-related-card {
    min-width: 0;
  }
  .more-card-title,
  .more-card-excerpt {
    overflow-wrap: anywhere;
  }
}

/* ==================== EMPTY & RESPONSIVE ==================== */
.empty-icon {
  font-size: 42px;
  margin-bottom: 12px;
}
.empty-title {
  font-size: 18px;
  font-weight: 900;
  margin-bottom: 6px;
}
.empty-sub {
  font-size: 14px;
  color: #5E6577;
  margin-bottom: 18px;
}
.reset-filter-btn {
  background: #121316;
  color: #CCFF00;
  border: 2px solid #121316;
  border-radius: 10px;
  padding: 8px 18px;
  font-size: 13px;
  font-weight: 800;
  cursor: pointer;
  box-shadow: 3px 3px 0px #121316;
}

@media (max-width: 900px) {
  .neo-featured-card {
    grid-template-columns: 1fr;
  }
  .featured-visual {
    padding: 20px;
  }
  .neo-cards-grid {
    grid-template-columns: 1fr;
  }
  .timeline-news-card {
    flex-direction: column;
    gap: 12px;
  }
  .timeline-rail {
    display: none;
  }
}

.neo-loading-box,
.neo-error-box,
.neo-empty-box {
  background: #FFFFFF;
  border: 2.5px solid #121316;
  border-radius: 16px;
  padding: 40px;
  text-align: center;
  font-weight: 800;
  box-shadow: 4px 4px 0px #121316;
}

.public-featured-section {
  --featured-surface: #FFFFFF;
  --featured-ink: #121316;
  --featured-muted: #5E6577;
  --featured-border: #121316;
  --featured-accent: #CCFF00;
  margin: 0 0 34px;
  color: var(--featured-ink);
}

.public-featured-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 0 0 14px;
}
.public-featured-header::before {
  content: "★ FEATURED · 编辑精选专栏";
  font-family: 'JetBrains Mono', 'PingFang SC', monospace;
  font-size: 12px;
  font-weight: 900;
  letter-spacing: 0.06em;
  color: var(--featured-ink);
  opacity: 0.85;
}

.public-featured-title {
  display: inline-block;
  padding: 7px 13px;
  border-radius: 999px;
  background: var(--featured-accent);
  color: #121316;
  font-size: 15px;
  font-weight: 900;
  letter-spacing: .02em;
}

.public-featured-subtitle {
  margin: 8px 0 0;
  color: var(--featured-muted);
  font-size: 12px;
}

.public-featured-all-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 14px;
  border-radius: 999px;
  border: 2px solid #121316;
  background: #FFFFFF;
  color: #121316;
  font-size: 12.5px;
  font-weight: 800;
  text-decoration: none;
  white-space: nowrap;
  box-shadow: 2.5px 2.5px 0 #121316;
  transition: transform 0.14s ease, box-shadow 0.14s ease, background 0.14s ease;
}

.public-featured-all-link:hover,
.public-featured-all-link:focus-visible {
  background: #CCFF00;
  color: #121316;
  text-decoration: none;
  transform: translate(-1px, -1px);
  box-shadow: 3.5px 3.5px 0 #121316;
}

.public-featured-layout {
  display: grid;
  grid-template-columns: minmax(280px, 2fr) minmax(0, 3fr);
  gap: 18px;
  align-items: stretch;
}

.public-featured-large,
.public-featured-small {
  display: flex;
  min-width: 0;
  overflow: hidden;
  color: var(--featured-ink);
  background: var(--featured-surface);
  border: 2px solid var(--featured-border);
  border-radius: 16px;
  box-shadow: 4px 4px 0 var(--featured-border);
  text-decoration: none;
  transition: transform .18s ease, box-shadow .18s ease;
}

.public-featured-large:hover,
.public-featured-large:focus-visible,
.public-featured-small:hover,
.public-featured-small:focus-visible {
  transform: translate(-2px, -2px);
  box-shadow: 7px 7px 0 var(--featured-border);
  outline: none;
}

.public-featured-large {
  flex-direction: column;
}

.public-featured-small-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  grid-auto-rows: 1fr;
  gap: 14px;
}

.public-featured-small {
  flex-direction: column;
}

.public-featured-cover {
  position: relative;
  display: grid;
  place-items: center;
  overflow: hidden;
  flex: 0 0 auto;
  background: linear-gradient(135deg, #25304a, #121316);
}

.public-featured-cover-large {
  aspect-ratio: 16 / 8;
  width: 100%;
}

.public-featured-cover-small {
  aspect-ratio: 16 / 9;
  width: 100%;
}

.public-featured-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.public-featured-cover-placeholder {
  color: var(--featured-accent);
  font-size: 34px;
  line-height: 1;
}

.public-featured-badge {
  position: absolute;
  left: 10px;
  bottom: 10px;
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--featured-accent);
  color: #121316;
  font-size: 11px;
  font-weight: 900;
}

.public-featured-large-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  padding: 16px;
}

.public-featured-small-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 7px;
  padding: 11px 12px 12px;
}

.public-featured-meta,
.public-featured-small-category {
  color: var(--featured-muted);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: .06em;
  text-transform: uppercase;
}

.public-featured-meta {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
}

.public-featured-card-title {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--featured-ink) !important;
  font-weight: 900;
  line-height: 1.35;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.public-featured-large-title {
  margin-top: 10px;
  font-size: 21px;
}

.public-featured-large-summary {
  display: -webkit-box;
  margin: 9px 0 0;
  overflow: hidden;
  color: var(--featured-muted);
  font-size: 13px;
  line-height: 1.55;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.public-featured-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 8px;
  color: var(--featured-muted);
  font-size: 11px;
}
.public-featured-tags span {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 6px;
  background: #F1F5F9;
  border: 1.5px solid #CBD5E1;
  color: #475569;
  font-size: 11px;
  font-weight: 800;
}

/* ---- 精选：一行一条（左大图 + 右文案） ---- */
.public-featured-layout.public-featured-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.public-featured-row {
  position: relative;
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr) 48px;
  overflow: hidden;
  color: var(--featured-ink);
  background: var(--featured-surface);
  border: 2.5px solid var(--featured-border);
  border-radius: 18px;
  box-shadow: 5px 5px 0 var(--featured-border);
  text-decoration: none;
  transition: transform .18s ease, box-shadow .18s ease, border-color .18s ease;
}
.public-featured-row:hover,
.public-featured-row:focus-visible {
  transform: translate(-2px, -2px);
  box-shadow: 7px 7px 0 var(--featured-border);
  outline: none;
}
.public-featured-cover-row {
  width: 100%;
  height: 100%;
  min-height: 176px;
  aspect-ratio: auto;
  border-right: 2px solid var(--featured-border);
  background:
    radial-gradient(circle at 82% 20%, rgba(204, 255, 0, 0.22), transparent 52%),
    radial-gradient(circle at 18% 82%, rgba(37, 99, 235, 0.35), transparent 58%),
    linear-gradient(135deg, #0F172A 0%, #1E293B 100%);
}
.public-featured-row:nth-child(3n+2) .public-featured-cover-row {
  background:
    radial-gradient(circle at 82% 22%, rgba(96, 165, 250, 0.34), transparent 52%),
    radial-gradient(circle at 16% 80%, rgba(139, 92, 246, 0.32), transparent 56%),
    linear-gradient(135deg, #111827 0%, #1E1B4B 100%);
}
.public-featured-row:nth-child(3n+2) .public-featured-cover-placeholder svg {
  border-color: rgba(96, 165, 250, 0.55);
  stroke: #93C5FD;
}
.public-featured-row:nth-child(3n+2) .cover-ph-dot {
  background: #60A5FA;
}
.public-featured-row:nth-child(3n) .public-featured-cover-row {
  background:
    radial-gradient(circle at 80% 22%, rgba(251, 191, 36, 0.28), transparent 52%),
    radial-gradient(circle at 18% 82%, rgba(37, 99, 235, 0.3), transparent 56%),
    linear-gradient(135deg, #18181B 0%, #27272A 100%);
}
.public-featured-row:nth-child(3n) .public-featured-cover-placeholder svg {
  border-color: rgba(251, 191, 36, 0.55);
  stroke: #FBBF24;
}
.public-featured-row:nth-child(3n) .cover-ph-dot {
  background: #FBBF24;
}
.public-featured-cover-row img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform .45s ease;
}
.public-featured-row:hover .public-featured-cover-row img {
  transform: scale(1.045);
}
.public-featured-cover-placeholder {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}
.cover-ph-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.06) 1px, transparent 1px);
  background-size: 20px 20px;
  pointer-events: none;
}
.cover-ph-watermark {
  position: absolute;
  right: 12px;
  bottom: 4px;
  font-size: 44px;
  font-weight: 900;
  line-height: 1;
  color: rgba(255, 255, 255, 0.09);
  letter-spacing: -0.04em;
  pointer-events: none;
}
.cover-ph-brand {
  position: absolute;
  top: 12px;
  left: 12px;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.78);
  border: 1px solid rgba(255, 255, 255, 0.18);
  color: #E2E8F0;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.06em;
  z-index: 1;
}
.cover-ph-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #CCFF00;
}
.public-featured-cover-row .public-featured-cover-placeholder svg {
  position: relative;
  z-index: 1;
  width: 40px;
  height: 40px;
  padding: 8px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.65);
  border: 1.5px solid rgba(204, 255, 0, 0.45);
  fill: none;
  stroke: #CCFF00;
  stroke-width: 1.6;
  stroke-linejoin: round;
}
.public-featured-row-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
  padding: 18px 22px 16px;
}
.public-featured-meta .pf-row-index {
  padding: 2px 7px;
  border-radius: 6px;
  border: 1.5px solid #121316;
  background: var(--featured-accent);
  color: #121316;
  font-size: 11px;
  font-weight: 900;
}
.pf-row-cat {
  padding: 2px 9px;
  border-radius: 6px;
  background: #EDF4FF;
  border: 1.5px solid #121316;
  color: #1D4ED8;
  font-size: 11px;
  font-weight: 900;
}
.pf-row-read {
  margin-left: auto;
  color: var(--featured-muted);
  font-size: 11.5px;
}
.public-featured-row-title {
  display: -webkit-box;
  overflow: hidden;
  font-size: 20.5px;
  line-height: 1.35;
  letter-spacing: -0.01em;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
.public-featured-row-summary {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--featured-muted);
  font-size: 13.5px;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
.public-featured-row .public-featured-tags {
  margin-top: auto;
  padding-top: 8px;
}
.public-featured-row-cta {
  display: grid;
  place-items: center;
  border-left: 2px solid var(--featured-border);
  background: #F8FAFC;
  font-size: 18px;
  font-weight: 900;
  transition: transform .18s ease, background .18s ease;
}
.public-featured-row:hover .public-featured-row-cta {
  transform: translateX(3px);
  background: var(--featured-accent);
  color: #121316;
}

@media (max-width: 1100px) {
  .public-featured-layout {
    grid-template-columns: minmax(240px, 1fr) minmax(0, 1.35fr);
  }
  .public-featured-row {
    grid-template-columns: 240px minmax(0, 1fr) 40px;
  }
  .public-featured-small-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 767px) {
  .public-featured-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 10px;
  }
  .public-featured-layout {
    grid-template-columns: 1fr;
  }
  .public-featured-small-grid {
    grid-template-columns: 1fr;
    grid-auto-rows: auto;
  }
  .public-featured-small {
    display: grid;
    grid-template-columns: 112px minmax(0, 1fr);
    flex-direction: row;
  }
  .public-featured-cover-small {
    height: 100%;
    min-height: 94px;
  }
  .public-featured-large-title {
    font-size: 18px;
  }
}

:global(.dark) .public-featured-section {
  --featured-surface: #1E2026;
  --featured-ink: #E8EAED;
  --featured-muted: #AEB4C0;
  --featured-border: #0B0C0F;
}

/* ==================== TASK-024 封面 · 阅读信息 · 相关推荐 ==================== */
.article-cover {
  display: flex;
  width: 100%;
  aspect-ratio: 16 / 9;
  max-height: 400px;
  margin: 6px 0 26px;
  border: 2.5px solid #121316;
  border-radius: 16px;
  overflow: hidden;
  background: #EEF2FA;
  box-shadow: 5px 5px 0px #121316;
}
.article-cover-img,
.list-card-cover img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-placeholder {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  overflow: hidden;
  color: #2563EB;
  background:
    radial-gradient(circle at 85% 18%, rgba(204, 255, 0, 0.28), transparent 48%),
    radial-gradient(circle at 15% 85%, rgba(37, 99, 235, 0.14), transparent 55%),
    linear-gradient(135deg, #F8FAFF 0%, #EEF4FF 100%);
}
.cover-placeholder.cover-placeholder-news {
  background:
    radial-gradient(circle at 85% 18%, rgba(37, 99, 235, 0.18), transparent 50%),
    radial-gradient(circle at 18% 82%, rgba(204, 255, 0, 0.25), transparent 55%),
    linear-gradient(135deg, #F4F7FF 0%, #E8F0FE 100%);
}
.cover-placeholder .cover-ph-grid {
  background-image:
    linear-gradient(rgba(37, 99, 235, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(37, 99, 235, 0.07) 1px, transparent 1px);
  background-size: 18px 18px;
}
.cover-placeholder .cover-ph-watermark {
  right: 10px;
  bottom: 2px;
  font-size: 32px;
  color: rgba(37, 99, 235, 0.12);
}
.cover-placeholder .cover-ph-pill {
  position: absolute;
  top: 8px;
  left: 8px;
  padding: 2px 7px;
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.92);
  border: 1.5px solid #121316;
  color: #121316;
  font-size: 9.5px;
  font-weight: 900;
  letter-spacing: 0.05em;
  box-shadow: 1.5px 1.5px 0 #121316;
  z-index: 1;
}
.cover-placeholder svg {
  position: relative;
  z-index: 1;
  width: 36px;
  height: 36px;
  padding: 7px;
  border-radius: 10px;
  background: #FFFFFF;
  border: 1.8px solid #121316;
  box-shadow: 2px 2px 0 #121316;
  color: #2563EB;
}
.cover-placeholder svg * { fill: none; stroke: currentColor; stroke-width: 2; stroke-linejoin: round; }
.list-card-cover {
  flex: 0 0 240px;
  width: 240px;
  max-width: 240px;
  aspect-ratio: 16 / 9;
  align-self: center;
  border: 2px solid #121316;
  border-radius: 12px;
  overflow: hidden;
  background: #EEF2FA;
}
/* 标题 2 行截断、标签不越界，避免布局跳动 */
.spotlight-title,
.tl-title,
.tl-excerpt {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
}
.tl-tags { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; max-height: 22px; overflow: hidden; }
.tl-tag {
  font-size: 11px;
  font-weight: 800;
  color: #2563EB;
  background: #EDF4FF;
  border: 1.5px solid #121316;
  border-radius: 6px;
  padding: 1px 7px;
  white-space: nowrap;
}

@media (min-width: 768px) and (max-width: 1279px) {
  .list-card-cover {
    flex: 0 0 200px;
    width: 200px;
    max-width: 200px;
  }
}
@media (max-width: 767px) {
  .article-cover { max-height: none; margin: 4px 0 20px; border-radius: 12px; }
  .neo-news-spotlight { flex-direction: column; align-items: stretch; }
  .list-card-cover {
    flex: 0 0 auto;
    width: 100%;
    max-width: 100%;
    margin: 0;
    border-left: 0;
    border-right: 0;
    border-top: 0;
    border-radius: 0;
  }
  .spotlight-cover,
  .timeline-cover { order: -1; }
}

:global(.dark) .article-cover,
:global(.dark) .list-card-cover {
  background: #1E2026;
  border-color: #0B0C0F;
  box-shadow: 5px 5px 0px #0B0C0F;
}
:global(.dark) .cover-placeholder {
  color: #60A5FA;
  background:
    radial-gradient(circle at 85% 18%, rgba(204, 255, 0, 0.14), transparent 50%),
    radial-gradient(circle at 15% 85%, rgba(59, 130, 246, 0.22), transparent 55%),
    linear-gradient(135deg, #141824 0%, #1E2436 100%);
}
:global(.dark) .cover-placeholder .cover-ph-grid {
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.05) 1px, transparent 1px);
}
:global(.dark) .cover-placeholder .cover-ph-watermark {
  color: rgba(255, 255, 255, 0.07);
}
:global(.dark) .cover-placeholder .cover-ph-pill {
  background: rgba(15, 23, 42, 0.85);
  border-color: #475569;
  color: #E2E8F0;
  box-shadow: 1.5px 1.5px 0 #08090C;
}
:global(.dark) .cover-placeholder svg {
  background: #0F172A;
  border-color: #475569;
  color: #60A5FA;
  box-shadow: 2px 2px 0 #08090C;
}

/* TASK-033 D3：窄视口（手机宽度，含桌面窗口缩到 390px）页头改为可换行流式布局。
   原三列 grid（1fr auto 1fr）在 390px 下最小内容宽 563px，硬撑出 599px 横向滚动条。 */
@media (max-width: 767px) {
  .neo-sub-header-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 10px;
    padding: 12px 16px;
  }
  .sub-header-left {
    flex: 0 1 auto;
    min-width: 0;
    gap: 8px;
  }
  /* 类型指示胶囊与中心分段导航语义重复，窄屏隐藏以给导航让出宽度 */
  .sub-nav-indicator,
  .sub-nav-divider {
    display: none;
  }
  .sub-segmented-nav {
    flex: 0 0 auto;
    margin-left: auto;
    box-shadow: 3px 3px 0px #121316;
  }
  .seg-item {
    padding: 6px 10px;
  }
  .cta-wrap {
    flex: 0 1 auto;
    margin-left: auto;
    gap: 8px;
  }
  .nav-home-btn {
    padding: 0 10px;
    font-size: 12px;
  }
  .lang-menu {
    left: auto;
    right: 0;
  }
}

@media (max-width: 420px) {
  .seg-code {
    display: none;
  }
  .seg-item {
    font-size: 12px;
  }
  .brand {
    font-size: 18px;
  }
}

/* PUB-14 补充：深色主题下这些浅纸面色块仍用浅底，需固定深墨色，避免全局深色文字色落浅底 */
:global(.dark) .hero-headline,
:global(.dark) .hero-headline span,
:global(.dark) .hero-sub,
:global(.dark) .core-title,
:global(.dark) .pain-item span {
  color: #121316 !important;
}
:global(.dark) .pain-code {
  color: #FFFFFF !important;
}

/* ===== v0.4 公开站响应式与深色对比度补齐 ===== */
:global(.dark) .hc-title {
  color: #121316 !important;
}
:global(.dark) .neo-reader-layout .more-section-title {
  color: #F8FAFC !important;
}
:global(.dark) .neo-article-row-card:not(.is-featured-article) .article-row-index {
  color: #475569 !important;
}
@media (max-width: 767px) {
  .page-shell {
    padding: 12px 14px !important;
    grid-template-rows: auto auto 1fr !important;
    max-width: 100vw;
    box-sizing: border-box;
    overflow-x: hidden;
  }
  .header-bar {
    display: flex !important;
    flex-wrap: wrap !important;
    justify-content: space-between !important;
    gap: 8px !important;
    width: 100%;
  }
  .breaker-wrap {
    order: 3;
    width: 100%;
    display: flex;
    justify-content: center;
  }
  .breaker-switch {
    max-width: 100%;
  }
  .sw-btn {
    padding: 6px 12px;
    font-size: 12px;
  }
  .big-trigger-btn {
    max-width: 100%;
    box-sizing: border-box;
  }
}

</style>

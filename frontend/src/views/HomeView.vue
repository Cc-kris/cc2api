<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"
      ></div>
      <div
        class="absolute bottom-1/4 right-1/4 h-64 w-64 rounded-full bg-primary-400/10 blur-3xl"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <div class="flex items-center">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
        </div>

        <div class="hidden items-center gap-7 text-sm font-medium text-gray-600 md:flex dark:text-dark-300">
          <a href="#features" class="transition-colors hover:text-primary-500">核心特性</a>
          <a v-if="docUrl" :href="docUrl" class="transition-colors hover:text-primary-500">{{ t('home.docs') }}</a>
          <a href="#flagship" class="transition-colors hover:text-primary-500">旗舰板块</a>
        </div>

        <!-- Nav Actions -->
        <div class="flex items-center gap-2">
          <!-- Language Switcher -->
          <LocaleSwitcher />

          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
            <svg
              class="h-3 w-3 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
              />
            </svg>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 py-16">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section - Left/Right Layout -->
        <div class="mb-16 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <!-- Left: Text Content -->
          <div class="flex-1 text-center lg:text-left">
            <div class="mb-5 inline-flex items-center gap-2 rounded-full border border-primary-200/70 bg-white/70 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-primary-600 shadow-sm backdrop-blur dark:border-primary-800/70 dark:bg-dark-800/70 dark:text-primary-300">
              <span class="h-2 w-2 animate-pulse rounded-full bg-primary-500"></span>
              CCAI · 高性能 API 网关
            </div>
            <h1 class="hero-title mb-5 text-4xl font-bold text-gray-900 dark:text-white md:text-5xl lg:text-7xl">
              {{ siteName }}<span class="text-primary-500">.</span>
            </h1>
            <p class="mb-8 text-lg text-gray-600 dark:text-dark-300 md:text-xl">
              {{ siteSubtitle }}
            </p>

            <!-- CTA Button -->
            <div class="flex flex-wrap items-center justify-center gap-3 lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
              <a v-if="docUrl" :href="docUrl" class="btn btn-secondary px-6 py-3 text-base">
                {{ t('home.docs') }}
                <Icon name="book" size="md" />
              </a>
            </div>
          </div>

          <!-- Right: Terminal Animation -->
          <div class="hero-art flex flex-1 justify-center lg:justify-end">
            <div class="hero-orbit orbit-one"></div><div class="hero-orbit orbit-two"></div>
            <div class="hero-card">
              <svg viewBox="0 0 560 420" role="img" aria-label="CCAI network visualization" class="h-auto w-full">
                <defs>
                  <linearGradient id="ccaiGlow" x1="0" x2="1" y1="0" y2="1"><stop offset="0" stop-color="#2dd4bf"/><stop offset="1" stop-color="#6366f1"/></linearGradient>
                  <filter id="ccaiShadow"><feDropShadow dx="0" dy="18" stdDeviation="18" flood-color="#0f172a" flood-opacity=".32"/></filter>
                </defs>
                <rect width="560" height="420" rx="28" fill="#0b1220"/>
                <circle cx="280" cy="205" r="116" fill="url(#ccaiGlow)" opacity=".16"/>
                <g stroke="#67e8f9" stroke-opacity=".5" stroke-width="1.5"><path d="M92 108L210 178L280 88L365 178L470 112M110 304L210 230L280 320L365 232L465 300M210 178L210 230M365 178L365 232M280 88V320"/></g>
                <g fill="#0f172a" stroke="#5eead4" stroke-width="2" filter="url(#ccaiShadow)"><circle cx="92" cy="108" r="16"/><circle cx="470" cy="112" r="16"/><circle cx="110" cy="304" r="16"/><circle cx="465" cy="300" r="16"/><circle cx="280" cy="88" r="19"/><circle cx="280" cy="320" r="19"/><circle cx="210" cy="205" r="23"/><circle cx="365" cy="205" r="23"/></g>
                <circle cx="280" cy="205" r="52" fill="url(#ccaiGlow)" filter="url(#ccaiShadow)"/><text x="280" y="213" text-anchor="middle" fill="white" font-size="22" font-family="Inter, sans-serif" font-weight="700">CCAI</text>
                <g fill="#cbd5e1" font-size="11" font-family="ui-monospace, monospace"><text x="56" y="78">OPENAI</text><text x="430" y="78">GEMINI</text><text x="72" y="340">CLAUDE</text><text x="425" y="340">GROK</text></g>
              </svg>
              <div class="hero-card-glass"><span class="h-2 w-2 rounded-full bg-emerald-400"></span><span>All systems operational</span><span class="ml-auto font-mono text-emerald-300">99.99%</span></div>
            </div>
            <div class="terminal-container">
              <div class="terminal-window">
                <!-- Window header -->
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <!-- Terminal content -->
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">https://api.cc-ai.xyz/v1/chat/completions</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># 智能路由至最优上游...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Feature Tags - Centered -->
        <div id="features" class="mb-12 grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="swap" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">订阅转标准 API</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="shield" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">会话粘性路由</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="chart" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">实时计费统计</span>
          </div>
        </div>

        <!-- Features Grid -->
        <div class="mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-blue-500 to-blue-600 shadow-lg shadow-blue-500/30 transition-transform group-hover:scale-110"
            >
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">统一网关接入</h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              一个地址接入全部主流模型，OpenAI 兼容格式，零迁移成本。
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">多账号负载均衡</h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              账号池智能调度，自动故障转移，配额利用率最大化。
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-purple-500 to-purple-600 shadow-lg shadow-purple-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">余额与配额管理</h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              实时扣费、明细可查，团队额度灵活分配。
            </p>
          </div>
        </div>

        <!-- Quick Access: API endpoint & key -->
        <div id="providers" class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">快速接入</h2>
          <p class="text-sm text-gray-600 dark:text-dark-400">
            统一接口地址，一个 Key 调用全部模型
          </p>
        </div>

        <div class="mb-16 mx-auto max-w-3xl">
          <div class="rounded-2xl border border-gray-200/50 bg-white/70 p-6 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/70">
            <div class="mb-4">
              <p class="mb-1 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">API 地址</p>
              <code class="block w-full rounded-lg bg-gray-100 px-4 py-2.5 font-mono text-sm text-gray-800 dark:bg-dark-900 dark:text-dark-100">https://api.cc-ai.xyz/v1</code>
            </div>
            <div>
              <p class="mb-1 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">API Key 示例</p>
              <code class="block w-full rounded-lg bg-gray-100 px-4 py-2.5 font-mono text-sm text-gray-800 dark:bg-dark-900 dark:text-dark-100">cc-ai-pro-sk-xxxxxxxx</code>
            </div>
          </div>
        </div>

        <!-- Flagship Section: clients (left 4) vs flagship models (right 4) -->
        <div id="flagship" class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">旗舰板块</h2>
          <p class="text-sm text-gray-600 dark:text-dark-400">
            主流客户端全面适配 · 一线旗舰模型全覆盖
          </p>
        </div>

        <div class="mb-16 grid gap-6 md:grid-cols-2">
          <!-- Left wing: client integrations (4) -->
          <div class="rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/60">
            <h3 class="mb-4 text-center text-lg font-semibold text-gray-900 dark:text-white">客户端接入</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <!-- Client 1 -->
              <div class="rounded-xl border border-gray-200/50 bg-white/70 p-4 dark:border-dark-700/50 dark:bg-dark-800/70">
                <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-sky-500 to-sky-600 text-sm font-bold text-white">聊</div>
                <p class="text-sm font-semibold text-gray-800 dark:text-dark-100">Chat / WebUI</p>
                <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">任意 OpenAI 兼容聊天界面即插即用</p>
              </div>
              <!-- Client 2 -->
              <div class="rounded-xl border border-gray-200/50 bg-white/70 p-4 dark:border-dark-700/50 dark:bg-dark-800/70">
                <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-emerald-500 to-emerald-600 text-sm font-bold text-white">码</div>
                <p class="text-sm font-semibold text-gray-800 dark:text-dark-100">Coding IDE</p>
                <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">各类编程助手插件无缝接入编码工作流</p>
              </div>
              <!-- Client 3 -->
              <div class="rounded-xl border border-gray-200/50 bg-white/70 p-4 dark:border-dark-700/50 dark:bg-dark-800/70">
                <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-violet-500 to-violet-600 text-sm font-bold text-white">流</div>
                <p class="text-sm font-semibold text-gray-800 dark:text-dark-100">Workflow Agent</p>
                <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">自动化 Agent 与工作流引擎标准调用</p>
              </div>
              <!-- Client 4 -->
              <div class="rounded-xl border border-gray-200/50 bg-white/70 p-4 dark:border-dark-700/50 dark:bg-dark-800/70">
                <div class="mb-2 flex h-9 w-9 items-center justify-center rounded-lg bg-gradient-to-br from-amber-500 to-amber-600 text-sm font-bold text-white">开</div>
                <p class="text-sm font-semibold text-gray-800 dark:text-dark-100">API / SDK</p>
                <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">OpenAI 兼容接口，全语言 SDK 直接调用</p>
              </div>
            </div>
          </div>

          <!-- Right wing: flagship models (4 + 4) -->
          <div class="rounded-2xl border border-primary-200/60 bg-white/60 p-6 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800/60 dark:bg-dark-800/60">
            <h3 class="mb-4 text-center text-lg font-semibold text-gray-900 dark:text-white">旗舰模型</h3>
            <div class="grid gap-4 sm:grid-cols-2">
              <!-- Coding flagship -->
              <div class="rounded-xl border border-primary-100 bg-white/70 p-4 dark:border-primary-900/40 dark:bg-dark-800/70">
                <p class="mb-3 text-xs font-bold uppercase tracking-wider text-primary-600 dark:text-primary-300">编程旗舰</p>
                <ul class="space-y-1.5 text-sm text-gray-700 dark:text-dark-200">
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400"></span><span><b>the model 5.5</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">编码能力天花板</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400"></span><span><b>Claude opus 4.7</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">复杂工程重构首选</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400"></span><span><b>gpt-6-astra</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">全栈生成一气呵成</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400"></span><span><b>gemini 4 argon</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">长上下文代码理解</span></span></li>
                </ul>
              </div>
              <!-- Top-tier flagship -->
              <div class="rounded-xl border border-primary-100 bg-white/70 p-4 dark:border-primary-900/40 dark:bg-dark-800/70">
                <p class="mb-3 text-xs font-bold uppercase tracking-wider text-primary-600 dark:text-primary-300">顶流旗舰</p>
                <ul class="space-y-1.5 text-sm text-gray-700 dark:text-dark-200">
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400"></span><span><b>gpt-6.1-sol</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">推理与创作双优</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400"></span><span><b>gemini 4 argon</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">多模态全能选手</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400"></span><span><b>Kimi-k3</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">超长文本处理专家</span></span></li>
                  <li class="flex items-start gap-1.5"><span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-400"></span><span><b>Grok 4.7</b><span class="ml-1 text-xs text-gray-500 dark:text-dark-400">实时信息快人一步</span></span></li>
                </ul>
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'CCAI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '一站式大模型 API 聚合网关')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// GitHub URL
const githubUrl = 'https://cc-ai.xyz'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.hero-title {
  letter-spacing: -0.055em;
  line-height: 0.98;
}

.hero-art {
  position: relative;
  min-height: 430px;
  width: min(100%, 560px);
}

.hero-card {
  position: relative;
  z-index: 2;
  width: min(100%, 510px);
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 28px;
  background: #0b1220;
  box-shadow: 0 30px 80px rgba(15, 23, 42, 0.28);
}

.hero-card-glass {
  position: absolute;
  right: 18px;
  bottom: 18px;
  left: 18px;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 12px;
  padding: 10px 13px;
  color: #e2e8f0;
  background: rgba(15, 23, 42, 0.72);
  backdrop-filter: blur(14px);
  font-size: 12px;
}

.hero-orbit {
  position: absolute;
  z-index: 1;
  border: 1px solid rgba(45, 212, 191, 0.24);
  border-radius: 999px;
  transform: rotate(-20deg);
}

.orbit-one { inset: 8% -8% 12% 8%; }
.orbit-two { inset: -6% 10% 8% -6%; transform: rotate(35deg); border-color: rgba(99, 102, 241, 0.25); }

.hero-art .terminal-container {
  position: absolute;
  z-index: 3;
  right: -8px;
  bottom: -34px;
  transform: scale(0.72);
  transform-origin: bottom right;
}

@media (max-width: 640px) {
  .hero-art { min-height: 350px; }
  .hero-art .terminal-container { right: -58px; bottom: -52px; transform: scale(0.52); }
}

/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: 420px;
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 14px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(30, 41, 59, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a78bfa;
}
.code-url {
  color: #14b8a6;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Blinking Cursor */
.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(20, 184, 166, 0.2),
    0 0 40px rgba(20, 184, 166, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
</style>

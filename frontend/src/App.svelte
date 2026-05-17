<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from '@iconify/svelte';
  import { Alert, Badge, Button } from 'flowbite-svelte';
  import { Doughnut, Line } from 'svelte-chartjs';
  import {
    ArcElement,
    CategoryScale,
    Chart as ChartJS,
    Filler,
    Legend,
    LineElement,
    LinearScale,
    PointElement,
    type Plugin,
    TimeScale,
    Tooltip
  } from 'chart.js';
  import type { AuditEvent, AuditFilters, AuditOptions, AuditPage, AdminMe, MailServerCheck, MailServerCheckResponse, MetricsResponse, Summary } from './api';
  import { ApiError, apiGet, apiGetWithAuthRecovery, apiPost, auditParams, isTransientAuthStatus, rangeToParams } from './api';
  import Detail from './components/Detail.svelte';
  import Field from './components/Field.svelte';
  import { resolvedTheme, setThemeMode, supportedThemeModes, themePreference, type ThemeMode } from './theme';

  const centerTextPlugin = {
    id: 'centerText',
    afterDraw(chart) {
      if ((chart.config as { type?: string }).type !== 'doughnut') return;
      const dataset = chart.data.datasets[0];
      const values = (dataset?.data ?? []).map((value) => Number(value) || 0);
      const total = values.reduce((sum, value) => sum + value, 0);
      if (total <= 0) return;

      const { ctx, chartArea } = chart;
      const centerX = (chartArea.left + chartArea.right) / 2;
      const centerY = (chartArea.top + chartArea.bottom) / 2;
      const fontFamily = 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';

      ctx.save();
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillStyle = '#475569';
      ctx.font = `600 12px ${fontFamily}`;
      ctx.fillText('Total', centerX, centerY - 9);
      ctx.fillStyle = '#0f172a';
      ctx.font = `700 17px ${fontFamily}`;
      ctx.fillText(total.toLocaleString(), centerX, centerY + 12);
      ctx.restore();
    }
  } satisfies Plugin<'doughnut'>;

  ChartJS.register(CategoryScale, LinearScale, LineElement, PointElement, TimeScale, Tooltip, Filler, ArcElement, Legend, centerTextPlugin);

  const ranges = [
    { value: '1h', label: '1 hour' },
    { value: '6h', label: '6 hours' },
    { value: '24h', label: '24 hours' },
    { value: '7d', label: '7 days' },
    { value: '30d', label: '1 month' }
  ];

  const pageSizeOptions = ['25', '50', '100', '200'];
  const sidebarStorageKey = 'mx-api-go.admin.sidebarCollapsed';

  type ActiveView = 'dashboard' | 'audit';
  type MailServerState = 'disabled' | 'checking' | 'unknown' | 'error' | 'ok';
  type MailServerBadgeColor = 'green' | 'red' | 'yellow' | 'blue';

  let me: AdminMe | null = null;
  let metrics: MetricsResponse | null = null;
  let auditOptions: AuditOptions = { actions: [], endpoints: [], results: [] };
  let page: AuditPage = { items: [], total: 0, nextCursor: null, hasNext: false };
  let activeView: ActiveView = 'dashboard';
  let selected: AuditEvent | null = null;
  let detailOpen = false;
  let resetOpen = false;
  let resetConfirmation = '';
  let resetReason = '';
  let resetting = false;
  let mailServerCheck: MailServerCheck | null = null;
  let mailServerChecking = false;
  let mailServerError = '';
  let loading = true;
  let auditLoading = false;
  let error = '';
  let recoverableAuthError = false;
  let metricsRange = '24h';
  let auditPageSize = '25';
  let themeMenuOpen = false;
  let mobileMenuOpen = false;
  let sidebarCollapsed = initialSidebarCollapsed();
  let currentPage = 0;
  let pageCursors: Array<string | null> = [null];
  let pageStarts = [0];
  let lastKnownTotal = 0;
  let filters: AuditFilters = {
    from: '',
    to: '',
    actor: '',
    action: '',
    endpoint: '',
    path: '',
    method: '',
    result: '',
    statusCode: '',
    requestId: ''
  };
  $: pageSize = Number(auditPageSize) > 0 ? Math.min(Number(auditPageSize), 200) : 25;
  $: pageRangeLabel = `${page.items.length ? pageStarts[currentPage] + 1 : 0}-${pageStarts[currentPage] + page.items.length}`;
  $: totalLabel = page.total ?? lastKnownTotal;
  $: currentThemeLabel = themeLabel($themePreference);
  $: currentThemeIcon = themeIcon($themePreference);
  $: pageTitle = activeView === 'audit' ? 'Audit Log' : 'Dashboard';

  $: summary = metrics?.summary ?? emptySummary();
  $: publicApiValues = [summary.successful, summary.failed];
  $: validationValues = [summary.validationSuccesses, summary.validationFailures];
  $: mailValues = [summary.mailSendSuccesses, summary.mailSendFailures];
  $: statusClassValues = [summary.count4xx, summary.count5xx];
  $: publicApiData = doughnutData(['business API success', 'business API failure'], publicApiValues, ['#16a34a', '#dc2626']);
  $: validationData = doughnutData(['validation success', 'validation failure'], validationValues, ['#0f766e', '#ea580c']);
  $: mailData = doughnutData(['mail send success', 'mail send failure'], mailValues, ['#2563eb', '#f97316']);
  $: statusClassData = doughnutData(['4xx', '5xx'], statusClassValues, ['#f59e0b', '#7f1d1d']);
  $: mailServerStateValue = mailServerState(me?.authDisabled, mailServerChecking, mailServerCheck);
  $: mailServerBadgeColorValue = mailServerBadgeColor(mailServerStateValue);
  $: mailServerStatusLabelValue = mailServerStatusLabel(mailServerStateValue);
  $: adminUserLabel = me?.user || me?.email || '';
  $: chartData = {
    labels: metrics?.points.map((point) => new Date(point.timestamp).toLocaleString()) ?? [],
    datasets: [
      {
        label: 'Business API requests',
        data: metrics?.points.map((point) => point.count) ?? [],
        borderColor: '#2563eb',
        backgroundColor: 'rgba(37, 99, 235, 0.12)',
        pointRadius: 2,
        tension: 0.25,
        fill: true
      }
    ]
  };
  $: chartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { display: false },
      tooltip: { mode: 'index' as const, intersect: false }
    },
    scales: {
      x: { ticks: { maxRotation: 0, autoSkip: true, maxTicksLimit: 8 } },
      y: { beginAtZero: true, ticks: { precision: 0 } }
    }
  };
  $: doughnutOptions = {
    responsive: true,
    maintainAspectRatio: false,
    cutout: '62%',
    plugins: {
      legend: {
        position: 'bottom' as const,
        labels: { boxWidth: 10, usePointStyle: true }
      }
    }
  };

  onMount(() => {
    syncViewFromHash();
    window.addEventListener('hashchange', syncViewFromHash);
    reloadAll();
    return () => window.removeEventListener('hashchange', syncViewFromHash);
  });

  async function reloadAll() {
    loading = true;
    error = '';
    recoverableAuthError = false;
    try {
      me = await apiGetWithAuthRecovery<AdminMe>('/me');
      auditOptions = await apiGetWithAuthRecovery<AuditOptions>('/audit-options');
      await loadMetrics(true);
      await loadAuditPage(0, true, true);
      if (!me.authDisabled) {
        void loadMailServerCheck();
      }
    } catch (err) {
      if (isRecoverableAuthError(err)) {
        recoverableAuthError = true;
        error = 'Authentication is not ready or admin access was denied. Refresh authentication and try again.';
      } else {
        error = err instanceof Error ? err.message : 'Failed to load dashboard data';
      }
    } finally {
      loading = false;
    }
  }

  async function loadMetrics(withAuthRecovery = false) {
    const params = rangeToParams(metricsRange);
    if (filters.endpoint) params.set('endpoint', filters.endpoint);
    if (filters.method) params.set('method', filters.method);
    if (filters.result) params.set('result', filters.result);
    metrics = withAuthRecovery
      ? await apiGetWithAuthRecovery<MetricsResponse>('/request-metrics', params)
      : await apiGet<MetricsResponse>('/request-metrics', params);
  }

  async function loadAuditPage(pageIndex: number, withAuthRecovery = false, includeTotal = false) {
    auditLoading = true;
    const targetPage = Math.max(0, pageIndex);
    const cursor = pageCursors[targetPage] ?? null;
    const params = auditParams(filters, pageSize, cursor, includeTotal);
    try {
      const nextPage = withAuthRecovery ? await apiGetWithAuthRecovery<AuditPage>('/audit-events', params) : await apiGet<AuditPage>('/audit-events', params);
      currentPage = targetPage;
      page = { ...nextPage, total: nextPage.total ?? lastKnownTotal };
      if (typeof nextPage.total === 'number') {
        lastKnownTotal = nextPage.total;
      }
    } finally {
      auditLoading = false;
    }
  }

  async function applyFilters() {
    error = '';
    recoverableAuthError = false;
    try {
      resetPagination();
      await loadMetrics();
      await loadAuditPage(0, false, true);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to apply filters';
    }
  }

  async function applyMetricsRange(range: string) {
    metricsRange = range;
    error = '';
    recoverableAuthError = false;
    try {
      await loadMetrics();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load request metrics';
    }
  }

  async function loadMailServerCheck() {
    mailServerChecking = true;
    mailServerError = '';
    try {
      const response = await apiPost<MailServerCheckResponse>('/mail-server-check', {});
      mailServerCheck = response.mailServer;
    } catch (err) {
      if (isRecoverableAuthError(err)) {
        recoverableAuthError = true;
        mailServerError = 'Authentication is not ready or admin access was denied.';
      } else {
        mailServerError = err instanceof Error ? err.message : 'Failed to check backend mail server';
      }
    } finally {
      mailServerChecking = false;
    }
  }

  function emptySummary(): Summary {
    return {
      total: 0,
      successful: 0,
      failed: 0,
      validationSuccesses: 0,
      validationFailures: 0,
      mailSendSuccesses: 0,
      mailSendFailures: 0,
      count4xx: 0,
      count5xx: 0
    };
  }

  function resultColor(result: string) {
    if (result === 'success') return 'green';
    if (result === 'denied') return 'yellow';
    return 'red';
  }

  function fmtTime(value: string) {
    return value ? new Date(value).toLocaleString() : '';
  }

  function fmtEventTime(item: AuditEvent | null) {
    if (!item) return '';
    return item.timestampDisplay || fmtTime(item.timestamp);
  }

  function eventTarget(item: AuditEvent) {
    return item.endpoint || item.path || '-';
  }

  function eventMessage(item: AuditEvent) {
    return item.message || item.errorCode || '-';
  }

  function showDetail(item: AuditEvent) {
    selected = item;
    detailOpen = true;
  }

  function syncViewFromHash() {
    activeView = window.location.hash === '#audit-log' ? 'audit' : 'dashboard';
    mobileMenuOpen = false;
  }

  function setView(view: ActiveView) {
    activeView = view;
    window.location.hash = view === 'audit' ? 'audit-log' : 'dashboard';
    mobileMenuOpen = false;
  }

  function resetFilters() {
    filters = { ...filters, from: '', to: '', actor: '', action: '', endpoint: '', path: '', method: '', result: '', statusCode: '', requestId: '' };
  }

  function resetPagination() {
    currentPage = 0;
    pageCursors = [null];
    pageStarts = [0];
    lastKnownTotal = 0;
    page = { items: [], total: 0, nextCursor: null, hasNext: false };
  }

  async function loadNextPage() {
    if (!page.nextCursor || auditLoading) return;
    error = '';
    const nextIndex = currentPage + 1;
    pageCursors = [...pageCursors.slice(0, nextIndex), page.nextCursor];
    pageStarts = [...pageStarts.slice(0, nextIndex), pageStarts[currentPage] + page.items.length];
    try {
      await loadAuditPage(nextIndex);
    } catch (err) {
      pageCursors = pageCursors.slice(0, nextIndex);
      pageStarts = pageStarts.slice(0, nextIndex);
      error = err instanceof Error ? err.message : 'Failed to load next audit page';
    }
  }

  async function loadPreviousPage() {
    if (currentPage === 0 || auditLoading) return;
    error = '';
    try {
      await loadAuditPage(currentPage - 1);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load previous audit page';
    }
  }

  function refreshAuthentication() {
    window.location.reload();
  }

  async function changePageSize() {
    error = '';
    recoverableAuthError = false;
    try {
      resetPagination();
      await loadAuditPage(0, false, true);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to change page size';
    }
  }

  function isRecoverableAuthError(err: unknown) {
    return err instanceof ApiError && isTransientAuthStatus(err.status);
  }

  function doughnutData(labels: string[], values: number[], colors: string[]) {
    return {
      labels,
      datasets: [
        {
          data: values,
          backgroundColor: colors,
          borderWidth: 0
        }
      ]
    };
  }

  function hasCounts(values: number[]) {
    return values.some((value) => value > 0);
  }

  function mailServerState(authDisabled: boolean | undefined, checking: boolean, check: MailServerCheck | null): MailServerState {
    if (authDisabled) return 'disabled';
    if (checking && !check) return 'checking';
    if (!check) return 'unknown';
    return check.code ? 'error' : 'ok';
  }

  function mailServerBadgeColor(state: MailServerState): MailServerBadgeColor {
    if (state === 'ok') return 'green';
    if (state === 'error') return 'red';
    if (state === 'disabled') return 'yellow';
    return 'blue';
  }

  function mailServerStatusLabel(state: MailServerState) {
    if (state === 'ok') return 'Connected';
    if (state === 'error') return 'Failed';
    if (state === 'disabled') return 'Unavailable';
    if (state === 'checking') return 'Checking';
    return 'Not checked';
  }

  function boolLabel(value: boolean) {
    return value ? 'Yes' : 'No';
  }

  function themeLabel(mode: ThemeMode) {
    if (mode === 'dark') return 'Dark';
    if (mode === 'light') return 'Light';
    return 'System';
  }

  function themeIcon(mode: ThemeMode) {
    if (mode === 'dark') return 'heroicons:moon';
    if (mode === 'light') return 'heroicons:sun';
    return 'heroicons:computer-desktop';
  }

  function selectTheme(mode: ThemeMode) {
    setThemeMode(mode);
    themeMenuOpen = false;
  }

  function initialSidebarCollapsed(): boolean {
    if (typeof window === 'undefined') return false;
    return window.localStorage.getItem(sidebarStorageKey) === 'true';
  }

  function setSidebarCollapsed(next: boolean) {
    sidebarCollapsed = next;
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(sidebarStorageKey, String(next));
    }
  }

  function openResetModal() {
    resetConfirmation = '';
    resetReason = '';
    resetOpen = true;
  }

  async function resetAuditLog() {
    resetting = true;
    error = '';
    try {
      await apiPost<{ status: string }>('/audit-events/reset', {
        confirmation: resetConfirmation,
        reason: resetReason
      });
      resetOpen = false;
      resetFilters();
      resetPagination();
      await reloadAll();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to reset audit log';
    } finally {
      resetting = false;
    }
  }
</script>

<main class="min-h-screen bg-[#f4f7fb] text-slate-950 dark:bg-slate-950 dark:text-slate-100">
  <div class="min-h-screen lg:flex">
    <aside class={`hidden flex-col border-sky-100 bg-[#eef7fb] shadow-sm transition-[width] duration-150 dark:border-slate-800 dark:bg-slate-950 lg:fixed lg:inset-y-0 lg:flex lg:border-r ${sidebarCollapsed ? 'lg:w-20' : 'lg:w-64'}`}>
      <div class={sidebarCollapsed ? 'px-3 py-6' : 'px-5 py-6'}>
        <div class={`flex items-start gap-1.5 ${sidebarCollapsed ? 'justify-center' : 'w-fit'}`}>
          <img src={sidebarCollapsed ? ($resolvedTheme === 'dark' ? './mx-api-go-logo-dark.svg' : './mx-api-go-logo.svg') : ($resolvedTheme === 'dark' ? './mx-api-go-logo-str-dark.svg' : './mx-api-go-logo-str.svg')} alt="mx-api-go" class={`h-10 w-auto md:h-11 ${sidebarCollapsed ? 'max-w-11 object-contain' : 'max-w-32'}`} />
          {#if me && !sidebarCollapsed}
            <span class="mt-1 whitespace-nowrap text-sm font-medium text-slate-700 dark:text-slate-300">{me.version}</span>
          {/if}
        </div>
      </div>

      <nav class="flex flex-col gap-2 px-3 py-3">
        <button
          class={`inline-flex min-h-10 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition ${sidebarCollapsed ? 'justify-center' : ''} ${activeView === 'dashboard' ? 'bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-sky-200' : 'text-slate-700 hover:bg-white/70 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-slate-900 dark:hover:text-white'}`}
          aria-current={activeView === 'dashboard' ? 'page' : undefined}
          aria-label="Dashboard"
          title="Dashboard"
          onclick={() => setView('dashboard')}
        >
          <Icon icon="lets-icons:chart" class="h-5 w-5 shrink-0" />
          {#if !sidebarCollapsed}
            <span>Dashboard</span>
          {/if}
        </button>
        <button
          class={`inline-flex min-h-10 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition ${sidebarCollapsed ? 'justify-center' : ''} ${activeView === 'audit' ? 'bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-sky-200' : 'text-slate-700 hover:bg-white/70 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-slate-900 dark:hover:text-white'}`}
          aria-current={activeView === 'audit' ? 'page' : undefined}
          aria-label="Audit Log"
          title="Audit Log"
          onclick={() => setView('audit')}
        >
          <Icon icon="lets-icons:order" class="h-5 w-5 shrink-0" />
          {#if !sidebarCollapsed}
            <span>Audit Log</span>
          {/if}
        </button>
      </nav>

      <div class={`mt-auto grid gap-3 border-t border-sky-100 px-3 py-4 text-sm text-slate-600 dark:border-slate-800 dark:text-slate-400 ${sidebarCollapsed ? 'justify-items-center' : 'px-5'}`}>
        <div class={`flex ${sidebarCollapsed ? 'justify-center' : 'justify-end'}`}>
          <button
            class="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-sky-100 bg-transparent text-slate-700 hover:bg-white/45 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600 dark:border-slate-800 dark:text-slate-100 dark:hover:bg-slate-900"
            aria-label={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-pressed={sidebarCollapsed}
            title={sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            onclick={() => setSidebarCollapsed(!sidebarCollapsed)}
          >
            <Icon icon={sidebarCollapsed ? 'lucide:panel-left-open' : 'lucide:panel-left-close'} class="h-5 w-5" />
          </button>
        </div>
        {#if me && !sidebarCollapsed}
          <div class="min-w-0">
            {#if me.commitURL}
              <a class="inline-flex max-w-full items-center gap-1.5 truncate text-slate-700 hover:text-blue-700 hover:underline dark:text-slate-300 dark:hover:text-sky-300" href={me.commitURL} target="_blank" rel="noreferrer">
                <Icon icon="mdi:github" class="h-4 w-4 shrink-0" />
                <span class="truncate">{me.shortCommit}</span>
              </a>
            {:else}
              <span class="block truncate">Commit {me.shortCommit}</span>
            {/if}
          </div>
        {/if}
      </div>
    </aside>

    <div class={`flex min-h-screen flex-1 flex-col transition-[padding-left] duration-150 ${sidebarCollapsed ? 'lg:pl-20' : 'lg:pl-64'}`}>
      <header class="sticky top-0 z-20 flex min-h-16 items-center justify-between gap-3 border-b border-slate-200 bg-[#f4f7fb]/95 px-4 backdrop-blur dark:border-slate-800 dark:bg-slate-950/90 sm:px-6 lg:px-8">
        <div class="flex min-w-0 items-center gap-3">
          <button
            class="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-slate-300 bg-white text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:bg-slate-800 lg:hidden"
            aria-label={mobileMenuOpen ? 'Close navigation menu' : 'Open navigation menu'}
            aria-expanded={mobileMenuOpen}
            aria-controls="mobile-admin-menu"
            onclick={() => (mobileMenuOpen = !mobileMenuOpen)}
          >
            <Icon icon={mobileMenuOpen ? 'heroicons:x-mark' : 'heroicons:bars-3'} class="h-5 w-5" />
          </button>
          <h1 class="truncate text-xl font-semibold text-slate-950 dark:text-slate-100 sm:text-2xl">{pageTitle}</h1>
        </div>
        <div class="flex items-center gap-2">
          <div class="relative">
            <button
              class="inline-flex h-10 w-10 items-center justify-center rounded-lg border border-slate-300 bg-white text-slate-700 hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:bg-slate-800"
              aria-haspopup="menu"
              aria-expanded={themeMenuOpen}
              aria-label="Select theme"
              title={`Theme: ${currentThemeLabel}`}
              onclick={() => (themeMenuOpen = !themeMenuOpen)}
            >
              <Icon icon={currentThemeIcon} class="h-5 w-5" />
            </button>
            {#if themeMenuOpen}
              <div class="absolute right-0 top-12 z-30 w-44 rounded-lg border border-slate-200 bg-white p-1.5 shadow-lg dark:border-slate-700 dark:bg-slate-900" role="menu" aria-label="Theme">
                {#each supportedThemeModes as mode}
                  <button
                    class={`flex w-full items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-sm ${$themePreference === mode ? 'bg-blue-50 text-blue-700 dark:bg-slate-800 dark:text-sky-200' : 'text-slate-700 hover:bg-slate-50 dark:text-slate-200 dark:hover:bg-slate-800'}`}
                    role="menuitemradio"
                    aria-checked={$themePreference === mode}
                    onclick={() => selectTheme(mode)}
                  >
                    <span class="inline-flex items-center gap-2">
                      <Icon icon={themeIcon(mode)} class="h-4 w-4" />
                      {themeLabel(mode)}
                    </span>
                    {#if $themePreference === mode}
                      <Icon icon="heroicons:check" class="h-4 w-4" />
                    {/if}
                  </button>
                {/each}
              </div>
            {/if}
          </div>
          {#if me}
            <div class="inline-flex min-w-0 items-center gap-2 text-sm text-slate-600 dark:text-slate-300" aria-label="Authenticated admin user">
              <Icon icon="heroicons:user-circle" class="h-6 w-6 shrink-0 text-blue-700 dark:text-sky-300" />
              <span class="max-w-48 truncate font-semibold text-slate-950 dark:text-slate-100">{adminUserLabel}</span>
            </div>
          {/if}
        </div>
      </header>

      {#if mobileMenuOpen}
        <div id="mobile-admin-menu" class="fixed inset-x-0 top-16 z-30 border-b border-slate-200 bg-[#eef7fb] p-3 shadow-lg dark:border-slate-800 dark:bg-slate-950 lg:hidden">
          <nav class="grid gap-2" aria-label="Mobile navigation">
            <button
              class={`inline-flex min-h-11 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium ${activeView === 'dashboard' ? 'bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-sky-200' : 'text-slate-700 hover:bg-white/70 dark:text-slate-300 dark:hover:bg-slate-900'}`}
              aria-current={activeView === 'dashboard' ? 'page' : undefined}
              onclick={() => setView('dashboard')}
            >
              <Icon icon="lets-icons:chart" class="h-5 w-5" />
              <span>Dashboard</span>
            </button>
            <button
              class={`inline-flex min-h-11 items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium ${activeView === 'audit' ? 'bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-sky-200' : 'text-slate-700 hover:bg-white/70 dark:text-slate-300 dark:hover:bg-slate-900'}`}
              aria-current={activeView === 'audit' ? 'page' : undefined}
              onclick={() => setView('audit')}
            >
              <Icon icon="lets-icons:order" class="h-5 w-5" />
              <span>Audit Log</span>
            </button>
          </nav>
        </div>
      {/if}

      <div class="flex-1 px-4 py-5 sm:px-6 lg:px-8">
        <div class="mx-auto flex w-full max-w-7xl flex-col gap-5">
          {#if me?.authDisabled}
            <Alert color="yellow">
              <div class="flex items-center gap-2">
                <Icon icon="lets-icons:warning" class="h-5 w-5" />
                <span>Authentication is disabled. Protect this dashboard with an upstream access-control layer.</span>
              </div>
            </Alert>
          {/if}

          {#if error}
            <Alert color="red">
              <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <span>{error}</span>
                {#if recoverableAuthError}
                  <Button color="red" size="sm" onclick={refreshAuthentication}>Refresh authentication</Button>
                {/if}
              </div>
            </Alert>
          {/if}

          {#if activeView === 'dashboard'}
            <section class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
                <h2 class="text-sm font-semibold text-slate-700 dark:text-slate-200">Business API request result</h2>
                <div class="mt-3 h-56">
                  {#if hasCounts(publicApiValues)}
                    <Doughnut data={publicApiData} options={doughnutOptions} />
                  {:else}
                    <div class="mx-auto flex aspect-square w-full max-w-52 items-center justify-center rounded-full border-[18px] border-slate-200 text-sm font-medium text-slate-500 dark:border-slate-700 dark:text-slate-400">No data</div>
                  {/if}
                </div>
              </div>
              <div class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
                <h2 class="text-sm font-semibold text-slate-700 dark:text-slate-200">Validation request result</h2>
                <div class="mt-3 h-56">
                  {#if hasCounts(validationValues)}
                    <Doughnut data={validationData} options={doughnutOptions} />
                  {:else}
                    <div class="mx-auto flex aspect-square w-full max-w-52 items-center justify-center rounded-full border-[18px] border-slate-200 text-sm font-medium text-slate-500 dark:border-slate-700 dark:text-slate-400">No data</div>
                  {/if}
                </div>
              </div>
              <div class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
                <h2 class="text-sm font-semibold text-slate-700 dark:text-slate-200">Mail send result</h2>
                <div class="mt-3 h-56">
                  {#if hasCounts(mailValues)}
                    <Doughnut data={mailData} options={doughnutOptions} />
                  {:else}
                    <div class="mx-auto flex aspect-square w-full max-w-52 items-center justify-center rounded-full border-[18px] border-slate-200 text-sm font-medium text-slate-500 dark:border-slate-700 dark:text-slate-400">No data</div>
                  {/if}
                </div>
              </div>
              <div class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
                <h2 class="text-sm font-semibold text-slate-700 dark:text-slate-200">Error status class</h2>
                <div class="mt-3 h-56">
                  {#if hasCounts(statusClassValues)}
                    <Doughnut data={statusClassData} options={doughnutOptions} />
                  {:else}
                    <div class="mx-auto flex aspect-square w-full max-w-52 items-center justify-center rounded-full border-[18px] border-slate-200 text-sm font-medium text-slate-500 dark:border-slate-700 dark:text-slate-400">No data</div>
                  {/if}
                </div>
              </div>
            </section>

            <section class="rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
              <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex min-w-0 items-start gap-3">
                  <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-700 dark:bg-slate-800 dark:text-slate-200">
                    <Icon icon="mdi:server-network" class="h-6 w-6" />
                  </div>
                  <div class="min-w-0">
                    <div class="flex flex-wrap items-center gap-2">
                      <h2 class="text-base font-semibold text-slate-950 dark:text-slate-100">Backend mail server</h2>
                      <Badge color={mailServerBadgeColorValue}>{mailServerStatusLabelValue}</Badge>
                    </div>
                    <div class="mt-2 grid gap-x-6 gap-y-1 text-sm text-slate-600 dark:text-slate-300 sm:grid-cols-2 xl:grid-cols-4">
                      <div>Reachable: <span class="font-medium text-slate-900 dark:text-slate-100">{mailServerCheck ? boolLabel(mailServerCheck.reachable) : '-'}</span></div>
                      <div>SMTP auth: <span class="font-medium text-slate-900 dark:text-slate-100">{mailServerCheck ? (mailServerCheck.authenticationEnabled ? boolLabel(mailServerCheck.authenticated) : 'Disabled') : '-'}</span></div>
                      <div>TLS active: <span class="font-medium text-slate-900 dark:text-slate-100">{mailServerCheck ? boolLabel(mailServerCheck.tlsActive) : '-'}</span></div>
                      <div>Latency: <span class="font-medium text-slate-900 dark:text-slate-100">{mailServerCheck ? `${mailServerCheck.latencyMs} ms` : '-'}</span></div>
                    </div>
                    <div class="mt-2 text-sm text-slate-500 dark:text-slate-400">
                      {#if me?.authDisabled}
                        Header authentication required
                      {:else if mailServerError}
                        {mailServerError}
                      {:else if mailServerCheck?.message}
                        {mailServerCheck.message}
                      {:else if mailServerCheck?.checkedAt}
                        Last checked {fmtTime(mailServerCheck.checkedAt)}
                      {:else}
                        Waiting for first check
                      {/if}
                    </div>
                  </div>
                </div>
                <Button class="gap-2" color="alternative" disabled={me?.authDisabled || mailServerChecking} onclick={loadMailServerCheck}>
                  <Icon icon="mdi:refresh" class={`h-4 w-4 ${mailServerChecking ? 'animate-spin' : ''}`} />
                  <span>{mailServerChecking ? 'Checking' : 'Check now'}</span>
                </Button>
              </div>
            </section>

            <section class="w-full rounded-lg border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
              <div class="mb-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div>
                  <h2 class="text-lg font-semibold">Business API trend</h2>
                  <p class="text-sm text-slate-500 dark:text-slate-400">Validation and sendmail volume over the selected time range</p>
                </div>
                <div class="flex flex-wrap gap-2">
                  {#each ranges as range}
                    <Button size="sm" color={metricsRange === range.value ? 'blue' : 'alternative'} onclick={() => applyMetricsRange(range.value)}>
                      {range.label}
                    </Button>
                  {/each}
                </div>
              </div>
              <div class="h-72">
                {#if loading}
                  <div class="flex h-full items-center justify-center text-sm text-slate-500 dark:text-slate-400">Loading chart</div>
                {:else}
                  <Line data={chartData} options={chartOptions} />
                {/if}
              </div>
            </section>
          {:else}
            <section class="w-full overflow-hidden rounded-lg border border-slate-300 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
              <div class="flex items-center justify-end border-b border-slate-200 px-5 py-4 dark:border-slate-800">
                <div class="flex flex-wrap justify-end gap-2">
                  <Button color="alternative" onclick={reloadAll}>Refresh</Button>
                  <Button color="red" onclick={openResetModal}>Reset</Button>
                </div>
              </div>

      <fieldset class="min-w-0 border-0 border-b border-slate-200 p-5 dark:border-slate-800" disabled={auditLoading} aria-busy={auditLoading}>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <Field label="Actor"><input class="admin-input" bind:value={filters.actor} autocomplete="off" /></Field>
          <Field label="Action">
            <select class="admin-input" bind:value={filters.action}>
              <option value="">Any</option>
              {#each auditOptions.actions as action}
                <option value={action}>{action}</option>
              {/each}
            </select>
          </Field>
          <Field label="Endpoint">
            <select class="admin-input" bind:value={filters.endpoint}>
              <option value="">Any</option>
              {#each auditOptions.endpoints as endpoint}
                <option value={endpoint}>{endpoint}</option>
              {/each}
            </select>
          </Field>
          <Field label="Result">
            <select class="admin-input" bind:value={filters.result}>
              <option value="">Any</option>
              {#each auditOptions.results as result}
                <option value={result}>{result}</option>
              {/each}
            </select>
          </Field>
          <Field label="From"><input class="admin-input" type="datetime-local" bind:value={filters.from} /></Field>
          <Field label="To"><input class="admin-input" type="datetime-local" bind:value={filters.to} /></Field>
          <Field label="Method">
            <select class="admin-input" bind:value={filters.method}>
              <option value="">Any</option>
              <option>GET</option>
              <option>POST</option>
              <option>OPTIONS</option>
            </select>
          </Field>
        </div>
        <div class="mt-4 flex flex-wrap items-end gap-4">
          <Field label="Request ID"><input class="admin-input min-w-72" bind:value={filters.requestId} autocomplete="off" /></Field>
          <Field label="Path"><input class="admin-input min-w-72" bind:value={filters.path} placeholder="/api/v1" autocomplete="off" /></Field>
        </div>
        <div class="mt-5 flex flex-wrap gap-2 border-t border-slate-100 pt-4 dark:border-slate-800">
          <Button color="blue" disabled={auditLoading} onclick={applyFilters}>{auditLoading ? 'Applying' : 'Apply'}</Button>
          <Button color="alternative" disabled={auditLoading} onclick={async () => { resetFilters(); await applyFilters(); }}>Clear filters</Button>
          <span class="self-center text-sm text-slate-500 dark:text-slate-400" aria-live="polite">{page.total} events match the current filters</span>
        </div>
      </fieldset>

      <div class="hidden px-5 md:block">
        <div class="overflow-x-auto pb-3">
          <table class="w-full min-w-[1040px] border-collapse border-t border-slate-300 text-left text-sm dark:border-slate-700">
            <thead class="bg-slate-50 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-300">
              <tr>
                <th class="px-4 py-3 font-semibold">Timestamp</th>
                <th class="px-4 py-3 font-semibold">Actor</th>
                <th class="px-4 py-3 font-semibold">Action</th>
                <th class="px-4 py-3 font-semibold">Target</th>
                <th class="px-4 py-3 font-semibold">Result</th>
                <th class="px-4 py-3 font-semibold">Remote</th>
                <th class="px-4 py-3 font-semibold">Message</th>
                <th class="px-4 py-3 font-semibold"></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-200 text-slate-700 dark:divide-slate-800 dark:text-slate-300">
              {#each page.items as item}
                <tr class="hover:bg-slate-50 dark:hover:bg-slate-800/70">
                  <td class="whitespace-nowrap px-4 py-4 text-slate-900 dark:text-slate-100">{fmtEventTime(item)}</td>
                  <td class="px-4 py-4 text-slate-900 dark:text-slate-100">{item.actor}</td>
                  <td class="px-4 py-4">
                    <Badge color="blue">{item.action}</Badge>
                  </td>
                  <td class="max-w-56 truncate px-4 py-4">{eventTarget(item)}</td>
                  <td class="px-4 py-3"><Badge color={resultColor(item.result)}>{item.result}</Badge></td>
                  <td class="max-w-48 truncate px-4 py-4">{item.remoteAddr}</td>
                  <td class="max-w-72 px-4 py-4 text-slate-900 dark:text-slate-100">{eventMessage(item)}</td>
                  <td class="px-4 py-4 text-right">
                    <Button size="xs" color="alternative" onclick={() => showDetail(item)}>
                      <Icon icon="lets-icons:view" class="h-4 w-4" />
                      <span class="sr-only">Show details</span>
                    </Button>
                  </td>
                </tr>
              {:else}
                <tr>
                  <td colspan={8} class="px-4 py-8 text-center text-slate-500">No audit events</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="space-y-4 px-4 pb-3 md:hidden">
        {#each page.items as item}
          <article class="rounded-lg border border-slate-300 bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-900">
            <div class="text-sm font-medium text-slate-800 dark:text-slate-200">{fmtEventTime(item)}</div>
            <div class="mt-2 text-sm font-semibold text-slate-950 dark:text-slate-100">{item.actor}</div>
            <div class="mt-4 flex flex-wrap gap-2">
              <Badge color="blue">{item.action}</Badge>
              <Badge color={resultColor(item.result)}>{item.result}</Badge>
            </div>
            <p class="mt-4 text-sm text-slate-950 dark:text-slate-100">{eventMessage(item)}</p>
            <div class="mt-4 grid grid-cols-2 gap-3 text-sm">
              <div>
                <div class="text-xs font-medium text-slate-500 dark:text-slate-400">Target</div>
                <div class="mt-1 break-words text-slate-950 dark:text-slate-100">{eventTarget(item)}</div>
              </div>
              <div>
                <div class="text-xs font-medium text-slate-500 dark:text-slate-400">Status</div>
                <div class="mt-1 text-slate-950 dark:text-slate-100">{item.statusCode || '-'}</div>
              </div>
              <div>
                <div class="text-xs font-medium text-slate-500 dark:text-slate-400">Remote</div>
                <div class="mt-1 break-words text-slate-950 dark:text-slate-100">{item.remoteAddr || '-'}</div>
              </div>
              <div>
                <div class="text-xs font-medium text-slate-500 dark:text-slate-400">Duration</div>
                <div class="mt-1 text-slate-950 dark:text-slate-100">{item.durationMs} ms</div>
              </div>
            </div>
            <Button class="mt-4" color="alternative" onclick={() => showDetail(item)}>Show details</Button>
          </article>
        {:else}
          <div class="rounded-lg border border-slate-200 bg-white px-4 py-8 text-center text-sm text-slate-500 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">No audit events</div>
        {/each}
      </div>

      <nav class="pagination-bar px-5 pb-5 pt-3 text-sm" aria-label="Pagination">
        <label class="flex items-center gap-2 text-slate-600 dark:text-slate-300">
          <span class="whitespace-nowrap font-medium">Rows per page</span>
          <select class="admin-input w-24 py-2" bind:value={auditPageSize} onchange={changePageSize} disabled={auditLoading}>
            {#each pageSizeOptions as option}
              <option value={option}>{option}</option>
            {/each}
          </select>
        </label>
        <div class="text-center text-slate-500 dark:text-slate-400" aria-live="polite">
          <span>Showing {pageRangeLabel} of {totalLabel}</span>
          {#if auditLoading}
            <span class="ml-2">Loading</span>
          {/if}
        </div>
        <div class="flex justify-end gap-2">
          <Button size="sm" color="alternative" disabled={currentPage === 0 || auditLoading} onclick={loadPreviousPage}>Previous</Button>
          <Button size="sm" color="alternative" disabled={!page.hasNext || page.nextCursor === null || auditLoading} onclick={loadNextPage}>Next</Button>
        </div>
      </nav>
            </section>
          {/if}
        </div>
      </div>
    </div>
  </div>
</main>

{#if detailOpen && selected}
  <div class="fixed inset-0 z-50 overflow-hidden bg-slate-950/45 px-[5vw] py-[10vh] sm:flex sm:items-center sm:justify-center sm:p-4" role="presentation">
    <div class="box-border max-h-[80vh] w-full overflow-auto rounded-lg border border-slate-300 bg-white shadow-xl dark:border-slate-700 dark:bg-slate-900 sm:max-h-[calc(100vh-2rem)] sm:max-w-4xl" role="dialog" aria-modal="true" aria-labelledby="audit-detail-title">
      <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3 dark:border-slate-800 sm:px-5 sm:py-4">
        <h2 id="audit-detail-title" class="text-base font-semibold text-slate-950 dark:text-slate-100">Audit event detail</h2>
        <button class="rounded p-1 text-slate-500 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800" aria-label="Close audit event detail" onclick={() => (detailOpen = false)}>
          <Icon icon="lets-icons:close-round" class="h-5 w-5" />
        </button>
      </div>
      <div class="px-4 py-4 sm:px-5 sm:py-5">
        <div class="grid gap-3 text-sm sm:grid-cols-2">
          <Detail label="ID" value={String(selected.id)} />
          <Detail label="Timestamp" value={fmtEventTime(selected)} />
          <Detail label="Actor" value={selected.actor} />
          <Detail label="Actor source" value={selected.actorSource} />
          <Detail label="Action" value={selected.action} />
          <Detail label="Result" value={selected.result} />
          <Detail label="Method" value={selected.method} />
          <Detail label="Path" value={selected.path} />
          <Detail label="Endpoint" value={selected.endpoint} />
          <Detail label="Status code" value={String(selected.statusCode || '')} />
          <Detail label="Remote address" value={selected.remoteAddr} />
          <Detail label="Duration" value={`${selected.durationMs} ms`} />
          <Detail label="Request ID" value={selected.requestId} />
          <Detail label="Error code" value={selected.errorCode} />
          <Detail label="Message" value={selected.message} wide />
          <Detail label="User agent" value={selected.userAgent} wide />
        </div>
        {#if selected.metadata}
          <pre class="mt-4 max-h-52 overflow-auto rounded border border-slate-200 bg-slate-950 p-3 text-xs text-slate-100">{JSON.stringify(selected.metadata, null, 2)}</pre>
        {/if}
      </div>
    </div>
  </div>
{/if}

{#if resetOpen}
  <div class="fixed inset-0 z-50 overflow-hidden bg-slate-950/45 px-[5vw] py-[10vh] sm:flex sm:items-center sm:justify-center sm:p-4" role="presentation">
    <div class="box-border max-h-[80vh] w-full overflow-auto rounded-lg border border-slate-300 bg-white shadow-xl dark:border-slate-700 dark:bg-slate-900 sm:max-h-[calc(100vh-2rem)] sm:max-w-xl" role="dialog" aria-modal="true" aria-labelledby="reset-audit-title">
      <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3 dark:border-slate-800 sm:px-5 sm:py-4">
        <h2 id="reset-audit-title" class="text-base font-semibold text-slate-950 dark:text-slate-100">Reset audit log</h2>
        <button class="rounded p-1 text-slate-500 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800" aria-label="Close reset dialog" disabled={resetting} onclick={() => (resetOpen = false)}>
          <Icon icon="lets-icons:close-round" class="h-5 w-5" />
        </button>
      </div>
      <div class="space-y-4 px-4 py-4 sm:px-5 sm:py-5">
        <div class="flex gap-3 rounded-lg border border-red-300 bg-red-50 p-4 text-red-700">
          <Icon icon="lets-icons:warning" class="mt-0.5 h-5 w-5 shrink-0" />
          <div>
            <div class="font-semibold">Warning</div>
            <p class="mt-1 text-sm">Existing audit events will be removed and a new reset marker will remain visible.</p>
          </div>
        </div>
        <Field label="Confirmation">
          <input class="admin-input" bind:value={resetConfirmation} placeholder="RESET" autocomplete="off" />
        </Field>
        <Field label="Reason">
          <textarea class="admin-input min-h-24 resize-y" bind:value={resetReason}></textarea>
        </Field>
        <div class="flex gap-2 pt-1">
          <Button color="red" disabled={resetConfirmation !== 'RESET' || resetting} onclick={resetAuditLog}>
            {resetting ? 'Resetting' : 'Reset'}
          </Button>
          <Button color="alternative" disabled={resetting} onclick={() => (resetOpen = false)}>Cancel</Button>
        </div>
      </div>
    </div>
  </div>
{/if}

/* ============================================================
   AIBALANCE Dashboard - Client Logic
   ============================================================ */

(function () {
    'use strict';

    var REFRESH_INTERVAL = 10000;
    var refreshTimer = null;
    var isLoading = false;

    // ============ Theme ============

    function getPreferredTheme() {
        var saved = localStorage.getItem('aibalance-theme');
        if (saved) return saved;
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }

    function applyTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('aibalance-theme', theme);
        var btn = document.getElementById('theme-toggle');
        if (btn) {
            btn.textContent = theme === 'dark' ? '\u2600 Light' : '\u263E Dark';
        }
    }

    function toggleTheme() {
        var current = document.documentElement.getAttribute('data-theme') || 'light';
        applyTheme(current === 'dark' ? 'light' : 'dark');
    }

    // ============ Formatting ============

    function formatNumber(n) {
        if (n === null || n === undefined) return '0';
        if (n >= 1e9) return (n / 1e9).toFixed(1) + 'B';
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return n.toLocaleString('en-US');
        return String(n);
    }

    function formatPercent(n) {
        if (n === null || n === undefined) return '0%';
        return n.toFixed(1) + '%';
    }

    // ============ Data fetching ============

    function fetchStats(callback) {
        isLoading = true;
        setRefreshDot(true);
        var xhr = new XMLHttpRequest();
        xhr.open('GET', '/public/stats', true);
        xhr.timeout = 8000;
        xhr.onload = function () {
            isLoading = false;
            setRefreshDot(false);
            if (xhr.status === 200) {
                try { callback(null, JSON.parse(xhr.responseText)); }
                catch (e) {
                    console.error('[AIBALANCE] JSON parse error:', e, 'response:', xhr.responseText.substring(0, 200));
                    showError('Data parse error');
                    callback(e, null);
                }
            } else {
                console.error('[AIBALANCE] HTTP error:', xhr.status, xhr.responseText.substring(0, 200));
                showError('API returned HTTP ' + xhr.status + ' - server may need restart after rebuild');
                callback(new Error('HTTP ' + xhr.status), null);
            }
        };
        xhr.onerror = function () {
            isLoading = false; setRefreshDot(false);
            console.error('[AIBALANCE] Network error');
            showError('Network error');
            callback(new Error('Network error'), null);
        };
        xhr.ontimeout = function () {
            isLoading = false; setRefreshDot(false);
            console.error('[AIBALANCE] Request timeout');
            showError('Request timeout');
            callback(new Error('Timeout'), null);
        };
        xhr.send();
    }

    function showError(msg) {
        var el = document.getElementById('hero-status-text');
        if (el) el.textContent = msg;
        var dot = document.querySelector('.hero-status-dot');
        if (dot) dot.style.background = '#f87171';
    }

    function setRefreshDot(loading) {
        var dot = document.getElementById('refresh-dot');
        if (dot) dot.classList.toggle('loading', loading);
    }

    // ============ KPI Render ============

    function setKPI(id, value, sub) {
        var el = document.getElementById(id);
        if (!el) return;
        var valEl = el.querySelector('.kpi-value');
        var subEl = el.querySelector('.kpi-sub');
        if (valEl) animateValue(valEl, value);
        if (subEl && sub !== undefined) subEl.innerHTML = sub;
    }

    function animateValue(el, newText) {
        if (el.textContent === newText) return;
        el.textContent = newText;
        el.style.color = 'var(--accent)';
        setTimeout(function () { el.style.color = ''; }, 600);
    }

    function setHealthBar(id, percent) {
        var card = document.getElementById(id);
        if (!card) return;
        var fill = card.querySelector('.health-bar-fill');
        if (!fill) return;
        fill.style.width = Math.min(100, Math.max(0, percent)) + '%';
        fill.className = 'health-bar-fill';
        if (percent >= 95) fill.classList.add('good');
        else if (percent >= 80) fill.classList.add('warn');
        else fill.classList.add('bad');
    }

    function showHideCard(id, show) {
        var el = document.getElementById(id);
        if (!el) return;
        el.classList.toggle('hidden', !show);
    }

    // ============ Latency SVG chart ============

    function buildLatencyChart(points) {
        if (!points || points.length < 2) return '';

        var W = 300, H = 40, PAD = 2;
        var maxMs = 0;
        for (var i = 0; i < points.length; i++) {
            if (points[i].latency_ms > maxMs) maxMs = points[i].latency_ms;
        }
        if (maxMs < 1000) maxMs = 1000;
        // Cap visual at 30s for readability
        var visualMax = Math.min(maxMs * 1.2, 30000);

        var stepX = (W - PAD * 2) / (points.length - 1);
        var coords = [];
        for (var j = 0; j < points.length; j++) {
            var x = PAD + j * stepX;
            var val = Math.min(points[j].latency_ms, visualMax);
            var y = H - PAD - (val / visualMax) * (H - PAD * 2);
            coords.push(x.toFixed(1) + ',' + y.toFixed(1));
        }

        // Determine line color from last point
        var lastMs = points[points.length - 1].latency_ms;
        var color = 'var(--success)';
        if (lastMs > 20000) color = 'var(--error)';
        else if (lastMs > 5000) color = 'var(--warning)';

        // Threshold lines
        var thresholds = '';
        var y5 = H - PAD - (5000 / visualMax) * (H - PAD * 2);
        var y20 = H - PAD - (20000 / visualMax) * (H - PAD * 2);
        if (5000 < visualMax) {
            thresholds += '<line x1="' + PAD + '" y1="' + y5.toFixed(1) + '" x2="' + (W - PAD) + '" y2="' + y5.toFixed(1) + '" stroke="var(--warning)" stroke-width="0.5" stroke-dasharray="3,3" opacity="0.5"/>';
        }
        if (20000 < visualMax) {
            thresholds += '<line x1="' + PAD + '" y1="' + y20.toFixed(1) + '" x2="' + (W - PAD) + '" y2="' + y20.toFixed(1) + '" stroke="var(--error)" stroke-width="0.5" stroke-dasharray="3,3" opacity="0.5"/>';
        }

        var lastLabel = (lastMs / 1000).toFixed(1) + 's';

        return '<div class="latency-chart-wrap">' +
            '<span class="latency-label">' + lastLabel + '</span>' +
            '<svg viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none">' +
            thresholds +
            '<polyline points="' + coords.join(' ') + '" fill="none" stroke="' + color + '" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round"/>' +
            '</svg></div>';
    }

    // ============ Render models ============

    function renderModels(models, latencyHistory) {
        var container = document.getElementById('model-list');
        if (!container) return;

        if (!models || models.length === 0) {
            container.innerHTML = '<div class="empty-state">No models available</div>';
            return;
        }

        models.sort(function (a, b) {
            if (a.is_memfit !== b.is_memfit) return a.is_memfit ? -1 : 1;
            if (a.is_free !== b.is_free) return a.is_free ? 1 : -1;
            if (b.provider_count !== a.provider_count) return b.provider_count - a.provider_count;
            return a.display_name.localeCompare(b.display_name);
        });

        var html = '';
        for (var i = 0; i < models.length; i++) {
            var m = models[i];
            if (m.provider_count <= 0) continue;

            var statusClass = m.is_healthy ? 'healthy' : 'unhealthy';
            var badges = '';
            if (m.is_memfit) badges += '<span class="badge badge-memfit">Memfit</span>';
            if (m.is_free) badges += '<span class="badge badge-free">Free</span>';

            var successText = m.success_rate > 0 ? formatPercent(m.success_rate) : '-';

            // Get latency chart for this model
            var chartHtml = '';
            if (latencyHistory && latencyHistory[m.display_name]) {
                chartHtml = buildLatencyChart(latencyHistory[m.display_name]);
            }

            html += '<div class="model-card">' +
                '<div class="model-card-top">' +
                    '<div class="model-status ' + statusClass + '"></div>' +
                    '<div class="model-info">' +
                        '<div class="model-name">' + escapeHTML(m.display_name) + '</div>' +
                        '<div class="model-meta">' + badges +
                            '<span>' + m.provider_count + ' provider' + (m.provider_count > 1 ? 's' : '') + '</span>' +
                        '</div>' +
                    '</div>' +
                    '<div class="model-stats">' +
                        '<div class="model-stat-value">' + successText + '</div>' +
                        '<div class="model-stat-label">success</div>' +
                    '</div>' +
                '</div>' +
                chartHtml +
            '</div>';
        }

        container.innerHTML = html || '<div class="empty-state">No models available</div>';
    }

    // ============ Render uptime ============

    function renderUptime(uptimeSummary) {
        var container = document.getElementById('uptime-list');
        if (!container) return;

        if (!uptimeSummary || uptimeSummary.length === 0) {
            container.innerHTML = '<div class="empty-state">No uptime data yet</div>';
            return;
        }

        uptimeSummary.sort(function (a, b) {
            if (b.uptime_rate !== a.uptime_rate) return b.uptime_rate - a.uptime_rate;
            return a.model_name.localeCompare(b.model_name);
        });

        var seen = {}, deduped = [];
        for (var i = 0; i < uptimeSummary.length; i++) {
            if (!seen[uptimeSummary[i].model_name]) {
                seen[uptimeSummary[i].model_name] = true;
                deduped.push(uptimeSummary[i]);
            }
        }
        if (deduped.length > 10) deduped = deduped.slice(0, 10);

        var html = '';
        for (var j = 0; j < deduped.length; j++) {
            var entry = deduped[j];
            var rate = Math.min(100, Math.max(0, entry.uptime_rate));
            var barColor = 'var(--success)';
            if (rate < 95) barColor = 'var(--warning)';
            if (rate < 80) barColor = 'var(--error)';

            html += '<div class="uptime-row">' +
                '<div class="uptime-name">' + escapeHTML(entry.model_name) + '</div>' +
                '<div class="uptime-bar-wrap"><div class="uptime-bar-fill" style="width:' + rate + '%;background:' + barColor + '"></div></div>' +
                '<div class="uptime-percent">' + rate.toFixed(1) + '%</div>' +
            '</div>';
        }
        container.innerHTML = html;
    }

    // ============ Hero status ============

    function updateHeroStatus(data) {
        var el = document.getElementById('hero-status-text');
        if (!el || !data) return;
        if (data.healthy_providers > 0 && data.healthy_providers === data.total_providers) {
            el.textContent = 'All ' + data.total_providers + ' providers healthy';
        } else if (data.total_providers > 0) {
            el.textContent = data.healthy_providers + '/' + data.total_providers + ' providers healthy';
        } else {
            el.textContent = 'Connecting...';
        }
    }

    // ============ Main render ============

    function render(data) {
        if (!data) return;

        updateHeroStatus(data);

        setKPI('kpi-requests', formatNumber(data.total_requests), '');
        setKPI('kpi-success', formatPercent(data.success_rate), '');
        setHealthBar('kpi-success', data.success_rate);

        setKPI('kpi-providers', data.healthy_providers + ' / ' + data.total_providers, '');
        var providerRate = data.total_providers > 0 ? (data.healthy_providers / data.total_providers * 100) : 0;
        setHealthBar('kpi-providers', providerRate);

        setKPI('kpi-traffic', data.total_traffic_str || '0 B', '\u2248 ' + (data.estimated_tokens || '0') + ' Tokens');
        setKPI('kpi-concurrent', String(data.concurrent_requests || 0), '');
        setKPI('kpi-memory', (data.memory_mb || 0) + ' MB', '');

        showHideCard('kpi-websearch', data.web_search_count > 0);
        setKPI('kpi-websearch', formatNumber(data.web_search_count), '');

        showHideCard('kpi-amap', data.amap_count > 0);
        setKPI('kpi-amap', formatNumber(data.amap_count), '');

        renderModels(data.models, data.latency_history);

        var uptimeSection = document.getElementById('uptime-section');
        if (uptimeSection) {
            if (data.uptime_summary && data.uptime_summary.length > 0) {
                uptimeSection.style.display = '';
                renderUptime(data.uptime_summary);
            } else {
                uptimeSection.style.display = 'none';
            }
        }

        var footerTime = document.getElementById('footer-time');
        if (footerTime) footerTime.textContent = 'Last update: ' + (data.current_time || '-');

        removeSkeleton();
    }

    function removeSkeleton() {
        var skeletons = document.querySelectorAll('.skeleton');
        for (var i = 0; i < skeletons.length; i++) skeletons[i].parentNode.removeChild(skeletons[i]);
    }

    function escapeHTML(str) {
        if (!str) return '';
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    // ============ Init ============

    function init() {
        applyTheme(getPreferredTheme());
        var themeBtn = document.getElementById('theme-toggle');
        if (themeBtn) themeBtn.addEventListener('click', toggleTheme);

        fetchStats(function (err, data) {
            if (!err && data) render(data);
        });

        refreshTimer = setInterval(function () {
            if (isLoading) return;
            fetchStats(function (err, data) {
                if (!err && data) render(data);
            });
        }, REFRESH_INTERVAL);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();

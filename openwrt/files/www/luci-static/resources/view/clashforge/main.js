'use strict';
'require view';
'require poll';
'require ui';

return view.extend({
    autoScroll: true,
    cfPort: 7777,

    getApiBase: function() {
        return 'http://' + window.location.hostname + ':' + this.cfPort;
    },

    fetchLogs: function() {
        var self = this;
        return fetch(self.getApiBase() + '/api/v1/logs?level=info&limit=200')
            .then(function(r) { return r.ok ? r.json() : null; })
            .catch(function() { return null; });
    },

    fetchStatus: function() {
        var self = this;
        return fetch(self.getApiBase() + '/api/v1/status')
            .then(function(r) { return r.ok ? r.json() : null; })
            .catch(function() { return null; });
    },

    postCore: function(action) {
        var self = this;
        return fetch(self.getApiBase() + '/api/v1/core/' + action, { method: 'POST' })
            .then(function(r) { return r.json(); })
            .catch(function(e) { return { ok: false, error: { message: String(e) } }; });
    },

    render: function() {
        var self = this;
        var uiUrl = self.getApiBase();

        // ── Status indicator ─────────────────────────────────────────────
        var statusDot = E('span', {
            'id': 'cf-status-dot',
            'style': 'display:inline-block; width:10px; height:10px; border-radius:50%; background:#64748b; margin-right:8px; vertical-align:middle;'
        });
        var statusText = E('span', { 'id': 'cf-status-text', 'style': 'vertical-align:middle; color:#94a3b8;' }, ['检查中…']);
        var statusUptime = E('span', { 'id': 'cf-status-uptime', 'style': 'margin-left:16px; font-size:12px; color:#64748b; vertical-align:middle;' });

        var startBtn = E('button', {
            'id': 'cf-btn-start',
            'class': 'btn cbi-button cbi-button-apply',
            'style': 'display:none; margin-right:8px;',
            'click': function() {
                self.postCore('start').then(function() { self.refreshStatus(statusDot, statusText, statusUptime, startBtn, stopBtn, restartBtn); });
            }
        }, ['▶ 启动核心']);

        var stopBtn = E('button', {
            'id': 'cf-btn-stop',
            'class': 'btn cbi-button cbi-button-negative',
            'style': 'display:none; margin-right:8px;',
            'click': function() {
                if (!window.confirm('确认停止 Mihomo 核心？')) return;
                self.postCore('stop').then(function() { self.refreshStatus(statusDot, statusText, statusUptime, startBtn, stopBtn, restartBtn); });
            }
        }, ['■ 停止核心']);

        var restartBtn = E('button', {
            'id': 'cf-btn-restart',
            'class': 'btn cbi-button',
            'style': 'display:none;',
            'click': function() {
                self.postCore('restart').then(function() { self.refreshStatus(statusDot, statusText, statusUptime, startBtn, stopBtn, restartBtn); });
            }
        }, ['↺ 重启']);

        // ── Overview tab ─────────────────────────────────────────────────
        var overviewTab = E('div', { 'data-tab': 'overview', 'data-tab-title': '概览' }, [
            E('div', { 'class': 'cbi-section', 'style': 'margin-top:16px;' }, [
                E('div', { 'style': 'background:#1e293b; border-radius:12px; padding:20px; color:#f1f5f9;' }, [
                    E('div', { 'style': 'display:flex; align-items:center; margin-bottom:16px;' }, [
                        statusDot, statusText, statusUptime
                    ]),
                    E('div', { 'style': 'display:flex; gap:8px; flex-wrap:wrap; margin-bottom:20px;' }, [
                        startBtn, stopBtn, restartBtn
                    ]),
                    E('hr', { 'style': 'border-color:#334155; margin:16px 0;' }),
                    E('p', { 'style': 'color:#94a3b8; margin:0 0 12px;' }, [
                        '完整功能请使用独立 Web UI：'
                    ]),
                    E('a', {
                        'href': uiUrl,
                        'target': '_blank',
                        'rel': 'noopener',
                        'style': 'display:inline-block; background:#3b82f6; color:#fff; padding:10px 24px; border-radius:8px; text-decoration:none; font-weight:600;'
                    }, [ '🚀 打开 ClashForge Web UI →' ]),
                    E('p', { 'style': 'color:#475569; font-size:12px; margin:10px 0 0;' }, [
                        E('code', { 'style': 'background:#1e293b; padding:2px 6px; border-radius:4px;' }, [uiUrl])
                    ])
                ])
            ])
        ]);

        // ── Log tab ───────────────────────────────────────────────────────
        var logEl = E('pre', {
            'id': 'cf-log-pre',
            'style': [
                'background:#0f172a', 'color:#94a3b8', 'font-family:monospace',
                'font-size:12px', 'line-height:1.6', 'padding:14px',
                'border-radius:8px', 'height:500px', 'overflow-y:auto',
                'white-space:pre-wrap', 'word-break:break-all', 'margin:0'
            ].join(';')
        }, ['正在加载日志…']);

        var autoScrollChk = E('input', {
            'type': 'checkbox', 'checked': true, 'style': 'margin-right:5px;',
            'change': function(ev) { self.autoScroll = ev.target.checked; }
        });

        var logTab = E('div', { 'data-tab': 'logs', 'data-tab-title': '运行日志' }, [
            E('div', { 'class': 'cbi-section', 'style': 'margin-top:16px;' }, [
                E('div', { 'style': 'display:flex; align-items:center; gap:12px; margin-bottom:10px; flex-wrap:wrap;' }, [
                    E('label', { 'style': 'font-size:13px; color:#666; cursor:pointer;' }, [ autoScrollChk, '自动滚动' ]),
                    E('button', {
                        'class': 'btn cbi-button', 'style': 'padding:4px 12px; font-size:12px;',
                        'click': function() { self.refreshLogs(logEl); }
                    }, ['刷新']),
                    E('span', { 'style': 'font-size:12px; color:#999;' }, [
                        '日志来自 ClashForge API（最近 200 条）'
                    ])
                ]),
                logEl
            ])
        ]);

        // ── Connectivity / DNS-leak tab ───────────────────────────────────
        var connTab = self.buildConnTab();

        var mapEl = E('div', { 'class': 'cbi-map' }, [
            E('h2', {}, ['ClashForge']),
            E('div', { 'class': 'cbi-map-tabbox' }, [ overviewTab, connTab, logTab ])
        ]);

        ui.tabs.initTabGroup(mapEl.lastElementChild.childNodes);

        // Polling
        poll.add(function() {
            return Promise.all([
                self.refreshStatus(statusDot, statusText, statusUptime, startBtn, stopBtn, restartBtn),
                self.refreshLogs(logEl)
            ]);
        }, 3);

        setTimeout(function() {
            self.refreshStatus(statusDot, statusText, statusUptime, startBtn, stopBtn, restartBtn);
            self.refreshLogs(logEl);
        }, 400);

        return mapEl;
    },

    refreshStatus: function(dot, text, uptime, startBtn, stopBtn, restartBtn) {
        return this.fetchStatus().then(function(data) {
            if (!data || !data.ok) {
                dot.style.background = '#ef4444';
                text.textContent = 'ClashForge 未运行';
                uptime.textContent = '';
                startBtn.style.display = 'none';
                stopBtn.style.display = 'none';
                restartBtn.style.display = 'none';
                return;
            }
            var state = data.data.core.state;
            var colors = { running: '#22c55e', stopped: '#ef4444', error: '#ef4444', starting: '#f59e0b', stopping: '#f59e0b' };
            dot.style.background = colors[state] || '#64748b';
            text.textContent = ({ running: '运行中', stopped: '已停止', error: '错误', starting: '启动中…', stopping: '停止中…' }[state] || state);
            if (data.data.core.uptime > 0) {
                var s = data.data.core.uptime;
                uptime.textContent = s < 60 ? s + 's' : s < 3600 ? Math.floor(s/60) + 'm' : Math.floor(s/3600) + 'h ' + Math.floor((s%3600)/60) + 'm';
            } else {
                uptime.textContent = '';
            }
            startBtn.style.display = (state !== 'running') ? 'inline-block' : 'none';
            stopBtn.style.display = (state === 'running') ? 'inline-block' : 'none';
            restartBtn.style.display = (state === 'running') ? 'inline-block' : 'none';
        });
    },

    refreshLogs: function(logEl) {
        var self = this;
        return this.fetchLogs().then(function(data) {
            if (!data || !data.ok) {
                logEl.textContent = '(无法连接到 ClashForge API，请确认服务已启动)';
                return;
            }
            var logs = (data.data && data.data.logs) || [];
            if (logs.length === 0) {
                logEl.textContent = '(暂无日志)';
                return;
            }
            var levelColor = { info: '#94a3b8', debug: '#64748b', warning: '#f59e0b', warn: '#f59e0b', error: '#ef4444' };
            var lines = logs.map(function(l) {
                var t = l.ts ? new Date(l.ts * 1000).toLocaleTimeString('zh-CN') : '';
                var lvl = (l.level || '').toLowerCase();
                return '[' + t + '] [' + (l.level || '').toUpperCase() + '] ' + (l.msg || '');
            });
            logEl.textContent = lines.join('\n');
            if (self.autoScroll) {
                logEl.scrollTop = logEl.scrollHeight;
            }
        });
    },

    // ── Connectivity tab builder ──────────────────────────────────────────

    buildConnTab: function() {
        var self = this;

        // ── DNS Leak Detection ─────────────────────────────────────────────
        var dnsStatusBadge = E('span', {
            'id': 'cf-dns-badge',
            'style': 'display:none; padding:4px 10px; border-radius:20px; font-size:12px; font-weight:600; margin-left:12px; vertical-align:middle;'
        });

        var dnsResultBody = E('tbody', { 'id': 'cf-dns-tbody' });

        var dnsTable = E('table', {
            'id': 'cf-dns-table',
            'style': 'display:none; width:100%; border-collapse:collapse; font-size:13px; margin-top:14px;'
        }, [
            E('thead', {}, [
                E('tr', { 'style': 'color:#94a3b8; text-align:left; border-bottom:1px solid #334155;' }, [
                    E('th', { 'style': 'padding:6px 10px; width:36px; text-align:center;' }, ['#']),
                    E('th', { 'style': 'padding:6px 10px;' }, ['DNS 解析器 IP']),
                    E('th', { 'style': 'padding:6px 10px;' }, ['地理位置']),
                    E('th', { 'style': 'padding:6px 10px;' }, ['服务提供商']),
                    E('th', { 'style': 'padding:6px 10px; text-align:center;' }, ['类型'])
                ])
            ]),
            dnsResultBody
        ]);

        var dnsSummaryEl = E('div', {
            'id': 'cf-dns-summary',
            'style': 'display:none; margin-top:12px; padding:10px 14px; border-radius:8px; font-size:13px; line-height:1.5;'
        });

        var dnsSpinner = E('span', {
            'id': 'cf-dns-spinner',
            'style': 'display:none; margin-left:10px; font-size:12px; color:#94a3b8; vertical-align:middle;'
        }, ['⏳ 检测中，约需 15–20 秒…']);

        var dnsBtn = E('button', {
            'class': 'btn cbi-button cbi-button-apply',
            'style': 'padding:7px 18px; font-size:13px;',
            'click': function() {
                self.runDNSLeakTest(dnsBtn, dnsSpinner, dnsStatusBadge, dnsResultBody, dnsTable, dnsSummaryEl);
            }
        }, ['🔍 开始检测']);

        var dnsLastEl = E('span', {
            'id': 'cf-dns-last',
            'style': 'font-size:11px; color:#475569; margin-left:14px; vertical-align:middle;'
        });

        var dnsCard = E('div', {
            'style': 'background:#1e293b; border-radius:12px; padding:20px; color:#f1f5f9; margin-bottom:16px;'
        }, [
            E('div', { 'style': 'display:flex; align-items:center; margin-bottom:6px;' }, [
                E('span', { 'style': 'font-size:15px; font-weight:600;' }, ['🛡️ DNS 泄露检测']),
                dnsStatusBadge
            ]),
            E('p', { 'style': 'font-size:12px; color:#94a3b8; margin:0 0 14px;' }, [
                '检测 DNS 查询是否通过代理隧道，还是泄露至 ISP 的 DNS 服务器。',
                '检测时路由器会向 bash.ws 发起探测请求，结果反映路由器本机的 DNS 出口。'
            ]),
            E('div', { 'style': 'display:flex; align-items:center; flex-wrap:wrap; gap:8px;' }, [
                dnsBtn, dnsSpinner, dnsLastEl
            ]),
            dnsTable,
            dnsSummaryEl
        ]);

        return E('div', { 'data-tab': 'connectivity', 'data-tab-title': '连通性测试' }, [
            E('div', { 'class': 'cbi-section', 'style': 'margin-top:16px;' }, [ dnsCard ])
        ]);
    },

    runDNSLeakTest: function(btn, spinner, badge, tbody, table, summaryEl) {
        var self = this;

        btn.disabled = true;
        spinner.style.display = 'inline';
        badge.style.display = 'none';
        table.style.display = 'none';
        summaryEl.style.display = 'none';
        tbody.innerHTML = '';

        fetch(self.getApiBase() + '/api/v1/health/dns-leak')
            .then(function(r) { return r.ok ? r.json() : null; })
            .then(function(resp) {
                btn.disabled = false;
                spinner.style.display = 'none';

                if (!resp || !resp.ok || !resp.data) {
                    badge.textContent = '请求失败';
                    badge.style.cssText += '; background:#7f1d1d; color:#fca5a5; display:inline;';
                    return;
                }

                var d = resp.data;
                var entries  = d.entries  || [];
                var hasLeak  = d.has_leak;
                var summary  = d.summary  || '';
                var testedAt = d.tested_at || '';

                // Last-tested timestamp
                var lastEl = document.getElementById('cf-dns-last');
                if (lastEl && testedAt) {
                    try {
                        lastEl.textContent = '上次检测: ' + new Date(testedAt).toLocaleString('zh-CN');
                    } catch(e) { lastEl.textContent = ''; }
                }

                // Error path
                if (d.error) {
                    badge.textContent = '检测失败';
                    badge.style.background = '#7f1d1d';
                    badge.style.color = '#fca5a5';
                    badge.style.display = 'inline';
                    summaryEl.textContent = d.error;
                    summaryEl.style.background = '#450a0a';
                    summaryEl.style.color = '#fca5a5';
                    summaryEl.style.display = 'block';
                    return;
                }

                // Status badge
                if (hasLeak) {
                    badge.textContent = '⚠ 检测到泄露';
                    badge.style.background = '#7c2d12';
                    badge.style.color = '#fdba74';
                } else {
                    badge.textContent = '✓ 未发现泄露';
                    badge.style.background = '#14532d';
                    badge.style.color = '#86efac';
                }
                badge.style.display = 'inline';

                // Build table rows
                var dnsIdx = 0;
                entries.forEach(function(e) {
                    var isDNS = (e.type === 'dns');
                    dnsIdx += isDNS ? 1 : 0;

                    var typeTag = isDNS
                        ? E('span', { 'style': 'background:#1d4ed8; color:#bfdbfe; padding:2px 8px; border-radius:10px; font-size:11px;' }, ['DNS 解析器'])
                        : E('span', { 'style': 'background:#065f46; color:#a7f3d0; padding:2px 8px; border-radius:10px; font-size:11px;' }, ['出口 IP']);

                    var statusDot = isDNS
                        ? (hasLeak
                            ? E('span', { 'style': 'color:#f97316; margin-right:4px;' }, ['⚠'])
                            : E('span', { 'style': 'color:#22c55e; margin-right:4px;' }, ['✓']))
                        : null;

                    var ipCell = E('td', {
                        'style': 'padding:8px 10px; font-family:monospace; color:#e2e8f0;'
                    }, [ statusDot, document.createTextNode(e.ip || '-') ]);

                    var loc = [e.country_name, e.country_code]
                        .filter(Boolean).join(' ');

                    var tr = E('tr', {
                        'style': 'border-bottom:1px solid #1e293b;' + (isDNS && hasLeak ? ' background:rgba(239,68,68,0.07);' : '')
                    }, [
                        E('td', { 'style': 'padding:8px 10px; text-align:center; color:#64748b; font-size:12px;' }, [isDNS ? String(dnsIdx) : '—']),
                        ipCell,
                        E('td', { 'style': 'padding:8px 10px; color:#94a3b8;' }, [loc || '-']),
                        E('td', { 'style': 'padding:8px 10px; color:#94a3b8;' }, [e.isp || '-']),
                        E('td', { 'style': 'padding:8px 10px; text-align:center;' }, [typeTag])
                    ]);
                    tbody.appendChild(tr);
                });

                if (entries.length > 0) {
                    table.style.display = 'table';
                }

                // Summary banner
                if (summary) {
                    summaryEl.textContent = (hasLeak ? '⚠️  ' : 'ℹ️  ') + summary;
                    summaryEl.style.background = hasLeak ? '#450a0a' : '#0f172a';
                    summaryEl.style.color      = hasLeak ? '#fca5a5' : '#94a3b8';
                    summaryEl.style.border     = hasLeak ? '1px solid #7f1d1d' : '1px solid #334155';
                    summaryEl.style.display    = 'block';
                }
            })
            .catch(function(e) {
                btn.disabled = false;
                spinner.style.display = 'none';
                badge.textContent = '连接失败';
                badge.style.background = '#7f1d1d';
                badge.style.color = '#fca5a5';
                badge.style.display = 'inline';
                summaryEl.textContent = '无法连接到 ClashForge API: ' + String(e);
                summaryEl.style.background = '#450a0a';
                summaryEl.style.color = '#fca5a5';
                summaryEl.style.display = 'block';
            });
    },

    handleSave: null,
    handleSaveApply: null,
    handleReset: null
});

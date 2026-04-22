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

        var mapEl = E('div', { 'class': 'cbi-map' }, [
            E('h2', {}, ['ClashForge']),
            E('div', { 'class': 'cbi-map-tabbox' }, [ overviewTab, logTab ])
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

    handleSave: null,
    handleSaveApply: null,
    handleReset: null
});

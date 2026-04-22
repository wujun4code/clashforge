'use strict';
'require view';

return view.extend({
    render: function() {
        var port = 7777;
        var host = window.location.hostname;
        var url = 'http://' + host + ':' + port;

        return E('div', { 'class': 'cbi-map' }, [
            E('h2', {}, [ 'ClashForge' ]),
            E('div', { 'class': 'cbi-section' }, [
                E('p', {}, [
                    'ClashForge Web UI 运行在独立端口上。点击下方按钮打开：'
                ]),
                E('p', { 'style': 'margin: 16px 0;' }, [
                    E('a', {
                        'href': url,
                        'target': '_blank',
                        'rel': 'noopener',
                        'class': 'btn cbi-button cbi-button-apply',
                        'style': 'padding: 8px 20px; font-size: 16px;'
                    }, [ '🚀 打开 ClashForge (' + url + ')' ])
                ]),
                E('p', { 'style': 'color: #666; font-size: 13px;' }, [
                    '也可以直接在浏览器地址栏输入：',
                    E('code', {
                        'style': 'background:#f0f0f0; padding:2px 6px; border-radius:3px;'
                    }, [ url ])
                ])
            ])
        ]);
    },

    handleSave: null,
    handleSaveApply: null,
    handleReset: null
});

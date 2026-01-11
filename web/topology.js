(() => {
        const channelLabelPriorities = [
                { test: label => /^clientout/i.test(label), score: 5 },
                { test: label => /^clientin/i.test(label), score: 4 },
                { test: label => /^serverout/i.test(label), score: 3 },
                { test: label => /^serverin/i.test(label), score: 2 },
                { test: label => /^reg/i.test(label), score: 1 },
                { test: label => /^join/i.test(label), score: 0 }
        ];
        function stripGoroutineSuffix(key) {
                if (!key) return key;
                const idx = key.indexOf('#g');
                if (idx >= 0 && idx + 2 < key.length) {
                        const tail = key.slice(idx + 2);
                        if (/^\d+$/.test(tail)) return key.slice(0, idx);
                }
                return key;
        }
        function stripSuffix(key) {
                return stripGoroutineSuffix(key);
        }
        function normalizeChannelKey(key) {
                if (!key) return '';
                const trimmed = String(key).trim();
                if (!trimmed) return '';
                return stripGoroutineSuffix(trimmed);
        }
        function normalizePointer(ptr) {
                if (!ptr) return '';
                let v = String(ptr).trim();
                if (!v) return '';
                if (v.startsWith('ptr=')) {
                        v = v.slice(4).trim();
                        if (!v) return '';
                }
                return v;
        }
        function channelIdentityForEvent(evt) {
                if (!evt) return '';
                const ptr = normalizePointer(evt.ch_ptr || evt.ChPtr || evt.chPtr);
                if (ptr) return ptr;
                return normalizeChannelKey(evt.channel_key || evt.ChannelKey || evt.channel || evt.Channel);
        }
        function isPairedChannelEvent(evt) {
                if (!evt || typeof evt.event !== 'string') return false;
                const name = evt.event.toLowerCase();
                if (name !== 'chan_send' && name !== 'chan_recv') return false;
                const src = String(evt.source || evt.Source || '').toLowerCase();
                return src === 'paired';
        }
        function normalizeChannelId(chId) {
                if (!chId) return 'ch:chan';
                return chId.startsWith('ch:') ? chId : `ch:${chId}`;
        }
        function channelPointerFromId(chId) {
                if (!chId) return null;
                if (chId.startsWith('ch:0x')) return chId.slice(3);
                return null;
        }
        function shortPointer(ptr) {
                if (!ptr) return '';
                const clean = ptr.startsWith('0x') ? ptr.slice(2) : ptr;
                return clean.slice(-4);
        }
        function channelLabelForEvent(e) {
                if (!e) return 'chan';
                return e.channel || e.channel_key || e.ch_ptr || 'chan';
        }
        function channelNodeId(e) {
                const key = channelIdentityForEvent(e);
                if (key) return `ch:${key}`;
                return `ch:${channelLabelForEvent(e) || 'chan'}`;
        }
        window.GTVTopology = {
                stripSuffix,
                normalizeChannelKey,
                normalizePointer,
                channelIdentityForEvent,
                isPairedChannelEvent,
                normalizeChannelId,
                channelPointerFromId,
                shortPointer,
                channelLabelForEvent,
                channelNodeId
        };
})();

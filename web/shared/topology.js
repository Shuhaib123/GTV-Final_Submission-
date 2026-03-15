(() => {
        const channelLabelPriorities = [
                { test: label => /^clientout/i.test(label), score: 5 },
                { test: label => /^clientin/i.test(label), score: 4 },
                { test: label => /^serverout/i.test(label), score: 3 },
                { test: label => /^serverin/i.test(label), score: 2 },
                { test: label => /^reg/i.test(label), score: 1 },
                { test: label => /^join/i.test(label), score: 0 }
        ];
        function isPointerLike(label) {
                if (!label) return false;
                const v = String(label).trim();
                return /^ptr=0x/i.test(v) || /^0x[0-9a-f]+$/i.test(v) || /^[0-9a-f]{4}$/i.test(v);
        }
        function isAliasLabel(label) {
                if (!label) return false;
                const v = String(label).trim().toLowerCase();
                return /^ch#\d+$/.test(v) || v.startsWith('unknown:');
        }
        function isHumanLabel(label) {
                if (!label) return false;
                return !isPointerLike(label) && !isAliasLabel(label);
        }
        function labelScore(label) {
                if (!label) return 0;
                for (const { test, score } of channelLabelPriorities) {
                        if (test(label)) return score;
                }
                return 0;
        }
        function pickChannelLabel(labels, fallbackKey) {
                const list = Array.isArray(labels) ? labels : [];
                const unique = [...new Set(list.map(l => String(l || '').trim()).filter(Boolean))];
                const human = unique.filter(isHumanLabel);
                const aliases = unique.filter(l => !isHumanLabel(l) && !isPointerLike(l));
                const pointers = unique.filter(isPointerLike);
                const choose = (arr) => {
                        const sorted = [...arr].sort((a, b) => {
                                const sa = labelScore(a);
                                const sb = labelScore(b);
                                if (sa !== sb) return sb - sa;
                                if (a.length !== b.length) return a.length - b.length;
                                return a.localeCompare(b);
                        });
                        return sorted[0];
                };
                if (human.length) return choose(human);
                if (aliases.length) return choose(aliases);
                if (pointers.length) return normalizePointer(pointers[0]);
                if (fallbackKey) return String(fallbackKey).trim();
                return 'chan';
        }
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
        function splitChannelsEnabled() {
                try { return localStorage.getItem('gtv.split') === '1'; } catch { return false; }
        }
        function normalizeChannelKey(key) {
                if (!key) return '';
                const trimmed = String(key).trim();
                if (!trimmed) return '';
                if (splitChannelsEnabled()) return trimmed;
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
                const key = normalizeChannelKey(evt.channel_key || evt.ChannelKey || evt.channel || evt.Channel);
                const ptr = normalizePointer(evt.ch_ptr || evt.ChPtr || evt.chPtr);
                if (splitChannelsEnabled() && key) return key;
                if (ptr) return ptr;
                if (key) return key;
                return '';
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
                const candidates = [e.channel, e.channel_key, e.ch_ptr].filter(Boolean);
                return pickChannelLabel(candidates, e.channel_key || e.channel || 'chan');
        }
        function channelNodeId(e) {
                const key = channelIdentityForEvent(e);
                if (key) return `ch:${key}`;
                return `ch:${channelLabelForEvent(e) || 'chan'}`;
        }
        window.GTVTopology = {
                stripSuffix,
                splitChannelsEnabled,
                normalizeChannelKey,
                normalizePointer,
                isPointerLike,
                isAliasLabel,
                isHumanLabel,
                pickChannelLabel,
                channelIdentityForEvent,
                isPairedChannelEvent,
                normalizeChannelId,
                channelPointerFromId,
                shortPointer,
                channelLabelForEvent,
                channelNodeId
        };
})();

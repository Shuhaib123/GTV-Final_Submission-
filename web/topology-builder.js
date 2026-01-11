(() => {
        if (!window.GTVTopology) {
                console.warn('GTVTopologyBuilder requires GTVTopology; loading order may need adjusting');
                window.GTVTopology = window.GTVTopology || {};
        }

        const canonSend = 'chan_send';
        const canonRecv = 'chan_recv';
        const commitSend = 'chan_send_commit';
        const commitRecv = 'chan_recv_commit';

        function isCommitEvent(evt) {
                if (!evt || typeof evt.event !== 'string') return false;
                const name = evt.event.toLowerCase();
                return name === commitSend || name === commitRecv;
        }

        function topologyEventMeta(evt) {
                if (!evt || typeof evt.event !== 'string') return null;
                const name = evt.event.toLowerCase();
                let allow = false;
                if (name === canonSend || name === canonRecv) {
                        const src = String(evt.source || evt.Source || '').toLowerCase();
                        allow = src === 'paired';
                } else if (name === commitSend || name === commitRecv) {
                        allow = true;
                }
                if (!allow) return null;
                const identity = GTVTopology.channelIdentityForEvent(evt);
                if (!identity) return null;
                return {
                        role: name.includes('send') ? 'send' : 'recv',
                        identity,
                        eventName: name,
                        chLabel: evt.channel || evt.channel_key || identity,
                };
        }

        function normalizeGoroutineRole(role) {
                if (!role) return '';
                return String(role);
        }

        function pushLink(links, source, target, kind, evt, seq) {
                const meta = {
                        time_ms: evt?.time_ms ?? 0,
                        value: evt?.value,
                        event: evt?.event,
                        channel: evt?.channel,
                        channel_key: evt?.channel_key,
                        ch_ptr: evt?.ch_ptr,
                        msg_id: evt?.msg_id,
                        peer_g: evt?.peer_g,
                        seq,
                        src: evt?.source || evt?.Source
                };
                links.push({
                        id: `${source}->${target}#${kind}#${meta.time_ms}`,
                        source,
                        target,
                        kind,
                        meta
                });
        }

        let topologyStatsLogged = false;

        function buildTopology(events) {
                const ordered = Array.isArray(events) ? [...events] : [];
                ordered.sort((a, b) => {
                        const ta = a?.time_ms ?? 0;
                        const tb = b?.time_ms ?? 0;
                        return ta - tb;
                });

                const goroutines = new Map();
                const channels = new Map();
                const depthByKey = new Map();
                let mainG = null;

                for (const evt of ordered) {
                        if (evt && typeof evt.g === 'number') {
                                const current = goroutines.get(evt.g) || {};
                                const role = normalizeGoroutineRole(evt.role) || current.role || '';
                                const func = evt.func || current.func;
                                goroutines.set(evt.g, {
                                        id: `g${evt.g}`,
                                        label: `g${evt.g}`,
                                        role: role || current.role || '',
                                        func: func,
                                });
                                if (!mainG && role && role.toLowerCase() === 'main') {
                                        mainG = evt.g;
                                }
                        }

                        if (evt && typeof evt.event === 'string') {
                                const lower = evt.event.toLowerCase();
                                if (lower === 'chan_depth') {
                                        const key = GTVTopology.channelIdentityForEvent(evt);
                                        if (key) {
                                                depthByKey.set(key, evt.value);
                                        }
                                }
                        }

                        const chKey = GTVTopology.channelIdentityForEvent(evt);
                        if (chKey) {
                                const id = `ch:${chKey}`;
                                const info = channels.get(chKey) || {
                                        key: chKey,
                                        id,
                                        ptr: chKey && chKey.startsWith('ptr=') ? chKey : undefined,
                                        cap: undefined,
                                        elem_type: undefined,
                                        labelCounts: new Map(),
                                };
                                if (!info.ptr && evt && evt.ch_ptr) {
                                        info.ptr = evt.ch_ptr;
                                }
                                if (evt && evt.event === 'chan_make') {
                                        if (evt.cap != null) info.cap = evt.cap;
                                        if (evt.elem_type) info.elem_type = evt.elem_type;
                                }
                                const labels = new Set();
                                if (evt?.channel) labels.add(evt.channel);
                                if (evt?.channel_key) labels.add(evt.channel_key);
                                const chanLabel = evt?.channel || evt?.channel_key || evt?.ch_ptr;
                                if (chanLabel) labels.add(chanLabel);
                                labels.forEach(label => {
                                        if (!label) return;
                                        info.labelCounts.set(label, (info.labelCounts.get(label) || 0) + 1);
                                });
                                channels.set(chKey, info);
                        }
                }

                const pairedEvents = ordered.filter(e => GTVTopology.isPairedChannelEvent(e));
                const commitEvents = ordered.filter(e => isCommitEvent(e));
                const topologyEvents = [];
                const seenKeys = new Set();
                function pushTopologyEvent(evt) {
                        const info = topologyEventMeta(evt);
                        if (!info) return;
                        const dedupKey = `${info.eventName}:${evt?.role || ''}:${evt?.g || ''}:${info.identity}:${evt?.peer_g || ''}:${evt?.pair_id || evt?.pairID || ''}`;
                        if (seenKeys.has(dedupKey)) return;
                        seenKeys.add(dedupKey);
                        topologyEvents.push(evt);
                }
                pairedEvents.forEach(pushTopologyEvent);
                commitEvents.forEach(pushTopologyEvent);


                const links = [];
                const drawnKeys = new Set();
                const chanSeq = new Map();
                function nextSeq(key) {
                        const next = (chanSeq.get(key) || 0) + 1;
                        chanSeq.set(key, next);
                        return next;
                }

                for (const evt of topologyEvents) {
                        const info = topologyEventMeta(evt);
                        if (!info) continue;
                        if (typeof evt.g !== 'number') continue;
                        const msgId = evt?.msg_id ?? evt?.msgId ?? evt?.MsgID ?? '';
                        const pairId = evt?.pair_id ?? evt?.pairID ?? evt?.PairID ?? '';
                        const msgKey = msgId || pairId || '';
                        const dedupKey = `${info.role}:${evt.g}:${info.identity}:${msgKey}`;
                        if (drawnKeys.has(dedupKey)) continue;
                        drawnKeys.add(dedupKey);
                        const chId = `ch:${info.identity}`;
                        const seq = nextSeq(info.identity);
                        if (info.role === 'send') {
                                pushLink(links, `g${evt.g}`, chId, 'send', evt, seq);
                        } else {
                                pushLink(links, chId, `g${evt.g}`, 'recv', evt, seq);
                        }
                }

                try {
                        const mainNodeId = mainG ? `g${mainG}` : null;
                        for (const evt of ordered) {
                                if (!evt || evt.event !== 'go_start') continue;
                                const child = `g${evt.g}`;
                                let parent = null;
                                if (evt.parent_g && evt.parent_g > 0) {
                                        parent = `g${evt.parent_g}`;
                                } else if (mainNodeId && mainNodeId !== child) {
                                        parent = mainNodeId;
                                }
                                if (!parent) continue;
                                links.push({
                                        id: `${parent}->${child}#spawn#${evt.time_ms ?? 0}`,
                                        source: parent,
                                        target: child,
                                        kind: 'spawn',
                                        meta: { time_ms: evt.time_ms ?? 0, event: 'spawn', role: evt.role },
                                });
                        }
                } catch (err) {
                        console.warn('Topology builder spawn edge error', err);
                }

                return { goroutines, channels, depthByKey, links };
        }

        window.GTVTopologyBuilder = {
                topologyEventMeta,
                buildTopology,
        };
})();

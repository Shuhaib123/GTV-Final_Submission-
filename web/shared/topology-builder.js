(() => {
        if (!window.GTVTopology) {
                console.warn('GTVTopologyBuilder requires GTVTopology; loading order may need adjusting');
                window.GTVTopology = window.GTVTopology || {};
        }

        const canonSend = 'chan_send';
        const canonRecv = 'chan_recv';
        const condWait = 'cond_wait';
        const condSignal = 'cond_signal';
        const condBroadcast = 'cond_broadcast';
        const commitSend = 'chan_send_commit';
        const commitRecv = 'chan_recv_commit';
        const completeSend = 'send_complete';
        const completeRecv = 'recv_complete';
        const syncEventDirections = {
                mutex_lock: 'g_to_resource',
                mutex_unlock: 'resource_to_g',
                rwmutex_lock: 'g_to_resource',
                rwmutex_unlock: 'resource_to_g',
                rwmutex_rlock: 'g_to_resource',
                rwmutex_runlock: 'resource_to_g',
                wg_add: 'resource_to_g',
                wg_done: 'resource_to_g',
                wg_wait: 'g_to_resource',
                cond_wait: 'g_to_resource',
                cond_signal: 'resource_to_g',
                cond_broadcast: 'resource_to_g',
        };
        const channelEventDirections = {
                [canonSend]: 'g_to_resource',
                [commitSend]: 'g_to_resource',
                [completeSend]: 'g_to_resource',
                [canonRecv]: 'resource_to_g',
                [commitRecv]: 'resource_to_g',
                [completeRecv]: 'resource_to_g',
        };
        const syncEventKinds = {
                mutex_lock: 'sync-lock',
                mutex_unlock: 'sync-unlock',
                rwmutex_lock: 'sync-lock',
                rwmutex_rlock: 'sync-lock',
                rwmutex_unlock: 'sync-unlock',
                rwmutex_runlock: 'sync-unlock',
                wg_add: 'sync-wg-add',
                wg_done: 'sync-wg-done',
                wg_wait: 'sync-wg-wait',
                cond_wait: 'sync-cond-wait',
                cond_signal: 'sync-cond-signal',
                cond_broadcast: 'sync-cond-broadcast',
        };
        function isSyncEventName(name) {
                return !!syncEventKinds[name];
        }

        function isCommitEvent(evt) {
                if (!evt || typeof evt.event !== 'string') return false;
                const name = String(evt.event).trim().toLowerCase();
                return name === commitSend || name === commitRecv;
        }

        function directionForEventName(name) {
                if (syncEventDirections[name]) return syncEventDirections[name];
                if (channelEventDirections[name]) return channelEventDirections[name];
                if (name.includes('send')) return 'g_to_resource';
                if (name.includes('recv') || name.includes('receive')) return 'resource_to_g';
                return 'g_to_resource';
        }

        function roleForDirection(direction, layer) {
                // Backward compatibility for legacy render paths: only channel edges carry send/recv role.
                if (layer !== 'channel') return '';
                return direction === 'resource_to_g' ? 'recv' : 'send';
        }

        function linkKindForEventName(name) {
                if (syncEventKinds[name]) return syncEventKinds[name];
                if (name === canonSend || name === commitSend || name === completeSend) return 'send';
                if (name === canonRecv || name === commitRecv || name === completeRecv) return 'recv';
                return directionForEventName(name) === 'resource_to_g' ? 'recv' : 'send';
        }

        function linkLayerForKind(kind) {
                const lower = String(kind || '').toLowerCase();
                if (lower === 'spawn' || lower === 'spawn-inferred') return 'spawn';
                if (lower.startsWith('sync-')) return 'sync';
                return 'channel';
        }

        function eventUniqueToken(evt, fallback = 0) {
                const bits = [];
                if (evt?.seq != null) bits.push(`seq:${evt.seq}`);
                if (evt?.time_ns != null) bits.push(`tn:${evt.time_ns}`);
                else if (evt?.time_ms != null) bits.push(`tm:${evt.time_ms}`);
                const msgId = evt?.msg_id ?? evt?.msgId ?? evt?.MsgID;
                const pairId = evt?.pair_id ?? evt?.pairID ?? evt?.PairID;
                if (msgId != null && msgId !== '') bits.push(`msg:${msgId}`);
                if (pairId != null && pairId !== '') bits.push(`pair:${pairId}`);
                if (evt?.g != null) bits.push(`g:${evt.g}`);
                if (evt?.peer_g != null) bits.push(`peer:${evt.peer_g}`);
                if (!bits.length) bits.push(`idx:${fallback}`);
                return bits.join('|');
        }

        function topologyEventMeta(evt) {
                if (!evt || typeof evt.event !== 'string') return null;
                const name = String(evt.event).trim().toLowerCase();
                let allow = false;
                let unknown = false;
                if (name === canonSend || name === canonRecv) {
                        const src = String(evt.source || evt.Source || '').toLowerCase();
                        allow = true;
                        unknown = src !== 'paired';
                } else if (name === condWait || name === condSignal || name === condBroadcast) {
                        allow = true;
                        unknown = true;
                } else if (isSyncEventName(name)) {
                        allow = true;
                } else if (name === commitSend || name === commitRecv) {
                        allow = true;
                } else if (name === completeSend || name === completeRecv) {
                        allow = true;
                }
                if (!allow) return null;
                let identity = GTVTopology.channelIdentityForEvent(evt) || (evt?.channel || evt?.channel_key || '');
                if (!identity) {
                        const seq = evt?.seq ?? 0;
                        const g = typeof evt?.g === 'number' ? evt.g : 0;
                        identity = `unknown:${g}:${seq}`;
                }
                const linkKind = linkKindForEventName(name);
                const layer = linkLayerForKind(linkKind);
                const direction = directionForEventName(name);
                const role = roleForDirection(direction, layer);
                return {
                        role,
                        linkKind,
                        layer,
                        direction,
                        identity,
                        eventName: name,
                        chLabel: GTVTopology.channelLabelForEvent(evt) || identity,
                        unknown: unknown || !GTVTopology.channelIdentityForEvent(evt),
                };
        }

        function topologyEventMetaWithReason(evt) {
                if (!evt || typeof evt.event !== 'string') return { info: null, reason: 'missing_event' };
                const name = String(evt.event).trim().toLowerCase();
                let allow = false;
                let reason = 'not_allowed';
                if (name === canonSend || name === canonRecv) {
                        allow = true;
                } else if (name === condWait || name === condSignal || name === condBroadcast) {
                        allow = true;
                } else if (isSyncEventName(name)) {
                        allow = true;
                } else if (name === commitSend || name === commitRecv || name === completeSend || name === completeRecv) {
                        allow = true;
                }
                if (!allow) return { info: null, reason };
                const identity = GTVTopology.channelIdentityForEvent(evt) || evt?.channel || evt?.channel_key;
                if (!identity) return { info: topologyEventMeta(evt), reason: 'missing_identity' };
                return { info: topologyEventMeta(evt), reason: '' };
        }

        function normalizeGoroutineRole(role) {
                if (!role) return '';
                return String(role);
        }
        function shortFuncName(fn) {
                if (!fn) return '';
                const parts = String(fn).split('/');
                const tail = parts[parts.length - 1] || fn;
                const segs = tail.split('.');
                if (segs.length >= 2) return segs.slice(-2).join('.');
                return tail;
        }

        function pushLink(links, source, target, kind, evt, seq, options = {}) {
                const uid = options.uniqueID || eventUniqueToken(evt, seq);
                const layer = options.layer || linkLayerForKind(kind);
                const direction = options.direction || '';
                const meta = {
                        time_ms: evt?.time_ms ?? 0,
                        time_ns: evt?.time_ns,
                        value: evt?.value,
                        event: evt?.event,
                        channel: evt?.channel,
                        channel_key: evt?.channel_key,
                        ch_ptr: evt?.ch_ptr,
                        msg_id: evt?.msg_id,
                        peer_g: evt?.peer_g,
                        unknown_peer: options.unknownPeer || false,
                        partial_pairing: options.partial || false,
                        seq,
                        src: evt?.source || evt?.Source,
                        role: options.role || '',
                        direction,
                        layer,
                        uid,
                };
                links.push({
                        id: `${source}->${target}#${kind}#${uid}`,
                        source,
                        target,
                        direction,
                        layer,
                        kind,
                        partial: options.partial || false,
                        meta
                });
        }

        let topologyStatsLogged = false;

        function buildTopology(input) {
                const ordered = Array.isArray(input) ? [...input] : ([...(input?.events || [])]);
                const entities = Array.isArray(input) ? null : (input && input.entities) || null;
                ordered.sort((a, b) => {
                        const ta = a?.time_ns ?? 0;
                        const tb = b?.time_ns ?? 0;
                        if (ta && tb && ta !== tb) return ta - tb;
                        const sa = a?.seq ?? 0;
                        const sb = b?.seq ?? 0;
                        if (sa !== sb) return sa - sb;
                        const ma = a?.time_ms ?? 0;
                        const mb = b?.time_ms ?? 0;
                        if (ma !== mb) return ma - mb;
                        return 0;
                });

                const stats = {
                        total: ordered.length,
                        dropped: 0,
                        by_reason: {},
                };
                const goroutines = new Map();
                const channels = new Map();
                const depthByKey = new Map();
                let mainG = null;

                if (entities && Array.isArray(entities.goroutines)) {
                        for (const g of entities.goroutines) {
                                if (!g || typeof g.gid !== 'number') continue;
                                const shortFn = shortFuncName(g.func);
                                goroutines.set(g.gid, {
                                        id: `g${g.gid}`,
                                        label: shortFn || `g${g.gid}`,
                                        role: g.role || '',
                                        func: g.func,
                                });
                                if (!mainG && (g.role || '').toLowerCase() === 'main') {
                                        mainG = g.gid;
                                }
                        }
                }
                if (entities && Array.isArray(entities.channels)) {
                        for (const ch of entities.channels) {
                                if (!ch || !ch.chan_id) continue;
                                const key = ch.chan_id;
                                channels.set(key, {
                                        key,
                                        id: `ch:${key}`,
                                        ptr: ch.ch_ptr || undefined,
                                        cap: ch.cap,
                                        elem_type: ch.elem_type,
                                        labelCounts: new Map(ch.name ? [[ch.name, 1]] : []),
                                });
                        }
                }

                for (const evt of ordered) {
                        if (evt && typeof evt.g === 'number') {
                                const current = goroutines.get(evt.g) || {};
                                const role = normalizeGoroutineRole(evt.role) || current.role || '';
                                const func = evt.func || current.func;
                                const shortFn = shortFuncName(func);
                                goroutines.set(evt.g, {
                                        id: `g${evt.g}`,
                                        label: shortFn || `g${evt.g}`,
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

                        const chKey = GTVTopology.channelIdentityForEvent(evt) || (evt?.channel || evt?.channel_key || '');
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
                        if (evt?.ch_ptr) labels.add(evt.ch_ptr);
                        labels.forEach(label => {
                                if (!label) return;
                                info.labelCounts.set(label, (info.labelCounts.get(label) || 0) + 1);
                        });
                                channels.set(chKey, info);
                        }
                }

                const pairedEvents = ordered.filter(e => {
                        const name = String(e?.event || '').trim().toLowerCase();
                        return name === canonSend || name === canonRecv;
                });
                const pairedChannelEvents = pairedEvents.filter(e => GTVTopology.isPairedChannelEvent(e));
                const commitEvents = ordered.filter(e => isCommitEvent(e));
                const completeEvents = ordered.filter(e => {
                        const name = String(e?.event || '').trim().toLowerCase();
                        return name === completeSend || name === completeRecv;
                });
                const syncEvents = ordered.filter(e => {
                        const name = String(e?.event || '').trim().toLowerCase();
                        return isSyncEventName(name) || name === condWait || name === condSignal || name === condBroadcast;
                });
                const topologyEvents = [];
                const seenKeys = new Set();
                function pushTopologyEvent(evt) {
                        const result = topologyEventMetaWithReason(evt);
                        if (!result.info) {
                                stats.dropped++;
                                stats.by_reason[result.reason] = (stats.by_reason[result.reason] || 0) + 1;
                                return;
                        }
                        const info = result.info;
                        // Trace rows should remain distinct unless they are literal duplicates.
                        // Using seq/time + ids prevents collapsing separate operations that share endpoints.
                        const seqKey = evt?.seq != null ? `seq:${evt.seq}` : '';
                        const timeKey = evt?.time_ns != null
                                ? `tn:${evt.time_ns}`
                                : (evt?.time_ms != null ? `tm:${evt.time_ms}` : '');
                        const msgKey = evt?.msg_id ?? evt?.msgId ?? evt?.MsgID ?? '';
                        const pairKey = evt?.pair_id ?? evt?.pairID ?? evt?.PairID ?? '';
                        const dedupKey = `${info.eventName}:${evt?.role || ''}:${evt?.g || ''}:${info.identity}:${evt?.peer_g || ''}:${pairKey}:${msgKey}:${seqKey}:${timeKey}:${evt?.value ?? ''}`;
                        if (seenKeys.has(dedupKey)) return;
                        seenKeys.add(dedupKey);
                        topologyEvents.push(evt);
                }
                const pairedEndpointKeys = new Set();
                for (const evt of pairedChannelEvents) {
                        const info = topologyEventMeta(evt);
                        if (!info || typeof evt.g !== 'number') continue;
                        const idKey = info.identity || info.chLabel || 'chan';
                        const direction = info.direction || directionForEventName(info.eventName || '');
                        pairedEndpointKeys.add(`${direction}:${evt.g}:${idKey}`);
                }
                pairedEvents.forEach(pushTopologyEvent);
                syncEvents.forEach(pushTopologyEvent);
                commitEvents.forEach(pushTopologyEvent);
                completeEvents.forEach(pushTopologyEvent);


                const links = [];
                const drawnKeys = new Set();
                const chanSeq = new Map();
                function nextSeq(key) {
                        const next = (chanSeq.get(key) || 0) + 1;
                        chanSeq.set(key, next);
                        return next;
                }

                let fallbackSeq = 0;
                for (const evt of topologyEvents) {
                        const info = topologyEventMeta(evt);
                        if (!info) continue;
                        if (typeof evt.g !== 'number') continue;
                        const msgId = evt?.msg_id ?? evt?.msgId ?? evt?.MsgID ?? '';
                        const pairId = evt?.pair_id ?? evt?.pairID ?? evt?.PairID ?? '';
                        let msgKey = msgId || pairId || '';
                        if (!msgKey) {
                                if (evt?.seq != null) msgKey = `seq:${evt.seq}`;
                                else if (evt?.time_ns != null) msgKey = `tn:${evt.time_ns}`;
                                else if (evt?.time_ms != null) msgKey = `tm:${evt.time_ms}`;
                                else msgKey = `idx:${++fallbackSeq}`;
                        }
                        const idKey = info.identity || info.chLabel || 'chan';
                        const direction = info.direction || directionForEventName(info.eventName || '');
                        const dedupKey = `${info.linkKind || info.role}:${info.eventName}:${direction}:${evt.g}:${idKey}:${msgKey}`;
                        if (drawnKeys.has(dedupKey)) continue;
                        drawnKeys.add(dedupKey);
                        const chId = `ch:${idKey}`;
                        const endpointKey = `${direction}:${evt.g}:${idKey}`;
                        const isComplete = info.eventName === completeSend || info.eventName === completeRecv;
                        if (isComplete && pairedEndpointKeys.has(endpointKey)) continue;
                        const hasPeer = typeof evt?.peer_g === 'number' && evt.peer_g > 0;
                        const unknownEndpoint = !!(info.unknown || evt?.unknown_sender || evt?.unknown_receiver);
                        const seq = nextSeq(idKey);
                        const linkKind = info.linkKind || info.role;
                        const linkOptions = {
                                partial: isComplete,
                                unknownPeer: unknownEndpoint || (isComplete && !hasPeer),
                                role: info.role,
                                direction,
                                layer: linkLayerForKind(linkKind),
                                uniqueID: eventUniqueToken(evt, seq),
                        };
                        if (direction === 'g_to_resource') {
                                pushLink(links, `g${evt.g}`, chId, linkKind, evt, seq, linkOptions);
                        } else {
                                pushLink(links, chId, `g${evt.g}`, linkKind, evt, seq, linkOptions);
                        }
                }

                try {
                        const mainNodeId = mainG ? `g${mainG}` : null;
                        let spawnOrdinal = 0;
                        const isSpawnLikeEvent = (evt) => {
                                if (!evt || typeof evt.event !== 'string') return false;
                                const name = String(evt.event).toLowerCase();
                                return name === 'spawn'
                                        || name === 'go_start'
                                        || name === 'go_create'
                                        || name === 'goroutine_created'
                                        || name === 'goroutine_started';
                        };
                        const explicitSpawnPairs = new Set();
                        for (const evt of ordered) {
                                if (!isSpawnLikeEvent(evt)) continue;
                                if (typeof evt.g !== 'number' || evt.g <= 0) continue;
                                if (!(evt.parent_g && evt.parent_g > 0)) continue;
                                explicitSpawnPairs.add(`g${evt.parent_g}->g${evt.g}`);
                        }
                        const spawnSeen = new Set();
                        for (const evt of ordered) {
                                if (!isSpawnLikeEvent(evt)) continue;
                                if (typeof evt.g !== 'number' || evt.g <= 0) continue;
                                const child = `g${evt.g}`;
                                const hasParent = !!(evt.parent_g && evt.parent_g > 0);
                                let parent = hasParent ? `g${evt.parent_g}` : null;
                                let inferred = false;
                                if (!parent && mainNodeId && mainNodeId !== child) {
                                        parent = mainNodeId;
                                        inferred = true;
                                }
                                if (!parent) continue;
                                const explicitPairKey = `${parent}->${child}`;
                                if (inferred && explicitSpawnPairs.has(explicitPairKey)) continue;
                                const pairKey = `${inferred ? 'inferred' : 'explicit'}:${explicitPairKey}`;
                                if (spawnSeen.has(pairKey)) continue;
                                spawnSeen.add(pairKey);
                                const uid = eventUniqueToken(evt, ++spawnOrdinal);
                                const kind = inferred ? 'spawn-inferred' : 'spawn';
                                links.push({
                                        id: `${parent}->${child}#${kind}#${uid}`,
                                        source: parent,
                                        target: child,
                                        direction: 'parent_to_child',
                                        layer: 'spawn',
                                        kind,
                                        meta: {
                                                time_ms: evt.time_ms ?? 0,
                                                time_ns: evt.time_ns,
                                                event: inferred ? 'spawn_inferred' : 'spawn',
                                                role: evt.role,
                                                direction: 'parent_to_child',
                                                layer: 'spawn',
                                                seq: evt.seq,
                                                uid,
                                                inferred,
                                        },
                                });
                        }
                } catch (err) {
                        console.warn('Topology builder spawn edge error', err);
                }

                if (!topologyStatsLogged && stats.dropped > 0) {
                        topologyStatsLogged = true;
                        console.warn('[topology] dropped events', stats);
                }
                return { goroutines, channels, depthByKey, links, stats };
        }

        window.GTVTopologyBuilder = {
                topologyEventMeta,
                buildTopology,
        };
})();

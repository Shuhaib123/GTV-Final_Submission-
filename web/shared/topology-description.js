(() => {
  const SEND_EVENTS = new Set(["chan_send", "send_complete"]);
  const RECV_EVENTS = new Set(["chan_recv", "recv_complete"]);
  const GO_CREATE_EVENTS = new Set([
    "go_create",
    "goroutine_created",
    "go_start",
    "goroutine_started",
    "spawn",
  ]);
  const GO_END_EVENTS = new Set([
    "go_end",
    "goroutine_ended",
    "goroutine_end",
    "go_stop",
  ]);
  const CHAN_CLOSE_EVENTS = new Set(["chan_close"]);

  function asArray(v) {
    return Array.isArray(v) ? v : [];
  }

  function asObject(v) {
    return v && typeof v === "object" ? v : null;
  }

  function asNonEmptyString(v) {
    if (v == null) return "";
    return String(v).trim();
  }

  function asFiniteNumber(v) {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }

  function asInt(v) {
    const n = asFiniteNumber(v);
    return n == null ? null : Math.trunc(n);
  }

  function eventName(ev) {
    if (!ev || typeof ev !== "object") return "";
    return asNonEmptyString(ev.event || ev.Event).toLowerCase();
  }

  function normalizeType(name) {
    const n = asNonEmptyString(name).toLowerCase();
    if (!n) return "";
    if (n === "chan_make") return "chan_make";
    if (SEND_EVENTS.has(n)) return "chan_send";
    if (RECV_EVENTS.has(n)) return "chan_recv";
    if (GO_CREATE_EVENTS.has(n)) return "go_create";
    if (GO_END_EVENTS.has(n)) return "go_end";
    if (CHAN_CLOSE_EVENTS.has(n)) return "chan_close";
    return "";
  }

  function eventTimeNs(ev, fallbackOrder) {
    const ns = asFiniteNumber(ev?.time_ns ?? ev?.timeNs);
    if (ns != null) return Math.trunc(ns);
    const ms = asFiniteNumber(ev?.time_ms ?? ev?.timeMs ?? ev?.TimeMs ?? ev?.t);
    if (ms != null) return Math.trunc(ms * 1e6);
    return fallbackOrder;
  }

  function eventTimeMs(ev, timeNs) {
    const ms = asFiniteNumber(ev?.time_ms ?? ev?.timeMs ?? ev?.TimeMs ?? ev?.t);
    if (ms != null) return ms;
    return timeNs / 1e6;
  }

  function isPointerLike(label) {
    const v = asNonEmptyString(label).toLowerCase();
    if (!v) return false;
    return (
      v.startsWith("ptr=0x") ||
      /^0x[0-9a-f]+$/i.test(v) ||
      /^[0-9a-f]{4}$/i.test(v)
    );
  }

  function isAliasLike(label) {
    const v = asNonEmptyString(label).toLowerCase();
    if (!v) return false;
    return /^ch#\d+$/i.test(v) || v.startsWith("unknown:");
  }

  function isHumanLabel(label) {
    const v = asNonEmptyString(label);
    if (!v) return false;
    return !isPointerLike(v) && !isAliasLike(v);
  }

  function channelIDFromEvent(ev) {
    if (!ev || typeof ev !== "object") return "";
    const candidates = [
      ev.channel_key,
      ev.channelKey,
      ev.ChannelKey,
      ev.channel_id,
      ev.channelId,
      ev.ChannelID,
      ev.ch_ptr,
      ev.chPtr,
      ev.ChPtr,
      ev.channel,
      ev.Channel,
    ];
    for (const c of candidates) {
      const id = asNonEmptyString(c);
      if (id) return id;
    }
    return "";
  }

  function channelNameFromEvent(ev) {
    if (!ev || typeof ev !== "object") return "";
    const candidates = [
      ev.channel,
      ev.Channel,
      ev.channel_name,
      ev.channelName,
      ev.name,
    ];
    for (const c of candidates) {
      const label = asNonEmptyString(c);
      if (isHumanLabel(label)) return label;
    }
    return "";
  }

  function addAlias(set, value) {
    const v = asNonEmptyString(value);
    if (!v) return;
    set.add(v);
  }

  function stableSortNumbers(a, b) {
    return a - b;
  }

  function ensureGoroutine(goroutinesByID, gid, timeNs) {
    let g = goroutinesByID.get(gid);
    if (!g) {
      g = {
        gid,
        parent_gid: null,
        role: "",
        func: "",
        first_seen_time: timeNs,
      };
      goroutinesByID.set(gid, g);
    } else if (timeNs < g.first_seen_time) {
      g.first_seen_time = timeNs;
    }
    return g;
  }

  function ensureChannel(channelsByID, chanID, timeNs) {
    let ch = channelsByID.get(chanID);
    if (!ch) {
      ch = {
        chan_id: chanID,
        name: "",
        cap: null,
        elem_type: "",
        ch_ptr: "",
        aliases: new Set(),
        creator_gid: null,
        first_time: timeNs,
      };
      channelsByID.set(chanID, ch);
    } else if (timeNs < ch.first_time) {
      ch.first_time = timeNs;
    }
    return ch;
  }

  function addComm(commByChannel, chanID, gid, kind) {
    if (!chanID) return;
    let c = commByChannel.get(chanID);
    if (!c) {
      c = {
        chan_id: chanID,
        senders: new Set(),
        receivers: new Set(),
        send_count: 0,
        recv_count: 0,
      };
      commByChannel.set(chanID, c);
    }
    if (kind === "chan_send") {
      c.send_count += 1;
      if (gid != null) c.senders.add(gid);
    } else if (kind === "chan_recv") {
      c.recv_count += 1;
      if (gid != null) c.receivers.add(gid);
    }
  }

  function pickMainGoroutine(goroutines) {
    if (!goroutines.length) return null;
    const withMainRole = goroutines
      .filter((g) => asNonEmptyString(g.role).toLowerCase() === "main")
      .sort((a, b) => a.first_seen_time - b.first_seen_time || a.gid - b.gid);
    if (withMainRole.length) return withMainRole[0].gid;
    const roots = goroutines
      .filter((g) => g.parent_gid == null)
      .sort((a, b) => a.first_seen_time - b.first_seen_time || a.gid - b.gid);
    if (roots.length) return roots[0].gid;
    return goroutines
      .slice()
      .sort((a, b) => a.first_seen_time - b.first_seen_time || a.gid - b.gid)[0].gid;
  }

  function toRoleLabel(g) {
    const role = asNonEmptyString(g?.role);
    if (role) return role;
    return "worker";
  }

  function pluralizeRole(role, n) {
    if (n === 1) return role;
    if (role.endsWith("s")) return role;
    return `${role}s`;
  }

  function roleMapByGID(trace) {
    const map = new Map();
    asArray(trace?.goroutines).forEach((g) => {
      if (g && g.gid != null) map.set(g.gid, toRoleLabel(g));
    });
    return map;
  }

  function roleGroupLabel(gids, roleByGID, mainGID) {
    const uniq = Array.from(new Set(asArray(gids))).filter((g) => g != null).sort(stableSortNumbers);
    if (!uniq.length) return "unknown";
    const buckets = new Map();
    uniq.forEach((gid) => {
      let role = roleByGID.get(gid) || `g${gid}`;
      if (gid === mainGID) role = "main";
      if (!buckets.has(role)) buckets.set(role, []);
      buckets.get(role).push(gid);
    });
    const parts = Array.from(buckets.entries())
      .sort((a, b) => b[1].length - a[1].length || String(a[0]).localeCompare(String(b[0])))
      .map(([role, ids]) => {
        if (role === "main") return ids.length === 1 ? `main (g${ids[0]})` : `main goroutines (${ids.length})`;
        if (/^g\d+$/i.test(role)) return ids.map((gid) => `g${gid}`).join(",");
        if (ids.length === 1) return `${role} (g${ids[0]})`;
        return `${pluralizeRole(role, ids.length)} (${ids.length})`;
      });
    return parts.join(", ");
  }

  function channelMapByID(trace) {
    const map = new Map();
    asArray(trace?.channels).forEach((c) => {
      if (c && c.chan_id) map.set(c.chan_id, c);
    });
    return map;
  }

  function commScore(commEntry, mainGID) {
    const senders = asArray(commEntry?.senders);
    const receivers = asArray(commEntry?.receivers);
    const total = Number(commEntry?.send_count || 0) + Number(commEntry?.recv_count || 0);
    const hasMainSender = mainGID != null && senders.includes(mainGID);
    const hasMainReceiver = mainGID != null && receivers.includes(mainGID);
    const hasNonMainSender = senders.some((g) => g !== mainGID);
    const hasNonMainReceiver = receivers.some((g) => g !== mainGID);
    const mainBridge =
      (hasMainSender && hasNonMainReceiver) || (hasMainReceiver && hasNonMainSender);
    return total + (mainBridge ? 1000 : 0);
  }

  function selectImportantChannels(trace, options = {}) {
    const maxFlow = Number(options.maxFlow || 3);
    const mainGID = trace?.main_gid ?? null;
    const ranked = asArray(trace?.comm)
      .slice()
      .sort((a, b) => commScore(b, mainGID) - commScore(a, mainGID));
    return ranked.slice(0, Math.max(1, maxFlow));
  }

  function detectTopologyPatterns(trace) {
    const found = [];
    const channelByID = channelMapByID(trace);
    asArray(trace?.comm).forEach((flow) => {
      const channel = channelByID.get(flow.chan_id);
      const label = asNonEmptyString(channel?.name || flow.chan_id);
      const low = label.toLowerCase();
      const senders = asArray(flow.senders);
      const receivers = asArray(flow.receivers);
      const cap = channel?.cap;
      const elemType = asNonEmptyString(channel?.elem_type).toLowerCase();

      if (low.includes("join") && senders.length >= 2 && receivers.length === 1) {
        found.push({
          kind: "join_bus",
          channel: label,
          confidence: cap != null && cap > 1 ? "high" : "medium",
          sentence: `Detected handshake bus on ${label}: many workers register to one receiver.`,
        });
      }
      if (low.includes("join") && cap === 1 && elemType.includes("chan")) {
        found.push({
          kind: "per_worker_reply",
          channel: label,
          confidence: "high",
          sentence: `Detected per-worker reply channel shape on ${label} (cap=1, channel payload).`,
        });
      }
      if (senders.length >= 2 && receivers.length === 1) {
        found.push({
          kind: "fan_in",
          channel: label,
          confidence: senders.length >= 3 ? "high" : "medium",
          sentence: `${label} behaves as fan-in: ${senders.length} senders to one receiver.`,
        });
      }
      if (senders.length === 1 && receivers.length >= 2) {
        found.push({
          kind: "fan_out",
          channel: label,
          confidence: receivers.length >= 3 ? "high" : "medium",
          sentence: `${label} behaves as fan-out: one sender to ${receivers.length} receivers.`,
        });
      }
    });
    return found;
  }

  function detectPhases(trace, patterns) {
    const events = asArray(trace?.events);
    const channels = asArray(trace?.channels);
    const phaseLines = [];
    const chanMakeCount = events.filter((e) => e.type === "chan_make").length;
    const spawnCount = events.filter((e) => e.type === "go_create").length;
    const goEndCount = events.filter((e) => e.type === "go_end").length;
    const closeCount = events.filter((e) => e.type === "chan_close").length;

    if (channels.length || chanMakeCount) {
      const made = chanMakeCount || channels.length;
      phaseLines.push(`Setup: ${made} channel create events form the communication graph.`);
    }
    if (spawnCount) {
      phaseLines.push(`Spawn: ${spawnCount} goroutine creation events establish concurrency.`);
    }
    if (asArray(patterns).some((p) => p.kind === "join_bus" || p.kind === "per_worker_reply")) {
      phaseLines.push("Handshake: registration/reply pattern detected before steady-state traffic.");
    }
    const dominant = selectImportantChannels(trace, { maxFlow: 2 })
      .map((c) => {
        const ch = asArray(trace.channels).find((x) => x.chan_id === c.chan_id);
        return ch?.name || c.chan_id;
      })
      .filter(Boolean);
    if (dominant.length) {
      phaseLines.push(`Steady-state: dominant traffic flows through ${dominant.join(", ")}.`);
    }
    if (goEndCount || closeCount) {
      phaseLines.push(
        `Teardown: observed ${goEndCount} goroutine end events and ${closeCount} channel closes.`
      );
    } else {
      phaseLines.push("Teardown: trace ends without explicit shutdown events.");
    }
    return phaseLines;
  }

  function confidenceRank(level) {
    const raw = asNonEmptyString(level).toLowerCase();
    if (raw === "high") return 3;
    if (raw === "medium") return 2;
    if (raw === "low") return 1;
    return 0;
  }

  function summarizeConfidence(patterns) {
    const list = asArray(patterns).filter((p) => p && p.kind);
    if (!list.length) {
      return "No known patterns detected; summary is generic.";
    }
    const ranked = list
      .slice()
      .sort((a, b) => confidenceRank(b.confidence) - confidenceRank(a.confidence));
    const top = ranked[0];
    const kind = asNonEmptyString(top.kind).replace(/_/g, " ");
    return `Detected ${kind} (${asNonEmptyString(top.confidence) || "unknown"} confidence).`;
  }

  function summarizeTopology(traceInput, options = {}) {
    const trace = asObject(traceInput) || buildTopologyTrace(traceInput);
    const mode = asNonEmptyString(options.mode).toLowerCase() === "normal" ? "normal" : "compact";
    const lines = [];
    const goroutines = asArray(trace.goroutines);
    const channels = asArray(trace.channels);
    const mainG = trace.main_gid;
    const main = goroutines.find((g) => g.gid === mainG) || null;
    const mainRole = main ? toRoleLabel(main) : "main";
    const mainFunc = main && main.func ? ` ${main.func}` : "";
    lines.push(`Program begins in ${mainRole}${mainG != null ? ` (g${mainG})` : ""}${mainFunc}.`);

    const createdByMain = channels
      .filter((c) => c.creator_gid != null && c.creator_gid === mainG)
      .map((c) => c.name || c.chan_id);
    if (createdByMain.length) {
      lines.push(`Main creates channels: ${createdByMain.join(", ")}.`);
    } else if (channels.length) {
      lines.push(`Channels observed: ${channels.map((c) => c.name || c.chan_id).join(", ")}.`);
    }

    const groups = asArray(trace.main_spawn_groups);
    if (groups.length) {
      const parts = groups.map((g) => {
        const fn = g.func ? ` (${g.func})` : "";
        return `${g.count} ${g.role || "worker"}${fn}`;
      });
      lines.push(`Main spawns: ${parts.join("; ")}.`);
    }

    const roleByGID = roleMapByGID(trace);
    const important = selectImportantChannels(trace, { maxFlow: mode === "normal" ? 6 : 3 });
    if (mode === "normal" && important.length) {
      lines.push("Message flow:");
    }
    important.forEach((c) => {
      const channel = channels.find((x) => x.chan_id === c.chan_id);
      const name = (channel && (channel.name || channel.chan_id)) || c.chan_id;
      const senders = roleGroupLabel(c.senders, roleByGID, mainG);
      const receivers = roleGroupLabel(c.receivers, roleByGID, mainG);
      const flowLine = `${name}: ${senders} -> ${receivers} (${c.send_count} send, ${c.recv_count} recv).`;
      lines.push(mode === "normal" ? `- ${flowLine}` : flowLine);
    });

    const patterns = detectTopologyPatterns(trace);
    const phases = detectPhases(trace, patterns);
    if (mode === "normal") {
      lines.push(`Confidence: ${summarizeConfidence(patterns)}`);
    }
    const maxPatterns = mode === "normal" ? 4 : 1;
    patterns.slice(0, maxPatterns).forEach((p) => {
      lines.push(`[${p.confidence}] ${p.sentence}`);
    });
    const phaseCap = mode === "normal" ? 5 : 2;
    phases.slice(0, phaseCap).forEach((p) => lines.push(p));

    const cap = mode === "normal" ? 25 : 10;
    return lines.slice(0, cap).join("\n");
  }

  function buildTopologyTrace(input) {
    const envelope = Array.isArray(input) ? null : asObject(input);
    const eventsIn = Array.isArray(input) ? input : asArray(envelope?.events);
    const entities = asObject(envelope?.entities);

    const goroutinesByID = new Map();
    const channelsByID = new Map();
    const commByChannel = new Map();
    const normalizedEvents = [];

    eventsIn.forEach((raw, idx) => {
      if (!raw || typeof raw !== "object") return;
      const tns = eventTimeNs(raw, idx + 1);
      const tms = eventTimeMs(raw, tns);
      const name = eventName(raw);
      const type = normalizeType(name);
      const gid = asInt(raw.g ?? raw.gid ?? raw.G);
      const parentG = asInt(raw.parent_g ?? raw.parentG ?? raw.parent_gid ?? raw.parentGID);
      const role = asNonEmptyString(raw.role || raw.Role);
      const fn = asNonEmptyString(raw.func || raw.Func);
      const chanID = channelIDFromEvent(raw);
      const chanName = channelNameFromEvent(raw);
      const chPtr = asNonEmptyString(raw.ch_ptr ?? raw.chPtr ?? raw.ChPtr);
      const cap = asInt(raw.cap ?? raw.Cap);
      const elemType = asNonEmptyString(raw.elem_type ?? raw.elemType ?? raw.ElemType);

      if (gid != null && gid > 0) {
        const g = ensureGoroutine(goroutinesByID, gid, tns);
        if (parentG != null && parentG > 0) g.parent_gid = parentG;
        if (role && !g.role) g.role = role;
        if (fn && !g.func) g.func = fn;
      }

      if (chanID) {
        const ch = ensureChannel(channelsByID, chanID, tns);
        if (chanName && !ch.name) ch.name = chanName;
        if (chPtr && !ch.ch_ptr) ch.ch_ptr = chPtr;
        if (cap != null && ch.cap == null) ch.cap = cap;
        if (elemType && !ch.elem_type) ch.elem_type = elemType;
        if (gid != null && gid > 0 && type === "chan_make" && ch.creator_gid == null) {
          ch.creator_gid = gid;
        }
        addAlias(ch.aliases, chanID);
        addAlias(ch.aliases, raw.channel);
        addAlias(ch.aliases, raw.channel_key);
        addAlias(ch.aliases, raw.channel_id);
        addAlias(ch.aliases, raw.ch_ptr);
      }

      if (!type) return;
      const ev = {
        time_ns: tns,
        time_ms: tms,
        gid: gid != null && gid > 0 ? gid : null,
        type,
        chan_id: chanID || "",
        value: raw.value,
      };
      if (type === "go_create" && parentG != null && parentG > 0) {
        ev.parent_gid = parentG;
      }
      normalizedEvents.push(ev);
      if (type === "chan_send" || type === "chan_recv") {
        addComm(commByChannel, chanID, ev.gid, type);
      }
    });

    asArray(entities?.goroutines).forEach((raw) => {
      const gid = asInt(raw?.gid ?? raw?.g ?? raw?.G);
      if (gid == null || gid <= 0) return;
      const g = ensureGoroutine(goroutinesByID, gid, Number.MAX_SAFE_INTEGER);
      const role = asNonEmptyString(raw.role || raw.Role);
      const fn = asNonEmptyString(raw.func || raw.Func);
      const parentG = asInt(raw.parent_g ?? raw.parentG ?? raw.parent_gid);
      if (role && !g.role) g.role = role;
      if (fn && !g.func) g.func = fn;
      if (parentG != null && parentG > 0 && g.parent_gid == null) g.parent_gid = parentG;
      if (g.first_seen_time === Number.MAX_SAFE_INTEGER) g.first_seen_time = 0;
    });

    asArray(entities?.channels).forEach((raw) => {
      const chanID = asNonEmptyString(raw?.chan_id ?? raw?.chanID ?? raw?.id);
      if (!chanID) return;
      const ch = ensureChannel(channelsByID, chanID, Number.MAX_SAFE_INTEGER);
      const name = asNonEmptyString(raw?.name);
      const elemType = asNonEmptyString(raw?.elem_type ?? raw?.elemType);
      const cap = asInt(raw?.cap);
      const chPtr = asNonEmptyString(raw?.ch_ptr ?? raw?.chPtr);
      if (name && !ch.name) ch.name = name;
      if (elemType && !ch.elem_type) ch.elem_type = elemType;
      if (cap != null && ch.cap == null) ch.cap = cap;
      if (chPtr && !ch.ch_ptr) ch.ch_ptr = chPtr;
      addAlias(ch.aliases, chanID);
      addAlias(ch.aliases, name);
      addAlias(ch.aliases, chPtr);
      asArray(raw?.aliases).forEach((a) => addAlias(ch.aliases, a));
      if (ch.first_time === Number.MAX_SAFE_INTEGER) ch.first_time = 0;
    });

    const goroutines = Array.from(goroutinesByID.values()).sort(
      (a, b) => a.first_seen_time - b.first_seen_time || a.gid - b.gid
    );
    const channels = Array.from(channelsByID.values())
      .map((ch) => ({
        ...ch,
        aliases: Array.from(ch.aliases).sort(),
      }))
      .sort((a, b) => a.first_time - b.first_time || String(a.chan_id).localeCompare(String(b.chan_id)));

    const mainGID = pickMainGoroutine(goroutines);

    const spawnGraph = {};
    goroutines.forEach((g) => {
      if (g.parent_gid == null) return;
      const parentKey = String(g.parent_gid);
      if (!spawnGraph[parentKey]) spawnGraph[parentKey] = [];
      spawnGraph[parentKey].push(g.gid);
    });
    Object.keys(spawnGraph).forEach((k) => {
      spawnGraph[k].sort(stableSortNumbers);
    });

    const mainChildren = mainGID != null ? (spawnGraph[String(mainGID)] || []) : [];
    const grouped = new Map();
    mainChildren.forEach((gid) => {
      const g = goroutinesByID.get(gid);
      if (!g) return;
      const key = `${toRoleLabel(g)}|${asNonEmptyString(g.func)}`;
      let rec = grouped.get(key);
      if (!rec) {
        rec = {
          role: toRoleLabel(g),
          func: asNonEmptyString(g.func),
          count: 0,
          gids: [],
        };
        grouped.set(key, rec);
      }
      rec.count += 1;
      rec.gids.push(gid);
    });
    const mainSpawnGroups = Array.from(grouped.values())
      .map((g) => ({ ...g, gids: g.gids.sort(stableSortNumbers) }))
      .sort((a, b) => b.count - a.count || a.role.localeCompare(b.role) || a.func.localeCompare(b.func));

    const comm = Array.from(commByChannel.values())
      .map((c) => ({
        chan_id: c.chan_id,
        senders: Array.from(c.senders).sort(stableSortNumbers),
        receivers: Array.from(c.receivers).sort(stableSortNumbers),
        send_count: c.send_count,
        recv_count: c.recv_count,
      }))
      .sort((a, b) => (b.send_count + b.recv_count) - (a.send_count + a.recv_count));

    normalizedEvents.sort((a, b) => a.time_ns - b.time_ns);

    return {
      schema_version: 1,
      main_gid: mainGID,
      goroutines,
      channels,
      events: normalizedEvents,
      spawn_graph: spawnGraph,
      main_spawn_groups: mainSpawnGroups,
      comm,
    };
  }

  window.GTVTopologyDescription = {
    buildTopologyTrace,
    selectImportantChannels,
    detectTopologyPatterns,
    detectPhases,
    summarizeTopology,
  };
})();

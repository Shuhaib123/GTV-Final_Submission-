import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");

function loadBrowserScripts() {
  const localStorageData = new Map();
  const localStorage = {
    getItem(key) {
      return localStorageData.has(key) ? localStorageData.get(key) : null;
    },
    setItem(key, value) {
      localStorageData.set(String(key), String(value));
    },
    removeItem(key) {
      localStorageData.delete(String(key));
    },
  };
  const window = { localStorage };
  const context = { window, localStorage, console };
  vm.createContext(context);
  for (const file of ["topology.js", "topology-builder.js", "viewer-layer-utils.js"]) {
    const code = fs.readFileSync(path.join(__dirname, file), "utf8");
    vm.runInContext(code, context, { filename: file });
    if (file === "topology.js") context.GTVTopology = context.window.GTVTopology;
  }
  return {
    topologyBuilder: context.window.GTVTopologyBuilder,
    layerUtil: context.window.GTVViewerLayer,
  };
}

function assertPageHooks() {
  const offlinePath = path.join(repoRoot, "web/pages/graph/graph.html");
  const livePath = path.join(repoRoot, "web/pages/graph-live/graph-live.html");
  const offline = fs.readFileSync(offlinePath, "utf8");
  const live = fs.readFileSync(livePath, "utf8");

  assert.match(
    offline,
    /viewer-layer-utils\.js/,
    "offline viewer should import shared layer utility",
  );
  assert.match(
    live,
    /viewer-layer-utils\.js/,
    "live viewer should import shared layer utility",
  );
  assert.match(offline, /id="toggleSyncOnly"/, "offline Sync only toggle missing");
  assert.match(live, /id="toggleSyncOnlyLive"/, "live Sync only toggle missing");
  assert.match(offline, /id="fltLayerSync"/, "offline layer filter missing");
  assert.match(live, /id="fltLayerSyncLive"/, "live layer filter missing");
}

function fakeClassList() {
  const set = new Set();
  return {
    add(...names) {
      names.forEach((n) => set.add(String(n)));
    },
    remove(...names) {
      names.forEach((n) => set.delete(String(n)));
    },
    toggle(name, force) {
      const key = String(name);
      if (force === undefined) {
        if (set.has(key)) {
          set.delete(key);
          return false;
        }
        set.add(key);
        return true;
      }
      if (force) set.add(key);
      else set.delete(key);
      return !!force;
    },
    contains(name) {
      return set.has(String(name));
    },
  };
}

function mkEdgeLike(link, mode) {
  const pathEl = { classList: fakeClassList() };
  const labelEl = { classList: fakeClassList() };
  if (mode === "live") {
    return {
      layer: link.layer,
      kind: link.kind,
      displayKind: link.kind,
      metaEvent: link.meta?.event || "",
      path: pathEl,
      label: labelEl,
      a: { id: link.source },
      b: { id: link.target },
    };
  }
  return {
    layer: link.layer,
    kind: link.kind,
    meta: { event: link.meta?.event || "" },
    path: pathEl,
    label: labelEl,
    source: { id: link.source },
    target: { id: link.target },
  };
}

function applyLayerFixture(edges, state, util, mode) {
  for (const ed of edges) {
    const hints = mode === "live"
      ? { displayKind: ed.displayKind, metaEvent: ed.metaEvent }
      : { displayKind: ed.displayKind, metaEvent: ed.meta?.event };
    const keep = util.keepForLayer(ed, state, hints);
    ed.path.classList.toggle("layer-hidden", !keep);
    ed.label.classList.toggle("layer-hidden", !keep);
  }
}

function applySyncOnlyFixture(edges, nodes, util, mode, on) {
  const visibleNodes = new Set();
  for (const ed of edges) {
    const hints = mode === "live"
      ? { displayKind: ed.displayKind, metaEvent: ed.metaEvent }
      : { displayKind: ed.displayKind, metaEvent: ed.meta?.event };
    const keep = !on || util.keepForSyncOnly(ed, hints);
    ed.path.classList.toggle("sync-only-hidden", on && !keep);
    ed.label.classList.toggle("sync-only-hidden", on && !keep);
    if (on && keep) {
      if (mode === "live") {
        visibleNodes.add(ed.a.id);
        visibleNodes.add(ed.b.id);
      } else {
        visibleNodes.add(ed.source.id);
        visibleNodes.add(ed.target.id);
      }
    }
  }
  for (const node of nodes) {
    const keep = !on || visibleNodes.has(node.id);
    node.el.classList.toggle("sync-only-hidden", on && !keep);
  }
  return visibleNodes;
}

function testOfflineAndLiveFixtures(topologyBuilder, layerUtil) {
  const events = [
    { event: "spawn", g: 2, parent_g: 1, role: "worker", time_ms: 0, time_ns: 10, seq: 1 },
    { event: "chan_send", source: "paired", g: 1, peer_g: 2, channel_key: "ch:work", channel: "work", time_ms: 1, time_ns: 20, seq: 2, msg_id: 1 },
    { event: "chan_recv", source: "paired", g: 2, peer_g: 1, channel_key: "ch:work", channel: "work", time_ms: 2, time_ns: 30, seq: 3, msg_id: 1 },
    { event: "mutex_lock", g: 2, role: "worker", channel_key: "mutex:mu", time_ms: 3, time_ns: 40, seq: 4 },
    { event: "mutex_unlock", g: 2, role: "worker", channel_key: "mutex:mu", time_ms: 4, time_ns: 50, seq: 5 },
  ];
  const topo = topologyBuilder.buildTopology(events);
  const syncLinks = topo.links.filter((l) => l.layer === "sync");
  assert.ok(syncLinks.length >= 2, "expected sync links from fixture");
  assert.ok(topo.channels.has("mutex:mu"), "expected explicit sync resource node (mutex)");

  const layersState = { channel: false, sync: true, spawn: false, causal: false };
  for (const mode of ["offline", "live"]) {
    const edges = topo.links.map((l) => mkEdgeLike(l, mode));
    const nodes = [
      ...Array.from(topo.goroutines.values()).map((g) => ({ id: g.id, el: { classList: fakeClassList() } })),
      ...Array.from(topo.channels.values()).map((c) => ({ id: c.id, el: { classList: fakeClassList() } })),
    ];

    applyLayerFixture(edges, layersState, layerUtil, mode);
    const hiddenNonSync = edges
      .filter((e) => layerUtil.edgeLayer(e, mode === "live" ? { displayKind: e.displayKind, metaEvent: e.metaEvent } : { metaEvent: e.meta?.event }) !== "sync")
      .every((e) => e.path.classList.contains("layer-hidden"));
    const shownSync = edges
      .filter((e) => layerUtil.edgeLayer(e, mode === "live" ? { displayKind: e.displayKind, metaEvent: e.metaEvent } : { metaEvent: e.meta?.event }) === "sync")
      .every((e) => !e.path.classList.contains("layer-hidden"));
    assert.equal(hiddenNonSync, true, `${mode}: non-sync edges should hide in Sync layer view`);
    assert.equal(shownSync, true, `${mode}: sync edges should stay visible in Sync layer view`);

    const visibleNodes = applySyncOnlyFixture(edges, nodes, layerUtil, mode, true);
    assert.ok(visibleNodes.has("ch:mutex:mu"), `${mode}: mutex sync resource should remain visible in Sync only`);
    const hasHiddenChannelTraffic = edges
      .filter((e) => layerUtil.edgeLayer(e, mode === "live" ? { displayKind: e.displayKind, metaEvent: e.metaEvent } : { metaEvent: e.meta?.event }) === "channel")
      .every((e) => e.path.classList.contains("sync-only-hidden"));
    assert.equal(hasHiddenChannelTraffic, true, `${mode}: channel traffic should hide in Sync only`);
  }
}

function main() {
  assertPageHooks();
  const { topologyBuilder, layerUtil } = loadBrowserScripts();
  testOfflineAndLiveFixtures(topologyBuilder, layerUtil);
  // eslint-disable-next-line no-console
  console.log("viewer-renderer integration tests: ok");
}

main();


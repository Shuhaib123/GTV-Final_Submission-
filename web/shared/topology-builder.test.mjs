import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

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
      localStorageData.delete(key);
    },
  };
  const window = { localStorage };
  const context = {
    window,
    localStorage,
    console,
  };
  vm.createContext(context);
  for (const file of ["topology.js", "topology-builder.js"]) {
    const code = fs.readFileSync(path.join(__dirname, file), "utf8");
    vm.runInContext(code, context, { filename: file });
    if (file === "topology.js") {
      context.GTVTopology = context.window.GTVTopology;
    }
  }
  return context.window;
}

function testSyncKindMapping(gtv) {
  const cases = [
    { event: "mutex_lock", direction: "g_to_resource", kind: "sync-lock", layer: "sync" },
    { event: "mutex_unlock", direction: "resource_to_g", kind: "sync-unlock", layer: "sync" },
    { event: "rwmutex_rlock", direction: "g_to_resource", kind: "sync-lock", layer: "sync" },
    { event: "wg_wait", direction: "g_to_resource", kind: "sync-wg-wait", layer: "sync" },
    { event: "wg_done", direction: "resource_to_g", kind: "sync-wg-done", layer: "sync" },
    { event: "cond_signal", direction: "resource_to_g", kind: "sync-cond-signal", layer: "sync" },
  ];
  for (const tc of cases) {
    const info = gtv.GTVTopologyBuilder.topologyEventMeta({
      event: tc.event,
      g: 9,
      channel_key: "mutex:mu",
      time_ms: 1,
      time_ns: 1,
      seq: 1,
    });
    assert.ok(info, `missing topology metadata for ${tc.event}`);
    assert.equal(info.role, "", `sync role should be empty for ${tc.event}`);
    assert.equal(info.direction, tc.direction, `direction mismatch for ${tc.event}`);
    assert.equal(info.linkKind, tc.kind, `link kind mismatch for ${tc.event}`);
    assert.equal(info.layer, tc.layer, `layer mismatch for ${tc.event}`);
    assert.notEqual(info.linkKind, "send", `sync event collapsed to send: ${tc.event}`);
    assert.notEqual(info.linkKind, "recv", `sync event collapsed to recv: ${tc.event}`);
  }
}

function testUniqueLinkIDs(gtv) {
  const topology = gtv.GTVTopologyBuilder.buildTopology([
    {
      event: "mutex_lock",
      g: 2,
      role: "worker",
      channel_key: "mutex:mu",
      time_ms: 10,
      time_ns: 10000,
      seq: 42,
    },
    {
      event: "mutex_lock",
      g: 2,
      role: "worker",
      channel_key: "mutex:mu",
      time_ms: 10,
      time_ns: 10001,
      seq: 43,
    },
  ]);
  assert.equal(topology.links.length, 2, "expected two sync links");
  const ids = new Set(topology.links.map((l) => l.id));
  assert.equal(ids.size, 2, "sync links collapsed by id");
}

function testSpawnEdgesRemainStable(gtv) {
  const topology = gtv.GTVTopologyBuilder.buildTopology([
    { event: "spawn", g: 2, parent_g: 1, role: "worker", time_ms: 5, time_ns: 5000, seq: 7 },
    { event: "spawn", g: 3, parent_g: 1, role: "worker", time_ms: 5, time_ns: 5001, seq: 8 },
  ]);
  const spawn = topology.links.filter((l) => l.kind === "spawn");
  assert.equal(spawn.length, 2, "expected spawn edges for both children");
  assert.equal(new Set(spawn.map((l) => l.id)).size, 2, "spawn IDs must remain unique");
  for (const link of spawn) {
    assert.equal(link.layer, "spawn", "spawn edge should stay in spawn layer");
    assert.equal(link.direction, "parent_to_child", "spawn direction mismatch");
  }
}

function testSpawnInferenceSeparatedFromExplicit(gtv) {
  const topology = gtv.GTVTopologyBuilder.buildTopology({
    events: [
      { event: "goroutine_created", g: 2, role: "worker", time_ms: 1, time_ns: 1000, seq: 1 },
      { event: "spawn", g: 3, parent_g: 1, role: "worker", time_ms: 2, time_ns: 2000, seq: 2 },
    ],
    entities: {
      goroutines: [{ gid: 1, role: "main" }],
      channels: [],
    },
  });
  const explicit = topology.links.filter((l) => l.kind === "spawn");
  const inferred = topology.links.filter((l) => l.kind === "spawn-inferred");
  assert.equal(explicit.length, 1, "explicit spawn should be preserved");
  assert.equal(inferred.length, 1, "inferred spawn should be emitted separately");
  assert.equal(explicit[0].source, "g1");
  assert.equal(explicit[0].target, "g3");
  assert.equal(inferred[0].source, "g1");
  assert.equal(inferred[0].target, "g2");
  assert.equal(inferred[0].meta?.inferred, true);
}

function testChannelLabelSelectionPrefersHumanNames(gtv) {
  const pick = gtv.GTVTopology.pickChannelLabel;
  assert.equal(
    pick(["4070", "jobs"], "ch#1"),
    "jobs",
    "short pointer-like suffix should not win over source channel name",
  );
  assert.equal(
    gtv.GTVTopology.channelLabelForEvent({ ch_ptr: "0x1400004070" }),
    "0x1400004070",
    "pointer-only channel labels should keep pointer form (not short hex suffix)",
  );
}

function main() {
  const gtv = loadBrowserScripts();
  testSyncKindMapping(gtv);
  testUniqueLinkIDs(gtv);
  testSpawnEdgesRemainStable(gtv);
  testSpawnInferenceSeparatedFromExplicit(gtv);
  testChannelLabelSelectionPrefersHumanNames(gtv);
  // eslint-disable-next-line no-console
  console.log("topology-builder tests: ok");
}

main();

import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

function loadLayerUtils() {
  const window = {};
  const context = { window, console };
  vm.createContext(context);
  const code = fs.readFileSync(path.join(__dirname, "viewer-layer-utils.js"), "utf8");
  vm.runInContext(code, context, { filename: "viewer-layer-utils.js" });
  return context.window.GTVViewerLayer;
}

function testSyncNames(util) {
  assert.equal(util.isSyncEventName("mutex_lock"), true);
  assert.equal(util.isSyncEventName("rwmutex_runlock"), true);
  assert.equal(util.isSyncEventName("wg_wait"), true);
  assert.equal(util.isSyncEventName("cond_signal"), true);
  assert.equal(util.isSyncEventName("chan_send"), false);
}

function testEdgeLayerClassification(util) {
  // Offline-style datum
  assert.equal(util.edgeLayer({ layer: "sync", kind: "sync-lock" }), "sync");
  assert.equal(util.edgeLayer({ kind: "sync-unlock" }), "sync");
  assert.equal(util.edgeLayer({ kind: "spawn" }), "spawn");
  assert.equal(util.edgeLayer({ kind: "causal" }), "causal");
  assert.equal(util.edgeLayer({ kind: "send" }), "channel");

  // Live-style datum
  assert.equal(
    util.edgeLayer({ displayKind: "sync-wg-wait", metaEvent: "wg_wait" }),
    "sync",
  );
  assert.equal(
    util.edgeLayer({ displayKind: "causal", metaEvent: "chan_recv_commit" }),
    "causal",
  );
  assert.equal(
    util.edgeLayer({ meta: { event: "cond_wait" } }),
    "sync",
  );
}

function testFilterBehavior(util) {
  const state = { channel: false, sync: true, spawn: false, causal: false };
  assert.equal(util.keepForLayer({ kind: "sync-lock" }, state), true);
  assert.equal(util.keepForLayer({ kind: "send" }, state), false);
  assert.equal(util.keepForLayer({ kind: "spawn" }, state), false);
  assert.equal(util.keepForLayer({ kind: "causal" }, state), false);

  assert.equal(util.keepForSyncOnly({ kind: "sync-lock" }), true);
  assert.equal(util.keepForSyncOnly({ kind: "send" }), false);
  assert.equal(util.keepForSyncOnly({ meta: { event: "wg_wait" } }), true);
}

function main() {
  const util = loadLayerUtils();
  testSyncNames(util);
  testEdgeLayerClassification(util);
  testFilterBehavior(util);
  // eslint-disable-next-line no-console
  console.log("viewer-layer-utils tests: ok");
}

main();


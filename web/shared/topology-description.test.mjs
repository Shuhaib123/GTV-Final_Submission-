import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

function loadDescriptionModule() {
  const window = {};
  const context = { window, console };
  vm.createContext(context);
  const code = fs.readFileSync(path.join(__dirname, "topology-description.js"), "utf8");
  vm.runInContext(code, context, { filename: "topology-description.js" });
  return context.window.GTVTopologyDescription;
}

function testCoreExtraction(desc) {
  const envelope = {
    events: [
      { time_ns: 10, event: "chan_make", g: 1, role: "main", channel: "jobs", channel_key: "ch#1", ch_ptr: "0x1001" },
      { time_ns: 11, event: "chan_make", g: 1, role: "main", channel: "results", channel_key: "ch#2", ch_ptr: "0x1002" },
      { time_ns: 12, event: "spawn", g: 2, parent_g: 1, role: "worker", func: "workload.worker" },
      { time_ns: 13, event: "spawn", g: 3, parent_g: 1, role: "worker", func: "workload.worker" },
      { time_ns: 14, event: "chan_send", g: 1, channel: "jobs", channel_key: "ch#1", source: "paired" },
      { time_ns: 15, event: "chan_recv", g: 2, channel: "jobs", channel_key: "ch#1", source: "paired" },
      { time_ns: 16, event: "send_complete", g: 2, channel: "results", channel_key: "ch#2" },
      { time_ns: 17, event: "recv_complete", g: 1, channel: "results", channel_key: "ch#2" },
    ],
  };

  const trace = desc.buildTopologyTrace(envelope);
  assert.equal(trace.main_gid, 1, "main goroutine should resolve from role=main");
  assert.deepEqual(Array.from(trace.spawn_graph["1"] || []), [2, 3], "spawn graph should preserve parent->children");
  assert.equal(trace.main_spawn_groups.length, 1, "main spawn groups should merge same role+func");
  assert.equal(trace.main_spawn_groups[0].count, 2);
  assert.equal(trace.main_spawn_groups[0].role, "worker");
  assert.equal(trace.main_spawn_groups[0].func, "workload.worker");

  const jobs = trace.channels.find((c) => c.chan_id === "ch#1");
  const results = trace.channels.find((c) => c.chan_id === "ch#2");
  assert.ok(jobs, "jobs channel missing");
  assert.ok(results, "results channel missing");
  assert.equal(jobs.name, "jobs");
  assert.equal(results.name, "results");
  assert.equal(jobs.creator_gid, 1);

  const jobsComm = trace.comm.find((c) => c.chan_id === "ch#1");
  const resultsComm = trace.comm.find((c) => c.chan_id === "ch#2");
  assert.deepEqual(Array.from(jobsComm.senders || []), [1]);
  assert.deepEqual(Array.from(jobsComm.receivers || []), [2]);
  assert.equal(resultsComm.send_count, 1, "send_complete should normalize to chan_send");
  assert.equal(resultsComm.recv_count, 1, "recv_complete should normalize to chan_recv");
}

function testFallbackMainResolution(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "go_create", g: 5 },
      { time_ns: 2, event: "go_create", g: 6, parent_g: 5 },
    ],
  });
  assert.equal(trace.main_gid, 5, "earliest root goroutine should be main fallback");
}

function testEntityFallbackWithoutChanMake(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 101, event: "chan_send", g: 9, channel_key: "ch#9" },
      { time_ns: 102, event: "chan_recv", g: 10, channel_key: "ch#9" },
    ],
    entities: {
      channels: [{ chan_id: "ch#9", name: "work", cap: 0, elem_type: "int", aliases: ["0x9009"] }],
      goroutines: [{ gid: 9, role: "producer" }, { gid: 10, role: "consumer" }],
    },
  });
  const ch = trace.channels.find((c) => c.chan_id === "ch#9");
  assert.ok(ch, "entity channel should be present");
  assert.equal(ch.name, "work", "entity fallback should fill missing channel names");
  assert.equal(ch.elem_type, "int");
}

function testCompactSummary(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "ping", channel_key: "ch#1" },
      { time_ns: 2, event: "spawn", g: 2, parent_g: 1, role: "worker", func: "workload.pingpong" },
      { time_ns: 3, event: "chan_send", g: 1, channel_key: "ch#1" },
      { time_ns: 4, event: "chan_recv", g: 2, channel_key: "ch#1" },
    ],
  });
  const compact = desc.summarizeTopology(trace, { mode: "compact" });
  const lines = String(compact).split("\n").filter(Boolean);
  assert.ok(lines.length <= 10, "compact output should be capped to 10 lines");
  assert.ok(compact.includes("Main creates channels"), "summary should include setup context");
}

function testRoleOrientedFlowSummary(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "jobs", channel_key: "ch#1" },
      { time_ns: 2, event: "spawn", g: 2, parent_g: 1, role: "worker", func: "workload.worker" },
      { time_ns: 3, event: "spawn", g: 3, parent_g: 1, role: "worker", func: "workload.worker" },
      { time_ns: 4, event: "chan_send", g: 1, channel_key: "ch#1" },
      { time_ns: 5, event: "chan_recv", g: 2, channel_key: "ch#1" },
      { time_ns: 6, event: "chan_recv", g: 3, channel_key: "ch#1" },
    ],
  });
  const normal = desc.summarizeTopology(trace, { mode: "normal" });
  assert.ok(normal.includes("main (g1)"), "flow summary should prioritize role names");
  assert.ok(normal.includes("workers (2)"), "flow summary should aggregate worker endpoints");
  assert.ok(normal.includes("Message flow:"), "normal mode should emit a message-flow section");
}

function testPatternAndPhaseDetection(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "join", channel_key: "ch#join", cap: 2, elem_type: "chan<- int" },
      { time_ns: 2, event: "spawn", g: 2, parent_g: 1, role: "worker" },
      { time_ns: 3, event: "spawn", g: 3, parent_g: 1, role: "worker" },
      { time_ns: 4, event: "spawn", g: 4, parent_g: 1, role: "worker" },
      { time_ns: 5, event: "chan_send", g: 2, channel_key: "ch#join", channel: "join" },
      { time_ns: 6, event: "chan_send", g: 3, channel_key: "ch#join", channel: "join" },
      { time_ns: 7, event: "chan_send", g: 4, channel_key: "ch#join", channel: "join" },
      { time_ns: 8, event: "chan_recv", g: 1, channel_key: "ch#join", channel: "join" },
    ],
  });
  const patterns = desc.detectTopologyPatterns(trace);
  const kinds = new Set(patterns.map((p) => p.kind));
  assert.ok(kinds.has("join_bus"), "join bus pattern should be detected");
  assert.ok(kinds.has("fan_in"), "fan-in pattern should be detected");
  const phases = desc.detectPhases(trace, patterns).join("\n");
  assert.ok(phases.includes("Setup:"), "phase detection should include setup");
  assert.ok(phases.includes("Spawn:"), "phase detection should include spawn");
  assert.ok(phases.includes("Handshake:"), "phase detection should include handshake");
  assert.ok(phases.includes("Steady-state:"), "phase detection should include steady-state");
}

function testConfidenceIndicatorAndJoinStyleNarrative(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "join", channel_key: "ch#join", cap: 4, elem_type: "chan<- int" },
      { time_ns: 2, event: "chan_make", g: 1, role: "main", channel: "serverin", channel_key: "ch#serverin" },
      { time_ns: 3, event: "chan_make", g: 1, role: "main", channel: "clientin", channel_key: "ch#clientin" },
      { time_ns: 4, event: "spawn", g: 2, parent_g: 1, role: "worker", func: "workload.server" },
      { time_ns: 5, event: "spawn", g: 3, parent_g: 1, role: "worker", func: "workload.client" },
      { time_ns: 6, event: "spawn", g: 4, parent_g: 1, role: "worker", func: "workload.client" },
      { time_ns: 7, event: "chan_send", g: 3, channel_key: "ch#join", channel: "join" },
      { time_ns: 8, event: "chan_send", g: 4, channel_key: "ch#join", channel: "join" },
      { time_ns: 9, event: "chan_recv", g: 2, channel_key: "ch#join", channel: "join" },
      { time_ns: 10, event: "chan_send", g: 2, channel_key: "ch#serverin", channel: "serverin" },
      { time_ns: 11, event: "chan_recv", g: 3, channel_key: "ch#serverin", channel: "serverin" },
      { time_ns: 12, event: "chan_recv", g: 4, channel_key: "ch#serverin", channel: "serverin" },
      { time_ns: 13, event: "chan_send", g: 3, channel_key: "ch#clientin", channel: "clientin" },
      { time_ns: 14, event: "chan_send", g: 4, channel_key: "ch#clientin", channel: "clientin" },
      { time_ns: 15, event: "chan_recv", g: 2, channel_key: "ch#clientin", channel: "clientin" },
    ],
  });
  const normal = desc.summarizeTopology(trace, { mode: "normal" });
  assert.ok(normal.includes("Confidence:"), "normal mode should expose confidence");
  assert.ok(normal.includes("join bus"), "join-style handshake should be called out in confidence text");
  assert.ok(normal.includes("[high]"), "pattern confidence should be surfaced");
}

function testTeardownNarrative(desc) {
  const noShutdown = desc.summarizeTopology(desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "work", channel_key: "ch#1" },
      { time_ns: 2, event: "spawn", g: 2, parent_g: 1, role: "worker" },
      { time_ns: 3, event: "chan_send", g: 1, channel_key: "ch#1" },
      { time_ns: 4, event: "chan_recv", g: 2, channel_key: "ch#1" },
    ],
  }), { mode: "normal" });
  assert.ok(noShutdown.includes("without explicit shutdown"), "missing teardown events should be called out");

  const explicitShutdown = desc.summarizeTopology(desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 1, role: "main", channel: "done", channel_key: "ch#done" },
      { time_ns: 2, event: "spawn", g: 2, parent_g: 1, role: "worker" },
      { time_ns: 3, event: "chan_send", g: 2, channel_key: "ch#done" },
      { time_ns: 4, event: "chan_recv", g: 1, channel_key: "ch#done" },
      { time_ns: 5, event: "chan_close", g: 1, channel_key: "ch#done" },
      { time_ns: 6, event: "go_end", g: 2 },
    ],
  }), { mode: "normal" });
  assert.ok(explicitShutdown.includes("observed 1 goroutine end events and 1 channel closes"), "explicit teardown should be summarized");
}

function testMissingRolesFallback(desc) {
  const trace = desc.buildTopologyTrace({
    events: [
      { time_ns: 1, event: "chan_make", g: 10, channel: "jobs", channel_key: "ch#jobs" },
      { time_ns: 2, event: "spawn", g: 11, parent_g: 10 },
      { time_ns: 3, event: "chan_send", g: 10, channel_key: "ch#jobs" },
      { time_ns: 4, event: "chan_recv", g: 11, channel_key: "ch#jobs" },
    ],
  });
  const summary = desc.summarizeTopology(trace, { mode: "compact" });
  assert.ok(summary.includes("Program begins"), "summary should still render when roles are missing");
  assert.ok(summary.includes("g10"), "gid fallback should be visible");
}

function main() {
  const desc = loadDescriptionModule();
  testCoreExtraction(desc);
  testFallbackMainResolution(desc);
  testEntityFallbackWithoutChanMake(desc);
  testCompactSummary(desc);
  testRoleOrientedFlowSummary(desc);
  testPatternAndPhaseDetection(desc);
  testConfidenceIndicatorAndJoinStyleNarrative(desc);
  testTeardownNarrative(desc);
  testMissingRolesFallback(desc);
  // eslint-disable-next-line no-console
  console.log("topology-description tests: ok");
}

main();

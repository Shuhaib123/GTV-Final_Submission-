(() => {
  function lower(v) {
    return String(v || "").toLowerCase();
  }

  function isSyncEventName(name) {
    const n = lower(name);
    return (
      n.startsWith("mutex_") ||
      n.startsWith("rwmutex_") ||
      n.startsWith("wg_") ||
      n.startsWith("cond_")
    );
  }

  function edgeLayer(edge, hints = {}) {
    const direct = lower(
      edge?.layer ||
      edge?.meta?.layer ||
      hints.layer
    );
    if (direct === "channel" || direct === "sync" || direct === "spawn" || direct === "causal") {
      return direct;
    }

    const displayKind = lower(hints.displayKind || edge?.displayKind);
    if (displayKind === "spawn" || displayKind === "spawn-inferred") return "spawn";
    if (displayKind === "causal") return "causal";
    if (displayKind.startsWith("sync-")) return "sync";

    const kind = lower(edge?.kind);
    if (kind === "spawn" || kind === "spawn-inferred") return "spawn";
    if (kind === "causal") return "causal";
    if (kind.startsWith("sync-")) return "sync";

    const eventName = lower(
      hints.metaEvent ||
      edge?.metaEvent ||
      edge?.meta?.event
    );
    if (isSyncEventName(eventName)) return "sync";
    return "channel";
  }

  function isSyncEdge(edge, hints = {}) {
    return edgeLayer(edge, hints) === "sync";
  }

  function keepForLayer(edge, state, hints = {}) {
    const layer = edgeLayer(edge, hints);
    if (layer === "sync") return !!state?.sync;
    if (layer === "spawn") return !!state?.spawn;
    if (layer === "causal") return !!state?.causal;
    return !!state?.channel;
  }

  function keepForSyncOnly(edge, hints = {}) {
    return isSyncEdge(edge, hints);
  }

  window.GTVViewerLayer = {
    isSyncEventName,
    edgeLayer,
    isSyncEdge,
    keepForLayer,
    keepForSyncOnly,
  };
})();

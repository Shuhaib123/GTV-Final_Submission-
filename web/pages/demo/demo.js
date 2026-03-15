(() => {
  const goTraceBtn = document.getElementById("btnGoTrace");
  const statusEl = document.getElementById("goTraceStatus");
  if (!goTraceBtn) return;

  const setStatus = (text, isError) => {
    if (!statusEl) return;
    statusEl.textContent = text || "";
    if (isError) statusEl.setAttribute("data-state", "error");
    else statusEl.removeAttribute("data-state");
  };

  const parseJSON = async (res) => {
    try {
      return await res.json();
    } catch {
      return null;
    }
  };

  goTraceBtn.addEventListener("click", async () => {
    const original = goTraceBtn.textContent;
    goTraceBtn.disabled = true;
    goTraceBtn.textContent = "Launching...";
    setStatus("Running demo pingpong workload and preparing trace.out...", false);

    let traceTab = null;
    try {
      traceTab = window.open("about:blank", "_blank");
      if (traceTab) {
        try {
          traceTab.opener = null;
        } catch {}
      }
      const res = await fetch("/demo/go-trace", { method: "POST" });
      const body = await parseJSON(res);
      if (!res.ok) {
        const msg = body && body.error ? body.error : `Request failed (${res.status})`;
        throw new Error(msg);
      }
      const url = body && typeof body.url === "string" ? body.url : "";
      if (!url) {
        throw new Error("Server did not return a Go trace viewer URL.");
      }
      if (traceTab) traceTab.location.replace(url);
      else window.open(url, "_blank");
      setStatus("Go trace viewer opened.", false);
    } catch (err) {
      if (traceTab) traceTab.close();
      setStatus(err && err.message ? err.message : String(err), true);
    } finally {
      goTraceBtn.disabled = false;
      goTraceBtn.textContent = original;
    }
  });
})();

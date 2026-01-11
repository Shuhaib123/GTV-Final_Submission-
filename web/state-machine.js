(() => {
  function createStateMachine({ ui, gAnims, circle, drawnEdges }) {
    function animateDot(x1, y1, x2, y2, kind = "send", durMs = 400) {
      const dot = circle(x1, y1, 5, `dot ${kind}`);
      gAnims.appendChild(dot);
      let last = performance.now();
      let elapsed = 0; // only accumulates when ui.animating is true
      function step(now) {
        if (!ui.animating) {
          last = now;
          requestAnimationFrame(step);
          return;
        }
        elapsed += now - last;
        last = now;
        const p = Math.min(1, elapsed / durMs);
        const x = x1 + (x2 - x1) * p,
          y = y1 + (y2 - y1) * p;
        dot.setAttribute("cx", x);
        dot.setAttribute("cy", y);
        if (p < 1) requestAnimationFrame(step);
        else dot.remove();
      }
      requestAnimationFrame(step);
    }

    // ======= Post-run flow loop (infinite pulses) =======
    const flowLoop = { running: false, idx: 0, timer: null };
    function startFlowLoop() {
      if (flowLoop.running || drawnEdges.length === 0) return;
      flowLoop.running = true;
      flowLoop.idx = 0;
      const STEP_MS = 250,
        DUR_MS = 500; // pacing for pulses
      (function tick() {
        if (!flowLoop.running) return;
        const ed = drawnEdges[flowLoop.idx % drawnEdges.length];
        if (ed)
          animateDot(
            ed.a.x,
            ed.a.y,
            ed.b.x,
            ed.b.y,
            ed.kind || "send",
            DUR_MS
          );
        flowLoop.idx++;
        flowLoop.timer = setTimeout(tick, STEP_MS);
      })();
    }
    function stopFlowLoop() {
      flowLoop.running = false;
      if (flowLoop.timer) {
        clearTimeout(flowLoop.timer);
        flowLoop.timer = null;
      }
    }

    return { animateDot, startFlowLoop, stopFlowLoop };
  }

  window.GTVStateMachine = { createStateMachine };
})();

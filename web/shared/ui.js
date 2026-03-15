(function () {
  const ROOT = document.documentElement;
  const STORAGE_KEY = "gtvTheme";

  function applyTheme(val) {
    const v = (val || "dark").toLowerCase();
    if (v === "light") ROOT.classList.add("theme-light");
    else ROOT.classList.remove("theme-light");
  }

  function getStored() {
    try {
      return localStorage.getItem(STORAGE_KEY) || "dark";
    } catch {
      return "dark";
    }
  }

  function setStored(v) {
    try {
      localStorage.setItem(STORAGE_KEY, v);
    } catch {}
  }

  // Initial apply
  applyTheme(getStored());

  function toggle() {
    const isLight = ROOT.classList.contains("theme-light");
    const next = isLight ? "dark" : "light";
    setStored(next);
    applyTheme(next);
  }

  // Bind all toggles
  document.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-gtv-theme-toggle]");
    if (!btn) return;
    e.preventDefault();
    toggle();
  });

  // Mark active nav links (best-effort)
  try {
    const path = location.pathname.split("/").pop() || "index.html";
    document.querySelectorAll(".gtv-nav a.gtv-link").forEach((a) => {
      const href = (a.getAttribute("href") || "").split("?")[0];
      if (href === path) a.setAttribute("aria-current", "page");
    });
  } catch {}
})();

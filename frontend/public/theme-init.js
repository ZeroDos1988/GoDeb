// Applies the stored theme colors before the app loads, so the page does not
// flash in the default palette. Kept as a file because the CSP forbids inline scripts.
try {
  var stored = JSON.parse(localStorage.getItem("godeb-theme") || "null");
  var light = { primary: "#a80030", bg: "#f3efe6", accent: "#246b45", text: "#1c1915" };
  var dark = { primary: "#e23b5a", bg: "#141210", accent: "#3dbe7a", text: "#f4efe6" };
  var mode = stored && stored.mode;
  var colors = mode === "custom" && stored.colors ? stored.colors : mode === "dark" || (mode === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches) ? dark : light;
  var root = document.documentElement;
  root.style.setProperty("--color-primary", colors.primary);
  root.style.setProperty("--color-bg", colors.bg);
  root.style.setProperty("--color-accent", colors.accent);
  root.style.setProperty("--color-text", colors.text);
} catch (e) {}

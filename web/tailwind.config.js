/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        bg: "var(--bg)",
        surface: "var(--surface)",
        "surface-2": "var(--surface-2)",
        border: "var(--border)",
        text: "var(--text)",
        muted: "var(--muted)",
        accent: "var(--accent)",
        "accent-2": "var(--accent-2)",
        warn: "var(--warn)",
        danger: "var(--danger)",
        ok: "var(--ok)",
      },
      fontFamily: {
        display: ["'Space Grotesk Variable'", "system-ui", "sans-serif"],
        sans: ["'Inter Variable'", "system-ui", "sans-serif"],
        mono: ["'JetBrains Mono Variable'", "ui-monospace", "monospace"],
      },
      boxShadow: {
        panel: "0 1px 2px rgba(0,0,0,0.06), 0 8px 24px -12px rgba(0,0,0,0.25)",
      },
    },
  },
  plugins: [],
};

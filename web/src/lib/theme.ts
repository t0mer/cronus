import { useEffect, useState } from "react";

export type Theme = "light" | "dark" | "system";
const KEY = "cronus-theme";

function systemDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function apply(theme: Theme) {
  const root = document.documentElement;
  root.classList.remove("light", "dark");
  if (theme === "system") return; // media query drives it
  root.classList.add(theme);
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    try {
      return (localStorage.getItem(KEY) as Theme) || "system";
    } catch {
      return "system";
    }
  });

  useEffect(() => {
    apply(theme);
    try {
      localStorage.setItem(KEY, theme);
    } catch {
      /* ignore */
    }
  }, [theme]);

  const isDark = theme === "dark" || (theme === "system" && systemDark());
  const toggle = () => setTheme(isDark ? "light" : "dark");
  return { theme, setTheme, isDark, toggle };
}

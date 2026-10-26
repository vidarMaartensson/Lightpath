import { useEffect, useState } from "react";
import { chooseTheme, currentTheme, followSystem, nextTheme } from "../lib/theme";

export function ThemeToggle() {
  const [theme, setTheme] = useState(currentTheme);
  useEffect(() => followSystem(setTheme), []);

  const next = nextTheme(theme);
  const label = `Switch to ${next} mode`;

  return (
    <button
      type="button"
      className="theme-toggle"
      aria-label={label}
      title={label}
      onClick={() => {
        chooseTheme(next);
        setTheme(next);
      }}
    >
      {next === "dark" ? <MoonIcon /> : <SunIcon />}
    </button>
  );
}

function MoonIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" strokeLinejoin="round" />
    </svg>
  );
}

function SunIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true">
      <circle cx="12" cy="12" r="4" />
      <path
        d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4"
        strokeLinecap="round"
      />
    </svg>
  );
}

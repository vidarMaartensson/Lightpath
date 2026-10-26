// Light or dark. The page follows the system until the user picks one;
// the pick is remembered across visits. index.html runs the same lookup
// inline before first paint, so the page never flashes the wrong theme.

export type Theme = "light" | "dark";

export const THEME_KEY = "lightpath-theme";

// Matches --paper in styles.css, for the browser chrome.
const PAPER: Record<Theme, string> = { light: "#f2f5f6", dark: "#0c1a22" };

type Store = Pick<Storage, "getItem" | "setItem">;

// Storage can be missing or throw (private windows, blocked site data),
// so every access is guarded and a failure means "no choice made".
export function readChoice(store: Store | undefined): Theme | null {
  try {
    const v = store?.getItem(THEME_KEY);
    return v === "light" || v === "dark" ? v : null;
  } catch {
    return null;
  }
}

export function saveChoice(store: Store | undefined, theme: Theme) {
  try {
    store?.setItem(THEME_KEY, theme);
  } catch {
    // The choice still applies for this visit.
  }
}

export function resolveTheme(choice: Theme | null, systemDark: boolean): Theme {
  return choice ?? (systemDark ? "dark" : "light");
}

export function nextTheme(theme: Theme): Theme {
  return theme === "dark" ? "light" : "dark";
}

// Browser glue below; the decisions above are what the tests cover.

function storage(): Store | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

const systemQuery = () => window.matchMedia("(prefers-color-scheme: dark)");

export function currentTheme(): Theme {
  return resolveTheme(readChoice(storage()), systemQuery().matches);
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
    meta.setAttribute("content", PAPER[theme]);
  }
}

export function chooseTheme(theme: Theme) {
  saveChoice(storage(), theme);
  applyTheme(theme);
}

// Until the user picks a theme, keep following the system if it changes
// while the page is open. Returns an unsubscribe function.
export function followSystem(onChange: (theme: Theme) => void): () => void {
  const query = systemQuery();
  const listener = () => {
    if (readChoice(storage())) return;
    const theme = resolveTheme(null, query.matches);
    applyTheme(theme);
    onChange(theme);
  };
  query.addEventListener("change", listener);
  return () => query.removeEventListener("change", listener);
}

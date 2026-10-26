import { describe, expect, it } from "vitest";
import { THEME_KEY, nextTheme, readChoice, resolveTheme, saveChoice } from "./theme";

const memory = (initial: Record<string, string> = {}) => {
  const data = new Map(Object.entries(initial));
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  };
};

const broken = {
  getItem: (): string | null => {
    throw new Error("SecurityError");
  },
  setItem: () => {
    throw new Error("QuotaExceededError");
  },
};

describe("resolveTheme", () => {
  it.each([
    { name: "no choice, light system", choice: null, systemDark: false, want: "light" },
    { name: "no choice, dark system", choice: null, systemDark: true, want: "dark" },
    { name: "chose light on a dark system", choice: "light", systemDark: true, want: "light" },
    { name: "chose dark on a light system", choice: "dark", systemDark: false, want: "dark" },
  ] as const)("$name", ({ choice, systemDark, want }) => {
    expect(resolveTheme(choice, systemDark)).toBe(want);
  });
});

describe("readChoice", () => {
  it.each([
    { name: "nothing stored", stored: null, want: null },
    { name: "light", stored: "light", want: "light" },
    { name: "dark", stored: "dark", want: "dark" },
    { name: "junk is ignored", stored: "purple", want: null },
  ])("$name", ({ stored, want }) => {
    expect(readChoice(memory(stored === null ? {} : { [THEME_KEY]: stored }))).toBe(want);
  });

  it("treats missing storage as no choice", () => {
    expect(readChoice(undefined)).toBeNull();
  });

  it("treats storage that throws as no choice", () => {
    expect(readChoice(broken)).toBeNull();
  });
});

describe("saveChoice", () => {
  it("remembers the choice for the next visit", () => {
    const store = memory();
    saveChoice(store, "dark");
    expect(readChoice(store)).toBe("dark");
  });

  it("does not throw when storage fails or is missing", () => {
    expect(() => saveChoice(broken, "dark")).not.toThrow();
    expect(() => saveChoice(undefined, "dark")).not.toThrow();
  });
});

describe("nextTheme", () => {
  it("toggles between the two", () => {
    expect(nextTheme("light")).toBe("dark");
    expect(nextTheme("dark")).toBe("light");
  });
});

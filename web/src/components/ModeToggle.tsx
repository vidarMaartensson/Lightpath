import type { Mode } from "../lib/url";

const MODES: { mode: Mode; label: string }[] = [
  { mode: "live", label: "Live" },
  { mode: "sandbox", label: "Sandbox" },
];

export function ModeToggle({ mode, onChange }: { mode: Mode; onChange: (mode: Mode) => void }) {
  return (
    <div className="mode-toggle" role="group" aria-label="View">
      {MODES.map((m) => (
        <button
          key={m.mode}
          type="button"
          aria-pressed={mode === m.mode}
          onClick={() => mode !== m.mode && onChange(m.mode)}
        >
          {m.label}
        </button>
      ))}
    </div>
  );
}

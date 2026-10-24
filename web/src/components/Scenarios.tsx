// Prepared outages that each tell a different redundancy story.
const SCENARIOS: { label: string; down: string[] }[] = [
  { label: "Ålidhem access site fails", down: ["acc-ali"] },
  { label: "Core A fails", down: ["core-a"] },
  { label: "Väst aggregation fails", down: ["agg-vast"] },
  { label: "Öst aggregation fails", down: ["agg-ost"] },
];

const same = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x));

export function Scenarios({ down, onChoose }: { down: string[]; onChoose: (down: string[]) => void }) {
  return (
    <div className="scenarios">
      <span className="scenarios-label">Try</span>
      {SCENARIOS.map((s) => (
        <button
          key={s.label}
          type="button"
          className="scenario"
          aria-pressed={same(down, s.down)}
          onClick={() => onChoose(same(down, s.down) ? [] : s.down)}
        >
          {s.label}
        </button>
      ))}
    </div>
  );
}

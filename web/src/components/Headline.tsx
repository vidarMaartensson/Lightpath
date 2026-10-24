import { headline } from "../lib/format";
import type { Outage } from "../types";

export function Headline({ outage }: { outage: Outage }) {
  return (
    <h1 className="headline">
      {headline(outage).map((part, i) =>
        part.tone ? (
          <span key={i} className={`tone-${part.tone}`}>
            {part.text}
          </span>
        ) : (
          part.text
        ),
      )}
    </h1>
  );
}

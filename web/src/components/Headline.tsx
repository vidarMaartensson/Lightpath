import { headline } from "../lib/format";
import type { Outage } from "../types";

export function Headline({ outage }: { outage: Outage }) {
  const parts = headline(outage);
  // A new answer remounts the heading, so it settles in instead of the text
  // swapping in place. The same answer again does not move.
  return (
    <h1 className="headline" key={parts.map((p) => p.text).join("")}>
      {parts.map((part, i) =>
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

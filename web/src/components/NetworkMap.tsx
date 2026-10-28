import { memo, useMemo, type KeyboardEvent } from "react";
import { count, kindLabel, stateLabel } from "../lib/format";
import { drawLinks, type DrawnLink } from "../lib/graph";
import type { NetNode, Network, NodeImpact, NodeState } from "../types";

interface Props {
  network: Network;
  states: ReadonlyMap<string, NodeState>;
  impacts: ReadonlyMap<string, NodeImpact>;
  down: string[];
  updating: boolean;
  /** The map follows the live network, so selecting a site changes nothing. */
  live: boolean;
  onToggle: (id: string) => void;
  onInspect: (id: string | null) => void;
}

const TIERS = [
  { y: 70, label: "Core" },
  { y: 205, label: "Aggregation" },
  { y: 350, label: "Access" },
  { y: 520, label: "Street cabinets" },
];

export function NetworkMap({ network, states, impacts, down, updating, live, onToggle, onInspect }: Props) {
  const links = useMemo(() => drawLinks(network, states), [network, states]);

  return (
    <figure className="map" aria-busy={updating} onMouseLeave={() => onInspect(null)}>
      <div className="map-scroll">
        <svg
          viewBox="-100 0 1100 620"
          role="group"
          aria-label={
            live
              ? "Network map. It follows the live network."
              : "Network map. Select a site to take it down or restore it."
          }
        >
          <g className="tiers" aria-hidden="true">
            {TIERS.map((t) => (
              <g key={t.label}>
                <line x1={-100} x2={1000} y1={t.y} y2={t.y} />
                <text x={-96} y={t.y - 8}>
                  {t.label}
                </text>
              </g>
            ))}
          </g>

          <g aria-hidden="true">
            {links.map((l) => (
              <Fibre key={l.link.id} drawn={l} />
            ))}
          </g>

          <g>
            {network.nodes.map((n) => (
              <Site
                key={n.id}
                node={n}
                state={states.get(n.id) ?? "ONLINE"}
                impact={impacts.get(n.id)}
                isDown={down.includes(n.id)}
                live={live}
                onToggle={onToggle}
                onInspect={onInspect}
              />
            ))}
          </g>
        </svg>
      </div>

      <figcaption className="legend">
        <span className="legend-item">
          <i className="swatch swatch-active" /> Light on its normal path
        </span>
        <span className="legend-item">
          <i className="swatch swatch-rerouted" /> Light on a backup path
        </span>
        <span className="legend-item">
          <i className="swatch swatch-standby" /> Spare fibre
        </span>
        <span className="legend-item">
          <i className="swatch swatch-dead" /> No light
        </span>
      </figcaption>
    </figure>
  );
}

function curve(a: NetNode, b: NetNode) {
  if (a.y === b.y) return `M${a.x} ${a.y}L${b.x} ${b.y}`;
  const my = (a.y + b.y) / 2;
  return `M${a.x} ${a.y}C${a.x} ${my} ${b.x} ${my} ${b.x} ${b.y}`;
}

const Fibre = memo(function Fibre({ drawn }: { drawn: DrawnLink }) {
  const d = curve(drawn.from, drawn.to);
  // The light is always drawn and hidden by state, so it can fade out
  // when a fibre goes dark instead of vanishing.
  return (
    <g className={`fibre fibre-${drawn.state}`}>
      <path className="fibre-track" d={d} />
      <path className="fibre-glow" d={d} />
      <path className="fibre-light" d={d} />
    </g>
  );
});

interface SiteProps {
  node: NetNode;
  state: NodeState;
  impact: NodeImpact | undefined;
  isDown: boolean;
  live: boolean;
  onToggle: (id: string) => void;
  onInspect: (id: string | null) => void;
}

function radius(node: NetNode, customers: number) {
  switch (node.kind) {
    case "CORE":
      return 15;
    case "AGGREGATION":
      return 11;
    case "ACCESS":
      return 9;
    case "CABINET":
      // Cabinet size follows the number of customers behind it.
      return 4 + Math.sqrt(customers) * 0.45;
  }
}

const Site = memo(function Site({ node, state, impact, isDown, live, onToggle, onInspect }: SiteProps) {
  const customers = impact?.customers ?? 0;
  const r = radius(node, customers);
  const { x, y } = node;

  const onKey = (e: KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onToggle(node.id);
    }
  };

  const label =
    node.kind === "CABINET"
      ? `${node.name}, ${stateLabel[state].toLowerCase()}, ${count(customers)} customers`
      : `${node.name}, ${kindLabel[node.kind].toLowerCase()}, ${stateLabel[state].toLowerCase()}`;

  return (
    <g
      className={`site site-${node.kind.toLowerCase()} state-${state.toLowerCase()}`}
      role="button"
      tabIndex={0}
      aria-pressed={isDown}
      aria-label={`${label}.${live ? "" : isDown ? " Press to restore." : " Press to take down."}`}
      onClick={() => onToggle(node.id)}
      onKeyDown={onKey}
      onMouseEnter={() => onInspect(node.id)}
      onFocus={() => onInspect(node.id)}
    >
      <circle className="site-hit" cx={x} cy={y} r={r + 10} />
      <circle className="site-focus" cx={x} cy={y} r={r + 6} />
      <Glyph kind={node.kind} x={x} y={y} r={r} />
      {state === "FAILED" && (
        <path
          className="site-cross"
          d={`M${x - r * 0.45} ${y - r * 0.45}L${x + r * 0.45} ${y + r * 0.45}M${x + r * 0.45} ${y - r * 0.45}L${x - r * 0.45} ${y + r * 0.45}`}
        />
      )}
      <SiteLabel node={node} r={r} customers={customers} affected={impact?.affected ?? 0} />
    </g>
  );
});

function Glyph({ kind, x, y, r }: { kind: NetNode["kind"]; x: number; y: number; r: number }) {
  switch (kind) {
    case "CORE":
      return (
        <>
          <circle className="glyph glyph-ring" cx={x} cy={y} r={r + 5} />
          <circle className="glyph" cx={x} cy={y} r={r} />
        </>
      );
    case "AGGREGATION":
      return <rect className="glyph" x={x - r} y={y - r} width={r * 2} height={r * 2} rx={3} />;
    case "ACCESS":
      return (
        <rect
          className="glyph"
          x={x - r}
          y={y - r}
          width={r * 2}
          height={r * 2}
          transform={`rotate(45 ${x} ${y})`}
        />
      );
    case "CABINET":
      return <circle className="glyph" cx={x} cy={y} r={r} />;
  }
}

function SiteLabel({
  node,
  r,
  customers,
  affected,
}: {
  node: NetNode;
  r: number;
  customers: number;
  affected: number;
}) {
  const { x, y } = node;
  switch (node.kind) {
    case "CORE":
      return (
        <text className="site-label site-label-strong" x={x} y={y - r - 14} textAnchor="middle">
          {node.name}
        </text>
      );
    case "AGGREGATION":
    case "ACCESS":
      return (
        <text className="site-label" x={x} y={y + r + 22} textAnchor="middle">
          {node.area}
        </text>
      );
    case "CABINET":
      return (
        <text
          className={`site-count${affected ? " site-count-down" : ""}`}
          x={x}
          y={y + r + 13}
          textAnchor="middle"
        >
          {count(affected || customers)}
        </text>
      );
  }
}

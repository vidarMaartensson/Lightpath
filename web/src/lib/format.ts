import type { NodeKind, NodeState, Outage } from "../types";

const numberFormat = new Intl.NumberFormat("en-GB");

export const count = (n: number) => numberFormat.format(n);

export const customers = (n: number) => `${count(n)} ${n === 1 ? "customer" : "customers"}`;

export const kindLabel: Record<NodeKind, string> = {
  CORE: "Core site",
  AGGREGATION: "Aggregation site",
  ACCESS: "Access site",
  CABINET: "Street cabinet",
};

export const stateLabel: Record<NodeState, string> = {
  ONLINE: "Online",
  REROUTED: "Online on a backup path",
  ISOLATED: "Cut off from the core",
  FAILED: "Down",
};

/** One segment of the headline sentence. `tone` colours the numbers. */
export interface HeadlinePart {
  text: string;
  tone?: "down" | "rerouted";
}

/**
 * The sentence at the top of the page. It answers the operator's first
 * question in plain words, so it changes shape with the situation rather
 * than showing zeros.
 */
export function headline(o: Outage): HeadlinePart[] {
  const { affectedCustomers: down, reroutedCustomers: rerouted, totalCustomers: total } = o;

  if (o.failedNodeIds.length === 0) {
    return [{ text: `All ${customers(total)} have service.` }];
  }
  if (down === 0 && rerouted === 0) {
    return [{ text: "Nobody is affected. The failed sites serve no customers." }];
  }
  if (down === 0) {
    return [
      { text: "Nobody has lost service. " },
      { text: customers(rerouted), tone: "rerouted" },
      { text: " are running on backup paths." },
    ];
  }
  const parts: HeadlinePart[] = [
    { text: customers(down), tone: "down" },
    { text: down === 1 ? " has lost service." : " have lost service." },
  ];
  if (rerouted > 0) {
    parts.push(
      { text: " " },
      { text: count(rerouted), tone: "rerouted" },
      { text: " more stay online only because traffic found another path." },
    );
  }
  return parts;
}

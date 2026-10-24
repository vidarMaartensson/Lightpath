// Works out which fibre carries light, for drawing only. The numbers on
// screen always come from the impact service; this file decides what the
// map looks like, using the same rules: paths start at the cores, link
// order picks the primary uplink, and street cabinets never carry transit.

import type { Link, NetNode, Network, NodeState } from "../types";

/** How a link is drawn. */
export type LinkState =
  /** Carries traffic on its normal path. */
  | "active"
  /** Carries traffic that has been rerouted around a failure. */
  | "rerouted"
  /** Spare fibre, lit but idle. */
  | "standby"
  /** One end has no service. */
  | "dead";

export interface DrawnLink {
  link: Link;
  /** Upstream end: light travels from here. */
  from: NetNode;
  to: NetNode;
  state: LinkState;
}

/** Breadth-first tree from the cores. Returns child id -> link that feeds it. */
export function feedTree(network: Network, down: ReadonlySet<string>): Map<string, Link> {
  const byId = new Map(network.nodes.map((n) => [n.id, n]));
  const adj = new Map<string, Link[]>();
  for (const l of network.links) {
    (adj.get(l.a) ?? adj.set(l.a, []).get(l.a)!).push(l);
    (adj.get(l.b) ?? adj.set(l.b, []).get(l.b)!).push(l);
  }

  const fed = new Map<string, Link>();
  const seen = new Set<string>();
  const queue: string[] = [];
  for (const n of network.nodes) {
    if (n.kind === "CORE" && !down.has(n.id)) {
      seen.add(n.id);
      queue.push(n.id);
    }
  }
  for (let i = 0; i < queue.length; i++) {
    const cur = queue[i]!;
    if (byId.get(cur)?.kind === "CABINET") continue;
    for (const l of adj.get(cur) ?? []) {
      const next = l.a === cur ? l.b : l.a;
      if (seen.has(next) || down.has(next)) continue;
      seen.add(next);
      fed.set(next, l);
      queue.push(next);
    }
  }
  return fed;
}

/** Decides how every link is drawn, given node states from the server. */
export function drawLinks(network: Network, states: ReadonlyMap<string, NodeState>): DrawnLink[] {
  const byId = new Map(network.nodes.map((n) => [n.id, n]));
  const failed = new Set([...states].filter(([, s]) => s === "FAILED").map(([id]) => id));

  const normal = feedTree(network, new Set());
  const now = failed.size ? feedTree(network, failed) : normal;

  return network.links.map((link) => {
    const a = byId.get(link.a)!;
    const b = byId.get(link.b)!;
    const sa = states.get(a.id) ?? "ONLINE";
    const sb = states.get(b.id) ?? "ONLINE";

    // Orient the link the way light flows: towards the node it feeds.
    const feedsB = now.get(b.id) === link ? true : now.get(a.id) === link ? false : normal.get(a.id) !== link;
    const [from, to] = feedsB ? [a, b] : [b, a];
    const toState = feedsB ? sb : sa;

    let state: LinkState;
    if (sa === "FAILED" || sb === "FAILED" || sa === "ISOLATED" || sb === "ISOLATED") {
      state = "dead";
    } else if (now.get(to.id) !== link) {
      state = "standby";
    } else if (normal.get(to.id) !== link || toState === "REROUTED") {
      state = "rerouted";
    } else {
      state = "active";
    }
    return { link, from, to, state };
  });
}

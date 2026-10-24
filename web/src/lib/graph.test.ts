import { describe, expect, it } from "vitest";
import type { Network, NodeState } from "../types";
import { drawLinks, feedTree } from "./graph";

// core -> accA -> cab, with a spare fibre accB -> cab and core -> accB.
const net: Network = {
  nodes: [
    { id: "core", name: "", area: "", kind: "CORE", x: 0, y: 0 },
    { id: "accA", name: "", area: "", kind: "ACCESS", x: 0, y: 0 },
    { id: "accB", name: "", area: "", kind: "ACCESS", x: 0, y: 0 },
    { id: "cab", name: "", area: "", kind: "CABINET", x: 0, y: 0 },
  ],
  links: [
    { id: "core-accA", a: "core", b: "accA" },
    { id: "core-accB", a: "core", b: "accB" },
    { id: "accA-cab", a: "accA", b: "cab" },
    { id: "accB-cab", a: "accB", b: "cab" },
  ],
  isps: [],
};

const states = (s: Record<string, NodeState>) => new Map(Object.entries(s));
const byId = (links: ReturnType<typeof drawLinks>) => Object.fromEntries(links.map((l) => [l.link.id, l]));

describe("feedTree", () => {
  it("uses the first listed uplink as the primary path", () => {
    expect(feedTree(net, new Set()).get("cab")?.id).toBe("accA-cab");
  });

  it("does not route through a cabinet", () => {
    const leafOnly: Network = { ...net, links: net.links.filter((l) => l.id !== "core-accB") };
    expect(feedTree(leafOnly, new Set()).has("accB")).toBe(false);
  });
});

describe("drawLinks", () => {
  it("lights the normal path and leaves spare fibre on standby", () => {
    const links = byId(drawLinks(net, new Map()));
    expect(links["accA-cab"]?.state).toBe("active");
    expect(links["accB-cab"]?.state).toBe("standby");
    expect(links["accA-cab"]?.from.id).toBe("accA");
  });

  it("shows traffic moving to the backup path when a site fails", () => {
    const links = byId(
      drawLinks(net, states({ core: "ONLINE", accA: "FAILED", accB: "ONLINE", cab: "REROUTED" })),
    );
    expect(links["core-accA"]?.state).toBe("dead");
    expect(links["accA-cab"]?.state).toBe("dead");
    expect(links["accB-cab"]?.state).toBe("rerouted");
    expect(links["accB-cab"]?.from.id).toBe("accB");
    expect(links["core-accB"]?.state).toBe("active");
  });
});

import { describe, expect, it } from "vitest";
import type { Outage } from "../types";
import { headline } from "./format";

const outage = (o: Partial<Outage>): Outage => ({
  failedNodeIds: ["x"],
  totalCustomers: 1526,
  affectedCustomers: 0,
  reroutedCustomers: 0,
  stale: false,
  isps: [],
  nodes: [],
  ...o,
});

const text = (o: Outage) =>
  headline(o)
    .map((p) => p.text)
    .join("");

describe("headline", () => {
  it("says everyone has service when nothing is down", () => {
    expect(text(outage({ failedNodeIds: [] }))).toBe("All 1,526 customers have service.");
  });

  it("leads with lost service and mentions rerouted customers", () => {
    const parts = headline(outage({ affectedCustomers: 76, reroutedCustomers: 104 }));
    expect(parts[0]).toEqual({ text: "76 customers", tone: "down" });
    expect(parts.map((p) => p.text).join("")).toContain("104 more stay online");
  });

  it("uses the singular for one customer", () => {
    expect(text(outage({ affectedCustomers: 1 }))).toBe("1 customer has lost service.");
  });

  it("reassures when redundancy absorbed the failure", () => {
    expect(text(outage({ reroutedCustomers: 1526 }))).toBe(
      "Nobody has lost service. 1,526 customers are running on backup paths.",
    );
  });
});

import { describe, expect, it } from "vitest";
import { initialMode, viewQuery } from "./url";

describe("initialMode", () => {
  it.each([
    ["live is on and the link is plain", "ON", "", "live"],
    ["live is off", "OFF", "", "sandbox"],
    ["live is off, even if the link asks for it", "OFF", "?mode=live", "sandbox"],
    ["a shared scenario", "ON", "?down=agg-cen", "sandbox"],
    ["the link asks for the sandbox", "ON", "?mode=sandbox", "sandbox"],
    ["the link asks for live over a scenario", "ON", "?down=agg-cen&mode=live", "live"],
    // Live may come back; showing it with a notice beats hiding it.
    ["live is not responding", "UNAVAILABLE", "", "live"],
  ] as const)("opens the right view when %s", (_, status, search, want) => {
    expect(initialMode(status, search)).toBe(want);
  });
});

describe("viewQuery", () => {
  it("writes the sandbox scenario and mode", () => {
    expect(viewQuery("", { mode: "sandbox", down: ["agg-cen", "acc-ali"], liveAvailable: true })).toBe(
      "?down=agg-cen,acc-ali&mode=sandbox",
    );
  });

  it("leaves the mode out when there is no live view", () => {
    expect(viewQuery("", { mode: "sandbox", down: ["agg-cen"], liveAvailable: false })).toBe("?down=agg-cen");
    expect(viewQuery("?down=agg-cen", { mode: "sandbox", down: [], liveAvailable: false })).toBe("");
  });

  it("clears the scenario in live mode", () => {
    expect(
      viewQuery("?down=agg-cen&mode=sandbox", { mode: "live", down: ["acc-ali"], liveAvailable: true }),
    ).toBe("");
  });

  it("keeps parameters it does not own", () => {
    expect(viewQuery("?ref=mail", { mode: "live", down: [], liveAvailable: true })).toBe("?ref=mail");
  });
});

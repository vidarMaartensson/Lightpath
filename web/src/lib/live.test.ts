import { describe, expect, it } from "vitest";
import {
  connectionAfterError,
  feedLine,
  feedReducer,
  feedTime,
  MAX_FEED,
  parseLiveMessage,
  retryDelay,
  type FeedEntry,
  type LiveCause,
} from "./live";

const cause = (id: string, over: Partial<LiveCause> = {}): LiveCause => ({
  id,
  kind: "SITE_DOWN",
  nodeId: "acc-ali",
  at: "2026-10-24T14:02:00Z",
  source: "simulator",
  ...over,
});

describe("parseLiveMessage", () => {
  it("reads a snapshot without a cause", () => {
    expect(parseLiveMessage('{"seq":0,"down":[]}')).toEqual({ seq: 0, down: [] });
  });

  it("reads a change with its cause", () => {
    const data = JSON.stringify({ seq: 2, down: ["acc-ali"], cause: cause("sim-1-2") });
    expect(parseLiveMessage(data)).toEqual({ seq: 2, down: ["acc-ali"], cause: cause("sim-1-2") });
  });

  it.each([
    ["not JSON", "{"],
    ["not an object", "42"],
    ["null", "null"],
    ["missing seq", '{"down":[]}'],
    ["down not a list", '{"seq":1,"down":"acc-ali"}'],
    ["down with a number", '{"seq":1,"down":[1]}'],
    ["cause not an object", '{"seq":1,"down":[],"cause":"x"}'],
    ["unknown kind", JSON.stringify({ seq: 1, down: [], cause: { ...cause("e"), kind: "SITE_ON_FIRE" } })],
    ["time that is not a time", JSON.stringify({ seq: 1, down: [], cause: { ...cause("e"), at: "soon" } })],
    ["cause without an id", JSON.stringify({ seq: 1, down: [], cause: { ...cause("e"), id: 7 } })],
  ])("rejects %s", (_, data) => {
    expect(parseLiveMessage(data)).toBeNull();
  });
});

describe("feedReducer", () => {
  it("puts the newest event first, with its numbers pending", () => {
    let feed = feedReducer([], { type: "event", cause: cause("e1") });
    feed = feedReducer(feed, { type: "event", cause: cause("e2", { nodeId: "cab-teg-1" }) });
    expect(feed.map((e) => e.id)).toEqual(["e2", "e1"]);
    expect(feed[0]?.change).toEqual({ status: "pending" });
  });

  it("keeps at most MAX_FEED entries, dropping the oldest", () => {
    let feed: FeedEntry[] = [];
    for (let i = 0; i < MAX_FEED + 5; i++) feed = feedReducer(feed, { type: "event", cause: cause(`e${i}`) });
    expect(feed).toHaveLength(MAX_FEED);
    expect(feed[0]?.id).toBe(`e${MAX_FEED + 4}`);
    expect(feed.at(-1)?.id).toBe("e5");
  });

  it("ignores an event it already has", () => {
    const feed = feedReducer([], { type: "event", cause: cause("e1") });
    expect(feedReducer(feed, { type: "event", cause: cause("e1") })).toBe(feed);
  });

  it("fills in the numbers when they arrive", () => {
    let feed = feedReducer([], { type: "event", cause: cause("e1") });
    feed = feedReducer(feed, { type: "event", cause: cause("e2") });
    feed = feedReducer(feed, { type: "change", id: "e1", change: { lost: 76, regained: 0, rerouted: 104 } });
    expect(feed[1]?.change).toEqual({ status: "ready", change: { lost: 76, regained: 0, rerouted: 104 } });
    expect(feed[0]?.change).toEqual({ status: "pending" });
  });

  it("marks numbers that could not be fetched as failed, not zero", () => {
    let feed = feedReducer([], { type: "event", cause: cause("e1") });
    feed = feedReducer(feed, { type: "changeFailed", id: "e1" });
    expect(feed[0]?.change).toEqual({ status: "failed" });
  });

  it("ignores numbers for an entry that has left the feed", () => {
    const feed = feedReducer([], { type: "event", cause: cause("e1") });
    expect(
      feedReducer(feed, { type: "change", id: "gone", change: { lost: 1, regained: 0, rerouted: 0 } }),
    ).toBe(feed);
  });
});

describe("feedLine", () => {
  const entry = (over: Partial<FeedEntry>): FeedEntry => ({
    id: "e1",
    kind: "SITE_DOWN",
    nodeId: "acc-ali",
    at: "2026-10-24T14:02:00Z",
    change: { status: "pending" },
    ...over,
  });
  const ready = (lost: number, regained: number, rerouted: number) =>
    ({ status: "ready", change: { lost, regained, rerouted } }) as const;

  it.each([
    // The scenario from the plan: Ålidhem goes down, then comes back.
    [
      "a site going down",
      entry({ change: ready(76, 0, 104) }),
      {
        what: "Ålidhem access site went down",
        effect: "76 customers lost service, 104 on backup paths.",
        tone: "down",
      },
    ],
    [
      "the same site coming back",
      entry({ kind: "SITE_RESTORED", change: ready(0, 76, 0) }),
      { what: "Ålidhem access site is back", effect: "76 customers have service again.", tone: "restored" },
    ],
    [
      "a failure the backup paths absorb",
      entry({ change: ready(0, 0, 1526) }),
      { what: "Ålidhem access site went down", effect: "1,526 on backup paths.", tone: undefined },
    ],
    [
      "one customer",
      entry({ change: ready(1, 0, 0) }),
      { what: "Ålidhem access site went down", effect: "1 customer lost service.", tone: "down" },
    ],
    [
      "nothing changes",
      entry({ change: ready(0, 0, 0) }),
      { what: "Ålidhem access site went down", effect: "No change in service." },
    ],
    [
      "numbers on the way",
      entry({}),
      { what: "Ålidhem access site went down", effect: "Counting customers…" },
    ],
    [
      "numbers that failed",
      entry({ change: { status: "failed" } }),
      { what: "Ålidhem access site went down", effect: "Couldn't count the customers affected." },
    ],
  ])("describes %s", (_, e, want) => {
    expect(feedLine(e, "Ålidhem access site")).toEqual(want);
  });
});

describe("feedTime", () => {
  it("shows hours and minutes", () => {
    expect(feedTime("2026-10-24T14:02:00Z", "UTC")).toBe("14:02");
    expect(feedTime("2026-10-24T14:02:00Z", "Europe/Stockholm")).toBe("16:02");
  });
});

describe("reconnecting", () => {
  it("lets the browser retry while it still is", () => {
    expect(connectionAfterError(0)).toBe("reconnecting");
  });

  it("takes over once the browser has given up", () => {
    expect(connectionAfterError(2)).toBe("unavailable");
  });

  it("backs off from 5 s to at most a minute", () => {
    expect([0, 1, 2, 3, 4, 10].map(retryDelay)).toEqual([5000, 10_000, 20_000, 40_000, 60_000, 60_000]);
  });
});

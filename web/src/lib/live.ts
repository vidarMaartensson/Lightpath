// The live view: messages from /live, the event feed built from them, and
// the rules for reconnecting. Pure functions, so useLive stays a thin
// wrapper around EventSource and everything here can be tested.

import type { CustomerChange } from "../types";
import { count, customers } from "./format";

export type EventKind = "SITE_DOWN" | "SITE_RESTORED";

export interface LiveCause {
  id: string;
  kind: EventKind;
  nodeId: string;
  /** RFC 3339. */
  at: string;
  source: string;
}

/** One SSE "state" event: the full state, and the event behind it. */
export interface LiveMessage {
  seq: number;
  down: string[];
  /** Absent on the first message after connecting, which is a snapshot. */
  cause?: LiveCause;
}

const isString = (v: unknown): v is string => typeof v === "string";

/**
 * Parses and checks a message from /live. Returns null for anything that
 * does not have the expected shape: data from another service is not
 * trusted just because it arrived.
 */
export function parseLiveMessage(data: string): LiveMessage | null {
  let raw: unknown;
  try {
    raw = JSON.parse(data);
  } catch {
    return null;
  }
  if (typeof raw !== "object" || raw === null) return null;
  const { seq, down, cause } = raw as Record<string, unknown>;
  if (typeof seq !== "number" || !Array.isArray(down) || !down.every(isString)) return null;

  const msg: LiveMessage = { seq, down };
  if (cause === undefined) return msg;

  if (typeof cause !== "object" || cause === null) return null;
  const c = cause as Record<string, unknown>;
  if (!isString(c.id) || !isString(c.nodeId) || !isString(c.at) || !isString(c.source)) return null;
  if (c.kind !== "SITE_DOWN" && c.kind !== "SITE_RESTORED") return null;
  if (Number.isNaN(Date.parse(c.at))) return null;
  msg.cause = { id: c.id, kind: c.kind, nodeId: c.nodeId, at: c.at, source: c.source };
  return msg;
}

/** The numbers for a feed entry arrive after the entry itself. */
export type ChangeState =
  { status: "pending" } | { status: "ready"; change: CustomerChange } | { status: "failed" };

export interface FeedEntry {
  id: string;
  kind: EventKind;
  nodeId: string;
  at: string;
  change: ChangeState;
}

export type FeedAction =
  | { type: "event"; cause: LiveCause }
  | { type: "change"; id: string; change: CustomerChange }
  | { type: "changeFailed"; id: string };

export const MAX_FEED = 20;

/** Newest first, at most MAX_FEED entries, each event once. */
export function feedReducer(feed: FeedEntry[], action: FeedAction): FeedEntry[] {
  switch (action.type) {
    case "event": {
      const { id, kind, nodeId, at } = action.cause;
      if (feed.some((e) => e.id === id)) return feed;
      const entry: FeedEntry = { id, kind, nodeId, at, change: { status: "pending" } };
      return [entry, ...feed].slice(0, MAX_FEED);
    }
    case "change":
      return update(feed, action.id, { status: "ready", change: action.change });
    case "changeFailed":
      return update(feed, action.id, { status: "failed" });
  }
}

function update(feed: FeedEntry[], id: string, change: ChangeState): FeedEntry[] {
  // An entry that has scrolled out of the feed is simply not there any more.
  if (!feed.some((e) => e.id === id)) return feed;
  return feed.map((e) => (e.id === id ? { ...e, change } : e));
}

export interface FeedLine {
  what: string;
  effect: string;
  tone?: "down" | "restored";
}

/** The two halves of a feed row, in plain words. */
export function feedLine(entry: FeedEntry, nodeName: string): FeedLine {
  const what = entry.kind === "SITE_DOWN" ? `${nodeName} went down` : `${nodeName} is back`;
  const c = entry.change;
  if (c.status === "pending") return { what, effect: "Counting customers…" };
  if (c.status === "failed") return { what, effect: "Couldn't count the customers affected." };

  const { lost, regained, rerouted } = c.change;
  const parts: string[] = [];
  if (lost > 0) parts.push(`${customers(lost)} lost service`);
  if (regained > 0) parts.push(`${customers(regained)} have service again`);
  if (rerouted > 0) parts.push(`${count(rerouted)} on backup paths`);
  if (parts.length === 0) return { what, effect: "No change in service." };

  const sentence = parts.join(", ") + ".";
  const effect = sentence.charAt(0).toUpperCase() + sentence.slice(1);
  const tone = lost > 0 ? "down" : regained > 0 ? "restored" : undefined;
  return { what, effect, tone };
}

/** "14:02". timeZone is for tests; the browser's own zone is the default. */
export function feedTime(at: string, timeZone?: string): string {
  return new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", timeZone }).format(
    new Date(at),
  );
}

export type Connection = "connecting" | "live" | "reconnecting" | "unavailable";

const EVENT_SOURCE_CLOSED = 2;

/**
 * What an EventSource error means. While the browser is still trying it is
 * reconnecting by itself. Once it has closed, which it does when /live
 * answers anything but 200, it will not try again and we must.
 */
export function connectionAfterError(readyState: number): Connection {
  return readyState === EVENT_SOURCE_CLOSED ? "unavailable" : "reconnecting";
}

/** Backoff for our own reconnects: 5 s, doubling, at most a minute. */
export function retryDelay(attempt: number): number {
  return Math.min(5000 * 2 ** attempt, 60_000);
}

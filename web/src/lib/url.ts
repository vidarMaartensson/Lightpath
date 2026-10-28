// The sandbox's failed sites live in the URL (?down=acc-ali,agg-ost), so a
// scenario can be shared as a link. The live view has no URL state: what
// is down there is whatever is down right now.

import type { LiveStatus } from "../types";

export type Mode = "live" | "sandbox";

export function readDownFromUrl(): string[] {
  const raw = new URLSearchParams(window.location.search).get("down");
  return raw ? [...new Set(raw.split(",").filter(Boolean))] : [];
}

/**
 * Which view to open. Live when it exists, except when the link asks for
 * the sandbox or carries a scenario: a shared ?down= link must show that
 * scenario, as it did before there was a live view.
 */
export function initialMode(status: LiveStatus, search: string): Mode {
  if (status === "OFF") return "sandbox";
  const params = new URLSearchParams(search);
  const mode = params.get("mode");
  if (mode === "sandbox" || mode === "live") return mode;
  return params.get("down") ? "sandbox" : "live";
}

export interface View {
  mode: Mode;
  down: string[];
  /** Without a live view there is only one mode, so it is not written. */
  liveAvailable: boolean;
}

/** The query string for a view, keeping any other parameters. */
export function viewQuery(search: string, view: View): string {
  const params = new URLSearchParams(search);
  params.delete("down");
  params.delete("mode");
  if (view.mode === "sandbox") {
    if (view.down.length) params.set("down", view.down.join(","));
    if (view.liveAvailable) params.set("mode", "sandbox");
  }
  const qs = params.toString().replaceAll("%2C", ",");
  return qs ? `?${qs}` : "";
}

export function writeViewToUrl(view: View) {
  const next = `${window.location.pathname}${viewQuery(window.location.search, view)}`;
  window.history.replaceState(null, "", next);
}

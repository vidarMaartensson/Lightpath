// The failed sites live in the URL (?down=acc-ali,agg-ost), so a scenario
// can be shared as a link.

export function readDownFromUrl(): string[] {
  const raw = new URLSearchParams(window.location.search).get("down");
  return raw ? [...new Set(raw.split(",").filter(Boolean))] : [];
}

export function writeDownToUrl(down: string[]) {
  const params = new URLSearchParams(window.location.search);
  if (down.length) params.set("down", down.join(","));
  else params.delete("down");
  const qs = params.toString();
  const next = `${window.location.pathname}${qs ? `?${qs.replaceAll("%2C", ",")}` : ""}`;
  window.history.replaceState(null, "", next);
}

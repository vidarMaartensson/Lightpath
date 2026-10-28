import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, fetchLiveStatus, fetchNetwork, fetchOutage } from "./api";
import { EventFeed } from "./components/EventFeed";
import { Headline } from "./components/Headline";
import { ModeToggle } from "./components/ModeToggle";
import { NetworkMap } from "./components/NetworkMap";
import { NodeDetail } from "./components/NodeDetail";
import { ProviderList } from "./components/ProviderList";
import { Scenarios } from "./components/Scenarios";
import { ThemeToggle } from "./components/ThemeToggle";
import { initialMode, readDownFromUrl, writeViewToUrl, type Mode } from "./lib/url";
import { useLive } from "./lib/useLive";
import type { LiveStatus, Network, NodeImpact, NodeState, Outage } from "./types";

type Load<T> = { status: "loading" } | { status: "ready"; data: T } | { status: "error"; message: string };

export function App() {
  const [network, setNetwork] = useState<Load<Network>>({ status: "loading" });
  const [liveStatus, setLiveStatus] = useState<LiveStatus | null>(null);
  const [mode, setMode] = useState<Mode | null>(null);
  const [sandboxDown, setSandboxDown] = useState<string[]>(readDownFromUrl);
  const [outage, setOutage] = useState<Outage | null>(null);
  const [outageError, setOutageError] = useState<string | null>(null);
  const [updating, setUpdating] = useState(false);
  const [inspected, setInspected] = useState<string | null>(null);
  const [showLiveHint, setShowLiveHint] = useState(false);

  useEffect(() => {
    const ctrl = new AbortController();
    fetchNetwork(ctrl.signal)
      .then((data) => setNetwork({ status: "ready", data }))
      .catch((err) => {
        if (!ctrl.signal.aborted) setNetwork({ status: "error", message: messageOf(err) });
      });
    return () => ctrl.abort();
  }, []);

  // Whether there is a live view decides which view opens. If the gateway
  // cannot say, the app opens as the sandbox it always was.
  useEffect(() => {
    const ctrl = new AbortController();
    fetchLiveStatus(ctrl.signal)
      .catch(() => "OFF" as const)
      .then((status) => {
        if (ctrl.signal.aborted) return;
        setLiveStatus(status);
        setMode(initialMode(status, window.location.search));
      });
    return () => ctrl.abort();
  }, []);

  const liveAvailable = liveStatus !== null && liveStatus !== "OFF";
  // The stream stays open in the sandbox too, so the feed keeps up and
  // switching back is instant.
  const live = useLive(liveAvailable);
  const down = mode === "live" ? live.down : sandboxDown;
  // The live view sends a new list with every message; compare by content.
  const downKey = down.join(",");

  useEffect(() => {
    if (mode) writeViewToUrl({ mode, down: sandboxDown, liveAvailable });
  }, [mode, sandboxDown, liveAvailable]);

  // Re-analyse whenever the set of failed sites changes. The previous
  // answer stays on screen until the new one arrives, so the map never
  // flashes empty between clicks.
  useEffect(() => {
    if (!mode) return;
    const failed = downKey ? downKey.split(",") : [];
    const ctrl = new AbortController();
    setUpdating(true);
    fetchOutage(failed, ctrl.signal)
      .then((o) => {
        setOutage(o);
        setOutageError(null);
      })
      .catch((err) => {
        if (!ctrl.signal.aborted) setOutageError(messageOf(err));
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setUpdating(false);
      });
    return () => ctrl.abort();
  }, [mode, downKey]);

  const toggle = useCallback(
    (id: string) => {
      setInspected(id);
      // The live map follows the network; clicking explains how to experiment.
      if (mode === "live") {
        setShowLiveHint(true);
        return;
      }
      setSandboxDown((cur) => (cur.includes(id) ? cur.filter((d) => d !== id) : [...cur, id]));
    },
    [mode],
  );

  // The sandbox starts from what is down live, to ask "what if X fails too?"
  const changeMode = useCallback(
    (next: Mode) => {
      if (next === "sandbox" && mode === "live") setSandboxDown(live.down);
      setShowLiveHint(false);
      setMode(next);
    },
    [mode, live.down],
  );

  const impacts = useMemo(() => {
    const m = new Map<string, NodeImpact>();
    for (const n of outage?.nodes ?? []) m.set(n.nodeId, n);
    return m;
  }, [outage]);

  const states = useMemo(() => {
    const m = new Map<string, NodeState>();
    for (const n of outage?.nodes ?? []) m.set(n.nodeId, n.state);
    return m;
  }, [outage]);

  if (network.status === "loading" || mode === null) {
    return <div className="state-screen">Loading the network…</div>;
  }
  if (network.status === "error") {
    return (
      <div className="state-screen">
        <p className="state-title">The network could not be loaded.</p>
        <p>{network.message}</p>
        <p className="hint">
          Start the backend with <code>make up</code>, then reload.
        </p>
      </div>
    );
  }

  const net = network.data;
  const nodesById = new Map(net.nodes.map((n) => [n.id, n]));
  const inspectedNode = inspected ? nodesById.get(inspected) : undefined;
  const isLive = mode === "live";
  const liveDown = isLive && (liveStatus === "UNAVAILABLE" || live.connection === "unavailable");

  return (
    <div className="app">
      <header className="masthead">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          Lightpath
        </div>
        <p className="brand-tag">Outage impact on an open access fibre network</p>
        <div className="masthead-controls">
          {liveAvailable && <ModeToggle mode={mode} onChange={changeMode} />}
          <ThemeToggle />
        </div>
      </header>

      <section className="lede" aria-live="polite">
        {outage ? <Headline outage={outage} /> : <h1 className="headline">Analysing…</h1>}
        {!isLive && <Scenarios down={sandboxDown} onChoose={setSandboxDown} />}
      </section>

      {liveDown && (
        <div className="notice" role="status">
          The live view isn't responding, so the map may be out of date. The sandbox still works.{" "}
          <button type="button" className="text-button" onClick={() => changeMode("sandbox")}>
            Switch to sandbox
          </button>
        </div>
      )}

      {showLiveHint && isLive && !liveDown && (
        <div className="notice notice-info" role="status">
          The map follows the live network. Switch to sandbox to take sites down yourself.{" "}
          <button type="button" className="text-button" onClick={() => changeMode("sandbox")}>
            Switch to sandbox
          </button>
        </div>
      )}

      {(outageError || outage?.stale) && (
        <div className="notice" role="status">
          {outageError
            ? `Couldn't update the analysis. ${outageError}`
            : "The topology service isn't responding. Showing results from the last known network."}
        </div>
      )}

      <main className="workspace">
        <NetworkMap
          network={net}
          states={states}
          impacts={impacts}
          down={down}
          updating={updating}
          live={isLive}
          onToggle={toggle}
          onInspect={setInspected}
        />

        <aside className="side">
          {isLive && <EventFeed feed={live.feed} connection={live.connection} nodesById={nodesById} />}

          <ProviderList network={net} outage={outage} />

          <section className="panel" aria-labelledby="down-title">
            <h2 id="down-title">Sites down</h2>
            {down.length === 0 ? (
              <p className="empty">
                {isLive ? "Nothing is down right now." : "Select a site on the map to take it down."}
              </p>
            ) : (
              <ul className="down-list">
                {down.map((id) => (
                  <li key={id}>
                    <span>{nodesById.get(id)?.name ?? id}</span>
                    {!isLive && (
                      <button type="button" className="text-button" onClick={() => toggle(id)}>
                        Restore
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            )}
            {!isLive && down.length > 1 && (
              <button type="button" className="text-button restore-all" onClick={() => setSandboxDown([])}>
                Restore all sites
              </button>
            )}
          </section>

          <NodeDetail node={inspectedNode} impact={inspected ? impacts.get(inspected) : undefined} />
        </aside>
      </main>
    </div>
  );
}

function messageOf(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  return "Something unexpected went wrong.";
}

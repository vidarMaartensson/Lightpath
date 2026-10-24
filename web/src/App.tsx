import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, fetchNetwork, fetchOutage } from "./api";
import { Headline } from "./components/Headline";
import { NetworkMap } from "./components/NetworkMap";
import { NodeDetail } from "./components/NodeDetail";
import { ProviderList } from "./components/ProviderList";
import { Scenarios } from "./components/Scenarios";
import { readDownFromUrl, writeDownToUrl } from "./lib/url";
import type { Network, NodeImpact, NodeState, Outage } from "./types";

type Load<T> = { status: "loading" } | { status: "ready"; data: T } | { status: "error"; message: string };

export function App() {
  const [network, setNetwork] = useState<Load<Network>>({ status: "loading" });
  const [down, setDown] = useState<string[]>(readDownFromUrl);
  const [outage, setOutage] = useState<Outage | null>(null);
  const [outageError, setOutageError] = useState<string | null>(null);
  const [updating, setUpdating] = useState(false);
  const [inspected, setInspected] = useState<string | null>(null);

  useEffect(() => {
    const ctrl = new AbortController();
    fetchNetwork(ctrl.signal)
      .then((data) => setNetwork({ status: "ready", data }))
      .catch((err) => {
        if (!ctrl.signal.aborted) setNetwork({ status: "error", message: messageOf(err) });
      });
    return () => ctrl.abort();
  }, []);

  // Re-analyse whenever the set of failed sites changes. The previous
  // answer stays on screen until the new one arrives, so the map never
  // flashes empty between clicks.
  useEffect(() => {
    writeDownToUrl(down);
    const ctrl = new AbortController();
    setUpdating(true);
    fetchOutage(down, ctrl.signal)
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
  }, [down]);

  const toggle = useCallback((id: string) => {
    setDown((cur) => (cur.includes(id) ? cur.filter((d) => d !== id) : [...cur, id]));
    setInspected(id);
  }, []);

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

  if (network.status === "loading") {
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

  return (
    <div className="app">
      <header className="masthead">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          Lightpath
        </div>
        <p className="brand-tag">Outage impact on an open access fibre network</p>
      </header>

      <section className="lede" aria-live="polite">
        {outage ? <Headline outage={outage} /> : <h1 className="headline">Analysing…</h1>}
        <Scenarios down={down} onChoose={setDown} />
      </section>

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
          onToggle={toggle}
          onInspect={setInspected}
        />

        <aside className="side">
          <ProviderList network={net} outage={outage} />

          <section className="panel" aria-labelledby="down-title">
            <h2 id="down-title">Sites down</h2>
            {down.length === 0 ? (
              <p className="empty">Select a site on the map to take it down.</p>
            ) : (
              <ul className="down-list">
                {down.map((id) => (
                  <li key={id}>
                    <span>{nodesById.get(id)?.name ?? id}</span>
                    <button type="button" className="text-button" onClick={() => toggle(id)}>
                      Restore
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {down.length > 1 && (
              <button type="button" className="text-button restore-all" onClick={() => setDown([])}>
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

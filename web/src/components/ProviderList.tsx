import { count } from "../lib/format";
import type { Network, Outage } from "../types";

// Who has to be told. Each bar is one provider's customer base: the red
// part has lost service, the violet part is running on backup paths.
export function ProviderList({ network, outage }: { network: Network; outage: Outage | null }) {
  const names = new Map(network.isps.map((i) => [i.id, i.name]));
  const anyDown = (outage?.affectedCustomers ?? 0) > 0;

  return (
    <section className="panel" aria-labelledby="providers-title">
      <h2 id="providers-title">{anyDown ? "Providers to notify" : "Providers on the network"}</h2>
      <ul className="providers">
        {(outage?.isps ?? []).map((isp) => {
          const pct = (n: number) => (isp.customers ? (n / isp.customers) * 100 : 0);
          return (
            <li key={isp.ispId} className={isp.affected ? "provider provider-down" : "provider"}>
              <div className="provider-row">
                <span className="provider-name">{names.get(isp.ispId) ?? isp.ispId}</span>
                <span className="provider-figure">
                  {isp.affected ? (
                    <>
                      <span className="tone-down">{count(isp.affected)}</span> of {count(isp.customers)} down
                    </>
                  ) : (
                    `${count(isp.customers)} customers`
                  )}
                </span>
              </div>
              <div
                className="provider-bar"
                role="img"
                aria-label={`${count(isp.affected)} without service, ${count(isp.rerouted)} on backup paths, of ${count(isp.customers)} customers`}
              >
                <span className="bar-down" style={{ width: `${pct(isp.affected)}%` }} />
                <span className="bar-rerouted" style={{ width: `${pct(isp.rerouted)}%` }} />
              </div>
              {isp.rerouted > 0 && <p className="provider-note">{count(isp.rerouted)} on backup paths</p>}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

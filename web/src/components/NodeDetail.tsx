import { count, customers, kindLabel, stateLabel } from "../lib/format";
import type { NetNode, NodeImpact } from "../types";

export function NodeDetail({ node, impact }: { node: NetNode | undefined; impact: NodeImpact | undefined }) {
  return (
    <section className="panel detail" aria-labelledby="detail-title" aria-live="polite">
      <h2 id="detail-title">Site</h2>
      {!node ? (
        <p className="empty">Point at a site to see its details.</p>
      ) : (
        <>
          <p className="detail-name">{node.name}</p>
          <p className="detail-meta">
            {kindLabel[node.kind]} in {node.area}
          </p>
          {impact && (
            <dl className="detail-facts">
              <div>
                <dt>Status</dt>
                <dd className={`state-text state-text-${impact.state.toLowerCase()}`}>
                  {stateLabel[impact.state]}
                </dd>
              </div>
              {node.kind === "CABINET" && (
                <>
                  <div>
                    <dt>Customers</dt>
                    <dd>{count(impact.customers)}</dd>
                  </div>
                  {impact.affected > 0 && (
                    <div>
                      <dt>Without service</dt>
                      <dd className="tone-down">{customers(impact.affected)}</dd>
                    </div>
                  )}
                  {impact.rerouted > 0 && (
                    <div>
                      <dt>On a backup path</dt>
                      <dd className="tone-rerouted">{customers(impact.rerouted)}</dd>
                    </div>
                  )}
                </>
              )}
            </dl>
          )}
        </>
      )}
    </section>
  );
}

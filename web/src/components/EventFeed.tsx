import { feedLine, feedTime, type Connection, type FeedEntry } from "../lib/live";
import type { NetNode } from "../types";

const connectionLabel: Record<Connection, string> = {
  connecting: "Connecting…",
  live: "Live",
  reconnecting: "Reconnecting…",
  unavailable: "Not responding",
};

interface Props {
  feed: FeedEntry[];
  connection: Connection;
  nodesById: Map<string, NetNode>;
}

export function EventFeed({ feed, connection, nodesById }: Props) {
  return (
    <section className="panel" aria-labelledby="feed-title">
      <div className="feed-head">
        <h2 id="feed-title">Live events</h2>
        <span className={`live-dot live-dot-${connection}`} role="status">
          {connectionLabel[connection]}
        </span>
      </div>
      {feed.length === 0 ? (
        <p className="empty">Waiting for the first event. Sites fail and recover on their own.</p>
      ) : (
        <ol className="feed">
          {feed.map((entry) => {
            const line = feedLine(entry, nodesById.get(entry.nodeId)?.name ?? entry.nodeId);
            return (
              <li key={entry.id} className="feed-row">
                <time dateTime={entry.at}>{feedTime(entry.at)}</time>
                <span>
                  <span className="feed-what">{line.what}</span>
                  <span className={`feed-effect${line.tone ? ` feed-${line.tone}` : ""}`}>{line.effect}</span>
                </span>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}

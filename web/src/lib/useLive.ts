import { useEffect, useReducer, useRef, useState } from "react";
import { fetchChange } from "../api";
import {
  connectionAfterError,
  feedReducer,
  parseLiveMessage,
  retryDelay,
  type Connection,
  type FeedEntry,
} from "./live";

export interface Live {
  down: string[];
  feed: FeedEntry[];
  connection: Connection;
}

/**
 * Follows /live while enabled. A thin wrapper: what a message means, how
 * the feed is built and when to retry are decided in ./live.
 */
export function useLive(enabled: boolean): Live {
  const [down, setDown] = useState<string[]>([]);
  const [feed, dispatch] = useReducer(feedReducer, []);
  const [connection, setConnection] = useState<Connection>("connecting");
  // The sites down before the latest message, to ask what an event changed.
  const lastDown = useRef<string[] | null>(null);

  useEffect(() => {
    if (!enabled) return;
    const ctrl = new AbortController();
    let source: EventSource | null = null;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;

    const open = () => {
      const es = new EventSource("/live");
      source = es;
      es.onopen = () => {
        attempt = 0;
        setConnection("live");
      };
      es.addEventListener("state", (ev) => {
        const msg = parseLiveMessage((ev as MessageEvent<string>).data);
        if (!msg) return;
        const before = lastDown.current;
        lastDown.current = msg.down;
        setDown(msg.down);
        setConnection("live");
        // The first message after connecting is a snapshot with no cause:
        // it updates the map but is not an event.
        if (!msg.cause || !before) return;
        const { id } = msg.cause;
        dispatch({ type: "event", cause: msg.cause });
        fetchChange(msg.down, before, ctrl.signal)
          .then((change) => dispatch({ type: "change", id, change }))
          .catch(() => {
            if (!ctrl.signal.aborted) dispatch({ type: "changeFailed", id });
          });
      });
      es.onerror = () => {
        const next = connectionAfterError(es.readyState);
        setConnection(next);
        if (next === "unavailable") {
          es.close();
          retry = setTimeout(open, retryDelay(attempt++));
        }
      };
    };

    open();
    return () => {
      ctrl.abort();
      source?.close();
      clearTimeout(retry);
    };
  }, [enabled]);

  return { down, feed, connection };
}

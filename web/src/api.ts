import type { CustomerChange, LiveStatus, Network, Outage } from "./types";

const NETWORK_QUERY = /* GraphQL */ `
  query Network {
    network {
      nodes {
        id
        name
        area
        kind
        x
        y
      }
      links {
        id
        a
        b
      }
      isps {
        id
        name
      }
    }
  }
`;

const OUTAGE_QUERY = /* GraphQL */ `
  query Outage($failed: [ID!]!) {
    outage(failedNodeIds: $failed) {
      failedNodeIds
      totalCustomers
      affectedCustomers
      reroutedCustomers
      stale
      isps {
        ispId
        customers
        affected
        rerouted
      }
      nodes {
        nodeId
        state
        customers
        affected
        rerouted
      }
    }
  }
`;

const LIVE_STATUS_QUERY = /* GraphQL */ `
  query LiveStatus {
    liveStatus
  }
`;

// The effect of one live event: the outage after it, compared with the
// sites that were down just before. The impact service does the counting.
const CHANGE_QUERY = /* GraphQL */ `
  query Change($failed: [ID!]!, $baseline: [ID!]!) {
    outage(failedNodeIds: $failed, baselineNodeIds: $baseline) {
      change {
        lost
        regained
        rerouted
      }
    }
  }
`;

export class ApiError extends Error {}

async function request<T>(query: string, variables: object, signal?: AbortSignal): Promise<T> {
  let res: Response;
  try {
    res = await fetch("/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, variables }),
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError("Can't reach the gateway. Check that the backend is running.");
  }
  if (!res.ok) {
    throw new ApiError(`The gateway answered ${res.status}.`);
  }
  const body = (await res.json()) as { data?: T; errors?: { message: string }[] };
  if (body.errors?.length) {
    throw new ApiError(body.errors.map((e) => e.message).join(" "));
  }
  if (!body.data) {
    throw new ApiError("The gateway returned no data.");
  }
  return body.data;
}

export async function fetchNetwork(signal?: AbortSignal): Promise<Network> {
  const data = await request<{ network: Network }>(NETWORK_QUERY, {}, signal);
  return data.network;
}

export async function fetchOutage(failed: string[], signal?: AbortSignal): Promise<Outage> {
  const data = await request<{ outage: Outage }>(OUTAGE_QUERY, { failed }, signal);
  return data.outage;
}

export async function fetchLiveStatus(signal?: AbortSignal): Promise<LiveStatus> {
  const data = await request<{ liveStatus: LiveStatus }>(LIVE_STATUS_QUERY, {}, signal);
  return data.liveStatus;
}

export async function fetchChange(
  failed: string[],
  baseline: string[],
  signal?: AbortSignal,
): Promise<CustomerChange> {
  const data = await request<{ outage: { change: CustomerChange | null } }>(
    CHANGE_QUERY,
    { failed, baseline },
    signal,
  );
  if (!data.outage.change) {
    throw new ApiError("The gateway returned no change.");
  }
  return data.outage.change;
}

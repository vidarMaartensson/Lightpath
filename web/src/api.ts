import type { Network, Outage } from "./types";

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

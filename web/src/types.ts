// Mirrors internal/gateway/schema.graphql.

export type NodeKind = "CORE" | "AGGREGATION" | "ACCESS" | "CABINET";
export type NodeState = "ONLINE" | "REROUTED" | "ISOLATED" | "FAILED";

export interface NetNode {
  id: string;
  name: string;
  area: string;
  kind: NodeKind;
  x: number;
  y: number;
}

export interface Link {
  id: string;
  a: string;
  b: string;
}

export interface Isp {
  id: string;
  name: string;
}

export interface Network {
  nodes: NetNode[];
  links: Link[];
  isps: Isp[];
}

export interface NodeImpact {
  nodeId: string;
  state: NodeState;
  customers: number;
  affected: number;
  rerouted: number;
}

export interface IspImpact {
  ispId: string;
  customers: number;
  affected: number;
  rerouted: number;
}

export interface Outage {
  failedNodeIds: string[];
  totalCustomers: number;
  affectedCustomers: number;
  reroutedCustomers: number;
  stale: boolean;
  isps: IspImpact[];
  nodes: NodeImpact[];
}

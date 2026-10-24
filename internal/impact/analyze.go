// Package impact answers the question operations ask during an outage:
// with these sites down, which customers have actually lost service, and
// which ISPs need to be told?
//
// The network is treated as an undirected graph. A node has service if
// there is still a path from it to any core site. Street cabinets are
// passive: they can be reached, but traffic never transits through them.
// A customer has service if any of the cabinets it is attached to has
// service. Customers who keep service only because traffic found another
// path are reported as rerouted, not affected.
package impact

import (
	"errors"
	"fmt"
	"sort"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// NodeState describes a node after the failures are applied.
type NodeState string

const (
	// Online: the node's normal path to the core is intact.
	Online NodeState = "ONLINE"
	// Rerouted: the normal path is broken, but another path to a core exists.
	Rerouted NodeState = "REROUTED"
	// Isolated: the node works but has no path to any core.
	Isolated NodeState = "ISOLATED"
	// Failed: the node itself is down.
	Failed NodeState = "FAILED"
)

// ErrUnknownNode is returned when a failed node id is not in the network.
var ErrUnknownNode = errors.New("unknown node")

// NodeImpact summarises one node. Customers are counted at their primary
// cabinet only, so the per-node numbers add up to the totals.
type NodeImpact struct {
	NodeID    string
	State     NodeState
	Customers int
	Affected  int
	Rerouted  int
}

// ISPImpact is what one ISP needs to hear about.
type ISPImpact struct {
	ISPID     string
	Customers int
	Affected  int
	Rerouted  int
}

// Result is the outcome of an outage analysis.
type Result struct {
	FailedNodeIDs     []string
	TotalCustomers    int
	AffectedCustomers int
	ReroutedCustomers int
	// ISPs is sorted by affected customers, most first, then by id.
	ISPs []ISPImpact
	// Nodes has one entry per node, in network order.
	Nodes []NodeImpact
}

// Analyze applies the failed nodes to the network and computes the impact.
// Duplicate ids are ignored. An unknown id is an error rather than being
// silently skipped: an operator who typed the wrong site should find out.
func Analyze(n *network.Network, failedIDs []string) (Result, error) {
	index := make(map[string]int, len(n.Nodes))
	for i, node := range n.Nodes {
		index[node.ID] = i
	}

	failed := make([]bool, len(n.Nodes))
	var failedList []string
	for _, id := range failedIDs {
		i, ok := index[id]
		if !ok {
			return Result{}, fmt.Errorf("%w: %q", ErrUnknownNode, id)
		}
		if !failed[i] {
			failed[i] = true
			failedList = append(failedList, id)
		}
	}
	sort.Strings(failedList)

	adj := adjacency(n, index)

	// The normal path: a breadth-first tree from all cores on the intact
	// network. Neighbours are visited in link order, so a node's first
	// listed uplink is its primary one when two paths are equally short.
	parent := bfs(n, adj, nil)
	// What can still reach a core with the failures applied.
	reach := bfs(n, adj, failed)

	states := make([]NodeState, len(n.Nodes))
	for i := range n.Nodes {
		switch {
		case failed[i]:
			states[i] = Failed
		case reach[i] == unreached:
			states[i] = Isolated
		case pathBroken(i, parent, failed):
			states[i] = Rerouted
		default:
			states[i] = Online
		}
	}

	res := Result{
		FailedNodeIDs:  failedList,
		TotalCustomers: len(n.Customers),
		Nodes:          make([]NodeImpact, len(n.Nodes)),
	}
	for i, node := range n.Nodes {
		res.Nodes[i] = NodeImpact{NodeID: node.ID, State: states[i]}
	}

	byISP := make(map[string]*ISPImpact, len(n.ISPs))
	for _, isp := range n.ISPs {
		byISP[isp.ID] = &ISPImpact{ISPID: isp.ID}
	}

	for _, c := range n.Customers {
		primary := index[c.NodeIDs[0]]
		affected, rerouted := customerOutcome(c, index, states)

		ni := &res.Nodes[primary]
		ni.Customers++
		isp := byISP[c.ISPID]
		isp.Customers++
		switch {
		case affected:
			res.AffectedCustomers++
			ni.Affected++
			isp.Affected++
		case rerouted:
			res.ReroutedCustomers++
			ni.Rerouted++
			isp.Rerouted++
		}
	}

	for _, isp := range n.ISPs {
		res.ISPs = append(res.ISPs, *byISP[isp.ID])
	}
	sort.SliceStable(res.ISPs, func(a, b int) bool {
		if res.ISPs[a].Affected != res.ISPs[b].Affected {
			return res.ISPs[a].Affected > res.ISPs[b].Affected
		}
		return res.ISPs[a].ISPID < res.ISPs[b].ISPID
	})
	return res, nil
}

// customerOutcome: affected if no attachment has service; rerouted if the
// customer has service but not through its primary attachment's normal path.
func customerOutcome(c network.Customer, index map[string]int, states []NodeState) (affected, rerouted bool) {
	served := false
	for _, id := range c.NodeIDs {
		s := states[index[id]]
		if s == Online || s == Rerouted {
			served = true
			break
		}
	}
	if !served {
		return true, false
	}
	return false, states[index[c.NodeIDs[0]]] != Online
}

const (
	unreached = -2
	root      = -1
)

// bfs runs a multi-source breadth-first search from every core that is not
// skipped. It returns each node's parent in the search tree: root for a
// core, unreached for nodes with no path.
func bfs(n *network.Network, adj [][]int, skip []bool) []int {
	parent := make([]int, len(n.Nodes))
	for i := range parent {
		parent[i] = unreached
	}
	var queue []int
	for i, node := range n.Nodes {
		if node.Kind == network.KindCore && (skip == nil || !skip[i]) {
			parent[i] = root
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if n.Nodes[cur].Kind == network.KindCabinet {
			continue // passive: reachable, but not a transit point
		}
		for _, next := range adj[cur] {
			if parent[next] != unreached || (skip != nil && skip[next]) {
				continue
			}
			parent[next] = cur
			queue = append(queue, next)
		}
	}
	return parent
}

// pathBroken reports whether a node's normal path to the core passes
// through a failed node.
func pathBroken(i int, parent []int, failed []bool) bool {
	for cur := parent[i]; cur >= 0; cur = parent[cur] {
		if failed[cur] {
			return true
		}
	}
	return false
}

func adjacency(n *network.Network, index map[string]int) [][]int {
	adj := make([][]int, len(n.Nodes))
	for _, l := range n.Links {
		a, b := index[l.A], index[l.B]
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	return adj
}

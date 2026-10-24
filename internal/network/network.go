// Package network holds the domain model for a fibre access network:
// physical sites (nodes), the fibre between them (links), the ISPs selling
// services on the open access network, and the customers attached to it.
package network

import (
	"errors"
	"fmt"
)

// NodeKind is a node's role in the network hierarchy, from the core outwards.
type NodeKind string

const (
	KindCore        NodeKind = "CORE"
	KindAggregation NodeKind = "AGGREGATION"
	KindAccess      NodeKind = "ACCESS"
	KindCabinet     NodeKind = "CABINET"
)

// Valid reports whether k is one of the known node kinds.
func (k NodeKind) Valid() bool {
	switch k {
	case KindCore, KindAggregation, KindAccess, KindCabinet:
		return true
	}
	return false
}

// Node is a physical site: a core data centre, an aggregation or access
// site, or a street cabinet that customers are spliced into.
type Node struct {
	ID   string
	Name string
	Area string
	Kind NodeKind
	// X and Y place the node on a schematic map (0..1000 by 0..620).
	X, Y float64
}

// Link is an undirected fibre connection between two nodes.
type Link struct {
	ID string
	A  string
	B  string
}

// ISP is a service provider selling subscriptions on the open access network.
type ISP struct {
	ID   string
	Name string
}

// Customer is a subscription. NodeIDs lists the cabinets the customer is
// attached to, primary first. Most homes have one; dual-homed business
// customers have two.
type Customer struct {
	ID       string
	ISPID    string
	NodeIDs  []string
	Business bool
}

// Network is a full topology snapshot.
type Network struct {
	Nodes     []Node
	Links     []Link
	ISPs      []ISP
	Customers []Customer
}

// ErrInvalid is wrapped by every error Validate returns.
var ErrInvalid = errors.New("invalid network")

// Validate checks referential integrity so that analysis can trust the
// snapshot. It returns the first problem it finds.
func (n *Network) Validate() error {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}

	nodes := make(map[string]bool, len(n.Nodes))
	cores := 0
	for _, node := range n.Nodes {
		if node.ID == "" {
			return fail("node with empty id")
		}
		if nodes[node.ID] {
			return fail("duplicate node %q", node.ID)
		}
		if !node.Kind.Valid() {
			return fail("node %q has unknown kind %q", node.ID, node.Kind)
		}
		if node.Kind == KindCore {
			cores++
		}
		nodes[node.ID] = true
	}
	if cores == 0 {
		return fail("no core node")
	}

	links := make(map[string]bool, len(n.Links))
	for _, l := range n.Links {
		if links[l.ID] {
			return fail("duplicate link %q", l.ID)
		}
		links[l.ID] = true
		if !nodes[l.A] || !nodes[l.B] {
			return fail("link %q references an unknown node", l.ID)
		}
		if l.A == l.B {
			return fail("link %q is a loop", l.ID)
		}
	}

	isps := make(map[string]bool, len(n.ISPs))
	for _, isp := range n.ISPs {
		if isps[isp.ID] {
			return fail("duplicate isp %q", isp.ID)
		}
		isps[isp.ID] = true
	}

	customers := make(map[string]bool, len(n.Customers))
	for _, c := range n.Customers {
		if customers[c.ID] {
			return fail("duplicate customer %q", c.ID)
		}
		customers[c.ID] = true
		if !isps[c.ISPID] {
			return fail("customer %q references unknown isp %q", c.ID, c.ISPID)
		}
		if len(c.NodeIDs) == 0 {
			return fail("customer %q is not attached to any node", c.ID)
		}
		for _, id := range c.NodeIDs {
			if !nodes[id] {
				return fail("customer %q references unknown node %q", c.ID, id)
			}
		}
	}
	return nil
}

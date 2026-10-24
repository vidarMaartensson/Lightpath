package network

import (
	"errors"
	"testing"
)

func TestSeedIsValidAndStable(t *testing.T) {
	a, b := Seed(), Seed()
	if err := a.Validate(); err != nil {
		t.Fatalf("seed is invalid: %v", err)
	}
	if len(a.Customers) != len(b.Customers) {
		t.Fatalf("seed customer count differs between runs")
	}
	for i := range a.Customers {
		if a.Customers[i].ISPID != b.Customers[i].ISPID {
			t.Fatalf("seed ISP assignment is not deterministic at %d", i)
		}
	}
	if len(a.Customers) < 1000 {
		t.Errorf("expected a realistic number of customers, got %d", len(a.Customers))
	}
}

func TestValidateRejectsBadData(t *testing.T) {
	base := func() *Network {
		return &Network{
			Nodes: []Node{
				{ID: "core", Kind: KindCore},
				{ID: "cab", Kind: KindCabinet},
			},
			Links:     []Link{{ID: "l1", A: "core", B: "cab"}},
			ISPs:      []ISP{{ID: "isp"}},
			Customers: []Customer{{ID: "c1", ISPID: "isp", NodeIDs: []string{"cab"}}},
		}
	}

	tests := []struct {
		name   string
		mutate func(n *Network)
	}{
		{"duplicate node", func(n *Network) { n.Nodes = append(n.Nodes, Node{ID: "cab", Kind: KindCabinet}) }},
		{"unknown kind", func(n *Network) { n.Nodes[1].Kind = "SPLITTER" }},
		{"no core", func(n *Network) { n.Nodes[0].Kind = KindAccess }},
		{"dangling link", func(n *Network) { n.Links[0].B = "nowhere" }},
		{"loop link", func(n *Network) { n.Links[0].B = "core" }},
		{"unknown isp", func(n *Network) { n.Customers[0].ISPID = "ghost" }},
		{"unattached customer", func(n *Network) { n.Customers[0].NodeIDs = nil }},
		{"customer on unknown node", func(n *Network) { n.Customers[0].NodeIDs = []string{"nope"} }},
		{"duplicate customer", func(n *Network) { n.Customers = append(n.Customers, n.Customers[0]) }},
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("base network should be valid: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := base()
			tt.mutate(n)
			if err := n.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

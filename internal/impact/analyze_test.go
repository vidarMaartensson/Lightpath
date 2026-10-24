package impact

import (
	"errors"
	"testing"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

// testNetwork is small enough to reason about by hand:
//
//	        core
//	       /    \
//	   accA      accB
//	   /  \      /
//	cabA1  cabA2   cabB1
//	          \____/        cabA2 has a second fibre to accB
//
// cA1: home on cabA1 (isp-1)
// cA2: home on cabA2 (isp-1)
// cB1: home on cabB1 (isp-2)
// biz: business on cabA1, secondary cabB1 (isp-2)
func testNetwork() *network.Network {
	return &network.Network{
		Nodes: []network.Node{
			{ID: "core", Kind: network.KindCore},
			{ID: "accA", Kind: network.KindAccess},
			{ID: "accB", Kind: network.KindAccess},
			{ID: "cabA1", Kind: network.KindCabinet},
			{ID: "cabA2", Kind: network.KindCabinet},
			{ID: "cabB1", Kind: network.KindCabinet},
		},
		Links: []network.Link{
			{ID: "1", A: "core", B: "accA"},
			{ID: "2", A: "core", B: "accB"},
			{ID: "3", A: "accA", B: "cabA1"},
			{ID: "4", A: "accA", B: "cabA2"},
			{ID: "5", A: "accB", B: "cabB1"},
			{ID: "6", A: "accB", B: "cabA2"},
		},
		ISPs: []network.ISP{{ID: "isp-1"}, {ID: "isp-2"}},
		Customers: []network.Customer{
			{ID: "cA1", ISPID: "isp-1", NodeIDs: []string{"cabA1"}},
			{ID: "cA2", ISPID: "isp-1", NodeIDs: []string{"cabA2"}},
			{ID: "cB1", ISPID: "isp-2", NodeIDs: []string{"cabB1"}},
			{ID: "biz", ISPID: "isp-2", NodeIDs: []string{"cabA1", "cabB1"}, Business: true},
		},
	}
}

func states(r Result) map[string]NodeState {
	m := make(map[string]NodeState, len(r.Nodes))
	for _, n := range r.Nodes {
		m[n.NodeID] = n.State
	}
	return m
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name         string
		failed       []string
		wantAffected int
		wantRerouted int
		wantStates   map[string]NodeState
	}{
		{
			name:       "no failures",
			wantStates: map[string]NodeState{"cabA1": Online, "cabA2": Online, "cabB1": Online},
		},
		{
			// The case from the job ad: the site goes down, but customers on
			// a redundant path are not reported as affected.
			name:         "access site down, redundant cabinet survives",
			failed:       []string{"accA"},
			wantAffected: 1, // cA1
			wantRerouted: 2, // cA2 via accB, biz via its secondary cabinet
			wantStates:   map[string]NodeState{"accA": Failed, "cabA1": Isolated, "cabA2": Rerouted, "cabB1": Online},
		},
		{
			name:         "cabinet down, dual-homed business keeps service",
			failed:       []string{"cabA1"},
			wantAffected: 1, // cA1
			wantRerouted: 1, // biz
			wantStates:   map[string]NodeState{"cabA1": Failed, "cabB1": Online},
		},
		{
			name:         "both access sites down",
			failed:       []string{"accA", "accB"},
			wantAffected: 4,
			wantStates:   map[string]NodeState{"cabA1": Isolated, "cabA2": Isolated, "cabB1": Isolated},
		},
		{
			name:         "only core down takes everyone offline",
			failed:       []string{"core"},
			wantAffected: 4,
			wantStates:   map[string]NodeState{"core": Failed, "accA": Isolated},
		},
		{
			name:         "duplicate ids are counted once",
			failed:       []string{"cabB1", "cabB1"},
			wantAffected: 1, // cB1; biz's primary is up
			wantStates:   map[string]NodeState{"cabB1": Failed, "cabA1": Online},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Analyze(testNetwork(), tt.failed)
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if r.AffectedCustomers != tt.wantAffected {
				t.Errorf("affected = %d, want %d", r.AffectedCustomers, tt.wantAffected)
			}
			if r.ReroutedCustomers != tt.wantRerouted {
				t.Errorf("rerouted = %d, want %d", r.ReroutedCustomers, tt.wantRerouted)
			}
			got := states(r)
			for id, want := range tt.wantStates {
				if got[id] != want {
					t.Errorf("state[%s] = %s, want %s", id, got[id], want)
				}
			}
		})
	}
}

func TestAnalyzeCountsAddUp(t *testing.T) {
	r, err := Analyze(testNetwork(), []string{"accA"})
	if err != nil {
		t.Fatal(err)
	}
	var nodeCustomers, nodeAffected, ispAffected int
	for _, n := range r.Nodes {
		nodeCustomers += n.Customers
		nodeAffected += n.Affected
	}
	for _, i := range r.ISPs {
		ispAffected += i.Affected
	}
	if nodeCustomers != r.TotalCustomers {
		t.Errorf("per-node customers sum to %d, total is %d", nodeCustomers, r.TotalCustomers)
	}
	if nodeAffected != r.AffectedCustomers || ispAffected != r.AffectedCustomers {
		t.Errorf("affected sums disagree: nodes %d, isps %d, total %d", nodeAffected, ispAffected, r.AffectedCustomers)
	}
	// isp-1 loses cA1; isp-2 loses nobody, so isp-1 must come first.
	if r.ISPs[0].ISPID != "isp-1" || r.ISPs[0].Affected != 1 {
		t.Errorf("ISPs not sorted by impact: %+v", r.ISPs)
	}
}

func TestCabinetsDoNotCarryTransit(t *testing.T) {
	// accB's only uplink is gone. cabA2 is spliced to both access sites, but
	// a passive cabinet must not bridge accB back to the core through accA.
	n := testNetwork()
	n.Links = n.Links[:1] // keep core--accA only
	n.Links = append(n.Links,
		network.Link{ID: "3", A: "accA", B: "cabA1"},
		network.Link{ID: "4", A: "accA", B: "cabA2"},
		network.Link{ID: "5", A: "accB", B: "cabB1"},
		network.Link{ID: "6", A: "accB", B: "cabA2"},
	)
	r, err := Analyze(n, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := states(r); s["accB"] != Isolated {
		t.Fatalf("accB = %s, want ISOLATED", s["accB"])
	}
}

func TestAnalyzeUnknownNode(t *testing.T) {
	_, err := Analyze(testNetwork(), []string{"accA", "typo"})
	if !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("want ErrUnknownNode, got %v", err)
	}
}

// Scenarios on the demo network, so the story told in the UI is pinned down.
func TestSeedScenarios(t *testing.T) {
	n := network.Seed()

	t.Run("core failure is fully absorbed", func(t *testing.T) {
		r, err := Analyze(n, []string{"core-a"})
		if err != nil {
			t.Fatal(err)
		}
		if r.AffectedCustomers != 0 {
			t.Errorf("affected = %d, want 0", r.AffectedCustomers)
		}
	})

	t.Run("Ålidhem access site: some customers rerouted", func(t *testing.T) {
		r, err := Analyze(n, []string{"acc-ali"})
		if err != nil {
			t.Fatal(err)
		}
		s := states(r)
		if s["cab-ali-1"] != Isolated {
			t.Errorf("cab-ali-1 = %s, want ISOLATED", s["cab-ali-1"])
		}
		if s["cab-ali-2"] != Rerouted {
			t.Errorf("cab-ali-2 = %s, want REROUTED (second fibre to Mariehem)", s["cab-ali-2"])
		}
		if r.AffectedCustomers == 0 || r.ReroutedCustomers == 0 {
			t.Errorf("want both affected and rerouted customers, got %d / %d", r.AffectedCustomers, r.ReroutedCustomers)
		}
	})

	t.Run("Väst aggregation: Haga survives, Västerslätt does not", func(t *testing.T) {
		r, err := Analyze(n, []string{"agg-vast"})
		if err != nil {
			t.Fatal(err)
		}
		s := states(r)
		if s["acc-hag"] != Rerouted || s["acc-vsl"] != Isolated {
			t.Errorf("acc-hag = %s, acc-vsl = %s", s["acc-hag"], s["acc-vsl"])
		}
		// cab-vsl-2 has a fibre to Haga, which still has service.
		if s["cab-vsl-2"] != Rerouted {
			t.Errorf("cab-vsl-2 = %s, want REROUTED", s["cab-vsl-2"])
		}
	})
}

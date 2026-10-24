package network

import "fmt"

// Seed returns a deterministic demo network: a fictional open access
// network laid over Umeå. The ISPs are invented.
//
// Redundancy is deliberately uneven so outages tell different stories:
//   - both core sites are meshed, and every aggregation site has an
//     uplink to each core, so a core failure takes nobody offline;
//   - access sites Haga and Ålidhem have a second uplink, the rest do not;
//   - some cabinets have a second fibre to a neighbouring access site;
//   - a handful of business customers are attached to two cabinets that
//     hang off different access sites.
func Seed() *Network {
	n := &Network{
		ISPs: []ISP{
			{ID: "isp-alvnat", Name: "Älvnät"},
			{ID: "isp-polarlank", Name: "Polarlänk"},
			{ID: "isp-byfiber", Name: "Byfiber"},
			{ID: "isp-norrbit", Name: "Norrbit"},
		},
	}

	node := func(id, name, area string, kind NodeKind, x, y float64) {
		n.Nodes = append(n.Nodes, Node{ID: id, Name: name, Area: area, Kind: kind, X: x, Y: y})
	}
	link := func(a, b string) {
		n.Links = append(n.Links, Link{ID: a + "--" + b, A: a, B: b})
	}

	node("core-a", "Core A", "Umeå DC North", KindCore, 400, 70)
	node("core-b", "Core B", "Umeå DC South", KindCore, 600, 70)
	link("core-a", "core-b")

	aggs := []struct {
		id, name string
		x        float64
	}{
		{"agg-vast", "Väst", 140},
		{"agg-cen", "Centrum", 390},
		{"agg-ost", "Öst", 620},
		{"agg-teg", "Teg", 870},
	}
	for _, a := range aggs {
		node(a.id, a.name+" aggregation", a.name, KindAggregation, a.x, 205)
		link("core-a", a.id)
		link("core-b", a.id)
	}

	type access struct {
		id, name, agg string
		x             float64
	}
	accesses := []access{
		{"acc-vsl", "Västerslätt", "agg-vast", 70},
		{"acc-hag", "Haga", "agg-vast", 215},
		{"acc-cen", "Centrum", "agg-cen", 330},
		{"acc-ali", "Ålidhem", "agg-cen", 455},
		{"acc-mar", "Mariehem", "agg-ost", 570},
		{"acc-tom", "Tomtebo", "agg-ost", 690},
		{"acc-teg", "Teg", "agg-teg", 810},
		{"acc-bol", "Böleäng", "agg-teg", 935},
	}
	for _, a := range accesses {
		node(a.id, a.name+" access site", a.name, KindAccess, a.x, 350)
		link(a.agg, a.id)
	}
	// Second uplinks: Haga and Ålidhem survive the loss of their primary
	// aggregation site.
	link("agg-cen", "acc-hag")
	link("agg-ost", "acc-ali")

	// Two cabinets per access site, customers per cabinet fixed for
	// reproducible numbers.
	sizes := []int{96, 72, 118, 64, 140, 88, 76, 104, 132, 58, 90, 112, 84, 66, 122, 80}
	i := 0
	for _, a := range accesses {
		for c := 1; c <= 2; c++ {
			id := fmt.Sprintf("cab-%s-%d", a.id[4:], c)
			dx := -32.0
			y := 495.0
			if c == 2 {
				dx, y = 32, 545
			}
			node(id, fmt.Sprintf("%s cabinet %d", a.name, c), a.name, KindCabinet, a.x+dx, y)
			link(a.id, id)
			i++
		}
	}
	// Cabinets with a second fibre to a neighbouring access site.
	link("acc-hag", "cab-vsl-2")
	link("acc-ali", "cab-cen-2")
	link("acc-mar", "cab-ali-2")
	link("acc-mar", "cab-tom-1")
	link("acc-bol", "cab-teg-2")

	// Residential customers, spread over ISPs with a small LCG so the
	// split is uneven but stable between runs.
	seed := uint32(20261024)
	next := func() uint32 {
		seed = seed*1664525 + 1013904223
		return seed >> 16
	}
	weights := []int{4, 3, 2, 1} // Älvnät is the biggest ISP, Norrbit the smallest
	pickISP := func() string {
		r := int(next() % 10)
		for idx, w := range weights {
			if r < w {
				return n.ISPs[idx].ID
			}
			r -= w
		}
		return n.ISPs[0].ID
	}

	i = 0
	for _, a := range accesses {
		for c := 1; c <= 2; c++ {
			cab := fmt.Sprintf("cab-%s-%d", a.id[4:], c)
			for k := 0; k < sizes[i]; k++ {
				n.Customers = append(n.Customers, Customer{
					ID:      fmt.Sprintf("cust-%s-%03d", cab[4:], k+1),
					ISPID:   pickISP(),
					NodeIDs: []string{cab},
				})
			}
			i++
		}
	}

	// Dual-homed business customers: primary and secondary cabinets sit
	// under different access sites.
	pairs := [][2]string{
		{"cab-cen-1", "cab-ali-1"},
		{"cab-hag-1", "cab-vsl-1"},
		{"cab-mar-2", "cab-tom-1"},
		{"cab-teg-1", "cab-bol-1"},
	}
	for _, p := range pairs {
		for k := 1; k <= 6; k++ {
			n.Customers = append(n.Customers, Customer{
				ID:       fmt.Sprintf("biz-%s-%d", p[0][4:], k),
				ISPID:    pickISP(),
				NodeIDs:  []string{p[0], p[1]},
				Business: true,
			})
		}
	}
	return n
}

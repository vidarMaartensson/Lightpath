package impact

import "github.com/vidarMaartensson/Lightpath/internal/network"

// Change is what moving from one set of failures to another did to
// customers. It is counted customer by customer, not as a difference of
// totals: when one site comes back as another fails, the totals can stay
// the same while different people lose and regain service.
type Change struct {
	// Lost: had service before, has none after.
	Lost int
	// Regained: had no service before, has it after.
	Regained int
	// Rerouted: has service before and after, but after only because
	// traffic found another path, which it did not need before.
	Rerouted int
}

// Diff compares the outage with before failed to the outage with after
// failed. It is how one event in the live feed is described: before is the
// state just ahead of the event, after the state it produced.
func Diff(n *network.Network, before, after []string) (Change, error) {
	b, err := evaluate(n, before)
	if err != nil {
		return Change{}, err
	}
	a, err := evaluate(n, after)
	if err != nil {
		return Change{}, err
	}

	var ch Change
	for _, c := range n.Customers {
		affB, rerB := customerOutcome(c, b.index, b.states)
		affA, rerA := customerOutcome(c, a.index, a.states)
		switch {
		case !affB && affA:
			ch.Lost++
		case affB && !affA:
			ch.Regained++
		case !affB && !affA && !rerB && rerA:
			ch.Rerouted++
		}
	}
	return ch, nil
}

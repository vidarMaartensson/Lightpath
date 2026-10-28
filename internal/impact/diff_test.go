package impact

import (
	"errors"
	"testing"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name          string
		before, after []string
		want          Change
	}{
		{"nothing happens", nil, nil, Change{}},
		{
			// cA1 has only cabA1. cA2 keeps service over cabA2's second
			// fibre, biz over its secondary cabinet.
			name: "access site fails", before: nil, after: []string{"accA"},
			want: Change{Lost: 1, Rerouted: 2},
		},
		{
			name: "access site restored", before: []string{"accA"}, after: nil,
			want: Change{Regained: 1},
		},
		{
			// cA2 was already rerouted; now its cabinet is gone too.
			name: "rerouted customer loses the backup", before: []string{"accA"}, after: []string{"accA", "cabA2"},
			want: Change{Lost: 1},
		},
		{
			// One affected customer before and after, but not the same one.
			// A difference of totals would report no change.
			name: "one site back, another down", before: []string{"accA"}, after: []string{"accB"},
			want: Change{Lost: 1, Regained: 1},
		},
		{
			name: "cabinet under a site that is already down", before: []string{"accA"}, after: []string{"accA", "cabA1"},
			want: Change{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Diff(testNetwork(), tt.before, tt.after)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffUnknownNode(t *testing.T) {
	if _, err := Diff(testNetwork(), []string{"typo"}, nil); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("unknown in before: want ErrUnknownNode, got %v", err)
	}
	if _, err := Diff(testNetwork(), nil, []string{"typo"}); !errors.Is(err, ErrUnknownNode) {
		t.Fatalf("unknown in after: want ErrUnknownNode, got %v", err)
	}
}

// Package postgres is the Postgres-backed topology store.
package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vidarMaartensson/Lightpath/internal/network"
)

//go:embed migrations/001_init.sql
var schema string

// Store loads the network from Postgres. It implements topology.Store.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps an open pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate creates the schema. Statements are idempotent.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// SeedIfEmpty writes n to the database if there are no nodes yet. It runs
// in one transaction, so a half-seeded database cannot happen.
func (s *Store) SeedIfEmpty(ctx context.Context, n *network.Network) (seeded bool, err error) {
	if err := n.Validate(); err != nil {
		return false, err
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM nodes`).Scan(&count); err != nil {
		return false, fmt.Errorf("count nodes: %w", err)
	}
	if count > 0 {
		return false, nil
	}

	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		copyRows := func(table string, cols []string, rows [][]any) error {
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{table}, cols, pgx.CopyFromRows(rows)); err != nil {
				return fmt.Errorf("seed %s: %w", table, err)
			}
			return nil
		}

		var isps, nodes, links, customers, attachments [][]any
		for _, i := range n.ISPs {
			isps = append(isps, []any{i.ID, i.Name})
		}
		for _, nd := range n.Nodes {
			nodes = append(nodes, []any{nd.ID, nd.Name, nd.Area, string(nd.Kind), nd.X, nd.Y})
		}
		for pos, l := range n.Links {
			links = append(links, []any{l.ID, l.A, l.B, int32(pos)})
		}
		for _, c := range n.Customers {
			customers = append(customers, []any{c.ID, c.ISPID, c.Business})
			for pos, nodeID := range c.NodeIDs {
				attachments = append(attachments, []any{c.ID, nodeID, int32(pos)})
			}
		}

		steps := []struct {
			table string
			cols  []string
			rows  [][]any
		}{
			{"isps", []string{"id", "name"}, isps},
			{"nodes", []string{"id", "name", "area", "kind", "x", "y"}, nodes},
			{"links", []string{"id", "a_id", "b_id", "position"}, links},
			{"customers", []string{"id", "isp_id", "business"}, customers},
			{"customer_attachments", []string{"customer_id", "node_id", "position"}, attachments},
		}
		for _, st := range steps {
			if err := copyRows(st.table, st.cols, st.rows); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// Network loads the full snapshot and validates it before returning it.
func (s *Store) Network(ctx context.Context) (*network.Network, error) {
	n := &network.Network{}

	if err := queryEach(ctx, s.pool, `SELECT id, name FROM isps ORDER BY id`, func(r pgx.Rows) error {
		var i network.ISP
		if err := r.Scan(&i.ID, &i.Name); err != nil {
			return err
		}
		n.ISPs = append(n.ISPs, i)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load isps: %w", err)
	}

	if err := queryEach(ctx, s.pool, `SELECT id, name, area, kind, x, y FROM nodes ORDER BY id`, func(r pgx.Rows) error {
		var nd network.Node
		var kind string
		if err := r.Scan(&nd.ID, &nd.Name, &nd.Area, &kind, &nd.X, &nd.Y); err != nil {
			return err
		}
		nd.Kind = network.NodeKind(kind)
		n.Nodes = append(n.Nodes, nd)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load nodes: %w", err)
	}

	if err := queryEach(ctx, s.pool, `SELECT id, a_id, b_id FROM links ORDER BY position`, func(r pgx.Rows) error {
		var l network.Link
		if err := r.Scan(&l.ID, &l.A, &l.B); err != nil {
			return err
		}
		n.Links = append(n.Links, l)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load links: %w", err)
	}

	attached := map[string][]string{}
	if err := queryEach(ctx, s.pool, `SELECT customer_id, node_id FROM customer_attachments ORDER BY customer_id, position`, func(r pgx.Rows) error {
		var cust, node string
		if err := r.Scan(&cust, &node); err != nil {
			return err
		}
		attached[cust] = append(attached[cust], node)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load attachments: %w", err)
	}

	if err := queryEach(ctx, s.pool, `SELECT id, isp_id, business FROM customers ORDER BY id`, func(r pgx.Rows) error {
		var c network.Customer
		if err := r.Scan(&c.ID, &c.ISPID, &c.Business); err != nil {
			return err
		}
		c.NodeIDs = attached[c.ID]
		n.Customers = append(n.Customers, c)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load customers: %w", err)
	}

	// Bad data in the database should fail loudly here, not as a wrong
	// answer during an outage.
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

func queryEach(ctx context.Context, pool *pgxpool.Pool, sql string, fn func(pgx.Rows) error) error {
	rows, err := pool.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

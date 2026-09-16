package database

import (
	"context"
	"fmt"
)

type TSIGKey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Algorithm string `json:"algorithm"`
	Secret    string `json:"secret"`
}

func (s *Store) ListTSIGKeys(ctx context.Context) ([]TSIGKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,algorithm,secret FROM tsig_keys ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var keys []TSIGKey
	for rows.Next() {
		var k TSIGKey
		if err = rows.Scan(&k.ID, &k.Name, &k.Algorithm, &k.Secret); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) GetTSIGKey(ctx context.Context, name string) (TSIGKey, error) {
	p := s.placeholder
	query := fmt.Sprintf("SELECT id,name,algorithm,secret FROM tsig_keys WHERE name=%s", p(1))
	var k TSIGKey
	err := s.db.QueryRowContext(ctx, query, name).Scan(&k.ID, &k.Name, &k.Algorithm, &k.Secret)
	if err != nil {
		return TSIGKey{}, err
	}
	return k, nil
}

func (s *Store) SaveTSIGKey(ctx context.Context, key TSIGKey) error {
	p := s.placeholder
	insertSQL := fmt.Sprintf("INSERT INTO tsig_keys(name,algorithm,secret) VALUES(%s,%s,%s)%s", p(1), p(2), p(3), s.insertReturning())
	if s.driver == "postgres" {
		var id int64
		return s.db.QueryRowContext(ctx, insertSQL, key.Name, key.Algorithm, key.Secret).Scan(&id)
	}
	_, err := s.db.ExecContext(ctx, insertSQL, key.Name, key.Algorithm, key.Secret)
	return err
}

func (s *Store) DeleteTSIGKey(ctx context.Context, name string) error {
	p := s.placeholder
	query := fmt.Sprintf("DELETE FROM tsig_keys WHERE name=%s", p(1))
	_, err := s.db.ExecContext(ctx, query, name)
	return err
}

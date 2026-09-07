package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/matta813/velora-dns/internal/zones"
	"modernc.org/sqlite"
)

var _ zones.Repository = (*Store)(nil)

func zoneError(err error) error {
	var e *sqlite.Error
	if errors.As(err, &e) && e.Code()&255 == 19 {
		return fmt.Errorf("%w: constraint violation", zones.ErrExists)
	}
	return err
}
func (s *Store) LoadZones(ctx context.Context) ([]zones.Zone, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	all := []zones.Zone{}
	index := map[int64]int{}
	rows, err := tx.QueryContext(ctx, "SELECT id,name,primary_ns,contact,revision FROM zones ORDER BY id LIMIT ?", zones.MaxZones+1)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var z zones.Zone
		if err = rows.Scan(&z.ID, &z.Name, &z.PrimaryNS, &z.Contact, &z.Revision); err != nil {
			_ = rows.Close()
			return nil, err
		}
		z.Records = []zones.Record{}
		index[z.ID] = len(all)
		all = append(all, z)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	if len(all) > zones.MaxZones {
		return nil, fmt.Errorf("stored zones exceed configured service limit")
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,zone_id,name,type,ttl,value,priority FROM zone_records ORDER BY id LIMIT ?", zones.MaxTotalRecords+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		count++
		var r zones.Record
		var id int64
		if err = rows.Scan(&r.ID, &id, &r.Name, &r.Type, &r.TTL, &r.Value, &r.Priority); err != nil {
			return nil, err
		}
		i, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("record references missing zone")
		}
		all[i].Records = append(all[i].Records, r)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	if count > zones.MaxTotalRecords {
		return nil, fmt.Errorf("stored records exceed service limit")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return all, nil
}
func (s *Store) SaveZone(ctx context.Context, z zones.Zone, expected uint32) (zones.Zone, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return z, err
	}
	defer func() { _ = tx.Rollback() }()
	if expected == 0 {
		result, e := tx.ExecContext(ctx, "INSERT INTO zones(name,primary_ns,contact,revision) VALUES(?,?,?,?)", z.Name, z.PrimaryNS, z.Contact, z.Revision)
		if e != nil {
			return z, zoneError(e)
		}
		z.ID, err = result.LastInsertId()
		if err != nil {
			return z, err
		}
	} else {
		result, e := tx.ExecContext(ctx, "UPDATE zones SET name=?,primary_ns=?,contact=?,revision=? WHERE id=? AND revision=?", z.Name, z.PrimaryNS, z.Contact, z.Revision, z.ID, expected)
		if e != nil {
			return z, zoneError(e)
		}
		n, e := result.RowsAffected()
		if e != nil {
			return z, e
		}
		if n != 1 {
			return z, zones.ErrConflict
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM zone_records WHERE zone_id=?", z.ID); err != nil {
		return z, err
	}
	z.Records = append([]zones.Record{}, z.Records...)
	for i, r := range z.Records {
		var id any
		if r.ID != 0 {
			id = r.ID
		}
		result, e := tx.ExecContext(ctx, "INSERT INTO zone_records(id,zone_id,name,type,ttl,value,priority) VALUES(?,?,?,?,?,?,?)", id, z.ID, r.Name, r.Type, r.TTL, r.Value, r.Priority)
		if e != nil {
			return z, zoneError(e)
		}
		z.Records[i].ID, err = result.LastInsertId()
		if err != nil {
			return z, err
		}
	}
	if err = tx.Commit(); err != nil {
		return z, err
	}
	return z, nil
}
func (s *Store) DeleteZone(ctx context.Context, id int64, expected uint32) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM zones WHERE id=? AND revision=?", id, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return zones.ErrConflict
	}
	return nil
}

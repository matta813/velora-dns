package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/matta813/velora-dns/internal/zones"
)

var _ zones.Repository = (*Store)(nil)

func (s *Store) zoneError(err error) error {
	if s.driver == "sqlite" {
		var e interface{ Code() int64 }
		if errors.As(err, &e) {
			return fmt.Errorf("%w: constraint violation", zones.ErrExists)
		}
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
	p := s.placeholder
	query := fmt.Sprintf("SELECT id,name,primary_ns,contact,revision,zone_type,primary_address,transfer_tsig_key,transfer_interval,last_transfer_at,next_refresh_at,last_transfer_serial FROM zones ORDER BY id LIMIT %s", p(1))
	rows, err := tx.QueryContext(ctx, query, zones.MaxZones+1)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var z zones.Zone
		var zoneType, primaryAddress, transferTSIGKey string
		var transferInterval int
		var lastTransferAt, nextRefreshAt sql.NullString
		var lastTransferSerial int64
		if err = rows.Scan(&z.ID, &z.Name, &z.PrimaryNS, &z.Contact, &z.Revision, &zoneType, &primaryAddress, &transferTSIGKey, &transferInterval, &lastTransferAt, &nextRefreshAt, &lastTransferSerial); err != nil {
			_ = rows.Close()
			return nil, err
		}
		z.ZoneType = zoneType
		z.PrimaryAddress = primaryAddress
		z.TransferTSIGKey = transferTSIGKey
		z.TransferInterval = transferInterval
		z.LastTransferSerial = uint32(lastTransferSerial)
		if lastTransferAt.Valid {
			t, err := time.Parse(time.RFC3339, lastTransferAt.String)
			if err == nil {
				z.LastTransferAt = &t
			}
		}
		if nextRefreshAt.Valid {
			t, err := time.Parse(time.RFC3339, nextRefreshAt.String)
			if err == nil {
				z.NextRefreshAt = &t
			}
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
	query = fmt.Sprintf("SELECT id,zone_id,name,type,ttl,value,priority FROM zone_records ORDER BY id LIMIT %s", p(1))
	rows, err = tx.QueryContext(ctx, query, zones.MaxTotalRecords+1)
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
	p := s.placeholder

	var lastTransferAt, nextRefreshAt any
	if z.LastTransferAt != nil {
		lastTransferAt = z.LastTransferAt.UTC().Format(time.RFC3339)
	}
	if z.NextRefreshAt != nil {
		nextRefreshAt = z.NextRefreshAt.UTC().Format(time.RFC3339)
	}
	if expected == 0 {
		insertSQL := fmt.Sprintf("INSERT INTO zones(name,primary_ns,contact,revision,zone_type,primary_address,transfer_tsig_key,transfer_interval,last_transfer_at,next_refresh_at,last_transfer_serial) VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9), p(10), p(11), s.insertReturning())
		if s.driver == "postgres" {
			var id int64
			if e := tx.QueryRowContext(ctx, insertSQL, z.Name, z.PrimaryNS, z.Contact, z.Revision, z.ZoneType, z.PrimaryAddress, z.TransferTSIGKey, z.TransferInterval, lastTransferAt, nextRefreshAt, z.LastTransferSerial).Scan(&id); e != nil {
				return z, s.zoneError(e)
			}
			z.ID = id
		} else {
			result, e := tx.ExecContext(ctx, insertSQL, z.Name, z.PrimaryNS, z.Contact, z.Revision, z.ZoneType, z.PrimaryAddress, z.TransferTSIGKey, z.TransferInterval, lastTransferAt, nextRefreshAt, z.LastTransferSerial)
			if e != nil {
				return z, s.zoneError(e)
			}
			z.ID, err = result.LastInsertId()
			if err != nil {
				return z, err
			}
		}
	} else {
		updateSQL := fmt.Sprintf("UPDATE zones SET name=%s,primary_ns=%s,contact=%s,revision=%s,zone_type=%s,primary_address=%s,transfer_tsig_key=%s,transfer_interval=%s,last_transfer_at=%s,next_refresh_at=%s,last_transfer_serial=%s WHERE id=%s AND revision=%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9), p(10), p(11), p(12), p(13))
		result, e := tx.ExecContext(ctx, updateSQL, z.Name, z.PrimaryNS, z.Contact, z.Revision, z.ZoneType, z.PrimaryAddress, z.TransferTSIGKey, z.TransferInterval, lastTransferAt, nextRefreshAt, z.LastTransferSerial, z.ID, expected)
		if e != nil {
			return z, s.zoneError(e)
		}
		n, e := result.RowsAffected()
		if e != nil {
			return z, e
		}
		if n != 1 {
			return z, zones.ErrConflict
		}
	}
	deleteSQL := fmt.Sprintf("DELETE FROM zone_records WHERE zone_id=%s", p(1))
	if _, err = tx.ExecContext(ctx, deleteSQL, z.ID); err != nil {
		return z, err
	}
	z.Records = append([]zones.Record{}, z.Records...)
	for i, r := range z.Records {
		var id any
		if r.ID != 0 {
			id = r.ID
		}
		insertSQL := fmt.Sprintf("INSERT INTO zone_records(id,zone_id,name,type,ttl,value,priority) VALUES(%s,%s,%s,%s,%s,%s,%s)%s", p(1), p(2), p(3), p(4), p(5), p(6), p(7), s.insertReturning())
		if s.driver == "postgres" {
			var rid int64
			if err = tx.QueryRowContext(ctx, insertSQL, id, z.ID, r.Name, r.Type, r.TTL, r.Value, r.Priority).Scan(&rid); err != nil {
				return z, s.zoneError(err)
			}
			z.Records[i].ID = rid
		} else {
			result, e := tx.ExecContext(ctx, insertSQL, id, z.ID, r.Name, r.Type, r.TTL, r.Value, r.Priority)
			if e != nil {
				return z, s.zoneError(e)
			}
			z.Records[i].ID, err = result.LastInsertId()
			if err != nil {
				return z, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return z, err
	}
	return z, nil
}

func (s *Store) DeleteZone(ctx context.Context, id int64, expected uint32) error {
	p := s.placeholder
	query := fmt.Sprintf("DELETE FROM zones WHERE id=%s AND revision=%s", p(1), p(2))
	result, err := s.db.ExecContext(ctx, query, id, expected)
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

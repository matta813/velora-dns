package database

import (
	"context"
	"fmt"
	"strconv"
)

type RateLimitSettings struct {
	Enabled        bool `json:"enabled"`
	GlobalQPS      int  `json:"global_qps"`
	ClientQPS      int  `json:"client_qps"`
	RateLimitBurst int  `json:"rate_limit_burst"`
}

func (s *Store) GetRateLimitSettings(ctx context.Context) (RateLimitSettings, error) {
	var out RateLimitSettings
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings WHERE key IN ('rate_limit_enabled', 'rate_limit_global_qps', 'rate_limit_client_qps', 'rate_limit_burst')")
	if err != nil {
		return RateLimitSettings{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, value string
		if err = rows.Scan(&key, &value); err != nil {
			return RateLimitSettings{}, err
		}
		switch key {
		case "rate_limit_enabled":
			out.Enabled = value == "true"
		case "rate_limit_global_qps":
			if n, e := strconv.Atoi(value); e == nil {
				out.GlobalQPS = n
			}
		case "rate_limit_client_qps":
			if n, e := strconv.Atoi(value); e == nil {
				out.ClientQPS = n
			}
		case "rate_limit_burst":
			if n, e := strconv.Atoi(value); e == nil {
				out.RateLimitBurst = n
			}
		}
	}
	return out, rows.Err()
}

func (s *Store) SetRateLimitSettings(ctx context.Context, settings RateLimitSettings) error {
	if settings.GlobalQPS < 1 || settings.GlobalQPS > 100000 || settings.ClientQPS < 1 || settings.ClientQPS > 100000 || settings.RateLimitBurst < 1 || settings.RateLimitBurst > 100000 {
		return fmt.Errorf("invalid rate limit settings")
	}
	p := s.placeholder
	upsert := fmt.Sprintf("INSERT INTO settings(key, value) VALUES(%s, %s) ON CONFLICT(key) DO UPDATE SET value=excluded.value", p(1), p(2))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	enabled := "false"
	if settings.Enabled {
		enabled = "true"
	}
	pairs := map[string]string{"rate_limit_enabled": enabled, "rate_limit_global_qps": strconv.Itoa(settings.GlobalQPS), "rate_limit_client_qps": strconv.Itoa(settings.ClientQPS), "rate_limit_burst": strconv.Itoa(settings.RateLimitBurst)}
	for key, value := range pairs {
		if _, err = tx.ExecContext(ctx, upsert, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

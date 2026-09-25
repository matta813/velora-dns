package backup

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// AcquireInstanceLock prevents an offline restore while the server is using the database.
func AcquireInstanceLock(databasePath string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(databasePath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("velora database is in use: %w", err)
	}
	return file, nil
}

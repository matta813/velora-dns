package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/scrypt"
	"gopkg.in/yaml.v3"
)

const bundleMagic = "VELBK001"
const bundleHeaderSize = len(bundleMagic) + 16 + aes.BlockSize
const bundleTagSize = sha256.Size

var ErrInvalidBundle = errors.New("invalid or incompatible backup bundle")

type Metadata struct {
	FormatVersion int       `json:"format_version"`
	VeloraVersion string    `json:"velora_version"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion int       `json:"schema_version"`
	Components    []string  `json:"components"`
}

func bundleKeys(passphrase string, salt []byte) ([]byte, []byte, error) {
	if len(passphrase) < 12 {
		return nil, nil, errors.New("backup passphrase must contain at least 12 characters")
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 64)
	if err != nil {
		return nil, nil, err
	}
	return key[:32], key[32:], nil
}

func writeTarFile(writer *tar.Writer, name string, source io.Reader, size int64) error {
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size, ModTime: time.Now().UTC()}); err != nil {
		return err
	}
	_, err := io.CopyN(writer, source, size)
	return err
}

// CreateBundle writes an encrypted, versioned archive to a temporary file. The
// caller must remove the returned file after streaming or moving it.
func (m *Manager) CreateBundle(ctx context.Context, passphrase string) (string, Metadata, error) {
	if len(passphrase) < 12 {
		return "", Metadata{}, errors.New("backup passphrase must contain at least 12 characters")
	}
	m.mu.RLock()
	currentConfig, version := m.config.Clone(), m.version
	m.mu.RUnlock()
	if m.db == nil || currentConfig.DatabaseDriver != "sqlite" {
		return "", Metadata{}, errors.New("backup creation requires a running SQLite installation")
	}
	if err := currentConfig.Validate(); err != nil {
		return "", Metadata{}, err
	}
	stageDir, err := os.MkdirTemp(filepath.Dir(m.databasePath), ".velora-snapshot-*")
	if err != nil {
		return "", Metadata{}, err
	}
	defer func() { _ = os.RemoveAll(stageDir) }()
	snapshot := filepath.Join(stageDir, "database.sqlite")
	schema, err := m.db.SnapshotSQLite(ctx, snapshot)
	if err != nil {
		return "", Metadata{}, err
	}
	databaseFile, err := os.Open(snapshot)
	if err != nil {
		return "", Metadata{}, err
	}
	defer func() { _ = databaseFile.Close() }()
	info, err := databaseFile.Stat()
	if err != nil {
		return "", Metadata{}, err
	}
	configBytes, err := yaml.Marshal(currentConfig)
	if err != nil {
		return "", Metadata{}, err
	}
	if len(configBytes) > 1<<20 {
		return "", Metadata{}, errors.New("configuration exceeds backup limit")
	}
	metadata := Metadata{FormatVersion: 1, VeloraVersion: version, CreatedAt: time.Now().UTC(), SchemaVersion: schema, Components: []string{"configuration", "database"}}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return "", Metadata{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(m.databasePath), ".velora-backup-*")
	if err != nil {
		return "", Metadata{}, err
	}
	success := false
	defer func() {
		_ = file.Close()
		if !success {
			_ = os.Remove(file.Name())
		}
	}()
	header := make([]byte, bundleHeaderSize)
	copy(header, bundleMagic)
	if _, err := rand.Read(header[len(bundleMagic):]); err != nil {
		return "", Metadata{}, err
	}
	encryptionKey, authenticationKey, err := bundleKeys(passphrase, header[len(bundleMagic):len(bundleMagic)+16])
	if err != nil {
		return "", Metadata{}, err
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", Metadata{}, err
	}
	mac := hmac.New(sha256.New, authenticationKey)
	if _, err := file.Write(header); err != nil {
		return "", Metadata{}, err
	}
	if _, err := mac.Write(header); err != nil {
		return "", Metadata{}, err
	}
	encrypted := &cipher.StreamWriter{S: cipher.NewCTR(block, header[len(bundleMagic)+16:]), W: io.MultiWriter(file, mac)}
	archive := tar.NewWriter(encrypted)
	if err := writeTarFile(archive, "metadata.json", bytes.NewReader(metadataBytes), int64(len(metadataBytes))); err != nil {
		return "", Metadata{}, err
	}
	if err := writeTarFile(archive, "config.yaml", bytes.NewReader(configBytes), int64(len(configBytes))); err != nil {
		return "", Metadata{}, err
	}
	if err := writeTarFile(archive, "database.sqlite", databaseFile, info.Size()); err != nil {
		return "", Metadata{}, err
	}
	if err := archive.Close(); err != nil {
		return "", Metadata{}, err
	}
	if _, err := file.Write(mac.Sum(nil)); err != nil {
		return "", Metadata{}, err
	}
	if err := file.Sync(); err != nil {
		return "", Metadata{}, err
	}
	if err := file.Close(); err != nil {
		return "", Metadata{}, err
	}
	m.mu.Lock()
	m.lastBackupTime = metadata.CreatedAt
	if info, statErr := os.Stat(file.Name()); statErr == nil {
		m.lastBackupSize = info.Size()
	}
	m.mu.Unlock()
	success = true
	return file.Name(), metadata, nil
}

func (m *Manager) CreateEncryptedBackup(ctx context.Context, passphrase string) (string, error) {
	path, _, err := m.CreateBundle(ctx, passphrase)
	return path, err
}

// inspectBundle authenticates the complete ciphertext before returning a
// bounded tar reader. Callers must close the returned file and use LimitReader.
func inspectBundle(path, passphrase string) (*os.File, *tar.Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, *tar.Reader, error) { _ = file.Close(); return nil, nil, err }
	stat, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	if !stat.Mode().IsRegular() || stat.Size() < int64(bundleHeaderSize+bundleTagSize) {
		return fail(ErrInvalidBundle)
	}
	header := make([]byte, bundleHeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		return fail(err)
	}
	if string(header[:len(bundleMagic)]) != bundleMagic {
		return fail(ErrInvalidBundle)
	}
	encryptionKey, authenticationKey, err := bundleKeys(passphrase, header[len(bundleMagic):len(bundleMagic)+16])
	if err != nil {
		return fail(err)
	}
	mac := hmac.New(sha256.New, authenticationKey)
	_, _ = mac.Write(header)
	cipherSize := stat.Size() - int64(bundleHeaderSize+bundleTagSize)
	if _, err := io.CopyN(mac, file, cipherSize); err != nil {
		return fail(err)
	}
	expected := make([]byte, bundleTagSize)
	if _, err := io.ReadFull(file, expected); err != nil {
		return fail(err)
	}
	if !hmac.Equal(mac.Sum(nil), expected) {
		return fail(ErrInvalidBundle)
	}
	if _, err := file.Seek(int64(bundleHeaderSize), io.SeekStart); err != nil {
		return fail(err)
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return fail(err)
	}
	decrypted := &cipher.StreamReader{S: cipher.NewCTR(block, header[len(bundleMagic)+16:]), R: io.LimitReader(file, cipherSize)}
	return file, tar.NewReader(decrypted), nil
}

func readBundle(path, passphrase, stageDir string) (Metadata, error) {
	file, archive, err := inspectBundle(path, passphrase)
	if err != nil {
		return Metadata{}, err
	}
	defer func() { _ = file.Close() }()
	expected := []string{"metadata.json", "config.yaml", "database.sqlite"}
	var metadata Metadata
	for _, name := range expected {
		header, err := archive.Next()
		if err != nil || header.Name != name || header.Typeflag != tar.TypeReg || header.Size < 1 {
			return Metadata{}, ErrInvalidBundle
		}
		limit := int64(16 << 10)
		if name == "config.yaml" {
			limit = 1 << 20
		}
		if name == "database.sqlite" {
			limit = 8 << 30
		}
		if header.Size > limit {
			return Metadata{}, ErrInvalidBundle
		}
		destination := filepath.Join(stageDir, name)
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return Metadata{}, err
		}
		_, copyErr := io.CopyN(output, archive, header.Size)
		closeErr := output.Close()
		if copyErr != nil {
			return Metadata{}, copyErr
		}
		if closeErr != nil {
			return Metadata{}, closeErr
		}
		if name == "metadata.json" {
			content, err := os.ReadFile(destination)
			if err != nil {
				return Metadata{}, err
			}
			if err := json.Unmarshal(content, &metadata); err != nil {
				return Metadata{}, ErrInvalidBundle
			}
		}
	}
	if _, err := archive.Next(); !errors.Is(err, io.EOF) {
		return Metadata{}, ErrInvalidBundle
	}
	if metadata.FormatVersion != 1 || metadata.SchemaVersion < 1 || len(metadata.Components) != 2 || metadata.Components[0] != "configuration" || metadata.Components[1] != "database" {
		return Metadata{}, ErrInvalidBundle
	}
	return metadata, nil
}

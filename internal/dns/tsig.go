// Package dns implements TSIG authentication for secure DNS operations.
package dns

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	wire "github.com/miekg/dns"
)

// TSIGKey represents a TSIG authentication key.
type TSIGKey struct {
	Name      string
	Algorithm string
	Secret    []byte
}

// TSIGStore manages TSIG keys for signing and verification.
type TSIGStore struct {
	mu   sync.RWMutex
	keys map[string]*TSIGKey
}

// NewTSIGStore creates a new TSIG key store.
func NewTSIGStore() *TSIGStore {
	return &TSIGStore{keys: make(map[string]*TSIGKey)}
}

// AddKey adds or updates a TSIG key.
func (s *TSIGStore) AddKey(name, algorithm, secretB64 string) error {
	secret, err := base64.StdEncoding.DecodeString(secretB64)
	if err != nil {
		return fmt.Errorf("decode TSIG secret: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[wire.CanonicalName(name)] = &TSIGKey{
		Name:      wire.CanonicalName(name),
		Algorithm: algorithm,
		Secret:    secret,
	}
	return nil
}

// GetKey retrieves a TSIG key by name.
func (s *TSIGStore) GetKey(name string) (*TSIGKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.keys[wire.CanonicalName(name)]
	return key, ok
}

// RemoveKey removes a TSIG key.
func (s *TSIGStore) RemoveKey(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, wire.CanonicalName(name))
	return true
}

// Keys returns all stored keys.
func (s *TSIGStore) Keys() []*TSIGKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*TSIGKey, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, k)
	}
	return out
}

// SignMessage signs a DNS message with the specified TSIG key.
// The actual TSIG is computed when the message is sent.
func (s *TSIGStore) SignMessage(msg *wire.Msg, keyName string) error {
	key, ok := s.GetKey(keyName)
	if !ok {
		return fmt.Errorf("TSIG key not found: %s", keyName)
	}
	algo, err := tsigAlgorithm(key.Algorithm)
	if err != nil {
		return err
	}
	msg.SetTsig(keyName, algo, 300, time.Now().Unix())
	return nil
}

// VerifyMessage verifies a TSIG signature on a DNS message.
func (s *TSIGStore) VerifyMessage(msg *wire.Msg) (string, error) {
	tsigRecord := msg.IsTsig()
	if tsigRecord == nil {
		return "", fmt.Errorf("no TSIG record")
	}
	keyName := tsigRecord.Hdr.Name
	key, ok := s.GetKey(keyName)
	if !ok {
		return "", fmt.Errorf("unknown TSIG key: %s", keyName)
	}
	buf, err := msg.Pack()
	if err != nil {
		return "", fmt.Errorf("pack message: %w", err)
	}
	if err := wire.TsigVerify(buf, base64.StdEncoding.EncodeToString(key.Secret), "", false); err != nil {
		return "", fmt.Errorf("TSIG verification failed: %w", err)
	}
	return keyName, nil
}

func tsigAlgorithm(algorithm string) (string, error) {
	switch algorithm {
	case "hmac-sha256":
		return wire.HmacSHA256, nil
	case "hmac-sha1":
		return wire.HmacSHA1, nil
	case "hmac-sha512":
		return wire.HmacSHA512, nil
	default:
		return "", fmt.Errorf("unsupported TSIG algorithm: %s", algorithm)
	}
}

// GenerateSecret generates a random TSIG secret suitable for the given algorithm.
func GenerateSecret(algorithm string) (string, error) {
	var size int
	switch algorithm {
	case "hmac-sha256":
		size = sha256.BlockSize
	case "hmac-sha1":
		size = 20
	case "hmac-sha512":
		size = 64
	default:
		return "", fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
	secret := make([]byte, size)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}

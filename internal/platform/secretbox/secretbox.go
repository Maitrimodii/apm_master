// Package secretbox seals small payloads with AES-256-GCM under a keyring of
// named keys, so keys can be rotated without making existing ciphertext
// unreadable.
//
// Keys come from configuration as "id:base64key,id2:base64key" (each key 32
// random bytes, e.g. `openssl rand -base64 32`). The active key encrypts; every
// listed key decrypts. To rotate: add the new key, make it active, and remove
// the old one once nothing sealed with it remains.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

var (
	// ErrUnknownKey means the ciphertext names a key that is not configured.
	ErrUnknownKey = errors.New("secretbox: unknown key id")
	// ErrDecrypt means authentication failed: wrong key, tampered ciphertext,
	// or associated data that does not match what it was sealed with.
	ErrDecrypt = errors.New("secretbox: decryption failed")
)

// Keyring is a set of AES-256-GCM keys by ID with one active for sealing.
type Keyring struct {
	activeID string
	aeads    map[string]cipher.AEAD
}

// ParseKeyring parses "id:base64key,..." and selects activeID for sealing.
// It rejects malformed entries, duplicate IDs, keys that are not 32 bytes
// and an active ID that is not in the list. Error messages never include
// key material.
func ParseKeyring(spec, activeID string) (*Keyring, error) {
	spec, activeID = strings.TrimSpace(spec), strings.TrimSpace(activeID)
	if spec == "" {
		return nil, errors.New("no keys configured")
	}
	if activeID == "" {
		return nil, errors.New("no active key id configured")
	}

	ring := &Keyring{activeID: activeID, aeads: map[string]cipher.AEAD{}}
	for i, entry := range strings.Split(spec, ",") {
		id, encoded, ok := strings.Cut(strings.TrimSpace(entry), ":")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			return nil, fmt.Errorf("key entry %d must be id:base64key", i+1)
		}
		if _, dup := ring.aeads[id]; dup {
			return nil, fmt.Errorf("key id %q is listed twice", id)
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("key %q is not valid base64", id)
		}
		if len(key) != KeySize {
			return nil, fmt.Errorf("key %q must be %d bytes, got %d", id, KeySize, len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", id, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", id, err)
		}
		ring.aeads[id] = aead
	}
	if _, ok := ring.aeads[activeID]; !ok {
		return nil, fmt.Errorf("active key id %q is not among the configured keys", activeID)
	}
	return ring, nil
}

// ActiveKeyID is the ID of the key Seal uses.
func (k *Keyring) ActiveKeyID() string { return k.activeID }

// Seal encrypts plaintext with the active key, binding it to aad (associated
// data that must be presented again to open it). It returns the key ID and
// nonce||ciphertext. Each call uses a fresh random nonce.
func (k *Keyring) Seal(plaintext, aad []byte) (keyID string, sealed []byte, err error) {
	aead := k.aeads[k.activeID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", nil, fmt.Errorf("secretbox: generate nonce: %w", err)
	}
	return k.activeID, aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Open decrypts a Seal result made with keyID and the same aad.
func (k *Keyring) Open(keyID string, sealed, aad []byte) ([]byte, error) {
	aead, ok := k.aeads[keyID]
	if !ok {
		return nil, ErrUnknownKey
	}
	if len(sealed) < aead.NonceSize() {
		return nil, ErrDecrypt
	}
	nonce, ciphertext := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

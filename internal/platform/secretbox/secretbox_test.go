package secretbox

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestSealOpenRoundTrip(t *testing.T) {
	ring, err := ParseKeyring("k1:"+newKey(t), "k1")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, aad := []byte("https://app/reset-password?token=secret"), []byte("job-1|org-1")

	keyID, sealed, err := ring.Seal(plaintext, aad)
	if err != nil || keyID != "k1" {
		t.Fatalf("Seal = %q, %v", keyID, err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("ciphertext contains the plaintext")
	}
	got, err := ring.Open(keyID, sealed, aad)
	if err != nil || !bytes.Equal(got, plaintext) {
		t.Fatalf("Open = %q, %v", got, err)
	}

	// Nonces are random: sealing twice gives different ciphertext.
	_, again, _ := ring.Seal(plaintext, aad)
	if bytes.Equal(sealed, again) {
		t.Fatal("two seals produced identical ciphertext")
	}
}

func TestOpenRejectsTamperingAndWrongAAD(t *testing.T) {
	ring, _ := ParseKeyring("k1:"+newKey(t), "k1")
	keyID, sealed, _ := ring.Seal([]byte("secret"), []byte("job-1"))

	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := ring.Open(keyID, tampered, []byte("job-1")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("tampered ciphertext: err = %v, want ErrDecrypt", err)
	}
	// Ciphertext moved to another job (different associated data).
	if _, err := ring.Open(keyID, sealed, []byte("job-2")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("wrong aad: err = %v, want ErrDecrypt", err)
	}
	if _, err := ring.Open(keyID, sealed[:4], []byte("job-1")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("truncated: err = %v, want ErrDecrypt", err)
	}
	if _, err := ring.Open("k9", sealed, []byte("job-1")); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown key: err = %v, want ErrUnknownKey", err)
	}
}

// Rotation: after k2 becomes active, k1 ciphertext still opens, new seals use
// k2, and once k1 is removed its ciphertext reports ErrUnknownKey.
func TestKeyRotation(t *testing.T) {
	k1, k2 := newKey(t), newKey(t)
	before, _ := ParseKeyring("k1:"+k1, "k1")
	oldID, oldSealed, _ := before.Seal([]byte("old"), []byte("a"))

	rotated, err := ParseKeyring("k1:"+k1+",k2:"+k2, "k2")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rotated.Open(oldID, oldSealed, []byte("a")); err != nil || string(got) != "old" {
		t.Fatalf("old ciphertext after rotation = %q, %v", got, err)
	}
	if newID, _, _ := rotated.Seal([]byte("new"), []byte("a")); newID != "k2" {
		t.Fatalf("new seals use %q, want k2", newID)
	}

	retired, _ := ParseKeyring("k2:"+k2, "k2")
	if _, err := retired.Open(oldID, oldSealed, []byte("a")); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("retired key: err = %v, want ErrUnknownKey", err)
	}
}

func TestParseKeyringRejectsBadConfig(t *testing.T) {
	good := newKey(t)
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	cases := map[string][2]string{
		"empty":          {"", "k1"},
		"no active":      {"k1:" + good, ""},
		"active missing": {"k1:" + good, "k2"},
		"no colon":       {good, "k1"},
		"empty id":       {":" + good, "k1"},
		"bad base64":     {"k1:not*base64", "k1"},
		"wrong length":   {"k1:" + short, "k1"},
		"duplicate id":   {"k1:" + good + ",k1:" + good, "k1"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseKeyring(c[0], c[1])
			if err == nil {
				t.Fatal("ParseKeyring accepted bad config")
			}
			if strings.Contains(err.Error(), good) {
				t.Fatal("error message leaks key material")
			}
		})
	}
}

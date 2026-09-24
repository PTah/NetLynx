package secrets

import (
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := NewBox(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := "snmp-community-secret"
	enc, err := b.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(enc) {
		t.Fatalf("expected enc prefix, got %q", enc)
	}
	if strings.Contains(enc, plain) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	got, err := b.Open(enc)
	if err != nil || got != plain {
		t.Fatalf("open: %q %v", got, err)
	}
}

func TestSealEmpty(t *testing.T) {
	b, _ := NewBox(testKey(t))
	s, err := b.Seal("")
	if err != nil || s != "" {
		t.Fatalf("got %q %v", s, err)
	}
}

func TestOpenLegacyPlaintext(t *testing.T) {
	b, _ := NewBox(testKey(t))
	got, err := b.Open("public")
	if err != nil || got != "public" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestOpenWithoutKeyRejectsCipher(t *testing.T) {
	b, _ := NewBox(testKey(t))
	enc, _ := b.Seal("x")
	var nilBox *Box
	_, err := nilBox.Open(enc)
	if err != ErrNoKey {
		// Open on nil box: Enabled false
		if err == nil {
			t.Fatal("expected error")
		}
	}
	_, err = (*Box)(nil).Open(enc)
	if err != ErrNoKey {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
}

func TestWrongKey(t *testing.T) {
	b1, _ := NewBox(testKey(t))
	enc, _ := b1.Seal("secret")
	k2 := testKey(t)
	k2[0] ^= 0xff
	b2, _ := NewBox(k2)
	_, err := b2.Open(enc)
	if err != ErrDecryptFail {
		t.Fatalf("want ErrDecryptFail, got %v", err)
	}
}

func TestNonceUnique(t *testing.T) {
	b, _ := NewBox(testKey(t))
	a, _ := b.Seal("same")
	c, _ := b.Seal("same")
	if a == c {
		t.Fatal("identical ciphertext for same plaintext (nonce reuse?)")
	}
}

func TestMaybeSealWithoutBox(t *testing.T) {
	s, err := MaybeSeal(nil, "public")
	if err != nil || s != "public" {
		t.Fatalf("%q %v", s, err)
	}
}

func TestLoadMasterKeyBase64(t *testing.T) {
	raw, err := GenerateMasterKeyBase64()
	if err != nil {
		t.Fatal(err)
	}
	key, err := LoadMasterKey(raw, "")
	if err != nil || len(key) != 32 {
		t.Fatalf("%v len=%d", err, len(key))
	}
}

func TestNewBoxBadLen(t *testing.T) {
	_, err := NewBox([]byte("short"))
	if err == nil {
		t.Fatal("expected error")
	}
}

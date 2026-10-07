package gateway

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const testKeyHex = "4c0b569e4c96df157eee1b65dd0e4d41"

// openLikeGuacamole mirrors guacamole-auth-json's UserDataService: base64-decode, AES-128-CBC
// decrypt with a zero IV and PKCS#5 unpadding, split the 32-byte HMAC from the JSON, verify.
func openLikeGuacamole(t *testing.T, key GuacKey, data string) GuacPayload {
	t.Helper()
	ct, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key[:])
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(plain, ct)
	pad := int(plain[len(plain)-1])
	if pad < 1 || pad > 16 || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		t.Fatal("bad padding")
	}
	plain = plain[:len(plain)-pad]
	sig, body := plain[:32], plain[32:]
	mac := hmac.New(sha256.New, key[:])
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		t.Fatal("signature does not verify")
	}
	var p GuacPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func samplePayload() GuacPayload {
	now := time.UnixMilli(1_790_000_000_000)
	return NewGuacPayload("rita@example.com", map[string]GuacConnection{
		"study/rita-vm-credentials": {Protocol: "rdp", Parameters: map[string]string{"hostname": "10.240.0.70", "port": "3389"}},
	}, now, HandoffTTL)
}

func TestSealRoundTripsLikeGuacamole(t *testing.T) {
	key, err := ParseGuacKey(testKeyHex)
	if err != nil {
		t.Fatal(err)
	}
	data, err := key.Seal(samplePayload())
	if err != nil {
		t.Fatal(err)
	}
	p := openLikeGuacamole(t, key, data)
	if p.Username != "rita@example.com" || !p.SingleUse || p.Expires != 1_790_000_060_000 {
		t.Fatalf("payload = %+v", p)
	}
	if p.Connections["study/rita-vm-credentials"].Parameters["hostname"] != "10.240.0.70" {
		t.Fatalf("connections = %+v", p.Connections)
	}
}

// Cross-check against OpenSSL, the tool Guacamole's own documentation uses to build payloads.
func TestSealMatchesOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not installed")
	}
	key, _ := ParseGuacKey(testKeyHex)
	data, _ := key.Seal(samplePayload())
	dir := t.TempDir()
	ct, _ := base64.StdEncoding.DecodeString(data)
	in := filepath.Join(dir, "ct.bin")
	_ = os.WriteFile(in, ct, 0o600)
	plain, err := exec.Command("openssl", "enc", "-d", "-aes-128-cbc", "-K", testKeyHex, "-iv", "00000000000000000000000000000000", "-in", in).Output()
	if err != nil {
		t.Fatalf("openssl decrypt: %v", err)
	}
	body := filepath.Join(dir, "body.json")
	_ = os.WriteFile(body, plain[32:], 0o600)
	sig, err := exec.Command("openssl", "dgst", "-sha256", "-mac", "HMAC", "-macopt", "hexkey:"+testKeyHex, "-binary", body).Output()
	if err != nil {
		t.Fatalf("openssl hmac: %v", err)
	}
	if !bytes.Equal(sig, plain[:32]) {
		t.Fatalf("signature mismatch: openssl %s vs payload %s", hex.EncodeToString(sig), hex.EncodeToString(plain[:32]))
	}
}

func TestParseGuacKeyRejectsBadKeys(t *testing.T) {
	for _, k := range []string{"", "abc", testKeyHex + "00", "zz0b569e4c96df157eee1b65dd0e4d41"} {
		if _, err := ParseGuacKey(k); err == nil {
			t.Errorf("accepted %q", k)
		}
	}
}

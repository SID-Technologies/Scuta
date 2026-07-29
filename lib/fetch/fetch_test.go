package fetch

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sid-technologies/scuta/lib/sigverify"
)

// newServer serves payload at /data and, when sig is non-nil, the signature at
// /data.sig.
func newServer(t *testing.T, payload []byte, sig []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/data", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	})
	if sig != nil {
		mux.HandleFunc("/data.sig", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(sig)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func keypair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	pub, priv, err := sigverify.GenerateEd25519Keys()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return pub, priv
}

func TestVerified_SignedOK(t *testing.T) {
	pub, priv := keypair(t)
	payload := []byte("tools: {}\n")
	sig, err := sigverify.Sign(payload, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	srv := newServer(t, payload, sig)

	got, err := Verified(srv.URL+"/data", Options{PublicKeyPEM: pub, RequireSignature: true})
	if err != nil {
		t.Fatalf("expected verified fetch to succeed: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch: got %q", got)
	}
}

func TestVerified_BadSignatureFailsEvenWhenOptional(t *testing.T) {
	pub, _ := keypair(t)
	_, otherPriv := keypair(t)
	payload := []byte("tools: {}\n")
	badSig, err := sigverify.Sign(payload, otherPriv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	srv := newServer(t, payload, badSig)

	if _, err := Verified(srv.URL+"/data", Options{PublicKeyPEM: pub}); err == nil {
		t.Fatal("expected failure for invalid signature even without RequireSignature")
	}
}

func TestVerified_MissingSigRequired(t *testing.T) {
	pub, _ := keypair(t)
	srv := newServer(t, []byte("tools: {}\n"), nil)

	if _, err := Verified(srv.URL+"/data", Options{PublicKeyPEM: pub, RequireSignature: true}); err == nil {
		t.Fatal("expected failure when required signature is missing")
	}
}

func TestVerified_MissingSigOptional(t *testing.T) {
	pub, _ := keypair(t)
	payload := []byte("tools: {}\n")
	srv := newServer(t, payload, nil)

	got, err := Verified(srv.URL+"/data", Options{PublicKeyPEM: pub})
	if err != nil {
		t.Fatalf("expected lenient fetch to succeed: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch: got %q", got)
	}
}

func TestVerified_RequiredWithoutKey(t *testing.T) {
	srv := newServer(t, []byte("x"), nil)

	_, err := Verified(srv.URL+"/data", Options{RequireSignature: true})
	if err == nil {
		t.Fatal("expected failure when signature required but no key configured")
	}
	if !strings.Contains(err.Error(), "no public key") {
		t.Errorf("expected misconfiguration error, got %v", err)
	}
}

func TestVerified_NoKeyPlainFetch(t *testing.T) {
	payload := []byte("plain")
	srv := newServer(t, payload, nil)

	got, err := Verified(srv.URL+"/data", Options{})
	if err != nil {
		t.Fatalf("plain fetch failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch: got %q", got)
	}
}

func TestVerified_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	if _, err := Verified(srv.URL+"/data", Options{}); err == nil {
		t.Fatal("expected error for 404 payload")
	}
}

// Package fetch provides HTTP fetching with optional detached-signature
// verification. It is the trust boundary for every piece of remote metadata
// Scuta consumes: the registry, remote policy, and remote org config. Each of
// those is an instruction channel (it can change what gets installed and how
// strictly it is verified), so their transport deserves the same fail-closed
// treatment as release binaries.
package fetch

import (
	"io"
	"net/http"
	"time"

	"github.com/sid-technologies/scuta/lib/errors"
	"github.com/sid-technologies/scuta/lib/output"
	"github.com/sid-technologies/scuta/lib/sigverify"
)

const (
	// defaultTimeout bounds each HTTP request.
	defaultTimeout = 10 * time.Second

	// defaultMaxSize caps response bodies (10 MB).
	defaultMaxSize = 10 * 1024 * 1024

	// maxSigSize caps detached signature bodies (64 KB is generous; an
	// Ed25519 signature is 64 bytes and an RSA-4096 one is 512).
	maxSigSize = 64 * 1024
)

// Options controls verification behavior for a fetch.
type Options struct {
	// PublicKeyPEM is the PEM-encoded trust root. When set, a detached
	// signature at <url>.sig is fetched and verified whenever available; a
	// present-but-invalid signature always fails the fetch.
	PublicKeyPEM []byte

	// RequireSignature makes the signature mandatory: the fetch fails when
	// the .sig is missing or when no public key is configured.
	RequireSignature bool

	// Timeout bounds each HTTP request. Zero means a 10s default.
	Timeout time.Duration

	// MaxSize caps the payload size in bytes. Zero means a 10 MB default.
	MaxSize int64
}

// Verified fetches url and, when a trust root is configured, verifies the
// payload against the detached signature served at url + ".sig".
//
// Behavior matrix:
//   - key set, .sig present:              verify; invalid signature = error
//   - key set, .sig missing, required:    error (fail closed)
//   - key set, .sig missing, optional:    warn, return unverified payload
//   - no key, required:                   error (misconfiguration)
//   - no key, optional:                   plain fetch
func Verified(url string, opts Options) ([]byte, error) {
	if opts.RequireSignature && len(opts.PublicKeyPEM) == 0 {
		return nil, errors.New("signed fetch required for %s but no public key is configured (set signature_public_key)", url)
	}

	client := &http.Client{
		Timeout: timeoutOrDefault(opts.Timeout),
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	data, err := get(client, url, maxSizeOrDefault(opts.MaxSize))
	if err != nil {
		return nil, err
	}

	if len(opts.PublicKeyPEM) == 0 {
		return data, nil
	}

	sig, sigErr := get(client, url+".sig", maxSigSize)
	if sigErr != nil {
		if opts.RequireSignature {
			return nil, errors.Wrap(sigErr, "signature required but fetching %s.sig failed", url)
		}
		output.Warning("No signature found for %s — content is unverified (%v)", url, sigErr)
		return data, nil
	}

	if err := sigverify.Verify(data, sig, opts.PublicKeyPEM); err != nil {
		return nil, errors.Wrap(err, "signature verification failed for %s", url)
	}

	output.Debugf("Signature verified for %s", url)
	return data, nil
}

// get performs a single bounded GET and returns the body.
func get(client *http.Client, url string, maxSize int64) ([]byte, error) {
	resp, err := client.Get(url) //nolint:noctx // bounded by client timeout
	if err != nil {
		return nil, errors.Wrap(err, "fetching %s", url)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("fetching %s returned %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return nil, errors.Wrap(err, "reading response from %s", url)
	}

	return data, nil
}

// timeoutOrDefault applies the default timeout when unset.
func timeoutOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultTimeout
	}
	return d
}

// maxSizeOrDefault applies the default size cap when unset.
func maxSizeOrDefault(n int64) int64 {
	if n <= 0 {
		return defaultMaxSize
	}
	return n
}

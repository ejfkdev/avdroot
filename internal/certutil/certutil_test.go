package certutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

// The SPKI must match what the openssl incantation in Chromium's documentation
// produces, otherwise Chrome silently ignores the flag. This value was produced
// by that command for the Reqable CA on this machine, and is pinned so the Go
// implementation cannot drift from it.
const (
	realCA     = "/tmp/cacmp/app.crt"
	realCASPki = "nHhlCoiiwRRZzTU+d2MDZrdTs7bvRBrIWpFSTTyqWDU="
)

func TestSPKIMatchesOpenSSL(t *testing.T) {
	data, err := os.ReadFile(realCA)
	if err != nil {
		t.Skipf("reference certificate unavailable: %v", err)
	}
	cert, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := SPKI(cert); got != realCASPki {
		t.Errorf("SPKI() = %q, want %q (the value openssl produces)", got, realCASPki)
	}
	if !IsCA(cert) {
		t.Error("the reference certificate should be a CA")
	}
	if !strings.Contains(Subject(cert), "Reqable") {
		t.Errorf("Subject() = %q, want it to mention Reqable", Subject(cert))
	}
}

// A generated certificate exercises the same path without needing an asset, and
// pins the encoding: base64 of SHA-256 over the DER SubjectPublicKeyInfo.
func TestSPKIOnGeneratedCert(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	pemCert, err := Parse(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err != nil {
		t.Fatalf("PEM: %v", err)
	}
	derCert, err := Parse(der)
	if err != nil {
		t.Fatalf("DER: %v", err)
	}
	if SPKI(pemCert) != SPKI(derCert) {
		t.Error("the two encodings produced different SPKI values")
	}
	if len(SPKI(derCert)) != 44 || !strings.HasSuffix(SPKI(derCert), "=") {
		t.Errorf("SPKI() = %q, which does not look like base64 of a 32-byte digest", SPKI(derCert))
	}
	if !IsCA(derCert) {
		t.Error("IsCA() should be true for a CA certificate")
	}
}

func TestParseRejectsNonCertificate(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"text", "hello, not a certificate"},
		{"json", `{"subject":"nope"}`},
		{"pem of something else", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"},
	} {
		if _, err := Parse([]byte(tc.data)); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

// A leaf certificate is not usable for interception, and must not be reported
// as one.
func TestIsCARejectsLeaf(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := Parse(der)
	if err != nil {
		t.Fatal(err)
	}
	if IsCA(cert) {
		t.Error("a leaf certificate should not report as a CA")
	}
}

// The four formats a proxy's export menu offers must be handled, or produce a
// clear explanation. PEM, DER and Android's ".0" are all accepted; PKCS#12 is a
// container and is reported as such.
func TestExportFormats(t *testing.T) {
	dir := "/tmp/fmt"
	if _, err := os.Stat(dir + "/export.pem"); err != nil {
		t.Skip("no fixtures; run the export comparison to create them")
	}
	for _, name := range []string{"export.pem", "export.crt", "export.0"} {
		data, err := os.ReadFile(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := Parse(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if SPKI(cert) != realCASPki {
			t.Errorf("%s: SPKI = %q, want the reference value", name, SPKI(cert))
		}
	}

	// A .p12 must be recognised and explained, not reported as garbage.
	for _, name := range []string{"export-nopass.p12", "export-pass.p12"} {
		data, err := os.ReadFile(dir + "/" + name)
		if err != nil {
			continue
		}
		if !IsPKCS12(data) {
			t.Errorf("%s was not recognised as PKCS#12", name)
		}
		_, err = Parse(data)
		if err == nil {
			t.Errorf("%s parsed, which is unexpected", name)
		}
		if !strings.Contains(err.Error(), "PKCS#12") || !strings.Contains(err.Error(), ".pem") {
			t.Errorf("%s: the error should name PKCS#12 and suggest an export format, got: %v", name, err)
		}
	}
}

// A bundle holding several certificates must yield the CA, since that is what
// trusting an issuer needs.
func TestParsePrefersCAInBundle(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "the CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(11),
		Subject:      pkix.Name{CommonName: "a leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	// Leaf first, so a naive "first block wins" would return the wrong one.
	bundle := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})...)

	cert, err := Parse(bundle)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !IsCA(cert) {
		t.Errorf("Parse returned %q, want the CA from the bundle", Subject(cert))
	}
	if cert.Subject.CommonName != "the CA" {
		t.Errorf("Subject = %q, want the CA", Subject(cert))
	}
}

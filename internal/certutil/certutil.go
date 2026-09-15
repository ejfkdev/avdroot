// Package certutil handles the certificate operations avdroot needs: reading a
// CA certificate in either common encoding, and deriving the fingerprints that
// Android and Chromium identify it by.
package certutil

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// ErrNotCertificate is returned when the input is not a certificate.
var ErrNotCertificate = errors.New("certutil: not a certificate")

// oidData is 1.2.840.113549.1.7.1, the content type every PKCS#12 file carries
// in its outer ContentInfo.
var oidData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}

// Parse reads a certificate from PEM or DER bytes.
//
// PEM and DER are both accepted, without caring about the file extension: a
// ".crt" may hold either, and Android's ".0" files hold PEM. A file containing
// several certificates is searched for a CA, since that is what a caller
// wanting to trust an issuer needs.
func Parse(data []byte) (*x509.Certificate, error) {
	var first *x509.Certificate
	for rest := data; len(rest) > 0; {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if first == nil {
			first = cert
		}
		// Prefer a CA, so a bundle yields the issuer rather than whichever
		// certificate happened to come first.
		if IsCA(cert) {
			return cert, nil
		}
	}
	if first != nil {
		return first, nil
	}

	// Some tools emit a bare base64 body without the PEM armour.
	if trimmed := strings.TrimSpace(string(data)); !strings.Contains(trimmed, "-----") {
		if raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(trimmed), "")); err == nil {
			if cert, err := x509.ParseCertificate(raw); err == nil {
				return cert, nil
			}
		}
	}

	if cert, err := x509.ParseCertificate(data); err == nil {
		return cert, nil
	}

	// PKCS#12 holds certificates but is a container, not a certificate, and
	// decoding it needs the password and the container's own algorithms. Saying
	// so is more useful than a parse error.
	if IsPKCS12(data) {
		return nil, fmt.Errorf("%w: this is a PKCS#12 (.p12) container, which cannot be read here; "+
			"export the certificate as PEM instead — Reqable offers .pem, .crt and .0, all of which work",
			ErrNotCertificate)
	}
	return nil, fmt.Errorf("%w: not PEM, DER or base64 certificate data", ErrNotCertificate)
}

// IsPKCS12 reports whether data looks like a PKCS#12 container.
//
// Decoding one needs its password and its own encryption algorithms, so this
// only recognises the shape: a structure whose first field is version 3, which
// is what every PFX file starts with, and which contains the PKCS#12 object
// identifier further in. It is only consulted once certificate parsing has
// already failed.
func IsPKCS12(data []byte) bool {
	// PFX ::= SEQUENCE { version INTEGER (3), authSafe ContentInfo }
	var pfx struct {
		Version  int
		AuthSafe struct {
			ContentType asn1.ObjectIdentifier
			Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
		}
	}
	if _, err := asn1.Unmarshal(data, &pfx); err != nil {
		return false
	}
	// The version is what distinguishes this from a certificate, whose second
	// element is a structure rather than an integer.
	return pfx.Version == 3 && pfx.AuthSafe.ContentType.Equal(oidData)
}

// SPKI returns the base64 SHA-256 of the certificate's SubjectPublicKeyInfo.
//
// This is the value Chromium's --ignore-certificate-errors-spki-list takes. It
// is the same digest that
//
//	openssl x509 -pubkey -noout | openssl pkey -pubin -outform der |
//	  openssl dgst -sha256 -binary | openssl enc -base64
//
// produces, computed from the certificate instead of requiring openssl.
func SPKI(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Subject renders the certificate's subject for display.
func Subject(cert *x509.Certificate) string {
	return cert.Subject.String()
}

// IsCA reports whether the certificate may act as a certificate authority,
// which a certificate used for interception must be.
func IsCA(cert *x509.Certificate) bool {
	return cert.IsCA && cert.BasicConstraintsValid
}

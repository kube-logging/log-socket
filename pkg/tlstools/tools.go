package tlstools

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"
)

func GenerateSelfSignedCA() (*x509.Certificate, crypto.PrivateKey, error) {
	prvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		NotAfter:              time.Now().Add(12 * time.Hour),
		NotBefore:             time.Now(),
		PublicKeyAlgorithm:    x509.ECDSA,
		PublicKey:             &prvKey.PublicKey,
		SerialNumber:          big.NewInt(0),
		Subject: pkix.Name{
			CommonName: "Test CA",
		},
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &prvKey.PublicKey, prvKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, prvKey, nil
}

func GenerateTLSCert(caCert *x509.Certificate, caKey crypto.PrivateKey, serial *big.Int, dnsNames []string, ipAddrs []net.IP) (cert tls.Certificate, err error) {
	prvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return
	}
	tmpl := &x509.Certificate{
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:           dnsNames,
		IPAddresses:        ipAddrs,
		KeyUsage:           x509.KeyUsageDigitalSignature,
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(12 * time.Hour),
		PublicKeyAlgorithm: x509.ECDSA,
		PublicKey:          &prvKey.PublicKey,
		SerialNumber:       serial,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &prvKey.PublicKey, caKey)
	if err != nil {
		return
	}
	leaf, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return
	}
	cert.Certificate = append(cert.Certificate, certBytes)
	cert.Leaf = leaf
	cert.PrivateKey = prvKey
	return
}

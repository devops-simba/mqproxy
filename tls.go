package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
)

type CertificateInformation struct {
	CertificateFile string `yaml:"cert"`
	PrivateKeyFile  string `yaml:"key"`
}

func (this *CertificateInformation) Load() (tls.Certificate, error) {
	return tls.LoadX509KeyPair(this.CertificateFile, this.PrivateKeyFile)
}

type TlsServerConfiguration struct {
	// Certificate of the server
	Certificate *CertificateInformation `yaml:"certificate,omitempty"`
	// RequireClientValidation should we verify certificate of the client?
	RequireClientValidation bool `yaml:"requireClientValidation,omitempty"`
	// ClientValidationCAFiles CA files that should be used to verify client certificate
	ClientValidationCAFiles []string `yaml:"caFiles,omitempty"`
}

func (this *TlsServerConfiguration) IsSecure() bool { return this.Certificate != nil }
func (this *TlsServerConfiguration) LoadAsTlsConfig() (*tls.Config, error) {
	if this.Certificate == nil {
		return nil, nil
	}

	cert, err := this.Certificate.Load()
	if err != nil {
		return nil, err
	}

	result := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	if this.RequireClientValidation {
		if len(this.ClientValidationCAFiles) != 0 {
			result.ClientAuth = tls.RequireAndVerifyClientCert
			result.ClientCAs = x509.NewCertPool()
			for i := 0; i < len(this.ClientValidationCAFiles); i++ {
				caFile := this.ClientValidationCAFiles[i]
				pem, err := ioutil.ReadFile(caFile)
				if err != nil {
					return nil, err
				}
				if result.ClientCAs.AppendCertsFromPEM(pem) {
					return nil, fmt.Errorf("Failed to parse CA file: %v", caFile)
				}
			}
		} else {
			result.ClientAuth = tls.RequireAnyClientCert
		}
	}
	return result, nil
}

package main

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
)

const (
	WS    = "ws"
	WSS   = "wss"
	MQTT  = "mqtt"
	MQTTS = "mqtts"
)

var (
	ErrSchemeIsNotSupported = errors.New("Address scheme is not supported ")
	ErrTlsConfigIsRequired  = errors.New("TLS configuration is required as this is a secure protocol")
)

type ClientHandler = func(connector MQTTConnector, conn net.Conn)

// Endpoint This interface represent a general representation of a proxy endpoint
type MQTTConnector interface {
	// IsSecure Return true if this is a secure connection and false otherwise
	IsSecure() bool

	// GetScheme Get scheme of this endpoint
	GetScheme() string

	// GetScheme Get address of this endpoint
	GetAddress() string

	// Connect Connect to the server of this endpoint
	Connect(clientCert *tls.Certificate) (net.Conn, error)

	// Listen Listen for clients and for each connected client call ``handler`` to handle this client
	Listen(stop <-chan struct{}, handler ClientHandler) (stopped <-chan struct{}, err error)
}

func NewConnector(address *url.URL, tlsConfig *tls.Config) (result MQTTConnector, err error) {
	result, err = NewMQTTConnector(address, tlsConfig)
	if err == nil {
		return result, nil
	}
	if err != ErrSchemeIsNotSupported {
		return nil, err
	}

	result, err = NewWSConnectorImpl(address, tlsConfig)
	if err == nil {
		return result, err
	}
	if err != ErrSchemeIsNotSupported {
		return nil, err
	}

	return nil, ErrSchemeIsNotSupported
}

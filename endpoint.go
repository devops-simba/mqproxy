package main

import (
	"net"

	"github.com/devops-simba/helpers"
)

type ClientHandler = func(conn net.Conn)

// MQTTEndpoint general representation of a MQTT endpoint.
type MQTTEndpoint interface {
	// IsSecure Return ``true`` if this connector is secure and ``false`` otherwise
	IsSecure() bool

	// GetProtocol Get protocol of this connector
	GetProtocol() string

	// GetScheme Get address of this endpoint
	GetAddress() string
}

// MQTTServerEndpoint general representation of MQTT server endpoint
type MQTTServerEndpoint interface {
	MQTTEndpoint

	// Create a service that we may use to listen for clients using this protocol
	CreateListenService(serviceName, frontendName string, handler ClientHandler) helpers.Service
}

// MQTTClientEndpoint general representation of a client for MQTT
type MQTTClientEndpoint interface {
	MQTTEndpoint

	// Connect connect to the target of this endpoint
	Connect(serviceName, backendName string) (net.Conn, error)
}

type MQTTServerEndpointConfig struct {
	TlsServerConfiguration `yaml:",inline"`
	// Address address of the endpoint(address that we should listen on)
	Address string `yaml:"address"`
}

type MQTTClientEndpointConfig struct {
	// Address address of the server of this endpoint(address that we should connect to)
	Address string `yaml:"address"`
	// ConnectionCertificate if this is a secure connection, this is certificate that we should use to connect to the backend
	ConnectionCertificate *CertificateInformation `yaml:"connectionCertificate,omitempty"`
}

type MQTTEndpointFactory interface {
	// CreateServerEndpoint create an endpoint for this protocol
	CreateServerEndpoint(config MQTTServerEndpointConfig) (MQTTServerEndpoint, error)
	// CreateClientEndpoint create an endpoint that may be used to connect to a MQTT broker
	CreateClientEndpoint(config MQTTClientEndpointConfig) (MQTTClientEndpoint, error)
}

var (
	factories = make([]MQTTEndpointFactory, 0)
)

const (
	SchemaNotSupported            = helpers.StringError("Schema is not supported")
	InvalidBackendAddress         = helpers.StringError("Invalid backend address")
	TlsInfoIsOnlyForSecureSchemes = helpers.StringError(
		"Certificate and client validation is only available for secure schemes")
	MissingTlsInfoForSecureScheme = helpers.StringError("Missing TLS certificate for secure scheme")
)

func RegisterEndpointFactory(factory MQTTEndpointFactory) {
	factories = append(factories, factory)
}

func CreateServerEndpoint(config MQTTServerEndpointConfig) (MQTTServerEndpoint, error) {
	for i := 0; i < len(factories); i++ {
		endpoint, err := factories[i].CreateServerEndpoint(config)
		if err != nil {
			return nil, err
		}
		if endpoint != nil {
			return endpoint, nil
		}
	}
	return nil, SchemaNotSupported
}

func CreateClientEndpoint(config MQTTClientEndpointConfig) (MQTTClientEndpoint, error) {
	for i := 0; i < len(factories); i++ {
		endpoint, err := factories[i].CreateClientEndpoint(config)
		if err != nil {
			return nil, err
		}
		if endpoint != nil {
			return endpoint, nil
		}
	}
	return nil, SchemaNotSupported
}

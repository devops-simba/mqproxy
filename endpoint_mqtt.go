package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/devops-simba/helpers"
)

//region GLOBALS
const maxTemporaryNetworkDelay = time.Second

func init() {
	RegisterEndpointFactory(mqtt_EndpointFactory(true))
}

//endregion

//region mqtt_ClientEndpoint
type mqtt_ClientEndpoint struct {
	// ServerAddress Address of the server that we should connect to it
	ServerAddress *url.URL
	// Certificate that we should use to connect to server.
	// This is only valid in case of WSS.
	Certificate *CertificateInformation
}

func (this *mqtt_ClientEndpoint) IsSecure() bool      { return this.ServerAddress.Scheme == "mqtts" }
func (this *mqtt_ClientEndpoint) GetProtocol() string { return "mqtt" }
func (this *mqtt_ClientEndpoint) GetAddress() string {
	host := GetUrlHostname(this.ServerAddress)
	port := GetUrlPort(this.ServerAddress)
	return fmt.Sprintf("%s://%s:%s", this.ServerAddress.Scheme, host, port)
}
func (this *mqtt_ClientEndpoint) Connect(serviceName, backendName string) (net.Conn, error) {
	addr := net.JoinHostPort(GetUrlHostname(this.ServerAddress), GetUrlPort(this.ServerAddress))
	if this.IsSecure() {
		cert, err := this.Certificate.Load()
		if err != nil {
			return nil, err
		}

		return tls.Dial("tcp", addr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	} else {
		return net.Dial("tcp", addr)
	}
}

//endregion

//region mqtt_Listener
type mqtt_Listener struct {
	Name          string
	Secure        bool
	ListenAddress string
	Logger        helpers.Logger
	TlsConfig     *TlsServerConfiguration
	Handler       ClientHandler
	Stopped       chan struct{}

	listener net.Listener
}

func (this *mqtt_Listener) newListener() error {
	var err error
	if this.Secure {
		var tlsConfig *tls.Config
		tlsConfig, err = this.TlsConfig.LoadAsTlsConfig()
		if err == nil {
			this.listener, err = tls.Listen("tcp", this.ListenAddress, tlsConfig)
		}
	} else {
		this.listener, err = net.Listen("tcp", this.ListenAddress)
	}
	return err
}

func (this *mqtt_Listener) GetName() string { return this.Name }
func (this *mqtt_Listener) Run() error {
	var err error
	if err = this.newListener(); err != nil {
		return err
	}
	defer this.listener.Close()

	var tempDelay time.Duration
	for {
		conn, err := this.listener.Accept()
		if err != nil {
			select {
			case <-this.Stopped:
				return helpers.ErrServiceStopped
			default:
			}

			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}

				if tempDelay > maxTemporaryNetworkDelay {
					tempDelay = maxTemporaryNetworkDelay
				}

				this.Logger.Verbosef(8, "Accept returned a temporary error: %v; retrying in %v", err, tempDelay)
				timedOut := time.After(tempDelay)
				select {
				case <-this.Stopped:
					return helpers.ErrServiceStopped
				case <-timedOut:
				}
				continue
			} else {
				this.Logger.Errorf("Accept returned an error: %v; Stopping accept loop", err)
				return err
			}
		}

		tempDelay = 0
		go this.Handler(conn)
	}
}
func (this *mqtt_Listener) Shutdown() {
	defer func() {
		if r := recover(); r == nil {
			this.listener.Close()
		}
	}()
	close(this.Stopped)
}

//endregion

//region mqtt_ServerEndpoint
type mqtt_ServerEndpoint struct {
	// ListenAddress address that we should listen on it
	ListenAddress *url.URL
	// Certificate
	TlsConfig TlsServerConfiguration
}

func (this *mqtt_ServerEndpoint) IsSecure() bool      { return this.ListenAddress.Scheme == "mqtts" }
func (this *mqtt_ServerEndpoint) GetProtocol() string { return "mqtt" }
func (this *mqtt_ServerEndpoint) GetAddress() string {
	host := GetUrlHostname(this.ListenAddress)
	port := GetUrlPort(this.ListenAddress)
	return fmt.Sprintf("%s://%s:%s", this.ListenAddress.Scheme, host, port)
}
func (this *mqtt_ServerEndpoint) CreateListenService(serviceName, frontendName string, handler ClientHandler) helpers.Service {
	name := fmt.Sprintf("%s/%s/mqtt-listener[%s]", serviceName, frontendName, this.GetAddress())
	return &mqtt_Listener{
		Name:          name,
		Secure:        this.IsSecure(),
		ListenAddress: net.JoinHostPort(GetUrlHostname(this.ListenAddress), GetUrlPort(this.ListenAddress)),
		Logger:        CreateLogger(name),
		TlsConfig:     &this.TlsConfig,
		Handler:       handler,
		Stopped:       make(chan struct{}),
	}
}

//endregion

//region mqtt_EndpointFactory
type mqtt_EndpointFactory bool

func (this mqtt_EndpointFactory) CreateServerEndpoint(config MQTTServerEndpointConfig) (MQTTServerEndpoint, error) {
	var u *url.URL
	var err error
	if u, err = ParseUrl(config.Address, "mqtt"); err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "mqtt":
		if config.Certificate != nil || config.RequireClientValidation {
			return nil, TlsInfoIsOnlyForSecureSchemes
		}
		return &mqtt_ServerEndpoint{ListenAddress: u}, nil

	case "mqtts":
		if config.Certificate == nil {
			return nil, MissingTlsInfoForSecureScheme
		}
		return &mqtt_ServerEndpoint{
			ListenAddress: u,
			TlsConfig:     config.TlsServerConfiguration,
		}, nil

	default:
		return nil, nil
	}
}
func (this mqtt_EndpointFactory) CreateClientEndpoint(config MQTTClientEndpointConfig) (MQTTClientEndpoint, error) {
	var u *url.URL
	var err error
	if u, err = ParseUrl(config.Address, "mqtt"); err != nil {
		return nil, err
	}
	if GetUrlHostname(u) == "0.0.0.0" {
		return nil, InvalidBackendAddress
	}

	switch u.Scheme {
	case "mqtt":
		if config.ConnectionCertificate != nil {
			return nil, TlsInfoIsOnlyForSecureSchemes
		}
		fallthrough
	case "mqtts":
		return &mqtt_ClientEndpoint{ServerAddress: u, Certificate: config.ConnectionCertificate}, nil

	default:
		return nil, nil
	}
}

//endregion

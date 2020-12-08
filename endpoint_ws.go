package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/devops-simba/helpers"

	"github.com/gorilla/websocket"
)

//region GLOBALS
var upgrader = websocket.Upgrader{
	// Timeout for WS upgrade request handshake
	HandshakeTimeout: 10 * time.Second,
	// Paho JS client expecting header Sec-WebSocket-Protocol:mqtt in Upgrade response during handshake.
	Subprotocols: []string{"mqttv3.1", "mqtt"},
	// Allow CORS
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func init() {
	RegisterEndpointFactory(ws_EndpointFactory(true))
}

//endregion

//region ws_ClientEndpoint
type ws_ClientEndpoint struct {
	// ServerAddress Address of the server that we should connect to it
	ServerAddress *url.URL
	// Certificate that we should use to connect to server.
	// This is only valid in case of WSS.
	Certificate *CertificateInformation
}

func (this *ws_ClientEndpoint) IsSecure() bool      { return this.ServerAddress.Scheme == "wss" }
func (this *ws_ClientEndpoint) GetProtocol() string { return "ws" }
func (this *ws_ClientEndpoint) GetAddress() string {
	host := GetUrlHostname(this.ServerAddress)
	port := GetUrlPort(this.ServerAddress)
	path := GetUrlDirPath(this.ServerAddress)
	return fmt.Sprintf("%s://%s:%s%s", this.ServerAddress.Scheme, host, port, path)
}
func (this *ws_ClientEndpoint) Connect(serviceName, backendName string) (net.Conn, error) {
	dialer := &websocket.Dialer{
		Subprotocols: []string{"mqtt"},
	}

	if this.Certificate != nil {
		cert, err := tls.LoadX509KeyPair(this.Certificate.CertificateFile, this.Certificate.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
		dialer.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	conn, _, err := dialer.Dial(this.GetAddress(), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to WS server: %v", err)
	}

	return &ws_Connection{Conn: conn}, nil
}

//endregion

//region ws_Connection
type ws_Connection struct {
	*websocket.Conn
	reader    io.Reader
	readLock  sync.Mutex
	writeLock sync.Mutex
}

func newWsConnection(conn *websocket.Conn) *ws_Connection {
	return &ws_Connection{Conn: conn}
}
func (this *ws_Connection) SetDeadline(t time.Time) error {
	if err := this.SetReadDeadline(t); err != nil {
		return err
	}

	return this.SetWriteDeadline(t)
}
func (this *ws_Connection) Write(buffer []byte) (int, error) {
	this.writeLock.Lock()
	defer this.writeLock.Unlock()

	if err := this.WriteMessage(websocket.BinaryMessage, buffer); err != nil {
		return 0, err
	}

	return len(buffer), nil
}
func (this *ws_Connection) Read(buffer []byte) (int, error) {
	this.readLock.Lock()
	defer this.readLock.Unlock()

	for {
		if this.reader == nil {
			// Advance to next message.
			var err error
			_, this.reader, err = this.NextReader()
			if err != nil {
				return 0, err
			}
		}

		n, err := this.reader.Read(buffer)
		if errors.Is(err, io.EOF) {
			// this message is finished
			this.reader = nil
			if n > 0 {
				return n, nil
			}

			// no data read from this reader, advance to next message
			continue
		}
		return n, err
	}
}
func (this *ws_Connection) Close() error { return this.Conn.Close() }

//endregion

//region ws_Listener
type ws_Listener struct {
	Name          string
	Secure        bool
	Logger        helpers.Logger
	Path          string
	ListenAddress string
	TlsConfig     *TlsServerConfiguration
	Handler       ClientHandler

	httpServer *http.Server
}

func (this *ws_Listener) handleRequest(w http.ResponseWriter, r *http.Request) {
	if rpath := GetUrlDirPath(r.URL); rpath != this.Path {
		this.Logger.Warnf("ws request received from %s for an invalid path: Got: %v, Expected: %v",
			r.RemoteAddr, rpath, this.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		this.Logger.Errorf("%v) Failed to upgrade to WS: %v", r.RemoteAddr, err)
		return
	}

	conn := newWsConnection(ws)
	this.Handler(conn)
}

func (this *ws_Listener) GetName() string { return this.Name }
func (this *ws_Listener) Run() error {
	if this.Secure != this.TlsConfig.IsSecure() {
		return helpers.StringError("Invalid TLS configuration")
	}

	tlsConfig, err := this.TlsConfig.LoadAsTlsConfig()
	if err != nil {
		return err
	}

	this.httpServer = &http.Server{
		TLSConfig: tlsConfig,
		Addr:      this.ListenAddress,
		Handler:   http.HandlerFunc(this.handleRequest),
	}

	if this.Secure {
		return this.httpServer.ListenAndServeTLS("", "")
	} else {
		return this.httpServer.ListenAndServe()
	}
}
func (this *ws_Listener) Shutdown() { this.httpServer.Shutdown(context.Background()) }

//endregion

//region ws_ServerEndpoint
type ws_ServerEndpoint struct {
	ListenAddress *url.URL
	TlsConfig     TlsServerConfiguration
}

func (this *ws_ServerEndpoint) IsSecure() bool      { return this.ListenAddress.Scheme == "wss" }
func (this *ws_ServerEndpoint) GetProtocol() string { return "ws" }
func (this *ws_ServerEndpoint) GetAddress() string {
	host := GetUrlHostname(this.ListenAddress)
	port := GetUrlPort(this.ListenAddress)
	path := GetUrlDirPath(this.ListenAddress)
	return fmt.Sprintf("%s://%s:%v%s", this.ListenAddress.Scheme, host, port, path)
}
func (this *ws_ServerEndpoint) CreateListenService(serviceName, frontendName string, handler ClientHandler) helpers.Service {
	name := fmt.Sprintf("%s/%s/ws-listener[%s]", serviceName, frontendName, this.GetAddress())
	return &ws_Listener{
		Name:          name,
		Path:          GetUrlDirPath(this.ListenAddress),
		ListenAddress: net.JoinHostPort(GetUrlHostname(this.ListenAddress), GetUrlPort(this.ListenAddress)),
		Secure:        this.IsSecure(),
		Logger:        CreateLogger(name),
		TlsConfig:     &this.TlsConfig,
		Handler:       handler,
	}
}

//endregion

// region ws_EndpointFactory
type ws_EndpointFactory bool

func (this ws_EndpointFactory) CreateServerEndpoint(config MQTTServerEndpointConfig) (MQTTServerEndpoint, error) {
	var u *url.URL
	var err error
	if u, err = ParseUrl(config.Address, "ws"); err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "ws":
		if config.Certificate != nil || config.RequireClientValidation {
			return nil, TlsInfoIsOnlyForSecureSchemes
		}
		return &ws_ServerEndpoint{ListenAddress: u}, nil

	case "wss":
		if config.Certificate == nil {
			return nil, MissingTlsInfoForSecureScheme
		}
		return &ws_ServerEndpoint{
			ListenAddress: u,
			TlsConfig:     config.TlsServerConfiguration,
		}, nil

	default:
		return nil, nil
	}
}
func (this ws_EndpointFactory) CreateClientEndpoint(config MQTTClientEndpointConfig) (MQTTClientEndpoint, error) {
	var u *url.URL
	var err error
	if u, err = ParseUrl(config.Address, "ws"); err != nil {
		return nil, err
	}
	if GetUrlHostname(u) == "0.0.0.0" {
		return nil, InvalidBackendAddress
	}

	switch u.Scheme {
	case "ws":
		if config.ConnectionCertificate != nil {
			return nil, TlsInfoIsOnlyForSecureSchemes
		}
		fallthrough
	case "wss":
		return &ws_ClientEndpoint{ServerAddress: u, Certificate: config.ConnectionCertificate}, nil

	default:
		return nil, nil
	}
}

//endregion

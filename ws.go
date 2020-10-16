package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	log "github.com/golang/glog"

	"github.com/gorilla/websocket"
)

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

type WSConnectorImpl struct {
	Secure    bool
	Addr      string
	Path      string
	TlsConfig *tls.Config
}

func NewWSConnectorImpl(address *url.URL, tlsConfig *tls.Config) (*WSConnectorImpl, error) {
	var secure bool
	port := address.Port()
	host := address.Hostname()
	path := address.Path
	switch address.Scheme {
	case WS:
		if port == "" {
			port = "80"
		}
	case WSS:
		secure = true
		if port == "" {
			port = "443"
		}
	default:
		return nil, ErrSchemeIsNotSupported
	}

	if port == "" {
		if secure {
			port = "80"
		} else {
			port = "443"
		}
	}
	if path == "" {
		path = "/"
	}
	return &WSConnectorImpl{
		Secure:    secure,
		Addr:      net.JoinHostPort(host, port),
		Path:      path,
		TlsConfig: tlsConfig,
	}, nil
}

func (this *WSConnectorImpl) GetAddress() string {
	return fmt.Sprintf("%s://%s%s", this.GetScheme(), this.Addr, this.Path)
}

func (this *WSConnectorImpl) IsSecure() bool { return this.Secure }
func (this *WSConnectorImpl) GetScheme() string {
	if this.Secure {
		return WSS
	} else {
		return WS
	}
}
func (this *WSConnectorImpl) Connect(clientCert *tls.Certificate) (net.Conn, error) {
	dialer := &websocket.Dialer{
		Subprotocols: []string{"mqtt"},
	}

	if clientCert != nil {
		dialer.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{*clientCert},
		}
	}

	url := this.GetAddress()
	srv, _, err := dialer.Dial(url, nil)
	if err != nil {
		log.Errorf("Failed to connect to the broker at %s: %v", url, err)
		return nil, err
	}

	return NewWSConnection(srv), nil
}
func (this *WSConnectorImpl) Listen(stop <-chan struct{}, handler ClientHandler) (<-chan struct{}, error) {
	if this.Secure && this.TlsConfig == nil {
		return nil, ErrTlsConfigIsRequired
	}

	server := http.Server{
		Addr:      this.Addr,
		TLSConfig: this.TlsConfig,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != this.Path {
				log.Errorf("websocket request received for an invalid path: Got: %v, Expected: %v", r.URL.Path, this.Path)
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}

			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Errorf("Failed to upgrade http connection to websocket: %v", err)
				return
			}

			wsConn := NewWSConnection(ws)
			go handler(this, wsConn)
		}),
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		var err error
		log.Infof("Trying to start listening on %s", this.GetAddress())
		if this.Secure {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		log.Infof("Server `%s` stopped: %v", this.GetAddress(), err)
	}()
	go func() {
		select {
		case <-stop:
			server.Shutdown(context.Background())
		case <-stopped:
			// already stopped
		}
	}()

	return stopped, nil
}

package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	log "github.com/golang/glog"
)

const (
	maxTemporaryNetworkDelay = time.Second
)

type MQTTConnectorImpl struct {
	Secure    bool
	Addr      string
	TlsConfig *tls.Config
}

func NewMQTTConnector(address *url.URL, tlsConfig *tls.Config) (*MQTTConnectorImpl, error) {
	var secure bool
	host := address.Hostname()
	port := address.Port()
	switch address.Scheme {
	case MQTT:
		if port == "" {
			port = "80"
		}
	case MQTTS:
		secure = true
		if port == "" {
			port = "443"
		}
	default:
		return nil, ErrSchemeIsNotSupported
	}

	if address.Path != "" && address.Path != "/" {
		return nil, fmt.Errorf("%s addresses can't contain path", address.Scheme)
	}

	return &MQTTConnectorImpl{
		Secure:    secure,
		Addr:      net.JoinHostPort(host, port),
		TlsConfig: tlsConfig,
	}, nil
}

func (this *MQTTConnectorImpl) IsSecure() bool { return this.Secure }
func (this *MQTTConnectorImpl) GetScheme() string {
	if this.Secure {
		return MQTT
	} else {
		return MQTTS
	}
}
func (this *MQTTConnectorImpl) GetAddress() string { return this.GetScheme() + "://" + this.Addr }
func (this *MQTTConnectorImpl) Connect(clientCert *tls.Certificate) (net.Conn, error) {
	if this.Secure {
		if clientCert != nil {
			return tls.Dial("tcp", this.Addr, &tls.Config{
				Certificates: []tls.Certificate{*clientCert},
			})
		} else {
			return tls.Dial("tcp", this.Addr, nil)
		}
	} else {
		return net.Dial("tcp", this.Addr)
	}
}
func (this *MQTTConnectorImpl) Listen(stop <-chan struct{}, handler ClientHandler) (<-chan struct{}, error) {
	var err error
	var listener net.Listener
	log.Infof("Trying to start listening on %s", this.GetAddress())
	if this.Secure {
		if this.TlsConfig == nil {
			return nil, ErrTlsConfigIsRequired
		}
		listener, err = tls.Listen("tcp", this.Addr, this.TlsConfig)
	} else {
		listener, err = net.Listen("tcp", this.Addr)
	}
	if err != nil {
		return nil, err
	}

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		var tempDelay time.Duration
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-stop:
					log.Infof("Close singal received in %s", this.GetAddress())
					return
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

					log.Warningf("%s:Accept returned a temporary error: %v; retrying in %v", this.GetScheme(), err, tempDelay)
					timedOut := time.After(tempDelay)
					select {
					case <-stop:
					case <-timedOut:
					}
					continue
				} else {
					log.Errorf("%s:Accept returned an error: %v; Stopping accept loop", this.GetScheme(), err)
					return
				}
			}

			tempDelay = 0
			go handler(this, conn)
		}
	}()
	go func() {
		select {
		case <-stop:
			listener.Close()
		case <-stopped:
			listener.Close()
		}
	}()

	return stopped, nil
}

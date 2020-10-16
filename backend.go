package main

import (
	"crypto/tls"
	"math/rand"
	"net"
)

type MQTTBackend interface {
	Connect(sourceConn net.Conn) (MQTTConnector, net.Conn, error)
}

type Backend struct {
	Name        string
	Connector   MQTTConnector
	Weight      int32
	Certificate *tls.Certificate
}

func (this *Backend) Connect(sourceConn net.Conn) (MQTTConnector, net.Conn, error) {
	conn, err := this.Connector.Connect(this.Certificate)
	return this.Connector, conn, err
}

type BackendGroup []*Backend

func randomSelect(items BackendGroup) int {
	weightSum := int32(0)
	for _, item := range items {
		weightSum += item.Weight
	}

	selection := rand.Int31n(weightSum)
	for i, item := range items {
		if selection < item.Weight {
			return i
		}
		selection -= item.Weight
	}

	return -1
}
func (this BackendGroup) Connect(sourceConn net.Conn) (MQTTConnector, net.Conn, error) {
	value := this
	var lastErr error
	for len(value) != 0 {
		selection := randomSelect(value)
		backend := value[selection]
		connector, conn, err := backend.Connect(sourceConn)
		if err == nil {
			return connector, conn, err
		}
		lastErr = err
	}

	return nil, nil, lastErr
}

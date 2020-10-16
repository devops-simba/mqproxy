package main

import (
	"net"

	log "github.com/golang/glog"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

type MQTTFrontend struct {
	Name      string
	Enabled   bool
	Connector MQTTConnector
	Backend   MQTTBackend
}

func (this *MQTTFrontend) Run(stop <-chan struct{}) (<-chan struct{}, error) {
	return this.Connector.Listen(stop, func(connector MQTTConnector, conn net.Conn) {
		log.Infof("Received a new connection from %s:%v", connector.GetScheme(), conn.RemoteAddr())
		_, backendConn, err := this.Backend.Connect(conn)
		if err != nil {
			log.Errorf("Failed to connect to the backend: %v", err)
		}

		stopped := make(chan error, 2)
		go proxy(stopped, conn, backendConn)
		go proxy(stopped, backendConn, conn)

		<-stopped

		// one connection closed, so we must close the other too of the connection
		conn.Close()
		backendConn.Close()

		// wait for other go routine to stop
		<-stopped
	})
}

func proxy(stopped chan<- error, input net.Conn, output net.Conn) {
	for {
		pkt, err := packets.ReadPacket(input)
		if err != nil {
			log.V(8).Infof("Error in reading package: %v", err)
			stopped <- err
			return
		}

		log.V(10).Infof("Read a packet: %#v", pkt)
		if err = pkt.Write(output); err != nil {
			log.V(6).Infof("Failed to write packet: %v", err)
			stopped <- err
			return
		}
	}
}

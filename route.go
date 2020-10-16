package main

import (
	"errors"
	"fmt"
	"net"
)

var (
	ErrNoRuleAcceptTheClient = errors.New("No client accept the client")
)

type MQTTRouteRule struct {
	Network net.IPNet
	Backend MQTTBackend
}
type MQTTRoute []MQTTRouteRule

func (this MQTTRoute) Connect(sourceConn net.Conn) (MQTTConnector, net.Conn, error) {
	addr := sourceConn.RemoteAddr()
	ip := net.ParseIP(addr.String())
	if ip == nil {
		return nil, nil, fmt.Errorf("Client address(%s) is not an IP", addr.String())
	}

	for i := 0; i < len(this); i++ {
		rule := this[i]
		if rule.Network.Contains(ip) {
			return rule.Backend.Connect(sourceConn)
		}
	}

	return nil, nil, ErrNoRuleAcceptTheClient
}

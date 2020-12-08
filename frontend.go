package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/devops-simba/helpers"
)

type frontendListener struct {
	Name             string
	Closed           bool
	Guard            sync.Mutex
	ConnectedClients []net.Conn
	Handler          func(net.Conn)
	Protocol         string
	Logger           helpers.Logger
	EndpointListener helpers.Service
}

func newFrontendListener(serviceName string, frontend *MQTTFrontend, handler func(net.Conn)) *frontendListener {
	name := fmt.Sprintf("frontend/%s/listener[%s]", frontend.Name, frontend.Endpoint.GetAddress())
	result := &frontendListener{
		Name:     name,
		Guard:    sync.Mutex{},
		Handler:  handler,
		Protocol: frontend.Endpoint.GetProtocol(),
		Logger:   CreateLogger(name),
	}
	result.EndpointListener = frontend.Endpoint.CreateListenService(serviceName, frontend.Name, result.handleClient)
	return result
}

func (this *frontendListener) addClient(c net.Conn) bool {
	this.Guard.Lock()
	defer this.Guard.Unlock()

	if this.Closed {
		return false
	}
	this.ConnectedClients = append(this.ConnectedClients, c)
	return true
}
func (this *frontendListener) removeClient(c net.Conn) {
	this.Guard.Lock()
	defer this.Guard.Unlock()

	for i := 0; i < len(this.ConnectedClients); i++ {
		if this.ConnectedClients[i] == c {
			this.ConnectedClients = append(this.ConnectedClients[:i], this.ConnectedClients[i+1:]...)
			break
		}
	}
}
func (this *frontendListener) handleClient(c net.Conn) {
	if !this.addClient(c) {
		return
	}

	this.Handler(c)

	this.removeClient(c)
}
func (this *frontendListener) GetName() string { return this.Name }
func (this *frontendListener) Run() error      { return this.EndpointListener.Run() }
func (this *frontendListener) Shutdown() {
	this.EndpointListener.Shutdown()

	this.Guard.Lock()
	if !this.Closed {
		this.Closed = true
		for i := 0; i < len(this.ConnectedClients); i++ {
			this.ConnectedClients[i].Close()
		}
	}
	this.ConnectedClients = nil
	this.Guard.Unlock()
}

type MQTTFrontend struct {
	// Name name of this frontend
	Name string
	// Endpoint of this frontend
	Endpoint MQTTServerEndpoint
}

func (this *MQTTFrontend) CreateListenService(serviceName string, handler func(net.Conn)) helpers.Service {
	return newFrontendListener(serviceName, this, handler)
}

type MQTTFrontendConfig struct {
	MQTTServerEndpointConfig `yaml:",inline"`
	Name                     string `yaml:"name"`
	Enabled                  *bool  `yaml:"enabled,omitempty"`
}

func CreateFrontend(config MQTTFrontendConfig) (*MQTTFrontend, bool, error) {
	server, err := CreateServerEndpoint(config.MQTTServerEndpointConfig)
	if err != nil {
		return nil, false, err
	}

	if config.Name == "" {
		config.Name = "frontend_" + server.GetAddress()
	}

	frontend := &MQTTFrontend{Name: config.Name, Endpoint: server}
	return frontend, GetOptionalBool(config.Enabled, true), nil
}

package main

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/devops-simba/helpers"
)

type MQTTService struct {
	Name      string
	Frontends []*MQTTFrontend
	Backends  MQTTBackendList
	ProxyMode ServiceProxyMode

	status          int32
	frontEndService helpers.Service
}

func (this *MQTTService) selectBackend(triedBackends MQTTBackendList) *MQTTBackend {
	// first try in active backends
	activeBackends := this.Backends.Filter(func(backend *MQTTBackend) bool {
		return backend.Weight > 0 && !triedBackends.Contains(backend) && backend.IsAvailable()
	})
	if len(activeBackends) != 0 {
		return activeBackends.RandomSelect(true)
	}

	// now we try passive backends
	passiveBackends := this.Backends.Filter(func(backend *MQTTBackend) bool {
		return backend.Weight <= 0 && !triedBackends.Contains(backend) && backend.IsAvailable()
	})
	if len(passiveBackends) != 0 {
		return passiveBackends.RandomSelect(false)
	}

	// now we try with unavailable backends
	activeBackends = this.Backends.Filter(func(backend *MQTTBackend) bool {
		return backend.Weight > 0 && !triedBackends.Contains(backend)
	})
	if len(activeBackends) != 0 {
		return activeBackends.RandomSelect(true)
	}

	passiveBackends = this.Backends.Filter(func(backend *MQTTBackend) bool {
		return backend.Weight <= 0 && !triedBackends.Contains(backend)
	})
	if len(passiveBackends) != 0 {
		return passiveBackends.RandomSelect(false)
	}

	return nil // no backend is available
}
func (this *MQTTService) handleClient(frontend *MQTTFrontend, c net.Conn) {
	OnClientConnect(this.Name, frontend.Name, frontend.Endpoint.GetProtocol())
	defer OnClientDisconnect(this.Name, frontend.Name, frontend.Endpoint.GetProtocol())

	logger := CreateLogger(fmt.Sprintf("client/%s{proto: %s, addr: %s}",
		frontend.Name, frontend.Endpoint.GetProtocol(), c.RemoteAddr()))

	var err error
	var backend *MQTTBackend
	var backendConn net.Conn
	var triedBackends MQTTBackendList
	for {
		backend = this.selectBackend(triedBackends)
		if backend == nil {
			logger.Errorf("Failed to select a backend a for client")
			return
		}

		logger.Debugf("Trying `%s` as backend for this client", backend.Name)
		backendConn, err = backend.Endpoint.Connect(this.Name, backend.Name)
		if err == nil {
			logger.Debugf("`%s` selected as backend", backend.Name)
			backend.OnConnectionSucceeded()
			break
		} else {
			logger.Warnf("Failed to connect to backend `%s`: %v", backend.Name, err)
			backend.OnConnectionFailed()
		}
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		this.ProxyMode.Proxy(logger, FrontendToBackend, c, backendConn)
		wg.Done()
	}()
	go func() {
		this.ProxyMode.Proxy(logger, BackendToFrontend, backendConn, c)
		wg.Done()
	}()
	wg.Wait()
}
func (this *MQTTService) startFrontends() helpers.Service {
	listeners := make([]helpers.Service, len(this.Frontends))
	for i := 0; i < len(this.Frontends); i++ {
		frontend := this.Frontends[i]
		listeners[i] = this.Frontends[i].CreateListenService(this.Name, func(c net.Conn) { this.handleClient(frontend, c) })
	}
	return helpers.MergeServices(fmt.Sprintf("%s/frontend_listener", this.Name), listeners...)
}

func (this *MQTTService) GetName() string { return this.Name }
func (this *MQTTService) Run() error {
	if !atomic.CompareAndSwapInt32(&this.status, 0, 1) {
		return helpers.StringError("Function must only called when service is stopped")
	}

	this.frontEndService = this.startFrontends()
	return this.frontEndService.Run()
}
func (this *MQTTService) Shutdown() {
	this.frontEndService.Shutdown()
}

type MQTTServiceConfig struct {
	Name      string               `yaml:"name"`
	Frontends []MQTTFrontendConfig `yaml:"frontends"`
	Backends  []MQTTBackendConfig  `yaml:"backends"`
	Enabled   *bool                `yaml:"enabled,omitempty"`
	ProxyMode *ServiceProxyMode    `yaml:"proxyMode,omitempty"`
}

func CreateService(name string, config MQTTServiceConfig) (*MQTTService, bool, error) {
	frontends := make([]*MQTTFrontend, 0, len(config.Frontends))
	for i := 0; i < len(config.Frontends); i++ {
		frontend, enabled, err := CreateFrontend(config.Frontends[i])
		if err != nil {
			return nil, false, err
		}
		if !enabled {
			continue
		}

		frontends = append(frontends, frontend)
	}
	if len(frontends) == 0 {
		return nil, false, fmt.Errorf("Service `%s` have no enabled frontend", name)
	}

	backends := make([]*MQTTBackend, 0, len(config.Backends))
	for i := 0; i < len(config.Backends); i++ {
		backend, enabled, err := CreateBackend(config.Backends[i])
		if err != nil {
			return nil, false, err
		}
		if !enabled {
			continue
		}

		backends = append(backends, backend)
	}
	if len(backends) == 0 {
		return nil, false, fmt.Errorf("Service `%s` have no enabled backend", name)
	}

	service := &MQTTService{Name: name, Frontends: frontends, Backends: backends, ProxyMode: Raw}
	if config.ProxyMode != nil {
		service.ProxyMode = *config.ProxyMode
	}
	return service, GetOptionalBool(config.Enabled, true), nil
}

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	MinWeight int32 = 1
	MaxWeight int32 = 65535
)

var (
	ErrNameIsRequired               = errors.New("name is required")
	ErrAddressIsRequired            = errors.New("address is required")
	ErrFrontendRoutenameIsRequired  = errors.New("frontend.route is required")
	ErrWeightOutOfRange             = fmt.Errorf("weight must be in range [%v, %v]", MinWeight, MaxWeight)
	ErrMissingNetwork               = errors.New("`network` is required")
	ErrBackendOrGroupIsRequired     = errors.New("either `backend` or `backendGroup` is required")
	ErrOnlyBackendOrGroupIsRequired = errors.New("only one of `backend` or `backendGroup` is allowed")
	ErrBackendIsDisabled            = errors.New("specified backend is disabled")
	ErrEmptyRoute                   = errors.New("route without any rules")
)

type configTlsCertificate struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

func (this *configTlsCertificate) Validate() error {
	if this == nil {
		return nil
	}
	if len(this.Cert) == 0 || len(this.Key) == 0 {
		return errors.New("Invalid certificate, both `cert` and `key` are required")
	}
	return nil
}
func (this *configTlsCertificate) Load() (tls.Certificate, error) {
	return tls.LoadX509KeyPair(this.Cert, this.Key)
}

type configTlsClientValidation struct {
	Enabled bool     `yaml:"emabled"`
	CAFiles []string `yaml:"caFiles"`
}

func (this *configTlsClientValidation) Validate() error {
	return nil
}
func (this *configTlsClientValidation) ApplyToConfig(config *tls.Config) error {
	if this == nil {
		return nil
	}

	if this.Enabled {
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if len(this.CAFiles) != 0 {
		roots := x509.NewCertPool()
		for i := 0; i < len(this.CAFiles); i++ {
			ca := this.CAFiles[i]
			caCertPEM, err := ioutil.ReadFile(ca)
			if err != nil {
				return fmt.Errorf("Failed to load certificate file(%s): %v", ca, err)
			}

			if ok := roots.AppendCertsFromPEM(caCertPEM); !ok {
				return fmt.Errorf("Failed to parse certificate file(%s): %v", ca, err)
			}
		}
		config.ClientCAs = roots
	}

	return nil
}

type configFrontendTlsConfig struct {
	Certificates     []configTlsCertificate     `yaml:"certificates"`
	ClientValidation *configTlsClientValidation `yaml:"clientValidation"`
}

func (this *configFrontendTlsConfig) Validate() error {
	if len(this.Certificates) == 0 {
		return errors.New("For secure frontend servers, at least one certificate is required")
	}
	for i := 0; i < len(this.Certificates); i++ {
		err := this.Certificates[i].Validate()
		if err != nil {
			return err
		}
	}
	return this.ClientValidation.Validate()
}
func (this *configFrontendTlsConfig) Load() (*tls.Config, error) {
	if this == nil {
		return nil, nil
	}

	var err error
	config := &tls.Config{}
	for i := 0; i < len(this.Certificates); i++ {
		cert, err := this.Certificates[i].Load()
		if err != nil {
			return nil, err
		}
		config.Certificates = append(config.Certificates, cert)
	}

	err = this.ClientValidation.ApplyToConfig(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

type configFrontend struct {
	Address   string                   `yaml:"address"`
	Enabled   *bool                    `yaml:"enabled"`
	Route     *string                  `yaml:"route"`
	TlsConfig *configFrontendTlsConfig `yaml:"tls"`
}

func (this *configFrontend) Validate() error {
	if this.Address == "" {
		return ErrAddressIsRequired
	}
	n := strings.IndexAny(this.Address, ":/")
	if n == -1 {
		this.Address += "://"
	}
	if this.Route != nil && *this.Route == "" {
		return ErrFrontendRoutenameIsRequired
	}
	if this.Enabled == nil {
		value := true
		this.Enabled = &value
	}
	return nil
}
func (this *configFrontend) Create(context *configLoadContext, name string) (*MQTTFrontend, error) {
	var route MQTTRoute
	if this.Route != nil {
		var ok bool
		route, ok = context.Routes[*this.Route]
		if !ok {
			return nil, fmt.Errorf("`%s` is not a valid route name", *this.Route)
		}
	} else {
		route = context.DefaultRoute
	}

	address, err := url.Parse(this.Address)
	if err != nil {
		return nil, fmt.Errorf("`%s` is not a valid address: %v", this.Address, err)
	}

	tlsConfig, err := this.TlsConfig.Load()
	if err != nil {
		return nil, err
	}

	connector, err := NewConnector(address, tlsConfig)
	if err != nil {
		return nil, err
	}

	return &MQTTFrontend{
		Name:      name,
		Connector: connector,
		Enabled:   *this.Enabled,
		Backend:   route,
	}, nil
}

type configBackend struct {
	Address     string                `yaml:"address"`
	Enabled     *bool                 `yaml:"enabled"`
	Weight      *int32                `yaml:"weight"`
	Certificate *configTlsCertificate `yaml:"certificate"`
}

func (this *configBackend) Validate() error {
	if this.Address == "" {
		return ErrAddressIsRequired
	}
	n := strings.IndexAny(this.Address, ":/")
	if n == -1 {
		return errors.New("Invalid backend address")
	}
	if this.Enabled == nil {
		value := true
		this.Enabled = &value
	}
	if this.Weight == nil {
		value := int32(1)
		this.Weight = &value
	} else if *this.Weight < MinWeight || *this.Weight > MaxWeight {
		return ErrWeightOutOfRange
	}

	return this.Certificate.Validate()
}
func (this *configBackend) Load(name string) (*Backend, error) {
	if !*this.Enabled {
		return nil, ErrBackendIsDisabled
	}

	address, err := url.Parse(this.Address)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse address: %v", err)
	}
	if address.Hostname() == "" {
		return nil, errors.New("Backend address require a host, if you want to connect to localhost use 127.0.0.1 for host name")
	}

	connector, err := NewConnector(address, nil)
	if err != nil {
		return nil, err
	}

	result := &Backend{Name: name, Weight: *this.Weight, Connector: connector}
	if this.Certificate != nil {
		certificate, err := this.Certificate.Load()
		if err != nil {
			return nil, err
		}

		result.Certificate = &certificate
	}

	return result, nil
}

type configBackendRef struct {
	Name   string `yaml:"name"`
	Weight *int32 `yaml:"weight"`
}

func (this *configBackendRef) Validate() error {
	if this.Name == "" {
		return ErrNameIsRequired
	}
	if this.Weight != nil && (*this.Weight < MinWeight || *this.Weight > MaxWeight) {
		return ErrWeightOutOfRange
	}

	return nil
}
func (this *configBackendRef) Load(context *configLoadContext) (*Backend, error) {
	backendData, ok := context.Data.Backend[this.Name]
	if !ok {
		return nil, fmt.Errorf("`%s` is not a valid backend", this.Name)
	}
	if backendData.Enabled != nil && !*backendData.Enabled {
		return nil, ErrBackendIsDisabled
	}

	backend := context.Backends[this.Name]
	if this.Weight != nil && *this.Weight != backend.Weight {
		copy := *backend
		copy.Weight = *this.Weight
		backend = &copy
	}

	return backend, nil
}

type configBackendGroup []configBackendRef

func (this configBackendGroup) Validate() error {
	for i := 0; i < len(this); i++ {
		err := this[i].Validate()
		if err != nil {
			return err
		}
	}

	return nil
}
func (this configBackendGroup) Load(context *configLoadContext) (BackendGroup, error) {
	result := make(BackendGroup, 0, len(this))
	for i := 0; i < len(this); i++ {
		backend, err := this[i].Load(context)
		if err == ErrBackendIsDisabled {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, backend)
	}
	return result, nil
}

type configRouteRule struct {
	Network      string `yaml:"clientAddr"`
	Backend      string `yaml:"backend"`
	BackendGroup string `yaml:"backendGroup"`
}

func (this *configRouteRule) Validate() error {
	if this.Network == "" {
		return ErrMissingNetwork
	}
	if this.Backend == "" && this.BackendGroup == "" {
		return ErrBackendOrGroupIsRequired
	}
	if this.Backend != "" && this.BackendGroup != "" {
		return ErrOnlyBackendOrGroupIsRequired
	}
	return nil
}
func (this *configRouteRule) Load(context *configLoadContext) (*MQTTRouteRule, error) {
	_, network, err := net.ParseCIDR(this.Network)
	if err != nil {
		_, network, err = net.ParseCIDR(this.Network + "/32")
		if err != nil {
			return nil, err
		}
	}

	result := &MQTTRouteRule{Network: *network}
	if this.Backend != "" {
		backend, ok := context.Backends[this.Backend]
		if !ok {
			return nil, fmt.Errorf("`%s` is not a valid backend", this.Backend)
		}
		result.Backend = backend
	} else {
		backendGroup, ok := context.BackendGroups[this.BackendGroup]
		if !ok {
			return nil, fmt.Errorf("`%s` is not a valid backendGroup", this.BackendGroup)
		}
		if len(backendGroup) == 0 {
			return nil, fmt.Errorf("`%s` is an empty gourp", this.BackendGroup)
		}
		result.Backend = backendGroup
	}

	return result, nil
}

type configRoute []configRouteRule

func (this configRoute) Validate() error {
	if len(this) == 0 {
		return ErrEmptyRoute
	}

	for i := 0; i < len(this); i++ {
		err := this[i].Validate()
		if err != nil {
			return err
		}
	}

	return nil
}
func (this configRoute) Load(context *configLoadContext) (MQTTRoute, error) {
	result := make(MQTTRoute, 0, len(this))
	for i := 0; i < len(this); i++ {
		rule, err := this[i].Load(context)
		if err != nil {
			return nil, err
		}
		result = append(result, *rule)
	}
	return result, nil
}

type configData struct {
	DefaultRoute   string                         `yaml:"defaultRoute"`
	NoDefaultGroup *bool                          `yaml:"noDefaultGroup"`
	Frontend       map[string]*configFrontend     `yaml:"frontend"`
	Backend        map[string]*configBackend      `yaml:"backend"`
	BackendGroups  map[string]*configBackendGroup `yaml:"backendGroups"`
	Routes         map[string]*configRoute        `yaml:"routes"`
}

func (this *configData) Validate() error {
	if len(this.Frontend) == 0 {
		return errors.New("No frontend defined in the config")
	}
	if len(this.Backend) == 0 {
		return errors.New("No backend defined in the config")
	}
	if len(this.Routes) == 0 {
		return errors.New("No route defined in the config")
	}

	for k, v := range this.Frontend {
		err := v.Validate()
		if err != nil {
			return fmt.Errorf("`%s` is not a valid frontend: %v", k, err)
		}
	}
	for k, v := range this.Backend {
		err := v.Validate()
		if err != nil {
			return fmt.Errorf("`%s` is not a valid backend: %v", k, err)
		}
	}
	for k, v := range this.BackendGroups {
		err := v.Validate()
		if err != nil {
			return fmt.Errorf("`%s` is not a valid backendGroup: %v", k, err)
		}
	}
	for k, v := range this.Routes {
		err := v.Validate()
		if err != nil {
			return fmt.Errorf("`%s` is not a valid route: %v", k, err)
		}
	}

	return nil
}
func (this *configData) Load(context *configLoadContext) ([]*MQTTFrontend, error) {
	// first of all load backends
	for k, v := range this.Backend {
		backend, err := v.Load(k)
		if err == ErrBackendIsDisabled {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to load a backend[%s]: %v", k, err)
		}
		context.Backends[k] = backend
	}
	// now load backendGroups
	for k, v := range this.BackendGroups {
		backendGroup, err := v.Load(context)
		if err != nil {
			return nil, fmt.Errorf("failed to load a backendGroup[%s]: %v", k, err)
		}
		context.BackendGroups[k] = backendGroup
	}
	if this.NoDefaultGroup == nil || !*this.NoDefaultGroup {
		addBackendGroupIfMissingByScheme := func(name string, cond func(scheme string) bool) {
			group, ok := context.BackendGroups[name]
			if ok {
				return
			}

			for _, v := range context.Backends {
				if cond(v.Connector.GetScheme()) {
					group = append(group, v)
				}
			}
			context.BackendGroups[name] = group
		}

		addBackendGroupIfMissingByScheme("all", func(scheme string) bool { return true })

		addBackendGroupIfMissingByScheme(WS, func(scheme string) bool { return scheme == WS })
		addBackendGroupIfMissingByScheme(WSS, func(scheme string) bool { return scheme == WSS })
		addBackendGroupIfMissingByScheme("ws-*", func(scheme string) bool { return scheme == WSS || scheme == WS })

		addBackendGroupIfMissingByScheme(MQTT, func(scheme string) bool { return scheme == MQTT })
		addBackendGroupIfMissingByScheme(MQTT, func(scheme string) bool { return scheme == MQTTS })
		addBackendGroupIfMissingByScheme("mqtt-*", func(scheme string) bool { return scheme == MQTTS || scheme == MQTT })
	}
	// and then load routes
	for k, v := range this.Routes {
		route, err := v.Load(context)
		if err != nil {
			return nil, fmt.Errorf("failed to load route[%s]: %v", k, err)
		}
		context.Routes[k] = route
	}
	// now that we have loaded the routes, we must try to load default route
	if this.DefaultRoute != "" {
		defaultRoute, ok := context.Routes[this.DefaultRoute]
		if !ok {
			return nil, fmt.Errorf("Invalid defaultRoute")
		}
		context.DefaultRoute = defaultRoute
	}
	// and at the end, we must load the frontends
	frontends := make([]*MQTTFrontend, 0, len(this.Frontend))
	for k, v := range this.Frontend {
		frontend, err := v.Create(context, k)
		if err != nil {
			return nil, fmt.Errorf("failed to load frontend[%s]: %v", k, err)
		}
		frontends = append(frontends, frontend)
	}
	return frontends, nil
}

func LoadConfig(config []byte) ([]*MQTTFrontend, error) {
	var o map[string]configData
	err := yaml.Unmarshal(config, &o)
	if err != nil {
		return nil, err
	}

	proxy, ok := o["proxy"]
	if !ok || len(o) != 1 {
		return nil, errors.New("Config should have one and only one configuration named `proxy`")
	}

	err = proxy.Validate()
	if err != nil {
		return nil, err
	}

	context := &configLoadContext{
		Data:          &proxy,
		Backends:      make(map[string]*Backend),
		BackendGroups: make(map[string]BackendGroup),
		Routes:        make(map[string]MQTTRoute),
	}
	return proxy.Load(context)
}

type configLoadContext struct {
	Data *configData

	DefaultRoute  MQTTRoute
	Backends      map[string]*Backend
	BackendGroups map[string]BackendGroup
	Routes        map[string]MQTTRoute
}

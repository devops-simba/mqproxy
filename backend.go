package main

import (
	"math/rand"
	"sync/atomic"
	"unsafe"
)

type MQTTBackend struct {
	Name     string
	Weight   int
	Endpoint MQTTClientEndpoint

	availabilityCounter unsafe.Pointer
}

func (this *MQTTBackend) IsAvailable() bool {
	availabilityCounter := (*AvailabilityCounter)(atomic.LoadPointer(&this.availabilityCounter))
	return availabilityCounter.IsAvailableToTry()
}
func (this *MQTTBackend) OnConnectionSucceeded() {
	for {
		oldPointer := atomic.LoadPointer(&this.availabilityCounter)
		availabilityCounter := (*AvailabilityCounter)(oldPointer)
		newAvailabilityCounter := unsafe.Pointer(availabilityCounter.OnConnectionSucceeded())
		if atomic.CompareAndSwapPointer(&this.availabilityCounter, oldPointer, newAvailabilityCounter) {
			break
		}
	}
}
func (this *MQTTBackend) OnConnectionFailed() {
	for {
		oldPointer := atomic.LoadPointer(&this.availabilityCounter)
		availabilityCounter := (*AvailabilityCounter)(oldPointer)
		newAvailabilityCounter := unsafe.Pointer(availabilityCounter.OnConnectionFailed())
		if atomic.CompareAndSwapPointer(&this.availabilityCounter, oldPointer, newAvailabilityCounter) {
			break
		}
	}
}

type MQTTBackendList []*MQTTBackend

func (this MQTTBackendList) Contains(backend *MQTTBackend) bool {
	for i := 0; i < len(this); i++ {
		if this[i] == backend {
			return true
		}
	}
	return false
}
func (this MQTTBackendList) Append(backend *MQTTBackend) MQTTBackendList {
	return append(this, backend)
}
func (this MQTTBackendList) RandomSelect(active bool) *MQTTBackend {
	if len(this) == 0 {
		return nil
	}
	if len(this) == 1 {
		return this[0]
	}

	weightSum := 0
	if active {
		for i := 0; i < len(this); i++ {
			weightSum += this[i].Weight
		}

		selection := rand.Intn(weightSum)
		for i := 0; i < len(this); i++ {
			if selection < this[i].Weight {
				return this[i]
			}
			selection -= this[i].Weight
		}
	} else {
		for i := 0; i < len(this); i++ {
			weightSum += 1 - this[i].Weight
		}

		selection := rand.Intn(weightSum)
		for i := 0; i < len(this); i++ {
			weight := 1 - this[i].Weight
			if selection < weight {
				return this[i]
			}
			selection -= weight
		}
	}

	panic("Must never reach here")
}
func (this MQTTBackendList) Filter(predicate func(*MQTTBackend) bool) MQTTBackendList {
	if len(this) == 0 {
		return this // nothing to filter
	}

	n := 0
	result := make(MQTTBackendList, len(this))
	for i := 0; i < len(this); i++ {
		if predicate(this[i]) {
			result[n] = this[i]
			n += 1
		}
	}
	return result[:n]
}

type MQTTBackendConfig struct {
	MQTTClientEndpointConfig `yaml:",inline"`
	Name                     string `yaml:"name"`
	Weight                   *int   `yaml:"weight"`
	Enabled                  *bool  `yaml:"enabled,omitempty"`
}

func CreateBackend(config MQTTBackendConfig) (*MQTTBackend, bool, error) {
	client, err := CreateClientEndpoint(config.MQTTClientEndpointConfig)
	if err != nil {
		return nil, false, err
	}

	if config.Name == "" {
		config.Name = "backend_" + client.GetAddress()
	}

	backend := &MQTTBackend{
		Name:                config.Name,
		Endpoint:            client,
		Weight:              1,
		availabilityCounter: unsafe.Pointer(NewAvailabilityCounter()),
	}
	if config.Weight != nil {
		backend.Weight = *config.Weight
	}
	return backend, GetOptionalBool(config.Enabled, true), nil
}

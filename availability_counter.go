package main

import (
	"time"
)

type AvailabilityStatus int

const (
	PossiblyAvailable AvailabilityStatus = iota
	UnknownAvailablityStatus
	NotAvailable
)

type AvailabilityCounter struct {
	Counter int
	NextTry time.Time
	Status  AvailabilityStatus
}

var unknownAvailablityCounter = &AvailabilityCounter{Status: UnknownAvailablityStatus}

func NewAvailabilityCounter() *AvailabilityCounter { return unknownAvailablityCounter }

func (this *AvailabilityCounter) IsAvailableToTry() bool {
	switch this.Status {
	case PossiblyAvailable, UnknownAvailablityStatus:
		return true
	default:
		return time.Now().Before(this.NextTry)
	}
}
func (this *AvailabilityCounter) OnConnectionSucceeded() *AvailabilityCounter {
	switch this.Status {
	case PossiblyAvailable:
		if this.Counter >= 50 {
			return this
		}
		return &AvailabilityCounter{Counter: this.Counter + 1, Status: PossiblyAvailable}
	case UnknownAvailablityStatus:
		return &AvailabilityCounter{Counter: 1, Status: PossiblyAvailable}
	default:
		if this.Counter <= 1 {
			return unknownAvailablityCounter
		}

		result := &AvailabilityCounter{NextTry: time.Now(), Status: NotAvailable}
		if this.Counter < 5 {
			result.Counter = this.Counter - 1
		} else if this.Counter < 10 {
			result.Counter = this.Counter - 2
		} else {
			result.Counter = this.Counter - 4
		}
		return result
	}
}
func (this AvailabilityCounter) OnConnectionFailed() *AvailabilityCounter {
	switch this.Status {
	case PossiblyAvailable:
		if this.Counter <= 10 {
			return unknownAvailablityCounter
		}
		return &AvailabilityCounter{Counter: this.Counter - 10, Status: PossiblyAvailable}
	case UnknownAvailablityStatus:
		return &AvailabilityCounter{Counter: 1, Status: NotAvailable, NextTry: time.Now()}
	default:
		if this.Counter >= 20 {
			// too many failure, retry again in 10 seconds
			return &AvailabilityCounter{
				Counter: 20,
				Status:  NotAvailable,
				NextTry: time.Now().Add(time.Second * 10),
			}
		}
		if this.Counter >= 10 {
			// we are really failing, wait a bit before retry
			return &AvailabilityCounter{
				Counter: this.Counter + 1,
				Status:  NotAvailable,
				NextTry: time.Now().Add(time.Second * 5),
			}
		}
		if this.Counter >= 3 {
			// we are failing a bit, wait a bit before retry
			return &AvailabilityCounter{
				Counter: this.Counter + 1,
				Status:  NotAvailable,
				NextTry: time.Now().Add(time.Second),
			}
		}
		// we are failing a bit, wait a bit before retry
		return &AvailabilityCounter{
			Counter: this.Counter + 1,
			Status:  NotAvailable,
			NextTry: time.Now().Add(time.Millisecond * 100),
		}
	}
}

package main

import (
	"github.com/godbus/dbus"
	"math"
)

type UPowerChecker struct {
	conn *dbus.Conn
}
var _ CheckerBackend = &UPowerChecker{}

func (c *UPowerChecker) Init() error {
	var err error
	c.conn, err = dbus.SystemBus()
	if err != nil {
		return err
	}
	return nil
}

const (
	upStateUnknown = iota
	upStateCharging
	upStateDischarging
	upStateEmpty
	upStateFull
	upStatePendingCharge
	upStatePendingDischarge
	upState
)

func (c *UPowerChecker) Check() (*PowerStatus, error) {
	upower := c.conn.Object("org.freedesktop.UPower",
		"/org/freedesktop/UPower/devices/DisplayDevice")

	props := map[string]dbus.Variant{}

	err := upower.Call("org.freedesktop.DBus.Properties.GetAll",
		0, "org.freedesktop.UPower.Device").Store(&props)
	if err != nil {
		return nil, err
	}

	energyFull := props["EnergyFull"].Value().(float64)
	energy := props["Energy"].Value().(float64)

	var timeRemaining float32
	var charging bool
	switch int(props["State"].Value().(uint32)) {
	case upStateCharging:
		timeRemaining = float32(props["TimeToFull"].Value().(int64))
		charging = true
	case upStateDischarging:
		timeRemaining = float32(props["TimeToEmpty"].Value().(int64))
		charging = false
	case upStateEmpty:
		charging = false
		timeRemaining = 0
	case upStateFull:
		charging = true
		timeRemaining = 0
	case upStatePendingCharge:
		charging = true
		timeRemaining = float32(math.NaN())
	case upStatePendingDischarge:
		charging = false
		timeRemaining = float32(math.NaN())
	default:
		charging = false
		timeRemaining = float32(math.NaN())
	}

	ret := &PowerStatus{
		ChargeLevel: float32(energy / energyFull),
		TimeRemaining: timeRemaining,
		Charging: charging,
	}

	return ret, nil
}

func (c *UPowerChecker) Stop() {
}

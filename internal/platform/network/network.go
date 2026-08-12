package network

import (
	"errors"
	"net/netip"
)

var (
	ErrNoLANAddress = errors.New("no usable LAN address found")
)

func AdvertiseAddress() (netip.Addr, error) {
	return advertiseAddress()
}

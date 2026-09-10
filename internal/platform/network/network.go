package network

import (
	"errors"
	"net/netip"
)

var (
	ErrNoLANAddress = errors.New("no usable LAN address found")
)

type Endpoint struct {
	Address   netip.Addr
	Prefix    netip.Prefix
	Interface string
}

func AdvertiseEndpoint() (Endpoint, error) {
	return advertiseEndpoint()
}

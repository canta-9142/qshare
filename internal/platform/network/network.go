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

func AdvertiseAddress() (netip.Addr, error) {
	endpoint, err := AdvertiseEndpoint()
	if err != nil {
		return netip.Addr{}, err
	}
	return endpoint.Address, nil
}

func AdvertiseEndpoint() (Endpoint, error) {
	return advertiseEndpoint()
}

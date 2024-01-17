package network

import (
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/errutil"
	"net"
	"strings"
	"sync"
)

type AddressManager struct {
	params           *chaincfg.Params
	mu               sync.RWMutex
	addressToConn    map[string]*connmgr.ConnReq
	connToAddress    map[*connmgr.ConnReq]string
	initialAddresses []*wire.NetAddressV2
	used             map[string]struct{}
	unUsed           map[string]struct{}
}

func NewAddressManager(params *chaincfg.Params) *AddressManager {
	return &AddressManager{
		addressToConn: make(map[string]*connmgr.ConnReq),
		connToAddress: make(map[*connmgr.ConnReq]string),
		used:          make(map[string]struct{}),
		unUsed:        make(map[string]struct{}),
		params:        params,
	}
}

func (a *AddressManager) OnSeed(addrs []*wire.NetAddressV2) {
	var isZero bool
	a.mu.Lock()
	isZero = len(a.initialAddresses) == 0
	if isZero {
		a.initialAddresses = addrs
	} else {
		a.initialAddresses = append(a.initialAddresses, addrs...)
	}
	a.mu.Unlock()
}

func (a *AddressManager) AddAddress(str string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, used := a.used[str]
	if used {
		return
	}
	_, used = a.unUsed[str]
	if used {
		return
	}

	a.unUsed[str] = struct{}{}
}

func (a *AddressManager) NewAddress() (net.Addr, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
retry:
	if len(a.initialAddresses) > 0 {
		address := a.initialAddresses[0]

		addr := fmt.Sprintf("%s:%d", address.Addr.String(), address.Port)
		addr = strings.TrimSuffix(addr, ":")
		a.initialAddresses = a.initialAddresses[1:]
		a.used[addr] = struct{}{}
		// ignore ipv6 for now
		if !strings.Contains(addr, ".") {
			goto retry
		}

		return addrStringToNetAddr(addr)
	}

	if len(a.unUsed) > 0 {
		for k := range a.unUsed {
			addr, err := addrStringToNetAddr(k)
			if err != nil {
				delete(a.unUsed, k)
				continue
			}
			delete(a.unUsed, k)
			a.used[k] = struct{}{}
			return addr, nil
		}
	}

	return nil, errutil.NewNotFound("no addresses found")
}

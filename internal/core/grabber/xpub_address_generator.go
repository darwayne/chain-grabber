package grabber

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"runtime"
	"sync"
)

type XPubAddressGenerator struct {
	derivationPath []uint32
	key            *hdkeychain.ExtendedKey
	chainParams    *chaincfg.Params
}

func NewXPubAddressGenerator(xpub string, derivationPath []uint32, params *chaincfg.Params) (*XPubAddressGenerator, error) {
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, err
	}
	return &XPubAddressGenerator{
		derivationPath: derivationPath,
		key:            key,
		chainParams:    params,
	}, nil
}

func (x XPubAddressGenerator) AddressRange(start, end uint32, addressTypes ...string) (AddressMap, error) {
	result := make(map[string]struct{}, int(end-start))
	maxCores := runtime.GOMAXPROCS(0)
	r := Range{int(start), int(end)}
	pieces := r.Split(maxCores)
	var wg sync.WaitGroup
	errChan := make(chan error, maxCores)
	completedWork := make(chan map[string]struct{}, maxCores)

	for _, p := range pieces {
		wg.Add(1)

		go func(rang Range) {
			defer wg.Done()
			work := make(map[string]struct{})
			for i := uint32(rang[0]); i <= uint32(rang[1]) && i >= uint32(rang[0]); i++ {
				args := make([]uint32, 0, len(x.derivationPath)+1)
				args = append(args, x.derivationPath...)
				args = append(args, i)

				m, err := derive(x.key, args...)
				if err != nil {
					errChan <- err
					return
				}

				pubKey, _ := m.ECPubKey()
				pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
				destAddr, err := btcutil.NewAddressWitnessPubKeyHash(
					pubKeyHash, x.chainParams,
				)
				if err != nil {
					errChan <- err
					return
				}
				work[destAddr.EncodeAddress()] = struct{}{}
			}

			completedWork <- work
		}(p)
	}

	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	for {
		select {
		case err := <-errChan:
			return nil, err
		case <-doneChan:
			return result, nil
		case work := <-completedWork:
			for k, v := range work {
				result[k] = v
			}
		}
	}

	return result, nil
}

type AddressMap map[string]struct{}

func (a AddressMap) Has(address string) (yes bool) {
	_, yes = a[address]
	return
}

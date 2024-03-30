package grabber

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/darwayne/chain-grabber/pkg/sigutil"
	"math"
	"runtime"
	"sync"
)

type GeneratedKey struct {
	AddressKeyMap map[string]*btcutil.WIF
	KnownKeys     map[string]*btcutil.WIF
	AddressOrder  map[string]float64
}

type Addr struct {
	btcutil.Address
	Compressed bool
}
type KeyStream struct {
	Num       int
	Key       *btcec.PrivateKey
	Addresses []Addr
	Err       error
}

func StreamKeys(start, end int) <-chan KeyStream {
	maxCores := runtime.GOMAXPROCS(0)
	r := Range{start, end}
	pieces := r.Split(maxCores)
	completedWork := make(chan KeyStream, maxCores)
	sem := make(chan struct{}, maxCores)
	var wg sync.WaitGroup

	done := sigutil.Done()

	sendWork := func(work KeyStream) bool {
		select {
		case completedWork <- work:
			if work.Err != nil {
				return true
			}
		case <-done:
			return true
		}

		return false
	}

	for _, p := range pieces {
		wg.Add(1)
		go func(rang Range) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() {
				<-sem
			}()
			for i := rang[1]; i >= rang[0] && i <= rang[1]; i-- {
				var mod btcec.ModNScalar
				mod.Zero()
				mod.SetInt(uint32(i))
				key := btcec.PrivKeyFromScalar(&mod)

				var addresses []Addr
				for _, compressed := range []bool{true, false} {
					for _, network := range []*chaincfg.Params{&chaincfg.MainNetParams} {
						addr, err := PrivToPubKeyHash(key, compressed, network)
						if err != nil {
							sendWork(KeyStream{Err: err})
							return
						}

						addresses = append(addresses, Addr{
							Address:    addr,
							Compressed: compressed,
						})

						addr, err = PrivToSegwit(key, compressed, network)
						if err != nil {
							sendWork(KeyStream{Err: err})
							return
						}

						addresses = append(addresses, Addr{
							Address:    addr,
							Compressed: compressed,
						})

						if compressed {
							addr, err = PrivToTaprootPubKey(key, network)
							if err != nil {
								sendWork(KeyStream{Err: err})
								return
							}

							addresses = append(addresses, Addr{
								Address:    addr,
								Compressed: compressed,
							})
						}
					}
				}

				if sendWork(KeyStream{Key: key, Num: i, Addresses: addresses}) {
					return
				}

			}
		}(p)
	}

	go func() {
		wg.Wait()
		close(completedWork)
	}()

	return completedWork
}

func GenerateKeys(start, end int, params *chaincfg.Params) (GeneratedKey, error) {
	maxCores := runtime.GOMAXPROCS(0)
	r := Range{start, end}
	pieces := r.Split(maxCores)
	var wg sync.WaitGroup
	errChan := make(chan error, maxCores)
	completedWork := make(chan GeneratedKey, maxCores)
	for _, p := range pieces {
		wg.Add(1)
		go func(rang Range) {
			defer wg.Done()
			m := make(map[string]*btcutil.WIF, rang[1]-rang[0]*3)
			order := make(map[string]float64)
			for i := rang[0]; i <= rang[1] && i >= rang[0]; i++ {
				var mod btcec.ModNScalar
				num := float64(i)
				mod.Zero()
				mod.SetInt(uint32(i))
				key := btcec.PrivKeyFromScalar(&mod)
				for _, compressed := range []bool{false, true} {

					num += 0.01
					addr, err := PrivToPubKeyHash(key, compressed, params)
					if err != nil {
						errChan <- err
						return
					}
					wif, err := btcutil.NewWIF(key, params, compressed)
					if err != nil {
						errChan <- err
						return
					}
					m[addr.EncodeAddress()] = wif
					order[addr.EncodeAddress()] = num

					if compressed {
						num += 0.01
						addr, err = PrivToSegwit(key, compressed, params)
						if err != nil {
							errChan <- err
							return
						}

						m[addr.EncodeAddress()] = wif
						order[addr.EncodeAddress()] = num

						if i != 0 {
							num += 0.01
							addr, err = PrivToTaprootPubKey(key, params)
							if err != nil {
								errChan <- err
								return
							}

							m[addr.EncodeAddress()] = wif
							order[addr.EncodeAddress()] = num
						}
					}

				}
			}

			completedWork <- GeneratedKey{
				AddressKeyMap: m,
				AddressOrder:  order,
			}
		}(p)
	}

	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	result := GeneratedKey{
		AddressKeyMap: make(map[string]*btcutil.WIF),
		AddressOrder:  make(map[string]float64),
		KnownKeys:     make(map[string]*btcutil.WIF),
	}
	for {
		select {
		case err := <-errChan:
			return GeneratedKey{}, err
		case <-doneChan:
			return result, nil
		case work := <-completedWork:
			for k, v := range work.AddressKeyMap {
				//if v.CompressPubKey {
				result.KnownKeys[hex.EncodeToString(v.SerializePubKey())] = v
				//}
				v := v
				result.AddressKeyMap[k] = v
			}
			for k, v := range work.AddressOrder {
				v := v
				result.AddressOrder[k] = v
			}
		}
	}
}

type Range [2]int

func (r Range) Split(amount int) []Range {
	min := r[0]
	max := r[1]
	if min == max || max < min || (max-min) <= amount || amount <= 1 {
		return []Range{r}
	}

	results := make([]Range, 0, amount)
	step := int(math.Ceil((float64(max) - float64(min)) / float64(amount)))
	for i := min; i <= max; i += step {
		rng := Range{i, i + step - 1}
		if rng[1] == max-1 {
			rng[1] = max
			i = max + 1
		}
		if rng[1] > max {
			rng[1] = max
			i = max + 1
		}
		results = append(results, rng)
	}

	return results
}

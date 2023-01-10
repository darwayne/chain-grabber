package grabber

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"math"
	"runtime"
	"sync"
)

type GeneratedKey struct {
	AddressKeyMap map[string]*btcutil.WIF
	AddressOrder  map[string]float64
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
				for idx, compressed := range []bool{false, true} {
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

					num += 0.01
					addr, err = PrivToSegwit(key, compressed, params)
					if err != nil {
						errChan <- err
						return
					}

					m[addr.EncodeAddress()] = wif
					order[addr.EncodeAddress()] = num

					if i != 0 && idx == 0 {
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
	}
	for {
		select {
		case err := <-errChan:
			return GeneratedKey{}, err
		case <-doneChan:
			return result, nil
		case work := <-completedWork:
			for k, v := range work.AddressKeyMap {
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

package indexer

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type AddressIndexAsJsonRecord struct {
	A string
	T map[string][2]int
}

type Address struct {
}

func (a Address) IndexFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	r, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	initial := filepath.Join(home, "btc", "mainnet")
	hashDB := filepath.Join(initial, "addressindex")
	os.MkdirAll(hashDB, os.ModePerm)
	db, err := leveldb.OpenFile(hashDB, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	reader := bufio.NewReader(r)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	errChan := make(chan error, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalWorkDone int64
	var processed int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				v := atomic.LoadInt64(&processed)
				since := time.Since(start).Seconds()
				done := atomic.LoadInt64(&totalWorkDone)
				log.Printf("%.2f addresses saved per second (%d work done)[%d processed]", float64(done)/since, done, v)
			}
		}
	}()
	for {
		data, err := reader.ReadBytes('\n')
		if err == io.EOF || len(data) < 4 {
			break
		} else if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			select {
			case <-sem:
			case <-ctx.Done():
				return
			}

			dec := json.NewDecoder(bytes.NewReader(data))
			var r AddressIndexAsJsonRecord
			if err := dec.Decode(&r); err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
				}
				return
			}

			if _, err := db.Get([]byte(r.A), nil); err == nil {
				atomic.AddInt64(&processed, 1)
				return
			}

			orderedTransactions := make([]string, 0, len(r.T))
			for t := range r.T {
				orderedTransactions = append(orderedTransactions, t)
			}
			sort.Slice(orderedTransactions, func(i, j int) bool {
				a := orderedTransactions[i]
				b := orderedTransactions[j]
				aa := r.T[a]
				bb := r.T[b]
				x := (aa[0] + 1000) * (aa[1] * 1000)
				y := (bb[0] + 1000) * (bb[1] * 1000)
				return x < y
			})

			var buf bytes.Buffer
			enc := gob.NewEncoder(&buf)
			if err := enc.Encode(orderedTransactions); err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
				}
				return
			}

			if err := db.Put([]byte(r.A), buf.Bytes(), nil); err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
				}
				return
			}
		}()

		select {
		case err := <-errChan:
			return err
		default:

		}

		atomic.AddInt64(&processed, 1)
		atomic.AddInt64(&totalWorkDone, 1)
	}

	return nil
}

func (a Address) TXIndexFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	r, err := gzip.NewReader(f)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	initial := filepath.Join(home, "btc", "mainnet")
	hashDB := filepath.Join(initial, "transactionindex")
	os.MkdirAll(hashDB, os.ModePerm)
	db, err := leveldb.OpenFile(hashDB, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	reader := bufio.NewReader(r)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	errChan := make(chan error, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var totalWorkDone int64
	var processed int64
	start := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-ticker.C:
				v := atomic.LoadInt64(&processed)
				since := time.Since(start).Seconds()
				done := atomic.LoadInt64(&totalWorkDone)
				log.Printf("%.2f transactions saved per second (%d work done)[%d processed]", float64(done)/since, done, v)
			}
		}
	}()
	for {
		data, err := reader.ReadBytes('\n')
		if err == io.EOF || len(data) < 4 {
			break
		} else if err != nil {
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			select {
			case <-sem:
			case <-ctx.Done():
				return
			}

			dec := json.NewDecoder(bytes.NewReader(data))
			var r AddressIndexAsJsonRecord
			if err := dec.Decode(&r); err != nil {
				select {
				case errChan <- err:
				case <-ctx.Done():
				}
				return
			}

			var buf bytes.Buffer

			for transactionID, blockInfo := range r.T {
				txID := []byte(transactionID)
				if _, err := db.Get(txID, nil); err == nil {
					atomic.AddInt64(&processed, 1)
					continue
				}

				buf.Reset()
				enc := gob.NewEncoder(&buf)
				if err := enc.Encode(blockInfo); err != nil {
					select {
					case errChan <- err:
					case <-ctx.Done():
					}
					return
				}
				if err := db.Put(txID, buf.Bytes(), nil); err != nil {
					select {
					case errChan <- err:
					case <-ctx.Done():
					}
					return
				}
			}

		}()

		select {
		case err := <-errChan:
			return err
		default:

		}

		atomic.AddInt64(&processed, 1)
		atomic.AddInt64(&totalWorkDone, 1)
	}

	return nil
}

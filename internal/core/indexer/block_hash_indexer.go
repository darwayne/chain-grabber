package indexer

import (
	"bytes"
	"encoding/binary"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/syndtr/goleveldb/leveldb"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type BlockHash struct {
	mu   sync.Mutex
	mngr *grabber.TransactionManager
}

func NewBlockHash(mngr *grabber.TransactionManager) BlockHash {
	return BlockHash{
		mngr: mngr,
	}
}

func (b *BlockHash) Index() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	initial := filepath.Join(home, "btc", "mainnet")
	hashDB := filepath.Join(initial, "blockhashdb")
	os.MkdirAll(hashDB, os.ModePerm)
	db, err := leveldb.OpenFile(hashDB, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	mngr := b.mngr
	num, err := mngr.GetBlockCount()
	if err != nil {
		return err
	}

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
				log.Printf("%.2f hashes saved per second (%d work done)[%d processed]", float64(done)/since, done, v)
			}
		}
	}()

	maxWorkers := 10
	errChan := make(chan error, maxWorkers)
	work := make(chan int64, maxWorkers)
	var wg sync.WaitGroup
	for x := 0; x < maxWorkers; x++ {
		go func() {
			buf := new(bytes.Buffer)
			for {
				select {
				case i := <-work:
					func() {
						defer wg.Done()
						buf.Reset()
						hash, err := mngr.GetBlockHash(i)
						if err != nil {
							errChan <- err
							return
						}

						err = binary.Write(buf, binary.LittleEndian, i)
						if err != nil {
							errChan <- err
							return
						}
						err = db.Put(buf.Bytes(), hash.CloneBytes(), nil)
						if err != nil {
							errChan <- err
							return
						}
						atomic.AddInt64(&processed, 1)
						atomic.AddInt64(&totalWorkDone, 1)

					}()

				}
			}
		}()
	}

	buf := new(bytes.Buffer)
	for i := num; i >= 0; i-- {
		buf.Reset()
		err = binary.Write(buf, binary.LittleEndian, i)
		if err != nil {
			return err
		}
		if _, err := db.Get(buf.Bytes(), nil); err != nil && err == leveldb.ErrNotFound {
			wg.Add(1)
			select {
			case work <- i:
			case err = <-errChan:
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		atomic.AddInt64(&processed, 1)
	}

	wg.Wait()

	return nil
}

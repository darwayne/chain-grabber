package utxobuilder

import (
	"context"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain"
	"golang.org/x/sync/errgroup"
	"log"
	"sync"
	"time"
)

func NewBuilder(clis []blockchain.Client, store UTXOStore) *Builder {
	return &Builder{
		clis:       clis,
		store:      store,
		tempBlocks: make(map[int]*wire.MsgBlock),
	}
}

type Builder struct {
	clis          []blockchain.Client
	mu            sync.RWMutex
	cliIterator   int
	heightMu      sync.RWMutex
	currentHeight int
	store         UTXOStore
	blockMu       sync.RWMutex
	tempBlocks    map[int]*wire.MsgBlock
}

type UTXOSet struct {
	StartPoint int
	UTXOs      map[wire.OutPoint]int64
}

func (b *Builder) Build() error {
	maxHeight, err := b.clis[0].GetBlockHeight(context.Background())
	if err != nil {
		return err
	}

	data, err := b.store.Get(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("starting from height: %d\tsize:%d\n", data.StartPoint, len(data.UTXOs))
	b.currentHeight = data.StartPoint

	go b.poolBlocks(data.StartPoint, maxHeight)

	for i := data.StartPoint; i <= maxHeight; i++ {
		if err := b.processHeight(i, maxHeight, &data); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) processHeight(height, _ int, data *UTXOSet) error {
	block, err := b.waitForBlock(height)
	if err != nil {
		return err
	}
	utxoSet := data.UTXOs
	for _, tx := range block.Transactions {
		for _, input := range tx.TxIn {
			delete(utxoSet, input.PreviousOutPoint)
		}

		for idx, output := range tx.TxOut {
			utxoSet[wire.OutPoint{Hash: tx.TxHash(), Index: uint32(idx)}] = output.Value
		}
	}
	data.StartPoint = height
	b.heightMu.Lock()
	b.currentHeight = height
	b.heightMu.Unlock()
	if height%100 == 0 {
		if err := b.flush(data); err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) getClient() blockchain.Client {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cliIterator++
	if b.cliIterator >= len(b.clis) {
		b.cliIterator = 0
	}

	return b.clis[b.cliIterator]
}

func (b *Builder) getBlock(ctx context.Context, height int) (*wire.MsgBlock, error) {
	time.Sleep(10 * time.Millisecond)
	ctx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel1()
	hash, err := b.getClient().GetBlockHashFromHeight(ctx1, height)
	if err != nil {
		return nil, err
	}
	ctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()
	return b.getClient().GetBlock(ctx2, *hash)
}

func (b *Builder) waitForBlock(height int) (*wire.MsgBlock, error) {

	for i := 0; i < 30_000_000; i++ {
		b.blockMu.RLock()
		block, found := b.tempBlocks[height]
		b.blockMu.RUnlock()
		if !found {
			time.Sleep(time.Microsecond)
			continue
		}

		return block, nil
	}

	return nil, errors.New("took too long to process")
}

func (b *Builder) poolBlocks(startingPoint, maxHeight int) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group, ctx := errgroup.WithContext(parentCtx)
	doneChan := make(chan struct{})

	const maxWorkers = 11
	workChan := make(chan int, 1)
	for i := 0; i < maxWorkers; i++ {
		group.Go(func() error {
			for {
				select {
				case <-doneChan:
					return nil
				case height := <-workChan:
					block, err := b.getBlock(ctx, height)
					if err != nil {
						return err
					}
					b.blockMu.Lock()
					b.tempBlocks[height] = block
					b.blockMu.Unlock()
				}
			}
		})
	}

loop:
	for i := startingPoint; i <= maxHeight; i++ {
		select {
		case workChan <- i:
		case <-ctx.Done():
			break loop
		}
		if i%(maxWorkers*30) == 0 {
			b.heightMu.RLock()
			height := b.currentHeight
			b.heightMu.RUnlock()
			b.blockMu.Lock()
			x := 0
			for key := range b.tempBlocks {
				if key < height {
					x++
					delete(b.tempBlocks, key)
				}
			}
			if x > 0 {
				fmt.Println("pruned", x, "temp blocks")
			}

			b.blockMu.Unlock()
			//time.Sleep(100 * time.Millisecond)
		}

	}

	close(doneChan)
	if err := group.Wait(); err != nil {
		log.Fatalln("error getting blocks", err)
	}

}

func (b *Builder) flush(data *UTXOSet) error {
	start := time.Now()
	fmt.Printf("flushing... height: %d\nutxo size:%d\n",
		data.StartPoint, len(data.UTXOs))
	defer func() {
		fmt.Println("flush completed in", time.Since(start))
	}()
	copied := UTXOSet{
		StartPoint: data.StartPoint,
		UTXOs:      make(map[wire.OutPoint]int64, len(data.UTXOs)),
	}

	for k, v := range data.UTXOs {
		copied.UTXOs[k] = v
	}

	return b.store.Put(context.Background(), copied)
}

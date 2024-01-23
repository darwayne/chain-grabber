package lightnode

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPutBlocks(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})

	ctx := context.Background()
	height, err := store.GetTip(ctx)
	require.NoError(t, err)
	b, err := store.GetBlock(ctx, height.Hash)
	require.NoError(t, err)
	t.Log(b.BlockHash())
	var blocks []*wire.MsgBlock
	for i := 0; i < 1000; i++ {
		blocks = append(blocks, b)
	}

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(2)
	for _, chunk := range ChunkSlice(blocks, 255) {
		group.Go(func() error {
			return store.PutBlocks(gctx, chunk...)
		})
	}
	require.NoError(t, group.Wait())

}

func TestListBlocks(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)

	startFrom := 0
	limit := 10_000

	var mu sync.RWMutex
	outputs := make(map[wire.OutPoint]int64)
	start := time.Now()
	var bHeight int64
	//var total int64
	var blockFees int64
	go func() {
		for {
			time.Sleep(time.Second)
			//totals := atomic.LoadInt64(&total)
			blockHeight := atomic.LoadInt64(&bHeight)
			mu.RLock()
			total := len(outputs)
			mu.RUnlock()
			totalFees := atomic.LoadInt64(&blockFees)

			t.Log(float64(blockHeight)/time.Since(start).Seconds(), "blocks per second",
				"total utxos", total, "last block",
				blockHeight, "\n",
				"fee total:", btcutil.Amount(totalFees),
				"fee avg:", btcutil.Amount(totalFees/blockHeight),
			)
		}
	}()

	for {
		results, err := store.ListBlocks(context.Background(), startFrom, limit)
		require.NoError(t, err)
		if len(results) < limit {
			break
		}
		startFrom += len(results)

		for _, r := range results {
			var totalFee int64
			atomic.StoreInt64(&bHeight, int64(r.Height))
			for txIdx, tx := range r.Transactions {
				var txFee int64
				_ = tx
				hash := tx.TxHash()
				//atomic.AddInt64(&total, 1)
				mu.Lock()
				for _, input := range tx.TxIn {
					if txIdx > 0 {
						txFee += outputs[input.PreviousOutPoint]
					}

					delete(outputs, input.PreviousOutPoint)
				}

				for idx, out := range tx.TxOut {
					outputs[wire.OutPoint{
						Hash:  hash,
						Index: uint32(idx),
					}] = out.Value

					if txIdx > 0 {
						txFee -= out.Value
					}

				}
				mu.Unlock()
				totalFee += txFee

			}

			//fmt.Println(r.Height, "block fee", btcutil.Amount(totalFee))
			atomic.AddInt64(&blockFees, totalFee)

			//return
		}
	}
}

func TestValidateBlockSerialization(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	//validBlocks := make(map[chainhash.Hash]struct{})
	//
	//fmt.Println("fetching block headers")
	//for height := range store.StreamHeight(context.Background()) {
	//	require.NoError(t, height.Err)
	//	validBlocks[height.Hash] = struct{}{}
	//}
	//fmt.Println(len(validBlocks), "headers fetched")

	var blocksFetched int64
	start := time.Now()
	go func() {
		for {
			select {
			case <-time.After(time.Second):
				f := atomic.LoadInt64(&blocksFetched)
				fmt.Println(blocksFetched, "which is about",
					float64(f)/time.Since(start).Seconds(), "per second")
			}
		}
	}()

	//const expected = "1f8b08000000000002ff"
	var after string = "00000000400a81e69af7a8431f7e42ff27f55666969731a52b821c6b0f206a3c"
startAgain:
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for block := range store.StreamRawBlock(ctx, after) {
		//header := block.Data[:10]
		//fmt.Println(hex.EncodeToString(block.Data[:10]))
		atomic.AddInt64(&blocksFetched, 1)
		require.NoError(t, block.Err)
		hash, err := chainhash.NewHashFromStr(block.Hash)
		require.NoError(t, err)
		_, err = gzip.NewReader(bytes.NewBuffer(block.Data))
		if err != nil {
			//fmt.Println("bad encoding detected for block", block.Hash)
			cancel()
			//err = store.DeleteBlock(context.Background(), *hash)
			_ = hash
			//require.NoError(t, err)
			after = block.Hash
			goto startAgain
		}

		//_, found := validBlocks[*hash]
		//if !found {
		//	fmt.Println("bad block detected", block.Hash)
		//}

	}
}

func TestFillMissingBlocks(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})
	node, err := NewMainNet()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node.Connect()
	var mu sync.RWMutex
	var blocks []*wire.MsgBlock
	seenBlocks := make(map[chainhash.Hash]struct{})
	onPeerBlock := make(chan *wire.MsgBlock, 100)
	peerQueue := make(chan *peer.Peer, 100)

	var dataToSend []BlockHeight
	for missing := range store.MissingBlocks(ctx) {
		require.NoError(t, missing.Err)
		//if missing.Hash == *chaincfg.MainNetParams.GenesisHash {
		//	continue
		//}
		dataToSend = append(dataToSend, missing.BlockHeight)
	}
	t.Log(len(dataToSend), "missing blocks")

	select {
	case peer := <-node.peerConnected:
		peerQueue <- peer
		data := node.GetPeerData(peer)
		fmt.Println("connected to", peer, "\tlatency:", data.versionLatency)
	case <-time.After(30 * time.Second):
		t.Fatal("no peers detected")
	}

	go func() {
		wait := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-wait.C:
				t.Log("got to worker")
				mu.RLock()
				size := len(blocks)
				mu.RUnlock()
				if size == 0 {
					t.Log("skipping worker")
					continue
				}

				copied := make([]*wire.MsgBlock, 0, size)
				mu.Lock()
				copied = append(copied, blocks...)
				blocks = blocks[len(copied):]
				mu.Unlock()

				t.Log("writing blocks", len(copied))
				if err := store.PutBlocks(ctx, copied...); err != nil {
					t.Fatal("error putting blocks", err)
				}
				fmt.Println("wrote", len(copied), "to DB")

			}
		}
	}()
	go func() {
		for {
			select {
			case block := <-onPeerBlock:
				func() {
					mu.Lock()
					defer mu.Unlock()
					hash := block.BlockHash()
					_, known := seenBlocks[hash]
					if known {
						return
					}

					//fmt.Println("adding block", len(blocks))

					blocks = append(blocks, block)
					seenBlocks[hash] = struct{}{}
				}()

			}
		}
	}()

	const batchSize = 1
	blockReqChan := make(chan []chainhash.Hash, 1)
	newWorker := func(peer *peer.Peer) {
		doneChan := make(chan struct{})
		go func() {
			peer.WaitForDisconnect()
			close(doneChan)
		}()
		for {
			for {
				select {
				case <-doneChan:
					return
				case hashes := <-blockReqChan:
					pData := node.GetPeerData(peer)
					if peer == nil || pData == nil {
						blockReqChan <- hashes
						return
					}
					data := wire.NewMsgGetData()
					for _, hash := range hashes {
						hash := hash
						//fmt.Println("asking", peer, "for", hash.String())
						if err := data.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &hash)); err != nil {
							log.Fatal(err)
						}
					}

					select {
					case <-pData.connected:
					case <-time.After(10 * time.Second):
						blockReqChan <- hashes
						continue
					}
					peer.QueueMessage(data, nil)
				tryAgain:
					select {
					case <-time.After(5 * time.Second):
						if len(hashes) == batchSize {
							peer.Disconnect()
							peer.WaitForDisconnect()
							fmt.Println("disconnected from", peer, "\t", node.TotalPeers(), "remaining")
						}
						// if we're here the peer didn't respond
						// try another peer
						t.Log("no response from peer trying another peer", peer,
							"adding", len(hashes), "blocks back to the queue")
						blockReqChan <- hashes
						continue
					case b := <-pData.onBlock:
						//t.Log("got block from peer")
						onPeerBlock <- b
						match := b.BlockHash()
						newHashes := make([]chainhash.Hash, 0, len(hashes)-1)
						for _, h := range hashes {
							if h == match {
								continue
							}
							newHashes = append(newHashes, h)
						}
						if len(newHashes) > 0 {
							hashes = newHashes
							goto tryAgain
						}
					}

				}

			}
		}
	}

	go func() {
		for {
			select {
			case peer := <-peerQueue:
				go newWorker(peer)
			case peer := <-node.peerConnected:
				peerQueue <- peer
				data := node.GetPeerData(peer)
				if data == nil {
					continue
				}
				fmt.Println("connected to", peer, "\tlatency:", data.versionLatency)
			}
		}
	}()

	var blocksToReq []chainhash.Hash
	work := func() {
		if len(blocksToReq) == 0 {
			return
		}
		blockReqChan <- blocksToReq
		//time.Sleep(20 * time.Second)
		blocksToReq = nil
	}

	var sent int
	var total int

	for _, data := range dataToSend {
		total++
		//fmt.Println(missing.Height, "is missing", missing.Hash.String())
		blocksToReq = append(blocksToReq, data.Hash)
		if len(blocksToReq) == batchSize {
			work()
			sent++
		}
	}

	work()

	fmt.Println("all requests sent", sent)
	fmt.Println(total, "blocks requested")

	select {}

}

func TestUpdateBlockHeights(t *testing.T) {
	node, err := NewMainNet()
	require.NoError(t, err)

	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})

	ctx := context.Background()
	height, err := store.GetTip(ctx)
	require.NoError(t, err)

	var mu sync.RWMutex
	knownHash := make(map[chainhash.Hash]struct{})
	var hashes []BlockHeight
	hashes = append(hashes, *height)
	knownHash[height.Hash] = struct{}{}

	headerReq := make(chan chainhash.Hash, 1)
	headerReq <- height.Hash
	newWorker := func(peer *peer.Peer) {
		pData := node.GetPeerData(peer)
		if pData == nil {
			return
		}
		for {
			select {
			case <-pData.disconnected:
				return
			case req := <-headerReq:
				mu.RLock()
				lastHash := hashes[len(hashes)-1]
				mu.RUnlock()
				err := peer.PushGetHeadersMsg(blockchain.BlockLocator{&lastHash.Hash}, &chainhash.Hash{})
				if err != nil {
					headerReq <- req
					continue
				}

				select {
				case msg := <-pData.onHeaders:
					newChunk := make([]BlockHeight, 0)

					for _, header := range msg.Headers {
						if header.PrevBlock == lastHash.Hash {
							lastHash = BlockHeight{
								Height: lastHash.Height + 1,
								Hash:   header.BlockHash(),
							}
							newChunk = append(newChunk, lastHash)

						}
					}

					if err := store.PutHeights(ctx, newChunk...); err != nil {
						log.Println("error putting heights", err)
						continue
					}
					fmt.Println("wrote", len(newChunk), "new headers to DB")
					mu.Lock()
					for _, headers := range msg.Headers {
						lastHeight := hashes[len(hashes)-1]
						if lastHeight.Hash == headers.PrevBlock {
							hashes = append(hashes, BlockHeight{
								Height: lastHeight.Height + 1,
								Hash:   headers.BlockHash(),
							})
						}
					}

					lastHash = hashes[len(hashes)-1]
					mu.Unlock()
					if len(msg.Headers) > 1500 {
						headerReq <- lastHash.Hash
					}

				case <-time.After(10 * time.Second):
					block, err := store.GetBlock(ctx, lastHash.Hash)
					require.NoError(t, err)
					earliestTime := time.Now().Add(-3 * time.Hour)
					if block.Header.Timestamp.After(earliestTime) {
						fmt.Println("recent block mined at", block.Header.Timestamp, "skipping header check for now")
						continue
					}
					go func() {
						log.Println("disconnecting from peer", peer, "no headers detected")
						peer.Disconnect()
						peer.WaitForDisconnect()

						headerReq <- req
					}()

					return
				}
			}
		}
	}

	go node.Connect()
	go func() {
		for {
			select {
			case peer := <-node.peerConnected:
				data := node.GetPeerData(peer)
				if data == nil {
					continue
				}
				log.Println("connected to", peer, "latency", data.versionLatency)
				go newWorker(peer)
			}
		}
	}()

	select {}

	_ = height

	_ = node

}

func TestBlockStore_GetTip(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})
	ctx := context.Background()
	t.Log(store.GetTip(ctx))
}

func TestBlockStore_GetBlock(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})
	ctx := context.Background()
	h, err := chainhash.NewHashFromStr("0000000000000000000060e32d547b6ae2ded52aadbc6310808e4ae42b08cc6a")
	require.NoError(t, err)
	t.Log(store.GetBlock(ctx, *h))
}

func TestBlockStore_MissingBlocks(t *testing.T) {
	store := NewBlockStore(&chaincfg.MainNetParams)
	t.Cleanup(func() {
		store.Close()
	})
	ctx := context.Background()

	var total int
	for stream := range store.MissingBlocks(ctx) {
		require.NoError(t, stream.Err)
		total++
	}

	t.Log(total, "missing blocks")

}

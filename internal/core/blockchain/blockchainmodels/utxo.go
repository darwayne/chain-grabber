package blockchainmodels

import "github.com/btcsuite/btcd/chaincfg/chainhash"

type UTXO struct {
	Txid   chainhash.Hash `json:"txid"`
	Index  uint32         `json:"vout"`
	Status UTXOStatus     `json:"status"`
	Value  int64          `json:"value"`
}

type UTXOStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int    `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	BlockTime   int    `json:"block_time"`
}

type AcceptResult struct {
	RejectReason string `json:"reject-reason"`
	Allowed      bool   `json:"allowed"`
}

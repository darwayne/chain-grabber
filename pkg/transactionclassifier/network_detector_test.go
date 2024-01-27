package transactionclassifier

import (
	"encoding/hex"
	"fmt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestFromHex(t *testing.T) {

	t.Run("hmm", func(t *testing.T) {
		tx := hexToTX(t, "02000000000101267aafc466cc7165403984f0e3a159f34834ac7d9198a51013645aadfea6121b0000000000fdffffff014c42070a000000001600142a092d60233cfe5cdc4519e06d1d0b39300e06410247304402204663bc6e6558af54310c33cfc7d6fcd68cc14cd01b76557bd1725edd44bc8ec6022024c57d29c17f9bfbeb2e507c0b5f7e37180612c07925d64fcc00ddc3d719aa1a012103d51ee4f55f6c3afdb263475a5e5a5b70fd9e1909391cff57ee29e60b7406078100000000")

		script, err := txscript.ParsePkScript(tx.TxOut[0].PkScript)
		require.NoError(t, err)
		t.Log(script.Class())

		t.Log(tx.Version)

		tx = hexToTX(t, "02000000000101a891fece3244421a5e96209deaf55ca53f066ff28b97c0bc2c4dad5e8e4d6f490000000000fdffffff02f401010000000000160014f5e905e896820a34f97f956d7846174c1c404303c39e5d0801000000160014ebd33cbc9499799f60bcef3f0fcd017eabd31c3d0246304302201d46751ee2778a4afbdebc263db4795bbfc10101fa7170ba43da3d79dba28e68021f4d8311c42838a9cd2584406f6333d608b1b852a6e72cf832ba44cbddfc883c0121033772d7e5a6278f01d97365f9d3a22f5cad4cfb83442358b75a732ca6c56f9370da4d2700")
		t.Log(tx.Version)

	})

	t.Run("should detect", func(t *testing.T) {
		t.Run("mainnet", func(t *testing.T) {
			result, err := FromHex("02000000000101267aafc466cc7165403984f0e3a159f34834ac7d9198a51013645aadfea6121b0000000000fdffffff014c42070a000000001600142a092d60233cfe5cdc4519e06d1d0b39300e06410247304402204663bc6e6558af54310c33cfc7d6fcd68cc14cd01b76557bd1725edd44bc8ec6022024c57d29c17f9bfbeb2e507c0b5f7e37180612c07925d64fcc00ddc3d719aa1a012103d51ee4f55f6c3afdb263475a5e5a5b70fd9e1909391cff57ee29e60b7406078100000000")
			require.NoError(t, err)
			require.NotNil(t, result)
		})

		t.Run("tesnet", func(t *testing.T) {
			result, err := FromHex("02000000000101a891fece3244421a5e96209deaf55ca53f066ff28b97c0bc2c4dad5e8e4d6f490000000000fdffffff02f401010000000000160014f5e905e896820a34f97f956d7846174c1c404303c39e5d0801000000160014ebd33cbc9499799f60bcef3f0fcd017eabd31c3d0246304302201d46751ee2778a4afbdebc263db4795bbfc10101fa7170ba43da3d79dba28e68021f4d8311c42838a9cd2584406f6333d608b1b852a6e72cf832ba44cbddfc883c0121033772d7e5a6278f01d97365f9d3a22f5cad4cfb83442358b75a732ca6c56f9370da4d2700")
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	})

}

func TestMe(t *testing.T) {
	tx := hexToTX(t, "020000000001017cffdbc3d905f3990aa22bcf5a5a37262302f2910793585cafc1a239917108dd0000000000fdffffff02d62b0000000000001600148f8c776ee61d04d5f7a2715d0f287b997bf4aec2a12e0000000000001600144762f13fe549727a8cc50810a10ad15cd988185a024730440220609267be3d8762b1c8d8c6407dbe7c15cbe0a0ed783b59fe09f5a965b2b2677d02207804e6553d9377afe9f1d3ec37cd8061e1b82a6a685d11bf49ab0efc0dcdbdb30121034cde5fd5f2fef73ec5d5ab296ee06dfa28fa1b4c876e2c9afb5744d9da519541df4d2700")
	_ = tx
	fmt.Println()

}

func hexToTX(t *testing.T, hexString string) *wire.MsgTx {
	s, err := hex.DecodeString(hexString)
	require.NoError(t, err)
	if s[len(s)-1] == 0x01 {
		t.Log("main net")
	} else if s[len(s)-1] == 0x04 {
		t.Log("testnet")
	}
	var tx wire.MsgTx

	err = tx.Deserialize(hex.NewDecoder(strings.NewReader(hexString)))
	require.NoError(t, err)

	return &tx

}

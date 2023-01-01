package main

import (
	"context"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
)

func main() {
	mngr, err := grabber.DefaultManager()
	if err != nil {
		panic(err)
	}

	// keys retrieved from https://medium.com/swlh/create-raw-bitcoin-transaction-and-sign-it-with-golang-96b5e10c30aa
	privKey := "91izeJtyQ1DNGkiRtMGRKBEKYQTX46Ug8mGtKWpX9mDKqArsLpH"
	myAddress := "moLoz9Ao9VTFMKp6AQaAwSVdzhfdfpCGf1" //"mkqVMJ5HxEq424XBJV1GCJMUtRAH8wy5SQ" //"moLoz9Ao9VTFMKp6AQaAwSVdzhfdfpCGf1"
	//destination := "tb1q3q7l58y4yucg0m7u7gglvm8t8hn9mztwcqxfdg"
	xpub := "vpub5VwWv7DDqNmuvVHB8KSXiTJ6w7nSwnp9YZZ7tMpr9Aon4DTTFPx4nAKRQPyo3JJ2qLhB9ekPKUrgWVGQ58QUybuFHYDQy3coXmDtaxePJxL"
	wif, err := btcutil.DecodeWIF(privKey)
	if err != nil {
		panic(err)
	}

	go func() {
		// keys retrieved from https://en.bitcoin.it/wiki/List_of_address_prefixes
		privKey := "92Pg46rUhgTT7romnV7iGW6W1gbGdeezqdbJCzShkCsYNzyyNcc"
		myAddress := "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn" //"mkqVMJ5HxEq424XBJV1GCJMUtRAH8wy5SQ" //"moLoz9Ao9VTFMKp6AQaAwSVdzhfdfpCGf1"

		wif, err := btcutil.DecodeWIF(privKey)
		if err != nil {
			panic(err)
		}

		sender := grabber.NewSimpleXpubSender(mngr, wif, myAddress, xpub, []uint32{0})
		err = sender.Monitor(context.Background())
		if err != nil {
			panic(err)
		}
	}()

	sender := grabber.NewSimpleXpubSender(mngr, wif, myAddress, xpub, []uint32{0})
	err = sender.Monitor(context.Background())
	if err != nil {
		panic(err)
	}
}

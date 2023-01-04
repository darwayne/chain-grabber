package main

import (
	"context"
	"fmt"
	"github.com/darwayne/chain-grabber/internal/core/grabber"
)

func main() {
	mngr, err := grabber.DefaultManager()
	if err != nil {
		panic(err)
	}

	xpub := "vpub5VwWv7DDqNmuvVHB8KSXiTJ6w7nSwnp9YZZ7tMpr9Aon4DTTFPx4nAKRQPyo3JJ2qLhB9ekPKUrgWVGQ58QUybuFHYDQy3coXmDtaxePJxL"

	knownPrivateKeys := []string{
		"92Pg46rUhgTT7romnV7iGW6W1gbGdeezqdbJCzShkCsYNzyyNcc",
		"91izeJtyQ1DNGkiRtMGRKBEKYQTX46Ug8mGtKWpX9mDKqArsLpH",
	}

	sender, err := grabber.NewXpubRangeSender(mngr, [2]uint32{0, 50_000}, xpub, []uint32{0}, knownPrivateKeys...)
	if err != nil {
		panic(err)
	}

	go func() {
		fmt.Println("Attempting to spend all")
		fmt.Println(sender.SpendAll())
	}()
	err = sender.Monitor(context.Background())
	if err != nil {
		panic(err)
	}

}

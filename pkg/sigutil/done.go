package sigutil

import (
	"os"
	"os/signal"
)

func Done() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		select {
		case <-c:
			close(done)
		}
	}()

	return done
}

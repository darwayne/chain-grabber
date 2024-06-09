package sigutil

import (
	"os"
	"os/signal"
)

var doneChan = getDone()

func Done() <-chan struct{} {
	return doneChan
}

func getDone() <-chan struct{} {
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

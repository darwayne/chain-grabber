package broadcaster

import (
	"log"
	"sync"
	"testing"
	"time"
)

func TestBroker(t *testing.T) {
	broker := NewBroker[string]()
	go broker.Start()
	var wg sync.WaitGroup
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	for i := 0; i < 1000; i++ {
		id := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := broker.Subscribe()
			for {
				select {
				case msg := <-sub:
					log.Println(id, "received", msg)
				case <-broker.Done():
					return
				}
			}
		}()
	}

	time.Sleep(time.Second)
	broker.Publish("yo son")
	broker.Done()
	wg.Wait()
}

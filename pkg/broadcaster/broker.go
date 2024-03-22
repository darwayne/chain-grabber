package broadcaster

type Broker[T any] struct {
	doneChan chan struct{}
	publish  chan T
	sub      chan chan T
	unsub    chan chan T
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{
		doneChan: make(chan struct{}),
		publish:  make(chan T, 1),
		sub:      make(chan chan T, 1),
		unsub:    make(chan chan T, 1),
	}
}

func (b *Broker[T]) Start() {
	subs := make(map[chan T]struct{})
	for {
		select {
		case <-b.doneChan:
			return
		case sub := <-b.sub:
			subs[sub] = struct{}{}
		case unsub := <-b.unsub:
			delete(subs, unsub)
		case msg := <-b.publish:
			for ch := range subs {
				select {
				case ch <- msg:
				default:
					go func() {
						select {
						case <-b.Done():
						case ch <- msg:
						}
					}()
				}
			}
		}
	}
}

func (b *Broker[T]) Stop() {
	close(b.doneChan)
}

func (b *Broker[T]) Done() <-chan struct{} {
	return b.doneChan
}

func (b *Broker[T]) Subscribe() chan T {
	msgCh := make(chan T, 1)
	b.sub <- msgCh
	return msgCh
}

func (b *Broker[T]) UnSubscribe(msgChan chan T) {
	b.unsub <- msgChan
}

func (b *Broker[T]) Publish(msg T) {
	b.publish <- msg
}

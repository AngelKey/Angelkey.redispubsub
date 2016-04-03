package pubsub

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/garyburd/redigo/redis"
)

type testPubHandler struct {
	t           *testing.T
	mutex       sync.Mutex
	connections int
}

func (h *testPubHandler) OnPublishConnect(conn redis.Conn, address string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.connections++
}

func (h *testPubHandler) OnPublishConnectError(err error, nextTime time.Duration) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.t.Fatal(err)
}

func (h *testPubHandler) OnPublishError(err error, channel string, data []byte) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.t.Fatal(err)
}

func TestPublisherBasic(t *testing.T) {
	sh := newTestSubHandler(t)
	s := NewRedisSubscriber("localhost:6379", sh, 0)
	defer s.Shutdown()

	ph := &testPubHandler{t: t}
	p := NewRedisPublisher("localhost:6379", ph, 0, 0)
	defer p.Shutdown()

	count := 100
	channels := []string{"foo", "bar", "hi"}

	// subscribe to all channels
	for _, channel := range channels {
		_, errChan := s.Subscribe(channel)
		if err := <-errChan; err != nil {
			t.Fatal(err)
		}
	}

	// publish 100 messages to each channel concurrently
	var wg sync.WaitGroup
	wg.Add(count * len(channels))
	for _, channel := range channels {
		for i := 0; i < count; i++ {
			go func(channel string, i int) {
				p.Publish(channel, []byte(strconv.Itoa(i)))
				wg.Done()
			}(channel, i)
		}
	}
	wg.Wait()

	// wait for all messages
	sh.waitForMessages(count * len(channels))

	// check the messages
	for _, channel := range channels {
		messages := sh.messages[channel]
		for i := 0; i < count; i++ {
			if _, ok := messages[strconv.Itoa(i)]; !ok {
				t.Fatalf("%d not found from channel %s", i, channel)
			}
		}
	}

	if ph.connections != DefaultPublisherPoolSize {
		t.Fatalf("Expected %d connections, got: %d", DefaultPublisherPoolSize, ph.connections)
	}
}

func TestPublisherBatch(t *testing.T) {
	sh := newTestSubHandler(t)
	s := NewRedisSubscriber("localhost:6379", sh, 0)
	defer s.Shutdown()

	ph := &testPubHandler{t: t}
	p := NewRedisPublisher("localhost:6379", ph, 1, 0)
	defer p.Shutdown()

	count := 100
	channels := []string{"foo", "bar", "hi"}

	// subscribe to all channels
	for _, channel := range channels {
		_, errChan := s.Subscribe(channel)
		if err := <-errChan; err != nil {
			t.Fatal(err)
		}
	}

	// publish 100 messages to each channel in a batch
	c := make([]string, len(channels)*count)
	m := make([][]byte, len(channels)*count)
	n := 0
	for _, channel := range channels {
		for i := 0; i < count; i++ {
			c[n] = channel
			m[n] = []byte(strconv.Itoa(i))
			n++
		}
	}
	if err := p.PublishBatch(c, m); err != nil {
		t.Fatal(err)
	}

	// wait for all messages
	sh.waitForMessages(count * len(channels))

	// check the messages
	for _, channel := range channels {
		messages := sh.messages[channel]
		for i := 0; i < count; i++ {
			if _, ok := messages[strconv.Itoa(i)]; !ok {
				t.Fatalf("%d not found from channel %s", i, channel)
			}
		}
	}
}

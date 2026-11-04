package consumer

import kafkago "github.com/segmentio/kafka-go"

type messageBatch struct {
	messages []kafkago.Message
	maxLen   int
	maxBytes int
	bytes    int
}

func newMessageBatch(maxLen, maxBytes int) *messageBatch {
	return &messageBatch{
		messages: make([]kafkago.Message, 0, maxLen),
		maxLen:   maxLen,
		maxBytes: maxBytes,
	}
}

func (b *messageBatch) add(msg kafkago.Message) {
	b.messages = append(b.messages, msg)
	b.bytes += len(msg.Key) + len(msg.Value)
	for _, header := range msg.Headers {
		b.bytes += len(header.Key) + len(header.Value)
	}
}

func (b *messageBatch) full() bool {
	return len(b.messages) >= b.maxLen || b.bytes >= b.maxBytes
}

func (b *messageBatch) len() int {
	return len(b.messages)
}

func (b *messageBatch) reset() []kafkago.Message {
	messages := b.messages
	b.messages = make([]kafkago.Message, 0, b.maxLen)
	b.bytes = 0
	return messages
}

func (b *messageBatch) getMessages() []kafkago.Message {
	return b.messages
}

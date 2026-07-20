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

func (batch *messageBatch) add(msg kafkago.Message) {
	batch.messages = append(batch.messages, msg)
	batch.bytes += len(msg.Key) + len(msg.Value)
	for _, header := range msg.Headers {
		batch.bytes += len(header.Key) + len(header.Value)
	}
}

func (batch *messageBatch) full() bool {
	return len(batch.messages) >= batch.maxLen || batch.bytes >= batch.maxBytes
}

func (batch *messageBatch) len() int {
	return len(batch.messages)
}

func (batch *messageBatch) reset() []kafkago.Message {
	messages := batch.messages
	batch.messages = make([]kafkago.Message, 0, batch.maxLen)
	batch.bytes = 0
	return messages
}

func (batch *messageBatch) getMessages() []kafkago.Message {
	return batch.messages
}

package kafkax

import (
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

func TestNewWriterHasNoTopic(t *testing.T) {
	w, err := NewWriter(WriterConfig{Brokers: []string{"localhost:9092"}, Compression: CompressionLz4})
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	if w.Topic != "" {
		t.Fatalf("writer Topic = %q; messages carry the topic, so the writer must not", w.Topic)
	}
	if w.Compression != kafkago.Lz4 {
		t.Fatalf("Compression = %v, want lz4", w.Compression)
	}
}

func TestSplitBrokers(t *testing.T) {
	got := SplitBrokers(" a:9092, ,b:9092 ")
	if len(got) != 2 || got[0] != "a:9092" || got[1] != "b:9092" {
		t.Fatalf("SplitBrokers() = %v", got)
	}
}

func TestCompressionCodecRejectsUnknown(t *testing.T) {
	if _, err := CompressionCodec("brotli"); err == nil {
		t.Fatal("CompressionCodec(brotli) error = nil")
	}
}

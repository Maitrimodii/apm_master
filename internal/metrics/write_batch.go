package metrics

type WriteBatch struct {
	Metrics []Metric
	Buckets []HistogramBucket
}

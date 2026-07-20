package metric

type WriteBatch struct {
	Metrics []Metric
	Buckets []HistogramBucket
}

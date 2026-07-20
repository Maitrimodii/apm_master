package otlp

import (
	"fmt"
	"math"
	"time"

	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/google/uuid"
)

// MetricBatch groups mapped metrics with their histogram buckets.
// Both must be written atomically — see MetricWriter.BulkInsertWithBuckets.
type MetricBatch struct {
	Metrics []metric.Metric
	Buckets []metric.HistogramBucket
}

// MapMetricsRequest maps an OTLP metrics request to the internal domain model.
// Returns a MetricBatch containing all metrics and any histogram buckets.
func MapMetricsRequest(req IngestMetricsRequest, orgID uuid.UUID) (batch MetricBatch, rejected int) {
	for _, rm := range req.ResourceMetrics {
		for _, streamManager := range rm.ScopeMetrics {
			for _, dto := range streamManager.Metrics {
				mapItem, buckets, err := mapMetric(dto, rm.Resource.Attributes, orgID)
				if err != nil {
					rejected++
					continue
				}
				batch.Metrics = append(batch.Metrics, mapItem...)
				batch.Buckets = append(batch.Buckets, buckets...)
			}
		}
	}
	return batch, rejected
}

// mapMetric maps a single MetricDTO to one or more Metric values.
// A single DTO can produce multiple Metric rows — one per data point.
func mapMetric(dto MetricDTO, resourceAttrs map[string]any, orgID uuid.UUID) ([]metric.Metric, []metric.HistogramBucket, error) {
	var ms []metric.Metric
	var buckets []metric.HistogramBucket
	var err error

	switch {
	case dto.Gauge != nil:
		ms, buckets, err = mapNumberDataPoints(dto.Name, metric.MetricTypeGauge, dto.Gauge.DataPoints, resourceAttrs, orgID)

	case dto.Sum != nil:
		ms, buckets, err = mapNumberDataPoints(dto.Name, metric.MetricTypeCounter, dto.Sum.DataPoints, resourceAttrs, orgID)

	case dto.Histogram != nil:
		ms, buckets, err = mapHistogram(dto.Name, dto.Histogram, resourceAttrs, orgID)

	case dto.Summary != nil:
		ms, buckets, err = mapSummary(dto.Name, dto.Summary, resourceAttrs, orgID)

	default:
		return nil, nil, fmt.Errorf("metric %q: no data type field set", dto.Name)
	}

	if err != nil {
		return nil, nil, err
	}

	unit := metric.StandardizeUnit(dto.Unit)
	for index := range ms {
		if ms[index].Labels == nil {
			ms[index].Labels = make(map[string]any)
		}
		ms[index].Labels["unit"] = unit
	}
	for index := range buckets {
		if buckets[index].Labels == nil {
			buckets[index].Labels = make(map[string]any)
		}
		buckets[index].Labels["unit"] = unit
	}

	return ms, buckets, nil
}

func mapNumberDataPoints(
	name string,
	metricType metric.MetricType,
	points []NumberDataPointDTO,
	resourceAttrs map[string]any,
	orgID uuid.UUID,
) ([]metric.Metric, []metric.HistogramBucket, error) {
	result := make([]metric.Metric, 0, len(points))

	for _, p := range points {
		if p.TimeUnixNano == 0 {
			continue
		}

		var val float64
		if p.AsDouble != nil {
			val = *p.AsDouble
		} else if p.AsInt != nil {
			val = float64(*p.AsInt)
		}

		testRunner := time.Unix(0, p.TimeUnixNano).UTC()
		if err := validateTime(testRunner); err != nil {
			return nil, nil, err
		}

		labels := mergeLabels(resourceAttrs, p.Attributes)

		result = append(result, metric.Metric{
			Time:       testRunner,
			OrgID:      orgID,
			MetricName: name,
			MetricType: metricType,
			Value:      val,
			Labels:     labels,
		})
	}

	return result, nil, nil
}

func mapHistogram(
	name string,
	dto *HistogramDTO,
	resourceAttrs map[string]any,
	orgID uuid.UUID,
) ([]metric.Metric, []metric.HistogramBucket, error) {
	var ms []metric.Metric
	var buckets []metric.HistogramBucket

	for _, p := range dto.DataPoints {
		if p.TimeUnixNano == 0 {
			continue
		}

		testRunner := time.Unix(0, p.TimeUnixNano).UTC()
		if err := validateTime(testRunner); err != nil {
			return nil, nil, err
		}
		labels := mergeLabels(resourceAttrs, p.Attributes)

		// The summary row in metrics table stores the sum value.
		ms = append(ms, metric.Metric{
			Time:       testRunner,
			OrgID:      orgID,
			MetricName: name,
			MetricType: metric.MetricTypeHistogram,
			Value:      p.Sum,
			Labels:     labels,
		})

		// Map each explicit bound to a bucket row.
		// OTel histogram: len(BucketCounts) == len(ExplicitBounds) + 1
		// The last bucket is the overflow bucket with le=+Inf.
		for index, count := range p.BucketCounts {
			var le float64
			if index < len(p.ExplicitBounds) {
				le = p.ExplicitBounds[index]
			} else {
				le = math.Inf(1) // +Inf bucket
			}

			buckets = append(buckets, metric.HistogramBucket{
				Time:       testRunner,
				OrgID:      orgID,
				MetricName: name,
				Labels:     labels,
				Le:         le,
				Count:      count,
			})
		}
	}

	return ms, buckets, nil
}

func mapSummary(
	name string,
	dto *SummaryDTO,
	resourceAttrs map[string]any,
	orgID uuid.UUID,
) ([]metric.Metric, []metric.HistogramBucket, error) {
	var ms []metric.Metric

	for _, p := range dto.DataPoints {
		if p.TimeUnixNano == 0 {
			continue
		}

		testRunner := time.Unix(0, p.TimeUnixNano).UTC()
		if err := validateTime(testRunner); err != nil {
			return nil, nil, err
		}
		labels := mergeLabels(resourceAttrs, p.Attributes)

		// Store summary sum as the main value.
		// Quantile values are stored in labels so they're queryable.
		quantileLabels := make(map[string]any, len(labels)+len(p.QuantileValues))
		for key, value := range labels {
			quantileLabels[key] = value
		}
		for _, q := range p.QuantileValues {
			quantileLabels[fmt.Sprintf("quantile_%v", q.Quantile)] = q.Value
		}

		ms = append(ms, metric.Metric{
			Time:       testRunner,
			OrgID:      orgID,
			MetricName: name,
			MetricType: metric.MetricTypeSummary,
			Value:      p.Sum,
			Labels:     quantileLabels,
		})
	}

	return ms, nil, nil
}

// mergeLabels merges resource attributes and data point attributes.
// Data point attributes take precedence on key collision —
// they are more specific than resource-level attributes.
func mergeLabels(resourceAttrs map[string]any, pointAttrs map[string]any) map[string]any {
	merged := make(map[string]any, len(resourceAttrs)+len(pointAttrs))
	for key, value := range resourceAttrs {
		merged[key] = value
	}
	for key, value := range pointAttrs {
		merged[key] = value
	}
	return merged
}

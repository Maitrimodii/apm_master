package metric

import (
	"math"
	"sort"
	"testing"
)

func TestPercentile(testRunner *testing.T) {
	tests := []struct {
		name     string
		dataset  []float64
		p        float64
		expected float64
	}{
		// Dataset A: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
		{
			name:     "Dataset A p50",
			dataset:  []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        0.5,
			expected: 5.5,
		},
		{
			name:     "Dataset A p90",
			dataset:  []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        0.9,
			expected: 9.1,
		},
		{
			name:     "Dataset A p95",
			dataset:  []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        0.95,
			expected: 9.55,
		},
		{
			name:     "Dataset A p99",
			dataset:  []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			p:        0.99,
			expected: 9.91,
		},

		// Dataset B: 100 identical values (e.g. 5.0)
		{
			name:     "Dataset B p50",
			dataset:  makeIdenticalSlice(100, 5.0),
			p:        0.5,
			expected: 5.0,
		},
		{
			name:     "Dataset B p99",
			dataset:  makeIdenticalSlice(100, 5.0),
			p:        0.99,
			expected: 5.0,
		},

		// Dataset C: Mixed values with outliers
		// Sorted: [1.0, 1.5, 1.8, 2.0, 2.2, 2.5, 3.0, 3.5, 4.0, 100.0]
		{
			name:     "Dataset C p50",
			dataset:  []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 100.0, 2.2, 1.8, 3.5},
			p:        0.5,
			expected: 2.35,
		},
		{
			name:     "Dataset C p90",
			dataset:  []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 100.0, 2.2, 1.8, 3.5},
			p:        0.9,
			expected: 13.6,
		},
		{
			name:     "Dataset C p95",
			dataset:  []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 100.0, 2.2, 1.8, 3.5},
			p:        0.95,
			expected: 56.8,
		},
		{
			name:     "Dataset C p99",
			dataset:  []float64{1.0, 2.0, 1.5, 3.0, 2.5, 4.0, 100.0, 2.2, 1.8, 3.5},
			p:        0.99,
			expected: 91.36,
		},

		// Dataset D: Realistic latency samples
		// Sorted: [12.5, 13.1, 13.8, 14.0, 14.5, 14.8, 15.0, 15.2, 16.0, 16.5, 17.2, 18.0, 18.5, 19.1, 20.2, 21.0, 24.3, 25.1, 29.0, 110.3]
		{
			name:     "Dataset D p50",
			dataset:  []float64{12.5, 15.2, 14.8, 25.1, 110.3, 13.1, 14.0, 16.5, 18.0, 20.2, 19.1, 15.0, 14.5, 13.8, 16.0, 17.2, 18.5, 21.0, 24.3, 29.0},
			p:        0.5,
			expected: 16.85,
		},
		{
			name:     "Dataset D p90",
			dataset:  []float64{12.5, 15.2, 14.8, 25.1, 110.3, 13.1, 14.0, 16.5, 18.0, 20.2, 19.1, 15.0, 14.5, 13.8, 16.0, 17.2, 18.5, 21.0, 24.3, 29.0},
			p:        0.9,
			expected: 25.49,
		},
		{
			name:     "Dataset D p95",
			dataset:  []float64{12.5, 15.2, 14.8, 25.1, 110.3, 13.1, 14.0, 16.5, 18.0, 20.2, 19.1, 15.0, 14.5, 13.8, 16.0, 17.2, 18.5, 21.0, 24.3, 29.0},
			p:        0.95,
			expected: 33.065,
		},
		{
			name:     "Dataset D p99",
			dataset:  []float64{12.5, 15.2, 14.8, 25.1, 110.3, 13.1, 14.0, 16.5, 18.0, 20.2, 19.1, 15.0, 14.5, 13.8, 16.0, 17.2, 18.5, 21.0, 24.3, 29.0},
			p:        0.99,
			expected: 94.853,
		},
	}

	for _, testCase := range tests {
		testRunner.Run(testCase.name, func(testRunner *testing.T) {
			sorted := make([]float64, len(testCase.dataset))
			copy(sorted, testCase.dataset)
			sort.Float64s(sorted)

			actual := Percentile(sorted, testCase.p)
			if math.Abs(actual-testCase.expected) > 1e-9 {
				testRunner.Errorf("Percentile(%f) = %f; expected %f", testCase.p, actual, testCase.expected)
			}
		})
	}
}

func makeIdenticalSlice(size int, val float64) []float64 {
	res := make([]float64, size)
	for index := range res {
		res[index] = val
	}
	return res
}

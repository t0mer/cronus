package stats

import (
	"math"
	"testing"
)

const eps = 1e-9

func almost(a, b float64) bool { return math.Abs(a-b) <= eps }

func TestMedian(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{5}, 5},
		{"odd unsorted", []float64{3, 1, 2}, 2},
		{"even", []float64{4, 1, 3, 2}, 2.5},
		{"negatives", []float64{-2, -1, -3}, -2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Median(tt.in); !almost(got, tt.want) {
				t.Fatalf("Median(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMedianDoesNotMutateInput(t *testing.T) {
	in := []float64{3, 1, 2}
	_ = Median(in)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Fatalf("Median mutated its input: %v", in)
	}
}

func TestMean(t *testing.T) {
	if got := Mean([]float64{1, 2, 3, 4}); !almost(got, 2.5) {
		t.Fatalf("Mean = %v, want 2.5", got)
	}
	if got := Mean(nil); got != 0 {
		t.Fatalf("Mean(nil) = %v, want 0", got)
	}
}

func TestStdDev(t *testing.T) {
	// Classic population-variance example: values with mean 5, variance 4.
	if got := StdDev([]float64{2, 4, 4, 4, 5, 5, 7, 9}); !almost(got, 2) {
		t.Fatalf("StdDev = %v, want 2", got)
	}
	if got := StdDev([]float64{5}); got != 0 {
		t.Fatalf("StdDev(single) = %v, want 0", got)
	}
	if got := StdDev(nil); got != 0 {
		t.Fatalf("StdDev(nil) = %v, want 0", got)
	}
}

func TestOutlierIndices(t *testing.T) {
	// Three tightly-clustered servers around ~11ms and one falseticker at 500ms.
	xs := []float64{0.010, 0.012, 0.011, 0.500}
	got := OutlierIndices(xs, 0.100)
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("OutlierIndices = %v, want [3]", got)
	}
}

func TestOutlierIndicesNoneWhenClustered(t *testing.T) {
	xs := []float64{0.010, 0.012, 0.011, 0.013}
	if got := OutlierIndices(xs, 0.100); len(got) != 0 {
		t.Fatalf("OutlierIndices = %v, want none", got)
	}
}

func TestOutlierIndicesEmpty(t *testing.T) {
	if got := OutlierIndices(nil, 0.1); got != nil {
		t.Fatalf("OutlierIndices(nil) = %v, want nil", got)
	}
}

func TestPairwiseDeltas(t *testing.T) {
	got := PairwiseDeltas([]float64{1, 2, 4})
	want := [][]float64{
		{0, -1, -3},
		{1, 0, -2},
		{3, 2, 0},
	}
	for i := range want {
		for j := range want[i] {
			if !almost(got[i][j], want[i][j]) {
				t.Fatalf("delta[%d][%d] = %v, want %v", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestLinearRegressionPerfectLine(t *testing.T) {
	xs := []float64{0, 1, 2, 3}
	ys := []float64{1, 3, 5, 7} // y = 2x + 1
	reg, ok := LinearRegression(xs, ys)
	if !ok {
		t.Fatal("LinearRegression returned ok=false for a valid line")
	}
	if !almost(reg.Slope, 2) || !almost(reg.Intercept, 1) {
		t.Fatalf("slope=%v intercept=%v, want 2 and 1", reg.Slope, reg.Intercept)
	}
	if !almost(reg.R2, 1) {
		t.Fatalf("R2 = %v, want 1", reg.R2)
	}
	if reg.N != 4 {
		t.Fatalf("N = %d, want 4", reg.N)
	}
}

func TestLinearRegressionDegenerate(t *testing.T) {
	if _, ok := LinearRegression([]float64{1}, []float64{1}); ok {
		t.Fatal("expected ok=false for a single point")
	}
	if _, ok := LinearRegression([]float64{2, 2, 2}, []float64{1, 2, 3}); ok {
		t.Fatal("expected ok=false when all x are identical")
	}
	if _, ok := LinearRegression([]float64{1, 2}, []float64{1}); ok {
		t.Fatal("expected ok=false for mismatched lengths")
	}
}

func TestDriftPPM(t *testing.T) {
	// Offset grows 3.6ms per hour => 1e-6 s/s => exactly 1 ppm.
	times := []float64{0, 3600, 7200}
	offsets := []float64{0, 0.0036, 0.0072}
	ppm, reg, ok := DriftPPM(times, offsets)
	if !ok {
		t.Fatal("DriftPPM returned ok=false")
	}
	if !almost(ppm, 1.0) {
		t.Fatalf("DriftPPM = %v, want 1.0", ppm)
	}
	if !almost(reg.R2, 1) {
		t.Fatalf("R2 = %v, want 1", reg.R2)
	}
}

func TestDriftPPMNegative(t *testing.T) {
	// Clock gaining: offset shrinks over time => negative drift.
	times := []float64{0, 3600, 7200}
	offsets := []float64{0, -0.0036, -0.0072}
	ppm, _, ok := DriftPPM(times, offsets)
	if !ok || !almost(ppm, -1.0) {
		t.Fatalf("DriftPPM = %v (ok=%v), want -1.0", ppm, ok)
	}
}

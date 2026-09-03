// Package stats provides the numerical comparisons at the heart of Cronus:
// median offset and consensus, falseticker (outlier) detection, jitter, the
// pairwise offset-delta matrix, and drift estimation via linear regression.
//
// All functions operate on plain float64 slices. Callers are expected to work
// in a single consistent unit (Cronus uses seconds throughout); results are in
// the same unit as the inputs, except DriftPPM which returns parts-per-million.
package stats

import (
	"math"
	"sort"
)

// Median returns the median of xs. For an even number of elements it returns
// the mean of the two central values. Median of an empty slice is 0.
func Median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, xs)
	sort.Float64s(cp)
	mid := n / 2
	if n%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

// Mean returns the arithmetic mean of xs, or 0 for an empty slice.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// StdDev returns the population standard deviation of xs. This is Cronus's
// definition of jitter across the samples of a single run. It returns 0 for a
// slice of fewer than two elements.
func StdDev(xs []float64) float64 {
	n := len(xs)
	if n < 2 {
		return 0
	}
	m := Mean(xs)
	var sumSq float64
	for _, x := range xs {
		d := x - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n))
}

// OutlierIndices returns the indices of values in xs whose absolute deviation
// from the median exceeds threshold. These are the suspected falsetickers. The
// returned indices are in ascending order. A non-positive threshold flags every
// value that differs from the median at all.
func OutlierIndices(xs []float64, threshold float64) []int {
	if len(xs) == 0 {
		return nil
	}
	med := Median(xs)
	var out []int
	for i, x := range xs {
		if math.Abs(x-med) > threshold {
			out = append(out, i)
		}
	}
	return out
}

// PairwiseDeltas returns the matrix of signed differences xs[i]-xs[j] between
// every pair of values, independent of any local reference. Element [i][j] is
// xs[i]-xs[j]; the diagonal is 0 and the matrix is antisymmetric.
func PairwiseDeltas(xs []float64) [][]float64 {
	n := len(xs)
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		for j := range m[i] {
			m[i][j] = xs[i] - xs[j]
		}
	}
	return m
}

// Regression is the result of a least-squares linear fit y = Slope*x + Intercept.
type Regression struct {
	Slope     float64
	Intercept float64
	// R2 is the coefficient of determination in [0,1]; it is 0 when the fit is
	// degenerate (e.g. all y equal).
	R2 float64
	// N is the number of points used for the fit.
	N int
}

// LinearRegression performs an ordinary least-squares fit of ys against xs.
// It returns ok=false when there are fewer than two points or when all xs are
// identical (an undefined slope).
func LinearRegression(xs, ys []float64) (Regression, bool) {
	n := len(xs)
	if n < 2 || n != len(ys) {
		return Regression{}, false
	}
	meanX := Mean(xs)
	meanY := Mean(ys)
	var sxx, sxy, syy float64
	for i := range xs {
		dx := xs[i] - meanX
		dy := ys[i] - meanY
		sxx += dx * dx
		sxy += dx * dy
		syy += dy * dy
	}
	if sxx == 0 {
		return Regression{}, false
	}
	slope := sxy / sxx
	intercept := meanY - slope*meanX
	var r2 float64
	if syy != 0 {
		r2 = (sxy * sxy) / (sxx * syy)
	}
	return Regression{Slope: slope, Intercept: intercept, R2: r2, N: n}, true
}

// DriftPPM estimates clock drift as the rate of offset change over time,
// expressed in parts-per-million. times are in seconds and offsets are in
// seconds; the slope (seconds of offset per second elapsed) is scaled by 1e6.
// It returns ok=false when a regression cannot be computed.
func DriftPPM(times, offsets []float64) (float64, Regression, bool) {
	reg, ok := LinearRegression(times, offsets)
	if !ok {
		return 0, Regression{}, false
	}
	return reg.Slope * 1e6, reg, true
}

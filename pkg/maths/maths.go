// Copyright 2025 NetApp, Inc. All Rights Reserved.

package maths

// Pow is an integer version of exponentiation; existing builtin is float, we needed an int version.
func Pow(x int64, y int) int64 {
	if y == 0 {
		return 1
	}

	result := x
	for n := 1; n < y; n++ {
		result = result * x
	}
	return result
}

func Max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

// MinInt64 returns the lower of the two integers specified
func MinInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

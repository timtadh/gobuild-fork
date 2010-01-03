package main_test

import (
	"fmt"
	"testing"
)

func TestGobuild(t *testing.T) {
	// TODO: do something testy here
	fmt.Println("Here be some testing someday. Please?")
}

func BenchmarkGobuild(b *testing.B) {
	b.StopTimer()
	fmt.Println("Benchmarks would be nice too...")
	b.StartTimer()
}

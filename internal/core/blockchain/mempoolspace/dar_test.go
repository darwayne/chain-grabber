package mempoolspace

import (
	"fmt"
	"maps"
	"testing"
	"time"
)

func TestYo(t *testing.T) {
	start := time.Now()
	hey := make(map[[2]int]int)
	for i := 0; i < 1_000_000; i++ {
		hey[[2]int{i, i + 1}] = i
	}
	initialCreate := time.Since(start)

	start = time.Now()
	there := maps.Clone(hey)
	clone := time.Since(start)

	fmt.Println(&hey == &there)

	fmt.Println("it took", initialCreate, "to create")
	fmt.Println("it took", clone, "to clone")

}

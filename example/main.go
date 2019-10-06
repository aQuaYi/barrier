package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/aQuaYi/barrier"
)

func main() {
	participants := 5
	round := 3

	count := 0
	b := barrier.New(participants).SetAction(func() {
		count++
		fmt.Printf("\tcount: %d\n", count)
	})

	var wg sync.WaitGroup
	wg.Add(participants)

	for i := 0; i < participants; i++ {
		go func(id int) {
			for j := 0; j < round; j++ {
				dur := time.Duration(rand.Intn(200)) * time.Millisecond
				time.Sleep(dur)
				fmt.Printf("OK:%d\n", id)
				b.Wait(context.TODO())
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
}

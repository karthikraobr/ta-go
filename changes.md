### Handling huge number of URLs

#### Counting semaphore
I initially started off by implementing a counting semaphore wherein I would span all the required goroutines and only x number of goroutines would have get a place on the semaphore channel.

```go
sem := make(chan struct{}, min(maxConnections, len(urls)))

func fetch(){
    sem <- struct{}{}
    defer func() { <-sem }()
}
```

If the number of URLs = 100000 we would be spawning 100000 goroutines of which only x say 500 would get a place on the semaphore channel and actually execute the fetch logic. The rest of the goroutines would be busy waiting which would be wasteful.

#### Worker pool
After some careful deliberation and researching on various go forums and blog posts, the suggested method of solving this problem was to spawn x number of worker goroutines and perform the fetch operation. This ensures that no goroutines waste time waiting and that we do not hit the socket or the file descriptor limits. During benchmarking on my laptop, the application successfully handled 100000 URLs and will probably handle more but might take significant time.

PS. I have increased both context timeouts and individual request timeouts to accommodate for the time taken to process huge number of URLs and larger data sets. As a result some tests in the test table might fail. Also benchmark test have been added to the test file.

### De-duplication and sorting

Removing duplicates across results returned would be a trade-off between time and memory. The time efficient way of doing it is by using a Set(map in our case) which holds distinct elements.

The following approaches were considered to tackle the problem of sorting
* build-in sort package
* heap
* insertion sort
* radix sort

Before discussing about the sorting techniques, the question to when to sort should be handled.
* Continuous de-duplication and continuous sorting
* Continuous de-duplicate and sort once
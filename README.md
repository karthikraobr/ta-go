# go-challenge

Task - Retrieve data from URLs, merge, filter duplicates and sort in 500ms.

## Context package
The first thing that popped into my head when I saw "500ms" was go's context package.
The other alternatives which I considered were: 
* Using a http client with a timeout
* Wait on a channel inside the select statement using time.After. 

The rationale behind using context is cancellation propagation. In the aforementioned alternatives, there is no way to signal to the various goroutines to stop their task and return immediately after a certain time. But context provides a clean, easy and efficient way of doing this. This makes http clients and servers more efficient and less wasteful. For e.g. if a client that requested some resource from the server suddenly drops off, context helps us to cancel all the functions and goroutines that were running to satisfy the client's request. Context achieves cancellation propagation by maintaining a tree structure of "context dependencies". Also, cancellation is transitive i.e. if a particular node in the context tree is cancelled, all the child nodes are automatically cancelled.

## Timeout
The next question to be tackled was - "How long do we let the goroutines that fetch data from valid URLs execute?". The requirements specified that our endpoint need to return the results within 500ms, but no timeout was specified for the individual URLs that were passed as query params. Since we had to return within 500ms, the timeout for each goroutine was set to 450ms after which it would be cancelled and use the remaining 50ms to sort our result. However for a real-world application we would have to consider the following criteria
* The size of the result returned by each of the URLs in the query parameter.
* How sorted or unsorted each of the results are?
* Performance of go's sort package. For ints it uses quicksort, heapSort, insertionsort and shellsort based on various criteria.

## Buffered vs unbuffered channels

## Filtering duplicates and sorting
Once the timeout for the goroutines was decided, the next problem to solve was "When to filter duplicates and sort". "Do we filter duplicates and sort once we have the results from all the URLs or do we do it on a Per-URL basis?" filter per url and sort once.

## What to do with errors?

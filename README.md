# go-challenge

Task - Retrieve data from URLs, merge, filter duplicates and sort in 500ms.


The rationale behind the various decisions taken while developing the solution are explained below. Please read the comments in code for better understanding. If you wish to run the application in docker, a dockerfile is included. The application exposes an endpoint called /numbers and listens on port 8000.

## Context package
The first thing that popped into my head when I saw "500ms" was go's context package.
The other alternatives which I considered were: 
* Using a http client with a timeout
* Wait on a channel inside the select statement using time.After. 

The rationale behind using context is cancellation propagation. In the aforementioned alternatives, there is no way to signal to the various goroutines to stop their task and return immediately after a certain time. But context provides a clean, easy and efficient way of doing this. This makes http clients and servers more efficient and less wasteful. For e.g. if a client that requested some resource from the server suddenly drops off, context helps us to cancel all the functions and goroutines that were running to satisfy the client's request. Context achieves cancellation propagation by maintaining a tree structure of "context dependencies". Also, cancellation is transitive i.e. if a particular node in the context tree is cancelled, all the child nodes are automatically cancelled.

## Timeout
The next question to be tackled was - "How long do we let the goroutines that fetch data from valid URLs execute?". The requirements specified that the /numbers endpoint needs to return the results within 500ms, but no timeout was specified for the individual URLs that were passed as query params. Since results had to be returned within 500ms, the timeout for each goroutine was set to 450ms after which it would be cancelled and remaining 50ms would be utilized to sort the accumulated results. However for a real-world application we would have to consider the following criteria before deciding the timeout:

* The size of the result returned by each of the URLs in the query parameter.
* How sorted or unsorted are each of the results, returned by the URLs?
* Performance of go's sort package.

## Buffered vs unbuffered channels
Once the timeout for the goroutines was decided, the next question to answer was "Whether to use buffered or unbuffered channel?" Buffered channel was the obvious choice because we didn't want the various goroutines that were spawned to fetch results from each URL to be blocked by each other, waiting for the channel to be free. Also, we didn't want the goroutines to be blocked by our receiver (the main goroutine in our case).

## Filtering duplicates and sorting
The next problem to solve was "When to filter duplicates and sort the results returned by the various URLs?". 

* Filter duplicates and sort after the results from all the URLs have been accumulated or do it as and when the result from each URL are fetched?
* Use the built-in sort package or implement own sorting technique?
* Can the performance be improved by using data structures such as heaps?

The chosen solution to this problem was to remove duplicates as and when the URLs return results by maintaining a hashtable (since go does not have a native implementation of set) of all the encountered values. When new results are received, we check if the values present in the result are already present in the hashtable. If not, those values are added both to the hashtable and final result array. Hashtable was chosen as it provides lookups in O(1) and does not allow duplicates. Once duplicates are filtered across the results returned by the all the URLs, we sort the final result array before sending it over the wire. Sorting is done once 450ms have elapsed because sorting the results of individual URLs is wasteful and repetitive. Heap wasn't considered because we would have to iterate over the results to push it to the heap and iterate once more once we have results from all the URLs to pop individual elements from the heap. The built-in sort package was chosen after looking at the benchmarks published [here](https://stackimpact.com/blog/practical-golang-benchmarks/#sorting).

## What to do with errors?
For now errors are just being logged. The errors from the various URLs are collected from the goroutines using an error channel and logged in the main goroutine. Alternatively, this could be sent to the client.
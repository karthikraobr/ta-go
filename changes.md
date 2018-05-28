### Handling a large number of URLs

#### Counting semaphore
I started by implementing a counting semaphore wherein I would spawn all the required goroutines and only x number of goroutines would get a place on the semaphore channel.

```go
sem := make(chan struct{}, min(maxConnections, len(urls)))

func fetch(){
    sem <- struct{}{}
    defer func() { <-sem }()
}
```

If the number of URLs = 100000, we would be spawning 100000 goroutines of which only x, say 500, would get a place on the semaphore channel and actually execute the fetch logic. The rest of the goroutines would be busy waiting for a place on the semaphore channel.

#### Worker pool
After some careful deliberation and researching on various go forums and blog posts, the suggested method of solving this problem was to spawn x number of worker goroutines and perform the fetch operation. This ensures that no goroutines waste time waiting and that we do not hit the socket limit or the file descriptor limits. During benchmarking on my laptop, the application successfully handled 100000 URLs and will probably handle more but might take significant time.

PS. I have increased both context timeouts and individual request timeouts to accommodate for the time taken to process a large number of URLs and larger datasets. As a result, some tests in the test table might fail. Also, benchmarks have been added to the test file. 

### De-duplication and sorting

Removing duplicates across results returned by the URLs would be a trade-off between time and memory. The time efficient way of doing it is by using a Set(map in our case) which holds distinct elements. The other way of doing this (space efficient) would be by linearly comparing all elements and removing duplicates.


Before discussing the sorting techniques, the question about when to sort should be handled.
* Continuous de-duplication and continuous sorting - Every time a URL responds, we filter duplicates, merge the data with an "accumulator" slice and sort it. This would be less efficient as we would sort the data repeatedly and sorting is computationally expensive. But an advantage of this approach is that any given time, the results would be sorted and ready to be sent over the wire.

* Continuous de-duplication and sorting once - In this approach, we filter duplicates, merge the data with an "accumulator" slice every time a URL responds.  Once all the URLs have responded, we sort the accumulated data. This would be significantly efficient since we only sort once after the results from all the URLs have been accumulated. On the flip side, the results wouldn't be sorted to be sent over the wire at any given time.

The following approaches were considered to tackle the problem of sorting
* built-in sort package - Uses a combination of quicksort, shellsort and insertionsort. Generalized algorithm hence performs well in most general cases. O(nlogn)
* radix sort - Works best on integers. Sorts by comparing each digit of a number. Might perform badly if the number of digits in a number is huge. This is because radix sort performs a "sorting pass" for each digit in the numbers of the data set. O(kn) where is the number of digits in the largest number.
* heapsort - Gets instant access to the largest/smallest element in O(1). Was curious how a heap would perform in our case. 
* b-trees - Automatic sorting and de-duplicating of elements.

PS. For b-trees and radix sort I used external packages 
* radix sort - https://github.com/shawnsmithdev/zermelo - This package claims the following - "You will generally only want to use zermelo if you won't mind the extra memory used for buffers and your application frequently sorts slices of supported types with at least 256 elements (128 for 32-bit types). The larger the slices you are sorting, the more benefit you will gain by using zermelo instead of the standard library's in-place comparison sort." 
* b-trees - https://github.com/google/btree 

### Benchmarks

The benchmarks can be found at https://github.com/karthikraobr/go-sorting-bench. To mimic the problem at hand, a slice of 1 million int values is being split into 10 - 100,000 slices and then duplicates are filtered out.

* filterAndSortOnceDefault - Filters duplicates as and when URLs respond and sorts once all URLs have responded, using the built-in sort package.
* filterAndSortOnceZermelo - Same as above but uses zermelo radix sort package.
* filterAndContinuousSortDefault - Filters duplicates as and when URLs respond and sort them on a per-URL basis. Sort uses built-in sort package.
* filterAndContinuousSortZermelo - Same as above but uses zermelo radix sort package.
* heapsort - Filter duplicates and push into a heap. Then pop one element at a time.
* btree - Push each element of the URL responses into a tree. De-duplication and sorting is taking care by the implementation.

| Name        | Count           | Time taken  | Memory
| ------------- |:-------------:| -----:|-----:|
|BenchmarkEverything/filterAndSortOnceDefault-8                    |5     |296403520 ns/op    |53860502 B/op      | 19206 allocs/op|
|BenchmarkEverything/filterAndSortOnceZermelo-8                   |10     |162799810 ns/op    |58916528 B/op      | 19170 allocs/op|
|BenchmarkEverything/filterAndContinuousSortDefault-8              |2     |749996950 ns/op    |53849292 B/op       |19186 allocs/op|
|BenchmarkEverything/filterAndContinuousSortZermelo-8              |5     |262995680 ns/op    |85795990 B/op      | 19044 allocs/op|
|BenchmarkEverything/heap-8                                       |2     |552999600 ns/op    |92791836 B/op     |1283001 allocs/op|
|BenchmarkEverything/btree-8                                       |1    |1335997200 ns/op    |52337000 B/op     |1178894 allocs/op|

I expected the tree solution to perform the best since it takes care of the sorting and de-duplication. But it quite didn't stand up to the expectations. As seen from the above table the zermelo sort package performs the best in all the benchmarks.

To conclude sorting duplicates on a per-URL basis and then sorting the accumulated de-duplicated data performed the best.

PS. My solution still uses the default sort package since external packages are not allowed.

While profiling the application using pprof, it was discovered that when the number of URLs is large, the network is the bottleneck. When the data set is significantly large, finding out the pivot element is the bottleneck.
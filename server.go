package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"sort"
	"time"
)

const (
	//Represents the context timeout. Can be tuned depending on requirements and benchmarks.
	timeout        = 50000
	endpoint       = "/numbers"
	maxConnections = 500
)

//Type which represents the response of the given URLs as well as our response
type result struct {
	Numbers []int `json:"numbers"`
}

func main() {
	listenAddr := flag.String("http.addr", ":8000", "http listen address")
	flag.Parse()
	http.HandleFunc(endpoint, numbersHandler)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

//Http handler that handles the /numbers endpoint
func numbersHandler(w http.ResponseWriter, r *http.Request) {
	//We support only "GET" method.
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 - Method not supported!"))
		return
	}
	//Grab the context from the request. Incase a client drops we can cancel all the "tasks"
	//that were spawned for the client.
	ctx := r.Context()
	//Since the response needs to be sent on the wire before 500ms, we set the context timeout to 450ms
	//and use the remaining 50ms to sort the array.
	ctx, cancel := context.WithTimeout(ctx, timeout*time.Millisecond)
	//The cancel function signals the gc to collect resources allocated for context timers.
	defer cancel()
	u := r.URL
	q := u.Query()
	//Get all the query parameters with key "u".
	params := q["u"]
	log.Printf("Length of urls is %d\n", len(params))
	//If there are no "u" query parameters, simply return an empty array.
	if len(params) == 0 {
		json.NewEncoder(w).Encode(result{Numbers: []int{}})
	} else {
		numbers := validateAndFetch(ctx, params)
		json.NewEncoder(w).Encode(result{Numbers: numbers})
	}
}

func validateAndFetch(ctx context.Context, urls []string) []int {
	//Accumulator holds non duplicate values returned by all the URLs.
	accumulator := make([]int, 0)
	//Map to eliminate duplicates across responses. We actually need a set,
	//but since go doesn't provide a set implementation, we use an empty struct so as
	//to not waste precious memory :) https://play.golang.org/p/ea_19tva-0T
	visited := make(map[int]struct{})
	//Buffered channel - So as to not block goroutines.
	//In the case of unbuffered channel, the sender is blocked when the channel is full and receiver
	//is blocked when the channel is empty. If the receiver is busy with other tasks and is taking a
	//long time to receive on the channel, all the senders are blocked. With a buffered channel, sends
	//are blocked when the channel has reached its maximum capacity and receives are blocked when the
	//channel is empty. Hence the sender has a "buffer" to send on and is not blocked by the "slow" receiver.
	//In our case the receiver (main goroutine) might be filtering duplicates from a slice, while other goroutines
	//send on the buffered channel.
	ch := make(chan result, len(urls))
	//An error channel to hold the various errors that might occur while performing the request. For now we just
	//log it. We can define retry policies such as exponential backoff based on various error types.
	er := make(chan error, len(urls))
	//Counter to keep track of number of valid URLs in the request.
	counter := 0
	sem := make(chan struct{}, min(maxConnections, len(urls)))
	//Check if all URLs in the request are valid and if so spawn a goroutine to fetch data.
	for _, u := range urls {
		_, err := url.Parse(u)
		if err != nil {
			log.Printf("%s returned an error- %v", u, err)
			continue
		}
		counter++
		//Spawn a goroutine for each valid URL.
		go fetch(ctx, sem, u, ch, er)
	}

	//Loop to drain channels, filter out duplicates and check for timeout. Each goroutine either fills the
	//result channel or the error channel.
	for i := 0; i < counter; i++ {
		select {
		case res := <-ch:
			for _, val := range res.Numbers {
				//Eliminate duplicates. The rationale behind eliminating duplicates on a per-goroutine basis
				//as soon we receive on the channel rather than accumulating results from all the valid URLs
				//or after 450ms has elapsed is that - if no other goroutine has filled the channel, rather
				//than wasting precious processor clock on waiting, we may as well remove duplicates from
				//the array during that time.
				if _, ok := visited[val]; !ok {
					accumulator = append(accumulator, val)
					visited[val] = struct{}{}
				}
			}
		case err := <-er:
			fmt.Println(err)
			//After 450ms have elapsed, the context is finished. Done returns a closed channel that signals that
			//the context was cancelled, which in our case that is a timeout.
		case <-ctx.Done():
			fmt.Println(ctx.Err())
			//Sort and return as soon as the context is finished rather than waiting for other goroutines to be cancelled.
			sort.Ints(accumulator)
			return accumulator
		}
	}
	//Sort and return if all URLs respond within 450ms.
	sort.Ints(accumulator)
	return accumulator
}

func fetch(ctx context.Context, sem chan struct{}, u string, c chan<- result, e chan<- error) {
	var number result
	sem <- struct{}{}
	defer func() { <-sem }()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		e <- fmt.Errorf("%s returned an error while creating a request- %v", u, err)
		return
	}
	//Perform a request with a context to enable cancellation propagation after 450ms has elapsed.
	//As soon 450ms is elapsed the parent conext signals all the goroutines to abandon their work and return.
	req = req.WithContext(ctx)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e <- fmt.Errorf("%s returned an error while performing a request  - %v", u, err)
		return
	}
	//Close response body as soon as function returns to prevent resource lekage.
	//https://golang.org/pkg/net/http/#Response
	defer res.Body.Close()
	//If not 200 log the error.
	if res.StatusCode != http.StatusOK {
		e <- fmt.Errorf("%s server returned an error - %v", u, res.Status)
		return
	}
	if err := json.NewDecoder(res.Body).Decode(&number); err != nil {
		e <- fmt.Errorf("%s decoding error - %v", u, err)
		return
	}
	fmt.Println("success")
	c <- number
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

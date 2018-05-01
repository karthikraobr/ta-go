package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/url"
	"sort"
	"time"
)

//Type which represents the response of the given URLs as well as our response
type result struct {
	Numbers []int `json:"numbers"`
}

func main() {
	listenAddr := flag.String("http.addr", ":8000", "http listen address")
	flag.Parse()
	http.HandleFunc("/numbers", numberHandler)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

//Http handler that handles the /numbers endpoint
func numberHandler(w http.ResponseWriter, r *http.Request) {
	//Grab the context from the request. Incase a client drops we can cancel all the "tasks"
	//that were spawned for the client.
	ctx := r.Context()
	//Since the response needs to be sent on the wire before 500ms, we set the context timeout to 450ms
	//and use the remaining 50 seconds to sort the array.
	ctx, cancel := context.WithTimeout(ctx, 450*time.Millisecond)
	//The cancel signals the gc to collect resources allocated for context timers.
	defer cancel()
	u := r.URL
	q := u.Query()
	//Get all the query parameters with key "u".
	params := q["u"]
	//If there are no "u" query parameters, simply return an empty array.
	if len(params) == 0 {
		json.NewEncoder(w).Encode(map[string][]int{"numbers": nil})
	} else {
		numbers := validateAndFetch(ctx, params)
		json.NewEncoder(w).Encode(map[string][]int{"numbers": numbers})
	}
}

func validateAndFetch(ctx context.Context, urls []string) []int {
	//Accumulator holds non duplicate values returned by all the URLs.
	var accumulator []int
	//Map to eliminate duplicates across responses. We actually need a set,
	//but since go doesn't provide a set implementation, we use an empty struct so as
	//to not waste precious memory :) https://play.golang.org/p/ea_19tva-0T
	visited := make(map[int]struct{})
	//Buffered channel - So as to not starve the goroutines. Consider a scenario where in one of the
	//spawned goroutines puts a slice of size 1000000 into our channel. Draining the channel in the main goroutine
	//and filtering duplicates would take considerable time. If an unbuffered channel was used, all the
	//goroutines would have to wait until the channel is drained and duplicates removed.
	ch := make(chan result, len(urls))
	//Check if all URLs in the request are valid and if so spawn a goroutine to fetch data.
	for _, u := range urls {
		_, err := url.Parse(u)
		if err != nil {
			log.Printf("%s returned an error- %v", u, err)
			continue
		}
		go fetch(ctx, u, ch)
	}

	//Loop to drain channel, filter out duplicates and check for timeout.
	for range urls {
		select {
		case r := <-ch:
			for _, val := range r.Numbers {
				//Eliminate duplicates. The rationale behind eliminating duplicates on a per goroutine basis
				//rather than once we have accumulated results from all valid URLs or after 500ms has elapsed
				//is that - if no goroutine has filled the channel, rather than wasting precious processor clock
				//on waiting, we may as well remove duplicates.
				if _, ok := visited[val]; !ok {
					accumulator = append(accumulator, val)
					visited[val] = struct{}{}
				}
			}
			//After 450ms have elapsed, the context is finished. Done returns a closed channel that signals that
			//the context was cancelled, which in our case that is a timeout.
		case <-ctx.Done():
			log.Println(ctx.Err())
			//Sort and return as soon as the context is finished rather than waiting for other goroutines to be cancelled.
			sort.Ints(accumulator)
			return accumulator
		}
	}
	//Sort and return if all URLs respond within 450ms.
	sort.Ints(accumulator)
	return accumulator
}

func fetch(ctx context.Context, u string, c chan<- result) {
	var number result
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		log.Printf("%s returned an error while creating a request- %v", u, err)
		return
	}
	//Perform a request with a context to enable cancellation propagation after 450ms has elapsed.
	//As soon 450ms is elapsed the parent conext signals all the goroutines to abandon their work and return.
	req = req.WithContext(ctx)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("%s returned an error while performing a request  - %v", u, err)
		return
	}
	//If not 200 log the error.
	if res.StatusCode > http.StatusOK {
		log.Printf("%s server returned an error - %v", u, res.Status)
		return
	}
	//Close response body as soon as function returns to prevent resource lekage.
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(&number); err != nil {
		log.Printf("%s decoding error - %v", u, err)
		return
	}
	c <- number
}

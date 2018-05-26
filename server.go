package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"sort"
	"time"
)

const (
	//Represents the context timeout. Can be tuned depending on requirements and benchmarks.
	timeout        = 5000000
	endpoint       = "/numbers"
	maxConnections = 200
)

func foo() []string {
	query := []string{"http://127.0.0.1:8090/fibo", "http://127.0.0.1:8090/rand", "http://127.0.0.1:8090/odd", "http://127.0.0.1:8090/primes"}
	var res []string
	for i := 0; i < 10000000; i++ {
		res = append(res, query[rand.Intn(len(query))])
	}
	return res
}

//Type which represents the response of the given URLs as well as our response
type result struct {
	Numbers []int `json:"numbers"`
}

type payload struct {
	res chan result
	err chan error
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
	//u := r.URL
	//q := u.Query()
	//Get all the query parameters with key "u".
	params := foo()
	log.Printf("Length of urls is %d\n", len(params))
	//If there are no "u" query parameters, simply return an empty array.
	if len(params) == 0 {
		json.NewEncoder(w).Encode(result{Numbers: []int{}})
	} else {
		t := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConnsPerHost:   maxConnections,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
		ch := make(chan result, len(params))
		er := make(chan error, len(params))
		p := payload{res: ch, err: er}
		go validateAndFetch(ctx, t, params, &p)
		json.NewEncoder(w).Encode(result{Numbers: consume(ctx, &p)})
	}
}

func validateAndFetch(ctx context.Context, t *http.Transport, urls []string, p *payload) {
	c := make(chan string)
	// Spin up workers
	for i := 0; i < maxConnections; i++ {
		go doWork(ctx, t, c, p)
	}
	//Check if all URLs in the request are valid and if so spawn a goroutine to fetch data.
	for _, u := range urls {
		_, err := url.Parse(u)
		if err != nil {
			log.Printf("%s returned an error- %v", u, err)
			continue
		}
		c <- u
	}
	close(c)
}

func doWork(ctx context.Context, t *http.Transport, u chan string, p *payload) {
	for {
		url := <-u
		if url == "" {
			return
		}
		fetch(ctx, t, url, p)
	}
}

func fetch(ctx context.Context, t *http.Transport, u string, p *payload) {
	var number result
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		p.err <- fmt.Errorf("%s returned an error while creating a request- %v", u, err)
		return
	}
	//Perform a request with a context to enable cancellation propagation after 450ms has elapsed.
	//As soon 450ms is elapsed the parent conext signals all the goroutines to abandon their work and return.
	req = req.WithContext(ctx)
	res, err := t.RoundTrip(req)
	if err != nil {
		p.err <- fmt.Errorf("%s returned an error while performing a request  - %v", u, err)
		return
	}
	//Close response body as soon as function returns to prevent resource lekage.
	//https://golang.org/pkg/net/http/#Response
	defer res.Body.Close()
	//If not 200 log the error.
	if res.StatusCode != http.StatusOK {
		p.err <- fmt.Errorf("%s server returned an error - %v", u, res.Status)
		return
	}
	if err := json.NewDecoder(res.Body).Decode(&number); err != nil {
		p.err <- fmt.Errorf("%s decoding error - %v", u, err)
		return
	}
	fmt.Println("success")
	p.res <- number
}

func consume(ctx context.Context, p *payload) []int {
	accumulator := make([]int, 0)
	visited := make(map[int]struct{})
	for i := 0; i < 10000000; i++ {
		select {
		case res := <-p.res:
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
		case err := <-p.err:
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

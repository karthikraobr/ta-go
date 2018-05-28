package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"sort"
	"time"
)

const (
	endpoint = "/numbers"
	// The below 3 values should reside as environment variables for flexibility
	// Max number of simultaneous workers
	maxConnections = 200
	// Timeout for requests. This is high in case the result of each URL contains millions of digits
	individualTimeout = 50000
	// Timeout for context. This is high in case we have to process large number of URLs
	timeout = 50000
)

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

func numbersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("403 - Method not supported!"))
		return
	}
	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, timeout*time.Millisecond)
	defer cancel()
	u := r.URL
	q := u.Query()
	params := q["u"]
	if len(params) == 0 {
		json.NewEncoder(w).Encode(result{Numbers: []int{}})
	} else {
		// Create the http transport for reuse
		t := &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: maxConnections,
			// Timeout for individual requests
			ResponseHeaderTimeout: individualTimeout * time.Millisecond,
		}
		res := make(chan result, maxConnections)
		err := make(chan error, maxConnections)
		p := payload{res: res, err: err}
		// Spawn go routines for worker to consume
		go fetchAll(ctx, t, params, &p)
		// Consumer to consume from channels
		json.NewEncoder(w).Encode(result{Numbers: consume(ctx, len(params), &p)})
	}
}

// Spawns worker goroutines and generate work
func fetchAll(ctx context.Context, t *http.Transport, urls []string, p *payload) {
	c := make(chan string)
	// Spin up workers. Only 200 workers will be concurrently fetching from URLs.
	// This will ensure we do not run out of sockets or hit file descriptor limits
	for i := 0; i < maxConnections; i++ {
		go doWork(ctx, t, c, p)
	}
	// Queue up work by putting URLs in a queue. The doWork goroutine will consume this channel.
	for _, u := range urls {
		c <- u
	}
	// Closing channel to indicate to doWork that we have processed all URLs
	close(c)
}

func doWork(ctx context.Context, t *http.Transport, u chan string, p *payload) {
	// Consume URLs until the channel is closed
	for {
		url, ok := <-u
		//Channel is closed signaling all URLs have been processed
		if !ok {
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
	req = req.WithContext(ctx)
	res, err := t.RoundTrip(req)
	if err != nil {
		p.err <- fmt.Errorf("%s returned an error while performing a request  - %v", u, err)
		return
	}
	// Close body so that sockets can be reused.
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		p.err <- fmt.Errorf("%s server returned an error - %v", u, res.Status)
		return
	}
	if err := json.NewDecoder(res.Body).Decode(&number); err != nil {
		p.err <- fmt.Errorf("%s decoding error - %v", u, err)
		return
	}
	//log.Println("success")
	p.res <- number
}

// Consumer to drain result and error channel. Also handles context timeouts.
func consume(ctx context.Context, count int, p *payload) []int {
	accumulator := make([]int, 0)
	visited := make(map[int]struct{})
	for i := 0; i < count; i++ {
		select {
		case res := <-p.res:
			for _, val := range res.Numbers {
				if _, ok := visited[val]; !ok {
					accumulator = append(accumulator, val)
					visited[val] = struct{}{}
				}
			}
		case err := <-p.err:
			log.Println(err)
		case <-ctx.Done():
			log.Println(ctx.Err())
			sort.Ints(accumulator)
			return accumulator
		}
	}
	sort.Ints(accumulator)
	return accumulator
}

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
	endpoint       = "/numbers"
	maxConnections = 200
)

// func foo() []string {
// 	query := []string{"http://127.0.0.1:8090/fibo", "http://127.0.0.1:8090/rand", "http://127.0.0.1:8090/odd", "http://127.0.0.1:8090/primes"}
// 	var res []string
// 	for i := 0; i < 10000000; i++ {
// 		res = append(res, query[rand.Intn(len(query))])
// 	}
// 	return res
// }

//Type which represents the response of the given URLs as well as our response
type result struct {
	Numbers []int `json:"numbers"`
}

type payload struct {
	res chan result
	err chan error
}

var (
	timeout *time.Duration
)

func main() {
	listenAddr := flag.String("http.addr", ":8000", "http listen address")
	timeout = flag.Duration("timeout", 5000000, "timeout of the request")
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

	ctx, cancel := context.WithTimeout(ctx, *timeout*time.Millisecond)
	defer cancel()
	u := r.URL
	q := u.Query()
	params := q["u"]
	log.Printf("Length of urls is %d\n", len(params))
	if len(params) == 0 {
		json.NewEncoder(w).Encode(result{Numbers: []int{}})
	} else {
		t := &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConnsPerHost: maxConnections,
		}
		ch := make(chan result, maxConnections)
		er := make(chan error, maxConnections)
		p := payload{res: ch, err: er}
		go fetchAll(ctx, t, params, &p)
		json.NewEncoder(w).Encode(result{Numbers: consume(ctx, len(params), &p)})
	}
}

func fetchAll(ctx context.Context, t *http.Transport, urls []string, p *payload) {
	c := make(chan string)
	// Spin up workers
	for i := 0; i < maxConnections; i++ {
		go doWork(ctx, t, c, p)
	}
	for _, u := range urls {
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

func consume(ctx context.Context, count int, p *payload) []int {
	accumulator := make([]int, 0)
	visited := make(map[int]struct{})
	for i := 0; i < count; i++ {
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

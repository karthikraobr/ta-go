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
	endpoint          = "/numbers"
	maxConnections    = 200
	individualTimeout = 500
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
	timeout = flag.Duration("timeout", 50000, "the total timeout for all the request")
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
	fmt.Printf("Length of urls is %d\n", len(params))
	if len(params) == 0 {
		json.NewEncoder(w).Encode(result{Numbers: []int{}})
	} else {
		t := &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConnsPerHost:   maxConnections,
			ResponseHeaderTimeout: individualTimeout * time.Millisecond,
		}
		res := make(chan result, maxConnections)
		err := make(chan error, maxConnections)
		p := payload{res: res, err: err}
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
	req = req.WithContext(ctx)
	res, err := t.RoundTrip(req)
	if err != nil {
		p.err <- fmt.Errorf("%s returned an error while performing a request  - %v", u, err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		p.err <- fmt.Errorf("%s server returned an error - %v", u, res.Status)
		return
	}
	if err := json.NewDecoder(res.Body).Decode(&number); err != nil {
		p.err <- fmt.Errorf("%s decoding error - %v", u, err)
		return
	}
	log.Println("success")
	p.res <- number
}

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

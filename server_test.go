package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	localhost      = "localhost:8000"
	invalidRequest = "InvalidRequest"
	invalidURL     = "InvalidURL"
	randomURL      = "RandomURL"
	forbiddenTest  = "403Test"
)

func Test_numberHandler(t *testing.T) {
	actual := []int{1, 1, 2, 3, 5, 8, 13, 21}
	expected := []int{1, 2, 3, 5, 8, 13, 21}
	tt := []struct {
		name     string
		handler  func(http.ResponseWriter, *http.Request)
		expected result
	}{
		{name: "Simple", handler: simpleHandler(actual), expected: result{Numbers: expected}},
		{name: "NoParam", handler: nil, expected: result{Numbers: []int{}}},
		{name: "InvalidURL", handler: nil, expected: result{Numbers: []int{}}},
		{name: "InvalidRequest", handler: nil, expected: result{Numbers: []int{}}},
		{name: "RandomURL", handler: nil, expected: result{Numbers: []int{}}},
		{name: "SimpleError", handler: errHandler(), expected: result{Numbers: []int{}}},
		{name: "SimpleTimeOut", handler: timeOutHandler(actual), expected: result{Numbers: []int{}}},
		{name: "JustInTime", handler: justInTimeHandler(actual), expected: result{Numbers: expected}},
		{name: "ErrorAfterTime", handler: errAfterTimeHandler(), expected: result{Numbers: []int{}}},
		{name: forbiddenTest, handler: nil, expected: result{Numbers: []int{}}},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			var err error
			if tc.handler != nil {
				ts := httptest.NewServer(http.HandlerFunc(tc.handler))
				defer ts.Close()
				req, err = http.NewRequest(http.MethodGet, localhost+"?u="+ts.URL, nil)
				if err != nil {
					t.Fatalf("could not create request: %v", err)
				}
			} else {
				if tc.name == invalidRequest {
					req, err = http.NewRequest(http.MethodGet, localhost+"?u=hello", nil)
					if err != nil {
						t.Fatalf("could not create request: %v", err)
					}
				} else if tc.name == invalidURL {
					req, err = http.NewRequest(http.MethodGet, localhost+"?u=http://\\www.google.com//", nil)
					if err != nil {
						t.Fatalf("could not create request: %v", err)
					}
				} else if tc.name == randomURL {
					req, err = http.NewRequest(http.MethodGet, localhost+"?u=http://www.google.com", nil)
					if err != nil {
						t.Fatalf("could not create request: %v", err)
					}
				} else if tc.name == forbiddenTest {
					req, err = http.NewRequest(http.MethodPost, localhost+"?u=http://www.google.com", nil)
					if err != nil {
						t.Fatalf("could not create request: %v", err)
					}
				} else {
					req, err = http.NewRequest(http.MethodGet, localhost, nil)
					if err != nil {
						t.Fatalf("could not create request: %v", err)
					}
				}
			}
			rec := httptest.NewRecorder()
			numbersHandler(rec, req)
			res := rec.Result()
			defer res.Body.Close()
			if tc.name != forbiddenTest {
				if res.StatusCode != http.StatusOK {
					t.Fatalf("expected status OK; got %v", res.Status)
				}
				var num result
				if err = json.NewDecoder(res.Body).Decode(&num); err != nil {
					t.Fatalf("could not decode response: %v", err)
				}
				if !num.equals(tc.expected) {
					t.Errorf("expected %v but got %v", tc.expected, num)
				}
			} else if res.StatusCode != http.StatusForbidden {
				t.Fatalf("expected status forbidden; got %v", res.Status)
			}
		})
	}
}

func simpleHandler(numbers []int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"numbers": numbers})
	}
}

func errHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}
}

func timeOutHandler(numbers []int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(451) * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{"numbers": numbers})
	}
}

func justInTimeHandler(numbers []int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(440) * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{"numbers": numbers})
	}
}

func errAfterTimeHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(300) * time.Millisecond)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}
}
func (a *result) equals(b result) bool {
	if a.Numbers == nil && b.Numbers == nil {
		return true
	}

	if a.Numbers == nil || b.Numbers == nil {
		return false
	}

	if len(a.Numbers) != len(b.Numbers) {
		return false
	}

	for i := range a.Numbers {
		if a.Numbers[i] != b.Numbers[i] {
			return false
		}
	}
	return true
}

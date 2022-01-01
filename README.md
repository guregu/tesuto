## tesuto [![GoDoc](https://godoc.org/github.com/guregu/tesuto?status.svg)](https://godoc.org/github.com/guregu/tesuto)
`import "github.com/guregu/tesuto"`

tesuto is a little library for testing against HTTP services. It glues together the standard library's [net/http/httptest](https://pkg.go.dev/net/http/httptest) package and [Google's go-cmp](https://github.com/google/go-cmp) with [functional options](https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html) eliminating much tedious boilerplate.

### Example

```go
func TestGreeting(t *testing.T) {
	type Response struct {
		Msg  string    `json:"msg"`
		Time time.Time `json:"time"`
	}

	mux := http.NewServeMux()
	// example API we will test against
	mux.HandleFunc("/greet", func(w http.ResponseWriter, r *http.Request) {
		// return 405 Method Not Allowed if not a POST request
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// return 400 Bad Request if name is missing
		name := r.FormValue("name")
		if name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// return 200 OK and response JSON
		w.Header().Set("Content-Type", "application/json")
		resp := Response{
			Msg:  fmt.Sprintf("hello %s", name),
			Time: time.Now(),
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			panic(err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	suite := tesuto.New(server)

	t.Run("greet: happy path", suite.Test(
		"POST",
		"/greet",
		tesuto.WithFormInput(url.Values{
			"name": {"greg"},
		}),
		tesuto.ExpectStatusCode(http.StatusOK),
		tesuto.ExpectHeader("Content-Type", "application/json"),
		tesuto.ExpectJSONResponse(Response{
			Msg:  "hello greg",
			Time: time.Now(),
		}, tesuto.EquateApproxTime(10*time.Second)), // consider times within 10 seconds to be equal
	))

	t.Run("greet: missing name param", suite.Test(
		"POST",
		"/greet",
		tesuto.ExpectStatusCode(http.StatusBadRequest),
	))

	t.Run("greet: wrong method", suite.Test(
		"GET",
		"/greet",
		tesuto.ExpectStatusCode(http.StatusMethodNotAllowed),
	))
}

```

Handy error messages make it easy to see where tests fail:

```
--- FAIL: TestPost (0.00s)
    --- FAIL: TestPost/greet:_happy_path (0.00s)
        tesuto.go:83: output:
             {"msg":"hello greg","time":"2022-01-01T19:23:44.0272386+09:00"}

        tesuto.go:114: [POST /greet] output mismatch (-want +got):
              tesuto_test.Response{
            -   Msg:  "hello bob",
            +   Msg:  "hello greg",
                Time: s"2022-01-01 19:23:44.0272386 +0900 JST",
              }
```
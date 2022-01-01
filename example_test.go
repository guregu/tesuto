package tesuto_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/guregu/tesuto"
)

func TestPlainText(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "hello world")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	suite := tesuto.HTTP{
		Server: server,
	}

	t.Run("index says hello world", suite.Test(
		"GET",
		"/",
		tesuto.ExpectStatusCode(http.StatusOK),
		tesuto.ExpectHeader("Content-Type", "text/plain"),
		tesuto.ExpectRawResponse([]byte("hello world")),
	))
}

func TestPost(t *testing.T) {
	type Response struct {
		Msg  string    `json:"msg"`
		Time time.Time `json:"time"`
	}

	mux := http.NewServeMux()
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

	suite := tesuto.HTTP{
		Server: server,
	}

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

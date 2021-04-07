package request

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fastbill/go-httperrors/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Input struct {
	RequestValue string `json:"requestValue"`
}

type Output struct {
	ResponseValue string `json:"responseValue"`
}

func TestGetClient(t *testing.T) {
	t.Run("correct timeout setting", func(t *testing.T) {
		client := GetClient()
		assert.Equal(t, 30*time.Second, client.Timeout)
	})

	t.Run("returns a new client", func(t *testing.T) {
		client1 := GetClient()
		client2 := GetClient()
		assert.True(t, client1 != client2)
	})

	t.Run("does not follow redirects like the standard client", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/////")
			w.WriteHeader(http.StatusSeeOther)
		}))
		stdClient := http.Client{}
		r, err := stdClient.Get(ts.URL)
		assert.Error(t, err)
		assert.NoError(t, r.Body.Close())

		client := GetClient()
		res, err := client.Get(ts.URL)
		assert.NoError(t, err)
		assert.Equal(t, "/////", res.Header.Get("Location"))
		assert.NoError(t, res.Body.Close())
	})
}
func TestGetCachedClient(t *testing.T) {
	t.Run("returns the same client", func(t *testing.T) {
		client1 := getCachedClient()
		client2 := getCachedClient()
		assert.True(t, client1 == client2)
	})
}
func TestDoSuccessful(t *testing.T) {
	t.Run("full request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := ioutil.ReadAll(r.Body)
			assert.Equal(t, `{"requestValue":"someValueIn"}`+"\n", string(body))
			assert.Equal(t, r.Method, http.MethodPost)
			w.WriteHeader(http.StatusCreated)
			_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
			assert.NoError(t, err)
		}))
		defer ts.Close()

		params := Params{
			URL:                  ts.URL,
			Method:               http.MethodPost,
			Body:                 Input{RequestValue: "someValueIn"},
			ExpectedResponseCode: http.StatusCreated,
		}

		result := &Output{}
		err := Do(params, result)
		assert.NoError(t, err)
		assert.Equal(t, "someValueOut", result.ResponseValue)
	})

	t.Run("with query", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.URL.RawQuery, `beenhere=before&testKey=testValue&%C3%B6%C3%A4=%25%26%2F`)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL + "?beenhere=before",
			Method: http.MethodPost,
			Query: map[string]string{
				"testKey": "testValue",
				"öä":      "%&/",
			},
		}

		err := Do(params, nil)
		assert.NoError(t, err)
	})

	t.Run("no request body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, http.MethodPost)
			_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
			assert.NoError(t, err)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL,
			Method: http.MethodPost,
		}

		result := &Output{}
		err := Do(params, result)
		assert.NoError(t, err)
		assert.Equal(t, "someValueOut", result.ResponseValue)
	})

	t.Run("no response body and no request body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, r.Method, http.MethodGet)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL,
			Method: http.MethodGet,
			Body:   nil,
		}

		err := Do(params, nil)
		assert.NoError(t, err)
	})

	t.Run("with timeout", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		params := Params{
			URL:     ts.URL,
			Method:  http.MethodGet,
			Body:    nil,
			Timeout: 1 * time.Millisecond,
		}

		err := Do(params, nil)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "Client.Timeout exceeded")
		}
	})

	t.Run("body is reader", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := ioutil.ReadAll(r.Body)
			assert.Equal(t, `{"requestValue":"someValueIn"}`, string(body))
			assert.Equal(t, r.Method, http.MethodPost)
			_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
			assert.NoError(t, err)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL,
			Method: http.MethodPost,
			Body:   strings.NewReader(`{"requestValue":"someValueIn"}`),
		}

		result := &Output{}
		err := Do(params, result)
		assert.NoError(t, err)
		assert.Equal(t, "someValueOut", result.ResponseValue)
	})
}

func TestDoHeaders(t *testing.T) {
	t.Run("custom and default headers", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "testHeaderValue", r.Header.Get("Test-Header"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		}))
		defer ts.Close()

		params := Params{
			URL:     ts.URL,
			Headers: map[string]string{"Test-Header": "testHeaderValue"},
		}

		err := Do(params, nil)
		assert.NoError(t, err)
	})

	t.Run("overwrite default headers", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "image/*", r.Header.Get("Content-Type"))
		}))
		defer ts.Close()

		params := Params{
			URL:     ts.URL,
			Headers: map[string]string{"Content-Type": "image/*"},
		}

		err := Do(params, nil)
		assert.NoError(t, err)
	})
}

func TestDoHTTPErrors(t *testing.T) {
	t.Run("non 2xx response without body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL,
			Method: http.MethodGet,
		}

		err := Do(params, nil)
		assert.Error(t, err)
		assert.IsType(t, &httperrors.HTTPError{}, err)
		httpError := err.(*httperrors.HTTPError)
		assert.Equal(t, 500, httpError.StatusCode)
		assert.Equal(t, "Internal Server Error", httpError.Message)
	})

	t.Run("non 2xx response with body", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, err := w.Write([]byte("some error message"))
			assert.NoError(t, err)
		}))
		defer ts.Close()

		params := Params{
			URL:    ts.URL,
			Method: http.MethodGet,
		}

		err := Do(params, nil)
		assert.Error(t, err)
		assert.IsType(t, &httperrors.HTTPError{}, err)
		httpError := err.(*httperrors.HTTPError)
		assert.Equal(t, 400, httpError.StatusCode)
		assert.Equal(t, "some error message", httpError.Message)
	})
}

func TestDoOtherErrors(t *testing.T) {
	t.Run("request body cannot be parsed", func(t *testing.T) {
		params := Params{Body: make(chan int)}
		err := Do(params, nil)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "failed to parse request body to json")
		}
	})

	t.Run("request cannot be created", func(t *testing.T) {
		params := Params{Method: "some method"}
		err := Do(params, nil)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "failed to create request")
		}
	})

	t.Run("request cannot be sent", func(t *testing.T) {
		params := Params{URL: "http://"}
		err := Do(params, nil)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "failed to send request")
		}
	})

	t.Run("wrong response code", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		defer ts.Close()

		params := Params{
			URL:                  ts.URL,
			ExpectedResponseCode: http.StatusOK,
		}

		err := Do(params, nil)
		assert.Error(t, err)
		assert.EqualError(t, err, "expected response code 200 but got 201")
	})
}

func TestDoWithStringResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		response := `{"responseValue":"someValueOut"}`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(response))
			assert.NoError(t, err)
		}))
		defer ts.Close()

		params := Params{
			URL: ts.URL,
		}

		result, err := DoWithStringResponse(params)
		assert.NoError(t, err)
		assert.Equal(t, response, result)
	})
}

func TestDoWithCustomClient(t *testing.T) {
	customClient := GetClient()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	customClient.Jar = jar

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "foo=bar")
		w.WriteHeader(http.StatusOK)
	}))

	params := Params{
		URL: ts.URL,
	}
	err = DoWithCustomClient(params, nil, customClient)
	require.NoError(t, err)

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	require.Equal(t, "foo=bar", jar.Cookies(u)[0].String())
}

func TestGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.Method, http.MethodGet)
		_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
		assert.NoError(t, err)
	}))
	defer ts.Close()

	result := &Output{}
	err := Get(ts.URL, result)
	assert.NoError(t, err)
	assert.Equal(t, "someValueOut", result.ResponseValue)
}

func TestPost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		assert.Equal(t, `{"requestValue":"someValueIn"}`+"\n", string(body))
		assert.Equal(t, r.Method, http.MethodPost)
		_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
		assert.NoError(t, err)
	}))
	defer ts.Close()

	result := &Output{}
	err := Post(ts.URL, Input{RequestValue: "someValueIn"}, result)
	assert.NoError(t, err)
	assert.Equal(t, "someValueOut", result.ResponseValue)
}

func ExampleDo() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		fmt.Println(string(body))
		_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
		if err != nil {
			panic(err)
		}
	}))
	defer ts.Close()

	params := Params{
		URL:    ts.URL,
		Method: http.MethodPost,
		Body:   Input{RequestValue: "someValueIn"},
	}

	result := &Output{}
	err := Do(params, result)

	fmt.Println(result.ResponseValue, err)
	// Output:
	// {"requestValue":"someValueIn"}
	//
	// someValueOut <nil>
}

func ExampleDoWithStringResponse() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`{"responseValue":"someValueOut"}`))
		if err != nil {
			panic(err)
		}
	}))
	defer ts.Close()

	params := Params{
		URL:    ts.URL,
		Method: http.MethodPost,
	}

	result, err := DoWithStringResponse(params)

	fmt.Println(result, err)
	// Output:
	// {"responseValue":"someValueOut"} <nil>
}

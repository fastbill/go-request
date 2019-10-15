package request

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/fastbill/go-httperrors/v2"
	"github.com/pkg/errors"
)

// client is the global client instance.
var client *http.Client

// defaultTimeout is the timeout applied if there is none provided.
var defaultTimeout = 30 * time.Second

// getCachedClient returns the client instance or creates it if it did not exist.
// The client does not follow redirects and has a timeout of defaultTimeout.
func getCachedClient() *http.Client {
	if client == nil {
		client = GetClient()
	}

	return client
}

// GetClient returns an http client that does not follow redirects and has a timeout of defaultTimeout.
func GetClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: defaultTimeout,
	}
}

// Params holds all information necessary to set up the request instance.
type Params struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    interface{}
	Query   map[string]string
	Timeout time.Duration
}

// Do executes the request as specified in the request params
// The response body will be parsed into the provided struct
func Do(params Params, responseBody interface{}) (returnErr error) {
	req, err := createRequest(params)
	if err != nil {
		return err
	}

	var client *http.Client
	if params.Timeout != 0 {
		client = GetClient()
		client.Timeout = params.Timeout
	} else {
		client = getCachedClient()
	}

	res, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	defer func() {
		if cErr := res.Body.Close(); cErr != nil && returnErr == nil {
			returnErr = cErr
		}
	}()

	if !isSuccessCode(res.StatusCode) {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil || len(bodyBytes) == 0 {
			return httperrors.New(res.StatusCode, nil)
		}

		return httperrors.New(res.StatusCode, string(bodyBytes))
	}

	if responseBody == nil {
		return nil
	}

	return json.NewDecoder(res.Body).Decode(responseBody)
}

func createRequest(params Params) (*http.Request, error) {
	reader, err := convertToReader(params.Body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(params.Method, params.URL, reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	for key, value := range params.Headers {
		req.Header.Set(key, value)
	}

	if len(params.Query) > 0 {
		q := req.URL.Query()
		for key, value := range params.Query {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	return req, nil
}

// Get is a convience wrapper for "Do" to execute GET requests
func Get(url string, responseBody interface{}) error {
	return Do(Params{Method: "GET", URL: url}, responseBody)
}

// Post is a convience wrapper for "Do" to execute POST requests
func Post(url string, requestBody interface{}, responseBody interface{}) error {
	return Do(Params{Method: "POST", URL: url, Body: requestBody}, responseBody)
}

func isSuccessCode(statusCode int) bool {
	return 200 <= statusCode && statusCode <= 299
}

func convertToReader(body interface{}) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	reader, ok := body.(io.Reader)
	if ok {
		return reader, nil
	}

	buffer := &bytes.Buffer{}
	err := json.NewEncoder(buffer).Encode(body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse request body to json")
	}

	return buffer, nil
}

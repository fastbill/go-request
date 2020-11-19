package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/fastbill/go-httperrors/v2"
	"github.com/pkg/errors"
)

// cachedClient is the global client instance.
var cachedClient *http.Client

// defaultTimeout is the timeout applied if there is none provided.
var defaultTimeout = 30 * time.Second

// getCachedClient returns the client instance or creates it if it did not exist.
// The client does not follow redirects and has a timeout of defaultTimeout.
func getCachedClient() *http.Client {
	if cachedClient == nil {
		cachedClient = GetClient()
	}

	return cachedClient
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
	URL                  string
	Method               string
	Headers              map[string]string
	Body                 interface{}
	Query                map[string]string
	Timeout              time.Duration
	ExpectedResponseCode int
}

// Do executes the request as specified in the request params
// The response body will be parsed into the provided struct
func Do(params Params, responseBody interface{}) (returnErr error) {
	req, err := createRequest(params)
	if err != nil {
		return err
	}

	client := selectClient(params.Timeout)
	res, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	defer func() {
		if cErr := res.Body.Close(); cErr != nil && returnErr == nil {
			returnErr = cErr
		}
	}()

	err = checkResponseCode(res, params.ExpectedResponseCode)
	if err != nil {
		return err
	}

	if responseBody == nil {
		return nil
	}

	return json.NewDecoder(res.Body).Decode(responseBody)
}

// DoWithStringResponse is the same as Do but the response body is returned as string
// instead of being parsed into the provided struct.
func DoWithStringResponse(params Params) (result string, returnErr error) {
	req, err := createRequest(params)
	if err != nil {
		return "", err
	}

	client := selectClient(params.Timeout)
	res, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "failed to send request")
	}

	defer func() {
		if cErr := res.Body.Close(); cErr != nil && returnErr == nil {
			returnErr = cErr
		}
	}()

	err = checkResponseCode(res, params.ExpectedResponseCode)
	if err != nil {
		return "", err
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(bodyBytes), nil
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
	return Do(Params{Method: http.MethodGet, URL: url}, responseBody)
}

// Post is a convience wrapper for "Do" to execute POST requests
func Post(url string, requestBody interface{}, responseBody interface{}) error {
	return Do(Params{Method: http.MethodPost, URL: url, Body: requestBody}, responseBody)
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

func selectClient(timeout time.Duration) *http.Client {
	if timeout != 0 {
		client := GetClient()
		client.Timeout = timeout
		return client
	}

	return getCachedClient()
}

func checkResponseCode(res *http.Response, expectedResponseCode int) error {
	if expectedResponseCode != 0 && res.StatusCode != expectedResponseCode {
		return fmt.Errorf("expected response code %d but got %d", expectedResponseCode, res.StatusCode)
	}

	if !isSuccessCode(res.StatusCode) {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil || len(bodyBytes) == 0 {
			return httperrors.New(res.StatusCode, nil)
		}

		return httperrors.New(res.StatusCode, string(bodyBytes))
	}

	return nil
}

func isSuccessCode(statusCode int) bool {
	return 200 <= statusCode && statusCode <= 299
}

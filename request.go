package request

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/fastbill/go-httperrors/v2"
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

// Do executes the request as specified in the request params.
// The response body will be parsed into the provided struct.
// Optionally, the headers will be copied if a header map was provided.
func Do(params Params, responseBody interface{}, responseHeaderArg ...http.Header) (returnErr error) {
	req, err := createRequest(params)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := selectClient(params.Timeout)
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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

	err = populateResponseHeader(res, responseHeaderArg)
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
		return "", fmt.Errorf("failed to send request: %w", err)
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

// DoWithCustomClient is the same as Do but will make the request using the
// supplied http.Client instead of the cachedClient.
// TODO client should become the first parameter in the next major update
// so we can add the response headers at the end. They are currently not supported.
func DoWithCustomClient(params Params, responseBody interface{}, client *http.Client) (returnErr error) {
	req, err := createRequest(params)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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

func createRequest(params Params) (*http.Request, error) {
	reader, err := convertToReader(params.Body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(params.Method, params.URL, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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

// Get is a convenience wrapper for "Do" to execute GET requests
func Get(url string, responseBody interface{}) error {
	return Do(Params{Method: http.MethodGet, URL: url}, responseBody)
}

// Post is a convenience wrapper for "Do" to execute POST requests
func Post(url string, requestBody interface{}, responseBody interface{}) error {
	return Do(Params{Method: http.MethodPost, URL: url, Body: requestBody}, responseBody)
}

// ReformatMap converts map[string][]string to map[string]string by
// converting the values to comma-separated strings.
// The function can be used to make http.Header or url.Values compatible
// with the request parameters.
func ReformatMap(inputMap map[string][]string) map[string]string {
	result := map[string]string{}
	for key, values := range inputMap {
		result[key] = strings.Join(values, ",")
	}
	return result
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
		return nil, fmt.Errorf("failed to parse request body to json: %w", err)
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

func populateResponseHeader(res *http.Response, responseHeaderArg []http.Header) error {
	if len(responseHeaderArg) == 0 { // go-staticcheck says no need to check for nil separately.
		return nil
	}

	if len(responseHeaderArg) > 1 {
		return errors.New("too many arguments supplied")
	}

	responseHeader := responseHeaderArg[0]
	for key, value := range res.Header {
		responseHeader[key] = value
	}

	return nil
}

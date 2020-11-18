# Request [![Build Status](https://travis-ci.com/fastbill/go-request.svg?branch=master)](https://travis-ci.com/fastbill/go-request) [![Go Report Card](https://goreportcard.com/badge/github.com/fastbill/go-request)](https://goreportcard.com/report/github.com/fastbill/go-request) [![GoDoc](https://godoc.org/github.com/fastbill/go-request?status.svg)](https://godoc.org/github.com/fastbill/go-request)

> An opinionated but extremely easy to use HTTP request client for Go to make JSON request and retrieve the results

## Description
With this request package you just need to define the structs that correspond to the JSON request and response body. Together with the parameters like URL, method and headers you can directly execute a request with `Do`. If the request body is not of type `io.Reader` already it will be encoded as JSON. Also the response will be decoded back into the struct you provided for the result. Request and response body are optional which means they can be `nil`.  
If the request could be made but the response status code was not `2xx` an error of the type `HTTPError` from the package [httperrors](https://github.com/fastbill/go-httperrors) will be returned.

## Example
```go
import "github.com/fastbill/go-request/v2"

type Input struct {
	RequestValue string `json:"requestValue"`
}

type Output struct {
	ResponseValue string `json:"responseValue"`
}

params := request.Params{
    URL:    "https://example.com",
    Method: "POST",
    Headers: map[string]string{"my-header":"value", "another-header":"value2"},
    Body:   Input{RequestValue: "someValueIn"},
    Query: map[string]string{"key": "value"},
    Timeout: 10 * time.Second,
    ExpectedResponseCode: 201,
}

result := &Output{}
err := request.Do(params, result)
```
All parameters besides the `URL` and the `Method` are optional and can be omitted.

If you want to retrieve the response body as a string, e.g. for debugging or testing purposes, you can use `DoWithStringResponse` instead.
```go
result, err := request.DoWithStringResponse(params)
```

## Convenience wrappers
```go
err := request.Get("http://example.com", result)

err := request.Post("http://example.com", Input{RequestValue: "someValueIn"}, result)
```

## Defaults
* All `2xx` response codes are treated as success, all other codes lead to an error being returned, if you want to check for a specific response code set `ExpectedResponseCode` in the parameters
* If an HTTPError is returned it contains the response body as message if there was one
* The request package takes care of closing the response body after sending the request
* The http client does not follow redirects
* The http client timeout is set to 30 seconds, use the `Timeout` parameter in case you want to define a different timeout for one of the requests
* `Accept` and `Content-Type` request header are set to `application/json` and can be overwritten via the Headers parameter

## Streaming
The package allows the request body (`Body` property of `Params`) to be of type `io.Reader`. That way you can pass on request bodies to other services without parsing them.

## Why?
To understand why this package was created have a look at the code that would be the native equivalent of the code shown in the example above.
```go
import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type Input struct {
	RequestValue string `json:"requestValue"`
}

type Output struct {
	ResponseValue string `json:"responseValue"`
}

buf := &bytes.Buffer{}
err := json.NewEncoder(buf).Encode(&Input{RequestValue: "someValueIn"})
if err != nil {
    return err
}

req, err := http.NewRequest("POST", url, buf)
if err != nil {
    return err
}

req.Header.Set("Accept", "application/json")
req.Header.Set("Content-Type", "application/json")
req.Header.Set("my-header", "value")
req.Header.Set("another-header", "value2")

q := req.URL.Query()
q.Add("key", "value")
req.URL.RawQuery = q.Encode()

client := &http.Client{
    Timeout: 30 * time.Second,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse
    },
}

res, err := client.Do(req)
if err != nil {
    return err
}
defer func() {
    err = res.Body.Close()
    // handle err somehow
}()

result := &Output{}
err = json.NewDecoder(res.Body).Decode(result)
```
This shows the request package saves a lot of boilerplate code. instead of around 35 lines we just write the 9 lines shown in the example. That way the code is much easier to read and maintain.

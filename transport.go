package main

import (
  "net/http"
  "github.com/vadimpilyugin/debug_print_go"
  "net/http/httputil"
  "bufio"
  "bytes"
)

type Promise struct {
  Res chan<- []byte
  Err chan<- error
  Request *http.Request
}

type Requester struct {
  queue chan Promise
}

var rqstr *Requester

func init() {
  // init requester
  queue := make(chan Promise)
  rqstr = &Requester{queue: queue}
  go rqstr.Dequeue()
}


func (rqstr *Requester) Dequeue () {
  const DEFAULT_PORT = "80"
  for promise := range rqstr.queue {
    headers, err := httputil.DumpRequest(promise.Request, false)
    if err != nil {
      printer.Fatal(err)
    }
    port := promise.Request.URL.Port()
    if port == "" {
      port = DEFAULT_PORT
    }
    // do a real request
    buf, err := HTTPRequest(
      promise.Request.URL.Hostname(),
      port,
      headers, 
      promise.Request.Body,
    )
    if err != nil {
      promise.Res <- nil
      promise.Err <- err
    } else {
      promise.Res <- buf
      promise.Err <- nil
    }
  }
}

func fixResponse(resp []byte) []byte {
  const HTTP_VER = "HTTP"
  if len(resp) >= len(HTTP_VER) {
    for i,b := range []byte(HTTP_VER) {
      resp[i] = b
    }
  }
  return resp
}

func (rqstr *Requester) RoundTrip (r *http.Request) (*http.Response, error) {
  // send and receive requests
  ch := make(chan []byte)
  e := make(chan error)
  rqstr.queue <- Promise{Res: ch, Request: r, Err: e}
  res := <-ch
  err := <-e
  if err != nil {
    return nil, err
  } else {
    // success
    res = fixResponse(res)
    bufReader := bufio.NewReader(bytes.NewReader(res))
    printer.Debug(res, "Received response")
    response, err := http.ReadResponse(bufReader, r)
    if err != nil {
      printer.Error("Error when parsing response!")
      return nil, err
    }
    return response, nil
  }
}

// func makeRequest(url string, port string) {
//   c := Client{Transport: 
// }

func main() {
  bufReader := bufio.NewReader(
    bytes.NewReader(
      fixResponse([]byte("http/1.1 200 ok\r\ncontent-length: 13\r\n\r\nHello, world!")),
    ),
  )
  _, err := http.ReadResponse(bufReader, nil)
  if err != nil {
    printer.Fatal(err, "Cannot read response from bytes")
  }

  client := &http.Client{
    Transport: rqstr,
  }
  req, err := http.NewRequest("GET", "http://openplatform.website:8080", nil)
  // req, err := http.NewRequest("GET", "http://google.com", nil)
  if err != nil {
    printer.Fatal(err)
  }
  req.Header.Add("If-None-Match", `W/"wyzzy"`)

  foo := make(chan bool)
  go func() {
    resp, err := client.Do(req)
    if err != nil {
      printer.Fatal(err)
    }
    printer.Debug(resp)
    foo <- true
  }()
  _ = <-foo
}
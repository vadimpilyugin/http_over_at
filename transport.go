package http_over_at

import (
  "net/http"
  "github.com/vadimpilyugin/debug_print_go"
  "net/http/httputil"
  "bufio"
  "bytes"
)

type promise struct {
  Res chan<- []byte
  Err chan<- error
  Request *http.Request
}

type Requester struct {
  queue chan promise
}

var Rqstr *Requester


func init() {
  // init requester
  queue := make(chan promise)
  Rqstr = &Requester{queue: queue}
  go Rqstr.dequeue()
}


func (Rqstr *Requester) dequeue () {
  const DEFAULT_PORT = "80"
  for promise := range Rqstr.queue {
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

func (Rqstr *Requester) RoundTrip (r *http.Request) (*http.Response, error) {
  // send and receive requests
  ch := make(chan []byte)
  e := make(chan error)
  Rqstr.queue <- promise{Res: ch, Request: r, Err: e}
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


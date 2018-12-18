package http_over_at

import (
  "github.com/vadimpilyugin/debug_print_go"
  "github.com/vadimpilyugin/at_commands"
  "io"
  "fmt"
  "errors"
)

const (
  OK = "OK"
  CTRL_Z = "\x1a"
  BUFSIZE = 4096
  CHTTPACT = "+CHTTPACT"
  REQUEST = "REQUEST"
  CME = "CME"
  ERROR = "ERROR"
  DATA = "DATA"
  CR = "\r"
  LF = "\n"
)

func HTTPRequest(host string, port string, headers []byte, body io.Reader) ([]byte, error) {
  printer.Debug("New request", "HTTPRequest", map[string]string{
    "Host":host,
    "Port":port,
    "Headers":string(headers),
    "Body": "<<io.Reader>>",
  })

  at_commands.Commands <- []byte("AT+CGSOCKCONT=1,\"IP\",\"internet\"\r")
  for {
    resp := <-at_commands.Responses
    if resp.Name == OK || resp.Name == ERROR {
      break
    }
    printer.Debug("Skipping until OK or ERROR", "CGSOCKCONT")
  }

  at_commands.Commands <- []byte(fmt.Sprintf("AT+CHTTPACT=\"%s\",%s\r", host, port))
  err := receiveCHTTPACT()
  if err != nil {
    return nil, err
  }

  printer.Debug("received CHTTPACT!", "HTTPRequest")
  writeRequest(headers, body)

  buf, err := receiveCHTTPACTResponse()
  if err != nil {
    return nil, err
  }
  return buf, nil
}

func writeRequest(headers []byte, body io.Reader) {
  at_commands.Commands <- headers
  if body == nil {
    printer.Debug("No body")
    at_commands.Commands <- []byte(CTRL_Z)
  } else {
    printer.Debug("Sending body")
    buf := make([]byte, BUFSIZE)
    for {
      n, err := body.Read(buf)
      if n == 0 && err == io.EOF {
        // end of body
        printer.Debug("End of request body!", "writeRequest")
        at_commands.Commands <- []byte(CTRL_Z)
        break
      } else if err != nil && err != io.EOF {
        printer.Fatal(err, "writeRequest: cannot read from body")
      }
      at_commands.Commands <- makeCopy(buf, n)
    }
  }
}

const (
  ERR_220 = "Unknown error for HTTP"
  ERR_221 = "HTTP task is busy"
  ERR_222 = "Failed to resolve server address"
  ERR_223 = "HTTP timeout"
  ERR_224 = "Failed to transfer data"
  ERR_225 = "Memory error"
  ERR_226 = "Invalid parameter"
  ERR_227 = "Network error"
  ERR_UNKNOWN = "errCHTTPACT: unknown error"
  ERR_CME = "errCHTTPACT: CME ERROR"
)

func errCHTTPACT(s string) error {
  switch s {
    case "220": return errors.New(ERR_220)
    case "221": return errors.New(ERR_221)
    case "222": return errors.New(ERR_222)
    case "223": return errors.New(ERR_223)
    case "224": return errors.New(ERR_224)
    case "225": return errors.New(ERR_225)
    case "226": return errors.New(ERR_226)
    case "227": return errors.New(ERR_227)
    case "CME": return errors.New(ERR_CME)
    default: return errors.New(ERR_UNKNOWN)
  }
}

// TODO: create central manager to fan out at_commands.Responses
func receiveCHTTPACT() error {
  for {
    resp := <-at_commands.Responses
    if resp.Name == CHTTPACT {
      printer.Debug(resp, "receiveCHTTPACT: received response")
      if resp.Params[0] == REQUEST {
        return nil
      } else {
        err := errCHTTPACT(resp.Params[0])
        printer.Error(err, "Received error")
        /* Don't skip to ERROR, because modem does not send it */
        // for ; resp.Name != ERROR; resp = <-at_commands.Responses {
        //  // FIXME: we might skip some useful info
        //  printer.Debug("Skipping until ERROR is found")
        // }
        return err
      }
    } else if resp.Name == CME && resp.Status == ERROR {
      return errCHTTPACT(CME)
    // } else if resp.Name == CONNECT {
    //  printer.Note("Got CONNECT, trying to fix...")
    //  at_commands.Commands <- []byte(CTRL_Z)
    } else {
      printer.Debug("Skipping until HTTPACT is found")
    }
  }
}

func makeCopy(buf []byte, n int) []byte {
  sendBuf := make([]byte, n)
  copy(sendBuf, buf)
  return sendBuf
}

func receiveCHTTPACTResponse() ([]byte, error) {
  buf := make([]byte, 0, BUFSIZE)
  for {
    resp := <-at_commands.Responses
    if resp.Name == CHTTPACT {
      printer.Debug(resp, "receiveCHTTPACTResponse: received response")
      if resp.Params[0] == DATA {
        printer.Debug("received data package", "receiveCHTTPACTResponse")
        buf = append(buf, resp.Data...)
      } else {
        if resp.Params[0] == "0" {
          printer.Debug("Finished receiving data!")
          return buf, nil
        } else {
          err := errCHTTPACT(resp.Params[0])
          printer.Error(err, "Received error")
          /* Don't skip to ERROR, because modem does not send it */
          // for ; resp.Name != ERROR; resp = <-at_commands.Responses {
          //  // FIXME: we might skip some useful info
          //  printer.Debug("Skipping until ERROR is found")
          // }
          return nil, err
        }
      }
    } else {
      printer.Debug("Skipping until HTTPACT is found")
    }
  }
}

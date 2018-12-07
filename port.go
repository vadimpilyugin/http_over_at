package http_over_at

import (
	"github.com/jacobsa/go-serial/serial"
	"github.com/vadimpilyugin/debug_print_go"
	"io"
	"os"
	// "time"
	"fmt"
	// "bufio"
	// "strings"
	"strconv"
	"unicode"
	"errors"
)

const (
	BUFSIZE = 4096
	SMBUF = 32
	NO_COMMAND = iota
	FIRST_R
	COMMAND_RESPONSE
	RESPONSE
	NOT_A_COMMAND
	FIN_R
	COMMA_SEP_SPACE
	COMMA_SEP
	READ_DATA
	CTRL_Z = "\x1a"
	CHTTPACT = "+CHTTPACT"
	REQUEST = "REQUEST"
	ERROR = "ERROR"
	CME = "CME"
	DATA = "DATA"
	OK = "OK"
	CONNECT = "CONNECT"
	BPS_115200 = "115200"
)

type CommandResponse struct {
	Name string
	Params []string
	Data []byte
	Status string
}

var commands chan []byte
var responses chan *CommandResponse

func openPort() io.ReadWriteCloser {
	// Set up options.
	// 115200bps, 8 bit data, no parity, 1 bit stop, no data stream control.
	options := serial.OpenOptions{
		PortName:        "/dev/ttyUSB2",
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: 4,
	}

	// Open the port.
	port, err := serial.Open(options)
	if err != nil {
		printer.Fatal(err, "serial.Open")
	}

	// Make sure to close it later.
	return port
}

// TODO: use bufio.Reader instead
func portReader(ch chan<- []byte, port io.ReadWriteCloser) {
	buf := make([]byte, BUFSIZE)
	for {
		n, err := port.Read(buf)
		if err != nil {
			if err == io.EOF {
				printer.Error(err, "No more data from port")
			} else {
				printer.Error(err, "An error occured")
			}
			close(ch)
			return
		}
		// printer.Debug(n, "Got bytes")
		sendBuf := make([]byte, n)
		copy(sendBuf, buf)
		printer.Debug(string(sendBuf), "Got data")
		ch <- sendBuf
	}
}

func fakePortReader(ch chan<- []byte) {
	file, err := os.Open("req12")
	if err != nil {
		printer.Fatal(err, "Could not open log file")
	}
	defer file.Close()
	buf := make([]byte, SMBUF)
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			printer.Note("File ended")
			close(ch)
			return
		}
		// printer.Debug(n, "Got bytes")
		sendBuf := make([]byte, n)
		copy(sendBuf, buf)
		ch <- sendBuf
		// time.Sleep(time.Millisecond * 50)
	}
}

func fakePortWriter(commands chan []byte) {
	for command := range commands {
		printer.Debug(command, "fakePortWriter: new command")
	}
}

// FIXME: single Ctrl-Z at the end of responded data, Ctrl-Z escaping
func commandParser(ch <-chan []byte, responses chan<- *CommandResponse) {
	state := NO_COMMAND
	var command []byte
	var status []byte
	var param []byte
	var cmdresp *CommandResponse
	dataRead := 0
	dataAvail := 0
	for buf := range ch {
		for _, c := range buf {
			if state != READ_DATA {
				sym := ""
				switch c {
					case ' ': sym = "'SPACE'"
					case '\r': sym = "'CR'"
					case '\n': sym = "'LF'"
					default: sym = fmt.Sprintf("%c", c)
				}
				printer.Note(sym, "c")
			}
			switch state {
			case NO_COMMAND:
				printer.Debug("NO_COMMAND","State")
				if c == '\r' {
					state = FIRST_R
					printer.Debug("FIRST_R", "Change state")
				}
			case FIRST_R:
				printer.Debug("FIRST_R","State")
				if c == '\n' {
					cmdresp = &CommandResponse{}
					state = RESPONSE
					printer.Debug("RESPONSE", "Change state")
				} else {
					state = NO_COMMAND
					printer.Debug("NO_COMMAND", "Change state")
				}
			case RESPONSE:
				printer.Debug("RESPONSE","State")
				if unicode.IsSpace(int32(c)) {
					printer.Fatal("Command starts with space!")
				}
				command = make([]byte, 0, SMBUF)
				command = append(command, c)
				state = COMMAND_RESPONSE
				printer.Debug("COMMAND_RESPONSE", "Change state")
				// if c == '+' {
				// } else {
				// 	status = command
				// 	command = command[:0]
				// 	state = NOT_A_COMMAND
				// 	printer.Debug("NOT_A_COMMAND", "Change state")
				// }
			case NOT_A_COMMAND:
				printer.Debug("NOT_A_COMMAND","State")
				if c != '\r' {
					status = append(status, c)
				} else {
					statusStr := string(status)
					if statusStr != OK && statusStr != ERROR && statusStr != BPS_115200 {
						printer.Fatal(statusStr, "No such status")
					}
					cmdresp.Status = statusStr
					state = FIN_R
					printer.Debug("FIN_R", "Change state")
				}
			case COMMAND_RESPONSE:
				printer.Debug("COMMAND_RESPONSE","State")
				if c == ' ' {
					cmdresp.Name = string(command)
					status = make([]byte, 0)
					printer.Debug("NOT_A_COMMAND", "Change state")
					state = NOT_A_COMMAND
				} else if c == '\r' {
					cmdresp.Name = string(command)
					printer.Debug("FIN_R", "Change state")
					state = FIN_R
				} else if c == ':' {
					cmdresp.Name = string(command)
					state = COMMA_SEP_SPACE
					printer.Debug("COMMA_SEP_SPACE", "Change state")
					param = make([]byte, 0)
				} else {
					command = append(command, c)
				}
			case COMMA_SEP_SPACE:
				printer.Debug("COMMA_SEP_SPACE","State")
				if c == ' ' {
					state = COMMA_SEP
					printer.Debug("COMMA_SEP", "Change state")
				} else {
					printer.Fatal("Comma-separated values without space character", "Parser error")
				}
			case COMMA_SEP:
				printer.Debug("COMMA_SEP","State")
				if c == '\r' {
					cmdresp.Params = append(cmdresp.Params, string(param))
					state = FIN_R
					printer.Debug("FIN_R", "Change state")
				} else if c == ',' {
					cmdresp.Params = append(cmdresp.Params, string(param))
					param = make([]byte, 0)
				} else {
					param = append(param, c)
				}
			case READ_DATA:
				// printer.Debug("READ_DATA","State")
				// printer.Note(fmt.Sprintf("Available %d bytes, already read %d bytes", dataAvail, dataRead))
				if dataAvail > 0 {
					cmdresp.Data[dataRead] = c
					dataRead++
					dataAvail--
				}
				if dataAvail == 0 {
					responses <- cmdresp
					state = NO_COMMAND
					printer.Debug("NO_COMMAND", "Change state")
				}
			case FIN_R:
				printer.Debug("FIN_R","State")
				if c == '\n' {
					// FIXME: fails if there are no Params
					// if command is CHTTPACT: DATA, then read data
					if cmdresp.Name == "+CHTTPACT" && len(cmdresp.Params) > 0 && cmdresp.Params[0] == "DATA" {

						printer.Debug("Got a command with DATA!", "Command parser")
						dataRead = 0
						var err error
						dataAvail, err = strconv.Atoi(cmdresp.Params[1])
						if err != nil {
							printer.Error("Could not convert number of bytes")
						}
						cmdresp.Data = make([]byte, dataAvail)
						state = READ_DATA
						printer.Debug("READ_DATA", "Change state")
						printer.Note(fmt.Sprintf("Available %d bytes, already read %d bytes", dataAvail, dataRead))
					} else {
						state = NO_COMMAND
						printer.Debug("NO_COMMAND", "Change state")
						printer.Debug(cmdresp, "Sent command")
						responses <- cmdresp
					}
				} else {
					printer.Fatal("No \\n after FIN \\r")
				}
			}
		}
	}
	close(responses)
}

// func readResponses(responses chan *CommandResponse) {
// 	for resp := range responses {
// 		printer.Debug(resp, "New response")
// 	}
// }

// func collectData(ch chan []byte, done chan bool, commands chan string) {
// 	file, err := os.OpenFile("log", os.O_WRONLY|os.O_CREATE, 0544)
// 	if err != nil {
// 		printer.Fatal(err, "Could not open log file")
// 	}
// 	defer file.Close()
// 	buf := make([]byte, BUFSIZE)
// 	var sendBuf []byte
// 	for {
// 		select {
// 		case <-done:
// 			return
// 		case sendBuf = <-ch:
// 			buf = append(buf, sendBuf...)
// 			printer.Debug(sendBuf, "Got another portion of data")
// 			file.Write(sendBuf)
// 			file.Sync()
// 		}
// 	}
// }

// func readUserInput(commands chan string) {
// 	for {
		// reader := bufio.NewReader(os.Stdin)
		// command, _ := reader.ReadString('\n')

		// if strings.Contains(command, "#!") {
		// 	command = strings.Replace(command, "#!", "\x1a", -1)
			
		// } else {
		// 	command = command[:len(command)-1] + "\r"
		// }
		// printer.Note(command, "Command")
		// commands <- command

// 	}
// }

func makeCopy(buf []byte, n int) []byte {
  sendBuf := make([]byte, n)
  copy(sendBuf, buf)
  return sendBuf
}

func portWriter(commands chan []byte, port io.ReadWriteCloser) {
	for b := range commands {
		for {
			n, err := port.Write(b)
			if err != nil {
				// FIXME: do a graceful exit
			  printer.Fatal(err, "port.Write")	
			}
			printer.Note(b[:n], "Sent data")
			if n == len(b) {
				break
			}
			b = b[n:]
		}
	}
}

func writeRequest(headers []byte, body io.Reader) {
	commands <- headers
	if body == nil {
		commands <- []byte(CTRL_Z)
	} else {
		buf := make([]byte, BUFSIZE)
		for {
			n, err := body.Read(buf)
			if n == 0 && err == io.EOF {
				// end of body
				printer.Debug("End of request body!", "writeRequest")
				commands <- []byte(CTRL_Z)
				break
			} else if err != nil && err != io.EOF {
				printer.Fatal(err, "writeRequest: cannot read from body")
			}
			commands <- makeCopy(buf, n)
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

// TODO: create central manager to fan out responses
func receiveCHTTPACT() error {
	for {
		resp := <-responses
		if resp.Name == CHTTPACT {
			printer.Debug(resp, "receiveCHTTPACT: received response")
			if resp.Params[0] == REQUEST {
				return nil
			} else {
				err := errCHTTPACT(resp.Params[0])
				printer.Error(err, "Received error")
				/* Don't skip to ERROR, because modem does not send it */
				// for ; resp.Name != ERROR; resp = <-responses {
				// 	// FIXME: we might skip some useful info
				// 	printer.Debug("Skipping until ERROR is found")
				// }
				return err
			}
		} else if resp.Name == CME && resp.Status == ERROR {
			return errCHTTPACT(CME)
		} else if resp.Name == CONNECT {
			printer.Note("Got CONNECT, trying to fix...")
			commands <- []byte(CTRL_Z)
		} else {
			printer.Debug("Skipping until HTTPACT is found")
		}
	}
}

func receiveCHTTPACTResponse() ([]byte, error) {
	buf := make([]byte, 0, BUFSIZE)
	for {
		resp := <-responses
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
					// for ; resp.Name != ERROR; resp = <-responses {
					// 	// FIXME: we might skip some useful info
					// 	printer.Debug("Skipping until ERROR is found")
					// }
					return nil, err
				}
			}
		} else {
			printer.Debug("Skipping until HTTPACT is found")
		}
	}
}

func HTTPRequest(host string, port string, headers []byte, body io.Reader) ([]byte, error) {
	printer.Debug("New request", "HTTPRequest", map[string]string{
		"Host":host,
		"Port":port,
		"Headers":string(headers),
		"Body": "<<io.Reader>>",
	})
	commands <- []byte(fmt.Sprintf("AT+CHTTPACT=\"%s\",%s\r", host, port))
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

func init() {
	ch := make(chan []byte)
	responses = make(chan *CommandResponse)
	commands = make(chan []byte)

	const fake = false

	if fake {
		go fakePortReader(ch)
		go fakePortWriter(commands)
	} else {
		port := openPort() // FIXME: we don't close the port
		go portReader(ch, port)
		go portWriter(commands, port)
	}
	// collectData(ch, done)
	go commandParser(ch, responses)
}

package main

import (
	"github.com/jacobsa/go-serial/serial"
	"github.com/vadimpilyugin/debug_print_go"
	"io"
	"os"
	"time"
	"bufio"
	"strings"
)

const (
	BUFSIZE = 4096
	SMBUF = 32
	NO_COMMAND = iota
	FIRST_R
	COMMAND_RESPONSE
	RESPONSE
	OK_OR_ERROR
	FIN_R
	COMMA_SEP_SPACE
	COMMA_SEP
)

type CommandResponse struct {
	Name string
	Params []string
	Data []byte
}

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

func portReader(ch chan []byte, quit chan bool, port io.ReadWriteCloser) {
	buf := make([]byte, BUFSIZE)
	for {
		n, err := port.Read(buf)
		if err == io.EOF {
			quit <- true
			printer.Error(err, "No more data from port")
			return
		} else if err != nil {
			quit <- true
			printer.Error(err, "An error occured")
			return
		}
		// printer.Debug(n, "Got bytes")
		sendBuf := make([]byte, n)
		copy(sendBuf, buf)
		printer.Debug(string(sendBuf), "Got data")
		ch <- sendBuf
	}
}

func fakePortReader(ch chan []byte, quit chan bool) {
	file, err := os.Open("log")
	if err != nil {
		printer.Fatal(err, "Could not open log file")
	}
	defer file.Close()
	buf := make([]byte, SMBUF)
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			printer.Note("File ended")
			quit <- true
			return
		}
		// printer.Debug(n, "Got bytes")
		sendBuf := make([]byte, n)
		copy(sendBuf, buf)
		ch <- sendBuf
		time.Sleep(time.Millisecond * 200)
	}
}

func commandParser(ch chan []byte, quit chan bool, responses chan *CommandResponse) {
	state := NO_COMMAND
	var buf []byte
	var command []byte
	var param []byte
	var cmdresp *CommandResponse
	for {
		select {
			case buf = <-ch:
				for _, c := range buf {
					// printer.Note(fmt.Sprintf("%c", c), "c")
					switch state {
					case NO_COMMAND:
						// printer.Debug("NO_COMMAND","State")
						if c == '\r' {
							state = FIRST_R
							// printer.Note("FIRST_R", "Change state")
						}
					case FIRST_R:
						// printer.Debug("FIRST_R","State")
						if c == '\n' {
							state = RESPONSE
							// printer.Note("RESPONSE", "Change state")
						} else {
							state = NO_COMMAND
							// printer.Note("NO_COMMAND", "Change state")
						}
					case RESPONSE:
						// printer.Debug("RESPONSE","State")
						command = make([]byte, 0, SMBUF)
						command = append(command, c)
						if c == '+' {
							state = COMMAND_RESPONSE
							// printer.Note("COMMAND_RESPONSE", "Change state")
						} else {
							state = OK_OR_ERROR
							// printer.Note("OK_OR_ERROR", "Change state")
						}
					case OK_OR_ERROR:
						// printer.Debug("OK_OR_ERROR","State")
						if c != '\r' {
							command = append(command, c)
						} else {
							commStr := string(command)
							if commStr != "OK" && commStr != "ERROR" {
								printer.Fatal(command, "No such command")
							}
							cmdresp = &CommandResponse{
								Name: commStr,
							}
							state = FIN_R
							// printer.Note("FIN_R", "Change state")
						}
					case COMMAND_RESPONSE:
						// printer.Debug("COMMAND_RESPONSE","State")
						if c != ':' {
							command = append(command, c)
						} else {
							cmdresp = &CommandResponse{
								Name: string(command),
							}
							state = COMMA_SEP_SPACE
							// printer.Note("COMMA_SEP_SPACE", "Change state")
							param = make([]byte, 0)
						}
					case COMMA_SEP_SPACE:
						// printer.Debug("COMMA_SEP_SPACE","State")
						if c == ' ' {
							state = COMMA_SEP
							// printer.Note("COMMA_SEP", "Change state")
						} else {
							printer.Fatal("Comma-separated values without space character", "Parser error")
						}
					case COMMA_SEP:
						// printer.Debug("COMMA_SEP","State")
						if c == '\r' {
							state = FIN_R
							// printer.Note("FIN_R", "Change state")
						} else if c == ',' {
							cmdresp.Params = append(cmdresp.Params, string(param))
							param = make([]byte, 0)
						} else {
							param = append(param, c)
						}
					case FIN_R:
						// printer.Debug("FIN_R","State")
						if c == '\n' {
							state = NO_COMMAND
							// printer.Note("NO_COMMAND", "Change state")
							responses <- cmdresp
						} else {
							printer.Fatal("No \\n after FIN \\r")
						}
					}
				}
			case <-quit:
				close(responses)
				return
		}
	}
}

func readResponses(responses chan *CommandResponse) {
	for resp := range responses {
		printer.Debug(resp, "New response")
	}
}

func collectData(ch chan []byte, quit chan bool, commands chan string) {
	file, err := os.OpenFile("log", os.O_WRONLY|os.O_CREATE, 0544)
	if err != nil {
		printer.Fatal(err, "Could not open log file")
	}
	defer file.Close()
	buf := make([]byte, BUFSIZE)
	var sendBuf []byte
	for {
		select {
		case <-quit:
			return
		case sendBuf = <-ch:
			buf = append(buf, sendBuf...)
			printer.Debug(sendBuf, "Got another portion of data")
			file.Write(sendBuf)
			file.Sync()
		}
	}
}

func portWriter(commands chan string, port io.ReadWriteCloser) {
	for command := range commands {
	  b := []byte(command)
		n, err := port.Write(b)
		if err != nil {
		  printer.Fatal(err, "port.Write")	
		}
		printer.Note(n, "Sent data")
	}
}

func readUserInput(commands chan string) {
	for {
		reader := bufio.NewReader(os.Stdin)
		command, _ := reader.ReadString('\n')
		if strings.Contains(command, "#!") {
			command = strings.Replace(command, "#!", "\x1a", -1)
			
		} else {
			command = command[:len(command)-1] + "\r"
		}
		printer.Note(command, "Command")
		commands <- command
	}
}

func main() {

	// Read data from port and print it
	ch := make(chan []byte)
	quit := make(chan bool)
	responses := make(chan *CommandResponse)
	commands := make(chan string)

	port := openPort()
	defer port.Close()
	go portReader(ch, quit, port)
	go portWriter(commands, port)
	// go fakePortReader(ch, quit)
	// collectData(ch, quit)
	go commandParser(ch, quit, responses)
	go readResponses(responses)
	readUserInput(commands)
}

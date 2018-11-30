package main

import (
  "github.com/jacobsa/go-serial/serial"
  "github.com/vadimpilyugin/debug_print_go"
  "io"
  "os"
)

func reader(ch chan []byte, port io.ReadWriteCloser, forever bool) {
  file, err := os.OpenFile("req", os.O_WRONLY|os.O_CREATE, 0544)
  if err != nil {
    printer.Fatal(err, "Could not open req file")
  }
  defer file.Close()
  for {
    buf := make([]byte, 1234)
    n, err := port.Read(buf)
    if err != nil {
      if err != io.EOF {
        printer.Fatal(err, "Error reading from serial port")
      }
    } else {
      buf = buf[:n]
      // printer.Debug(hex.EncodeToString(buf), "Rx")
      printer.Debug(buf, "Read data")
      file.Write(buf)
      file.Sync()
      if !forever {
        ch <- buf
        return
      }
      buf = buf[:0]
    }
  }
}

func main() {

  // Set up options.
  // 115200bps, 8 bit data, no parity, 1 bit stop, no data stream control. 
  options := serial.OpenOptions{
    PortName: "/dev/ttyUSB2",
    BaudRate: 115200,
    DataBits: 8,
    StopBits: 1,
    MinimumReadSize: 4,
  }


  // Open the port.
  port, err := serial.Open(options)
  if err != nil {
    printer.Fatal(err, "serial.Open")
  }

  // Make sure to close it later.
  defer port.Close()

  // Write n bytes to the port.
  b := []byte("AT+CHTTPACT=\"openplatform.website\",8080\r")
  // b := []byte("AT+CREG\r")
  n, err := port.Write(b)
  if err != nil {
    printer.Fatal(err, "port.Write")
  }

  printer.Note(n, "Sent data")

  ch := make(chan []byte)
  go reader(ch, port, false)
  _ = <-ch


  // Write n bytes to the port.
  b = []byte("GET / HTTP/1.1\r\nHost: mobile-review.com\r\nUser-Agent: Foobar agent\r\nContent-Length: 0\r\n\r\n")
  b = append(b, 26)
  n, err = port.Write(b)
  if err != nil {
    printer.Fatal(err, "port.Write")
  }
  printer.Note(n, "Sent data")
  ch = make(chan []byte)
  go reader(ch, port, true)
  _ = <-ch

}

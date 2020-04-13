// MIT License

// Copyright (c) 2020 Shekh Ataul

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package memnet

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

type lnOptions struct {
	c int
	t int
	a string
}

var dLnOptn = lnOptions{1, 10, "0.0.0.0:4434"}

const (
	errIOMismatched   = "(input) %s != %s (output)"
	errRWBytes        = "readBytes is %d, but writeBytes is %d"
	errReadRemoteConn = "could not read from remote memconn: %v"
	errWriteLocalConn = "could not write in local memconn: %v"
	errAcceptMemConn  = "failed to accept memconn: %v"
	errMemServer      = "failed to connect to memserver: %v"
	errMemListener    = "failed to start memlistener: %v"
)

type ioResult struct {
	n   int
	err error
}

func doRead(r io.Reader, output []byte) <-chan ioResult {
	done := make(chan ioResult)
	var rLen int
	var rErr error

	go func() {
		for {
			if rLen >= len(output) || rErr != nil {
				break
			}

			var n int
			n, rErr = r.Read(output[rLen:])
			rLen += n

		}
		done <- ioResult{rLen, rErr}

	}()
	return done
}

func doWrite(w io.Writer, input []byte) <-chan ioResult {
	done := make(chan ioResult)
	var wLen int
	var wErr error

	go func() {
		wLen, wErr = w.Write(input)
		done <- ioResult{wLen, wErr}
	}()
	return done
}

func doReadWrite(rw io.ReadWriter) error {

	for i := 20; i > 0; i-- {
		output := make([]byte, i)
		input := make([]byte, i)

		for j := 0; j < i; j++ {
			input[j] = byte(i - j)
		}

		writeCh := doWrite(rw, input)
		readCh := doRead(rw, output)

		for {
			select {
			case result := <-writeCh:
				if result.n != i || result.err != nil {
					return fmt.Errorf("%v: rw.Write(%v) = %v, %v; want %v, nil",
						i, input, result.n, result.err, i)
				}

			case result := <-readCh:
				if result.n != i || result.err != nil {
					return fmt.Errorf("%v: rw.Read() = %v, %v; want %v, nil",
						i, result.n, result.err, i)
				}

				if !reflect.DeepEqual(input, output) {
					return fmt.Errorf("%v: rw.Read read %v; want %v", i, output, input)
				}
				goto end

			case <-time.After(500 * time.Millisecond):
				//return fmt.Errorf("%v: rw.Read blocked", i)
			}
		}
	end:
	}
	return nil
}

func memConnServe() (net.Conn, net.Conn, error) {
	ln, err := Listen(dLnOptn.c, dLnOptn.t, dLnOptn.a)
	if err != nil {
		return nil, nil, fmt.Errorf(errMemListener, err.Error())
	}

	local, err := ln.Dial()
	if err != nil {
		return nil, nil, fmt.Errorf(errMemServer, err.Error())
	}

	remote, err := ln.Accept()
	if err != nil {
		return nil, nil, fmt.Errorf(errAcceptMemConn, err.Error())
	}

	return local, remote, err
}

func TestRingBuff(t *testing.T) {
	p := newRingBuff(10)
	err := doReadWrite(p)
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func TestRingBuffClosed(t *testing.T) {
	rb := newRingBuff(10)

	rb.Close()
	if _, err := rb.Write(nil); err != io.ErrClosedPipe {
		t.Fatalf("p.Write = _, %v; want _, %v", err, io.ErrClosedPipe)
	}

	if _, err := rb.Read(nil); err != io.ErrClosedPipe {
		t.Fatalf("p.Read = _, %v; want _, %v", err, io.ErrClosedPipe)
	}
}

func TestListenerAddr(t *testing.T) {
	ln, _ := Listen(dLnOptn.c, dLnOptn.t, dLnOptn.a)
	if ln.Addr().String() != dLnOptn.a {
		t.Fatalf("ln.Addr() = %v, want %v", ln.Addr().String(), dLnOptn.a)
	}
}

func TestListenerClosed(t *testing.T) {
	ln, err := Listen(dLnOptn.c, dLnOptn.t, dLnOptn.a)
	if err != nil {
		t.Fatalf(errMemListener, err.Error())
	}

	ln.Close()

	_, err = ln.Dial()
	if err != io.ErrClosedPipe {
		t.Fatalf("ln.Dial = _, %v, want %v", err, io.ErrClosedPipe)
	}

}

func TestConnRW(t *testing.T) {
	local, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	input := []byte("shared")

	wn, err := local.Write(input)
	if err != nil {
		t.Fatalf(errWriteLocalConn, err.Error())
	}

	output := make([]byte, len(input))
	rn, err := remote.Read(output)
	if err != nil {
		t.Fatalf(errReadRemoteConn, err.Error())
	}

	if wn != rn {
		t.Fatalf(errRWBytes, rn, wn)
	}

	if !reflect.DeepEqual(input, output) {
		t.Fatalf(errIOMismatched, input, output)
	}

}

func TestLocalClosedRead(t *testing.T) {
	local, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	local.Close()

	_, err = remote.Read(nil)
	if err != io.EOF {
		t.Fatalf("remote.Read = _, %v, want %v", err, io.EOF)
	}
}

func TestLocalClosedWrite(t *testing.T) {
	local, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	local.Close()

	_, err = remote.Write(nil)
	if err != io.ErrClosedPipe {
		t.Fatalf("remote.Write = _, %v, want %v", err, io.ErrClosedPipe)
	}
}

func TestRemoteClosedRead(t *testing.T) {
	local, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	remote.Close()

	_, err = local.Read(nil)
	if err != io.EOF {
		t.Fatalf("local.Read = _, %v, want %v", err, io.EOF)
	}
}

func TestRemoteClosedWrite(t *testing.T) {
	local, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	remote.Close()

	_, err = local.Write(nil)
	if err != io.ErrClosedPipe {
		t.Fatalf("local.Write = _, %v, want %v", err, io.ErrClosedPipe)
	}
}

func TestRemoteSetReadDeadline(t *testing.T) {
	_, remote, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	remote.SetReadDeadline(time.Time{}.Add(1 * time.Second))

	_, err = remote.Read(nil)
	if err != errTimeout {
		t.Fatalf("remote.Read = _, %v, want %v", err, errTimeout)
	}
}

func TestLocalSetReadDeadline(t *testing.T) {
	local, _, err := memConnServe()
	if err != nil {
		t.Fatal(err.Error())
	}

	local.SetReadDeadline(time.Time{}.Add(1 * time.Second))

	_, err = local.Read(nil)
	if err != errTimeout {
		t.Fatalf("local.Read = _, %v, want %v", err, errTimeout)
	}
}

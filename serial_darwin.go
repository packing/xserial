// Copyright 2021 Abhijit Bose. All rights reserved.

//go:build darwin
// +build darwin

package xserial

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var baudRates = map[int]uint32{
	50:     unix.B50,
	75:     unix.B75,
	110:    unix.B110,
	134:    unix.B134,
	150:    unix.B150,
	200:    unix.B200,
	300:    unix.B300,
	600:    unix.B600,
	1200:   unix.B1200,
	1800:   unix.B1800,
	2400:   unix.B2400,
	4800:   unix.B4800,
	9600:   unix.B9600,
	19200:  unix.B19200,
	38400:  unix.B38400,
	57600:  unix.B57600,
	115200: unix.B115200,
	230400: unix.B230400,
}

// Linux Compatible Serial Port Structure
type serialPort struct {
	// Handle
	fd int
	// Lock for Handle - Make it Thread Safe by Default
	mx sync.Mutex
	// If Port is Open
	opened bool
	// Configuration
	conf Config
}

// Platform Specific Open Port Function
func openPort(cfg *Config) (Port, error) {
	s := &serialPort{}

	// Interpret the Config for Potential Errors
	t, err := getTermiosFor(cfg)
	if err != nil {
		return nil, err
	}

	// Open Port
	err = s.Open(cfg.Name)
	if err != nil {
		return nil, err
	}

	// Auto Close on Errors
	defer func(fd int, err error) {
		if fd != 0 && err != nil {
			unix.Close(fd)
			s.fd = 0 // Not Initialized state
			s.opened = false
		}
	}(s.fd, err)

	// Set Terminos
	err = s.SetTermios(t)
	if err != nil {
		return nil, err
	}

	// Set the Configuration
	s.conf = *cfg

	// Set Non-Blocking for Timeout and Blocking Purposes
	err = unix.SetNonblock(s.fd, false)
	if err != nil {
		return nil, err
	}

	// Finally Success
	return s, err
}

func (s *serialPort) Open(name string) error {
	// Establish Lock
	s.mx.Lock()
	defer s.mx.Unlock()

	// Check If its Open
	if s.opened {
		// Release Log temporarily
		s.mx.Unlock()
		// Ignore Errors for Forced Close
		s.Close()
		// Re-Engage Lock
		s.mx.Lock()
	}

	// Check if Port is already open
	err := exec.Command("lsof", "-t", name).Run()
	// This is ODD but yes if there is no error then we know port is open
	if err == nil {
		return ErrAlreadyOpen
	} else if err.Error() != "exit status 1" {
		return ErrAccessDenied
	}

	// Try to Open
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_EXCL, 0)
	if err != nil {
		return err
	}
	// Assign fd
	s.fd = fd
	s.opened = true

	// Auto Close on Errors
	defer func(fd int, err error) {
		if fd != 0 && err != nil {
			unix.Close(fd)
			s.fd = 0 // Not Initialized state
			s.opened = false
		}
	}(fd, err)

	//独占权限
	if _, _, e1 := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCEXCL), 0); e1 != 0 {
		return fmt.Errorf("failed to get exclusive access - %v", e1)
	}

	return err
}

// unixSelect is a wapper for unix.Select that only returns error.
func unixSelect(n int, r *unix.FdSet, w *unix.FdSet, e *unix.FdSet, tv *unix.Timeval) error {
	_, err := unix.Select(n, r, w, e, tv)
	return err
}

// fdget returns index and offset of fd in fds.
func fdget(fd int, fds *unix.FdSet) (index, offset int) {
	index = fd / (unix.FD_SETSIZE / len(fds.Bits)) % len(fds.Bits)
	offset = fd % (unix.FD_SETSIZE / len(fds.Bits))
	return
}

// fdset implements FD_SET macro.
func fdset(fd int, fds *unix.FdSet) {
	idx, pos := fdget(fd, fds)
	fds.Bits[idx] = 1 << uint(pos)
}

// fdisset implements FD_ISSET macro.
func fdisset(fd int, fds *unix.FdSet) bool {
	idx, pos := fdget(fd, fds)
	return fds.Bits[idx]&(1<<uint(pos)) != 0
}

func (s *serialPort) Read(p []byte) (n int, err error) {
	var rfds unix.FdSet
	fd := s.fd
	fdset(fd, &rfds)
	var tv *unix.Timeval
	//如果设置了超时
	if s.conf.ReadTimeout > 0 {
		//time.Millisecond代替 1000*1000
		tempTime := s.conf.ReadTimeout * time.Millisecond
		timeout := unix.NsecToTimeval(tempTime.Nanoseconds())
		tv = &timeout
	}

	// Establish Lock
	s.mx.Lock()
	defer s.mx.Unlock()

	// Check If its Open
	if !s.opened {
		return 0, ErrNotOpen
	}
	if s.conf.ReadTimeout > 0 {
		for {
			// If unix.Select() returns EINTR (Interrupted system call), retry it
			if err = unixSelect(fd+1, &rfds, nil, nil, tv); err == nil {
				break
			}
			if err != unix.EINTR {
				err = fmt.Errorf("serial: could not select: %v", err)
				return
			}
		}
		if !fdisset(fd, &rfds) {
			// Timeout
			err = ErrReadTimeout
			return
		}
		n, err = unix.Read(fd, p)
		return
	} else {
		for {
			// Perform the Actual Read
			n, err = unix.Read(s.fd, p)
			// In case the Read was interrupted by a Signal
			if err == unix.EINTR {
				continue
			}
			// Linux: when the port is disconnected during a read operation
			// the port is left in a "readable with zero-length-data" state.
			// https://stackoverflow.com/a/34945814/1655275

			//if n == 0 && err == nil {
			//	return 0, ErrPortClosed
			//}

			// In Case of Negative values of n due to other errors
			if n < 0 {
				n = 0 // Don't let -1 pass on
			}
			return n, err
		}
	}
	/*
		// Loop to Access the Port Data
		for {
			// Perform the Actual Read
			n, err = unix.Read(s.fd, p)
			// In case the Read was interrupted by a Signal
			if err == unix.EINTR {
				continue
			}
			// Linux: when the port is disconnected during a read operation
			// the port is left in a "readable with zero-length-data" state.
			// https://stackoverflow.com/a/34945814/1655275

			if n == 0 && err == nil {
				return 0, ErrPortClosed
			}

			// In Case of Negative values of n due to other errors
			if n < 0 {
				n = 0 // Don't let -1 pass on
			}
			return n, err
		}
	*/
}

func (s *serialPort) Write(p []byte) (n int, err error) {
	// Establish Lock
	//s.mx.Lock()
	//defer s.mx.Unlock()

	// Check If its Open
	if !s.opened {
		return 0, ErrNotOpen
	}

	n, err = unix.Write(s.fd, p)
	// In case -1 returned - don't pass it on
	if n < 0 {
		n = 0
	}
	return n, err
}

func (s *serialPort) Close() error {
	// Establish Lock
	s.mx.Lock()
	defer s.mx.Unlock()

	// Check If its Open
	if !s.opened {
		return ErrPortNotInitialized
		// return nil
	}

	// Auto Run at the End of the function
	defer func() {
		s.fd = 0
		s.opened = false
	}()

	// Release Exclusive Access
	if _, _, e1 := unix.Syscall(unix.SYS_IOCTL, uintptr(s.fd), uintptr(unix.TIOCNXCL), 0); e1 != 0 {
		return fmt.Errorf("failed to release exclusive access - %v", e1)
	}

	// Perform the Actual Close
	return unix.Close(s.fd)
}

func (s *serialPort) SetParity(parity string, stopbits int) (err error) {
	return nil
}

//清除缓存
func (s *serialPort) Flush() error {
	const TCFLSH = 0x540B
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(s.fd), uintptr(TCFLSH), uintptr(unix.TCIOFLUSH))
	if errno == 0 {
		return nil
	}
	return errno
}

func (s *serialPort) SetTermios(t unix.Termios) error {
	return nil
}

func (s *serialPort) GetTermios() (t unix.Termios, err error) {
	return t, nil
}

func getTermiosFor(cfg *Config) (unix.Termios, error) {
	var t unix.Termios
	return t, nil
}

package xserial

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go syscall_windows.go

import (
	"fmt"
	"io"
	"time"
)

const (
	// FlowNone for no flow control to be used for Serial port
	FlowNone byte = iota
	// FlowHardware for CTS / RTS base Hardware flow control to be used for Serial port
	FlowHardware byte = iota
	// FlowSoft for Software flow control to be used for Serial port
	FlowSoft byte = iota // XON / XOFF based - Not Supported
)

// Config stores the complete configuration of a Serial Port
type Config struct {
	Name        string
	Baud        int
	ReadTimeout time.Duration // Blocks the Read operation for a specified time
	Parity      string
	StopBits    int
	Flow        byte
}

// Default Errors

var (
	// ErrNotImplemented -
	ErrNotImplemented = fmt.Errorf("not implemented yet")
	// ErrPortNotInitialized -
	ErrPortNotInitialized = fmt.Errorf("port not initialized or closed")
	// ErrNotOpen -
	ErrNotOpen = fmt.Errorf("port not open")
	// ErrAlreadyOpen -
	ErrAlreadyOpen = fmt.Errorf("port is already open")
	// ErrAccessDenied -
	ErrAccessDenied = fmt.Errorf("access denied")
	// ErrPortClosed
	ErrPortClosed = fmt.Errorf("port closed")
	//超时
	ErrReadTimeout = fmt.Errorf("read port time out")
)

// Port Type for Multi platform implementation of Serial port functionality
type Port interface {
	io.ReadWriteCloser
	//设置校验位和停止位
	SetParity(parity string, stopbits int) (err error)
	//清理串口的缓存
	Flush() (err error)
}

// OpenPort is a Function to Create the Serial Port and return an Interface type enclosing the configuration
func OpenPort(cfg *Config) (Port, error) {
	return openPort(cfg)
}

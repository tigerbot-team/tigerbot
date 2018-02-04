package propeller

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Type representing a bus connection
type I2cBus struct {
	fd *os.File
}

// Data that is passed to/from ioctl calls
type i2c_smbus_ioctl_data struct {
	read_write uint8
	command    uint8
	size       int
	data       *uint8
}

// Constants used by ioctl, from i2c-dev.h
const (
	I2C_SMBUS_READ      = 1
	I2C_SMBUS_WRITE     = 0
	I2C_SMBUS_BYTE_DATA = 2

	// Talk to bus
	I2C_SMBUS = 0x0720
	// Set bus slave
	I2C_SLAVE = 0x0703
)

// Create an I2cBus for the specified address
func CreateBus(address uint8) *I2cBus {
	file, err := os.OpenFile("/dev/i2c-1", os.O_RDWR, os.ModeExclusive)
	if err != nil {
		panic(err)
	}

	fmt.Println("About to open Bus fd ", file.Fd(), " IOCTL ", I2C_SLAVE, " Arg", address)
	_, _, er := syscall.Syscall(syscall.SYS_IOCTL, uintptr(file.Fd()),
		I2C_SLAVE, uintptr(address))
	if er != 0 {
		panic(syscall.Errno(er))
	}

	return &I2cBus{
		fd: file,
	}
}

// Close the bus
func (bus *I2cBus) Close() {
	if err := bus.fd.Close(); err != nil {
		panic(err)
	}
}

// Write 1 byte onto the bus
func (bus *I2cBus) Write8(command, data uint8) {

	// i2c_smbus_access(file,I2C_SMBUS_WRITE,command,I2C_SMBUS_BYTE_DATA, &data);
	busData := i2c_smbus_ioctl_data{
		read_write: I2C_SMBUS_WRITE,
		command:    command,
		size:       I2C_SMBUS_BYTE_DATA,
		data:       &data,
	}

	fmt.Println("About to Write8 ", bus.fd.Fd(), " IOCTL ", I2C_SMBUS, "Command ", command, " Data ", data)

	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(bus.fd.Fd()),
		I2C_SMBUS, uintptr(unsafe.Pointer(&busData)))
	if err != 0 {
		panic(syscall.Errno(err))
	}
}

// Read 1 byte from the bus
func (bus *I2cBus) Read8(command uint8) uint8 {

	// i2c_smbus_access(file,I2C_SMBUS_READ,command,I2C_SMBUS_BYTE_DATA,&data)

	data := uint8(0)

	busData := i2c_smbus_ioctl_data{
		read_write: I2C_SMBUS_READ,
		command:    command,
		size:       I2C_SMBUS_BYTE_DATA,
		data:       &data,
	}

	fmt.Println("About to Read8 ", bus.fd.Fd(), " IOCTL ", I2C_SMBUS, " Command", command)

	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(bus.fd.Fd()),
		I2C_SMBUS, uintptr(unsafe.Pointer(&busData)))
	if err != 0 {
		panic(syscall.Errno(err))
	}

	return data
}

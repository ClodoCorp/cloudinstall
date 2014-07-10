package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	_IOC_NONE  = 0x0
	_IOC_WRITE = 0x1
	_IOC_READ  = 0x2

	_IOC_NRBITS   = 8
	_IOC_TYPEBITS = 8
	_IOC_SIZEBITS = 14
	_IOC_DIRBITS  = 2
	_IOC_NRSHIFT  = 0

	_IOC_TYPESHIFT = _IOC_NRSHIFT + _IOC_NRBITS
	_IOC_SIZESHIFT = _IOC_TYPESHIFT + _IOC_TYPEBITS
	_IOC_DIRSHIFT  = _IOC_SIZESHIFT + _IOC_SIZEBITS

	_IOC_NRMASK   = ((1 << _IOC_NRBITS) - 1)
	_IOC_TYPEMASK = ((1 << _IOC_TYPEBITS) - 1)
	_IOC_SIZEMASK = ((1 << _IOC_SIZEBITS) - 1)
	_IOC_DIRMASK  = ((1 << _IOC_DIRBITS) - 1)
)

func _IOC(dir int, t int, nr int, size int) int {
	return (dir << _IOC_DIRSHIFT) | (t << _IOC_TYPESHIFT) |
		(nr << _IOC_NRSHIFT) | (size << _IOC_SIZESHIFT)
}

func _IOR(t int, nr int, size int) int {
	return _IOC(_IOC_READ, t, nr, size)
}

func _IOW(t int, nr int, size int) int {
	return _IOC(_IOC_WRITE, t, nr, size)
}

func _IOWR(t int, nr int, size int) int {
	return _IOC(_IOC_READ|_IOC_WRITE, t, nr, size)
}

func _IO(t int, nr int) int {
	return _IOC(_IOC_NONE, t, nr, 0)
}

func ioctl(fd uintptr, name int, data unsafe.Pointer) error {
	_, _, err := syscall.RawSyscall(syscall.SYS_IOCTL, fd, uintptr(name), uintptr(data))
	if err != 0 {
		return errors.New("failed to run ioctl")
	}
	return nil
}

func blkpart(dev string) error {
	BLKRRPART := _IO(0x12, 95)

	w, err := os.OpenFile(dev, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer w.Close()

	var r int
	// TODO: firstly send BLKPG, if it fails send BLKRRPART
	err = ioctl(uintptr(w.Fd()), BLKRRPART, unsafe.Pointer(&r))
	if err != nil {
		return err
	}
	return nil
}

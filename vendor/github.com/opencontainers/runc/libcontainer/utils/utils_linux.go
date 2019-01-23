// +build linux

package utils

import (
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func memfdCreate(comment string) (*os.File, error) {
	fd, err := unix.MemfdCreate(comment, unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, errors.Wrapf(err, "memfd_create %s", comment)
	}
	return os.NewFile(uintptr(fd), "memfd:"+comment), nil
}

func memfdSeal(fh *os.File) error {
	_, err := unix.FcntlInt(fh.Fd(),
		unix.F_ADD_SEALS,
		unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_WRITE|unix.F_SEAL_SEAL)
	return errors.Wrap(err, "memfd sealing")
}

// CloneBinary will clone a binary into a private copy such that any attempt to
// modify it will not modify the host binary. This is used in place of just
// /proc/self/exe to avoid exposing host binaries to containers through procfs.
func CloneBinary(binPath string) (string, error) {
	binFh, err := os.Open(binPath)
	if err != nil {
		return "", errors.Wrap(err, "open clone-from binary")
	}
	if binFi, err := binFh.Stat(); err != nil {
		return "", errors.Wrap(err, "stat clone-from binary")
	} else if !binFi.Mode().IsRegular() {
		return "", errors.Errorf("clone-from binary (%s) is not a regular file!", binPath)
	}

	memFh, err := memfdCreate(binPath)
	if err != nil {
		return "", errors.Wrap(err, "create clone-to binary")
	}
	if _, err := io.Copy(memFh, binFh); err != nil {
		return "", errors.Wrapf(err, "copy binary %s", binPath)
	}
	if err := memFh.Chmod(0777); err != nil {
		return "", errors.Wrap(err, "make clone-to binary executable")
	}
	if err := memfdSeal(memFh); err != nil {
		return "", errors.Wrap(err, "seal binary copy")
	}

	procLink := fmt.Sprintf("/proc/self/fd/%d", memFh.Fd())
	return procLink, nil
}

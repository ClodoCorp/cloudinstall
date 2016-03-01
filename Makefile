SHELL := /bin/bash

all: clean x86_64 x86_32

x86_64:
	@echo Building x86_64
	$(CURDIR)/build x86_64

x86_32:
	@echo Building x86_32
	$(CURDIR)/build x86_32

clean:
	@echo Cleanup
	@rm -rf "$(CURDIR)/output"

test: ipv4 ipv6

ipv4:
	@rm -rf "$(CURDIR)/tests";\
	mkdir -p "$(CURDIR)/tests";\
	qemu-img create -f raw "$(CURDIR)/tests/disk.raw" 5G
	@echo ipv4 test
	time /usr/bin/qemu-system-x86_64 \
	-m 256 \
	-kernel output/kernel \
	-initrd output/initrd \
	-append "console=tty console=ttyS1 console=tty0 console=ttyS0 ip=eth0:auto4" \
	-device virtio-scsi-pci,id=scsi0 \
	-drive if=none,cache=unsafe,id=drive0,discard=unmap,file="$(CURDIR)/tests/disk.raw" \
	-device scsi-hd,bus=scsi0.0,drive=drive0 \
	-machine type=pc-1.3,accel=kvm \
	-vnc 0.0.0.0:97 \
	-netdev user,id=user.0,net=10.0.2.0/24 \
	-device virtio-net,netdev=user.0 && \
	/usr/bin/qemu-system-x86_64 \
	-m 256 \
	-device virtio-scsi-pci,id=scsi0 \
	-drive if=none,cache=unsafe,id=drive0,discard=unmap,file="$(CURDIR)/tests/disk.raw" \
	-device scsi-hd,bus=scsi0.0,drive=drive0 \
	-machine type=pc-1.3,accel=kvm \
	-vnc 0.0.0.0:97 \
	-netdev user,id=user.0,net=10.0.2.0/24 \
	-device virtio-net,netdev=user.0

ipv6:
	@rm -rf "$(CURDIR)/tests";\
  mkdir -p "$(CURDIR)/tests";\
  qemu-img create -f raw "$(CURDIR)/tests/disk.raw" 5G
	@echo ipv6 test
	time /usr/bin/qemu-system-x86_64 \
  -m 256 \
  -kernel output/kernel \
  -initrd output/initrd \
  -append "console=tty console=ttyS1 console=tty0 console=ttyS0 ip=eth0:auto6" \
  -device virtio-scsi-pci,id=scsi0 \
  -drive if=none,cache=unsafe,id=drive0,discard=unmap,file="$(CURDIR)/tests/disk.raw" \
	-device scsi-hd,bus=scsi0.0,drive=drive0 \
	-machine type=pc-1.3,accel=kvm \
	-vnc 0.0.0.0:97 \
	-netdev user,id=user.0,ip6-net=fed0::/64 \
	-device virtio-net,netdev=user.0 && \
	/usr/bin/qemu-system-x86_64 \
  -m 256 \
  -device virtio-scsi-pci,id=scsi0 \
  -drive if=none,cache=unsafe,id=drive0,discard=unmap,file="$(CURDIR)/tests/disk.raw" \
  -device scsi-hd,bus=scsi0.0,drive=drive0 \
  -machine type=pc-1.3,accel=kvm \
  -vnc 0.0.0.0:97 \
  -netdev user,id=user.0,ip6-net=fed0::/64 \
  -device virtio-net,netdev=user.0

SHELL := /bin/bash

all: clean x86_64 x86_32

x86_64:
	@echo Building
	@tmp=`mktemp --tmpdir -d`; \
	trap 'rm -rf "$$tmp";' EXIT SIGINT SIGQUIT ;\
	rm -f "$(CURDIR)/output-x86_64" ;\
	mkdir -p "$${tmp}/etc" "$${tmp}/bin" "$(CURDIR)/output" ;\
	touch "$${tmp}/etc/resolv.conf" ;\
	cp data/busybox-x86_64 "$${tmp}/bin/busybox" ;\
	cp data/init "$${tmp}/init" ;\
	bash -c 'env GOPATH=$(CURDIR) GO15VENDOREXPERIMENT=1 CGO_ENABLED=0 go build -a -o "$${tmp}/init2"' ;\
	cp -f data/vmlinuz-*-x86_64 "$(CURDIR)/output/kernel-x86_64" ;\
	pushd "$${tmp}/" >/dev/null;\
	find . | cpio -H newc -o 2>/dev/null | gzip > "$(CURDIR)/output/initrd-x86_64";\
	popd >/dev/null

x86_32:
	@echo Building
	@tmp=`mktemp --tmpdir -d`; \
	trap 'rm -rf "$$tmp";' EXIT SIGINT SIGQUIT ;\
	rm -f "$(CURDIR)/output/*-x86_32" ;\
	mkdir -p "$${tmp}/etc" "$${tmp}/bin" "$(CURDIR)/output" ;\
	touch "$${tmp}/etc/resolv.conf" ;\
	cp data/busybox-x86_32 "$${tmp}/bin/busybox" ;\
	cp data/init "$${tmp}/init" ;\
	bash -c 'env GOPATH=$(CURDIR) GO15VENDOREXPERIMENT=1 CGO_ENABLED=0 GOARCH=386 go build -a -o "$${tmp}/init2"' ;\
	cp -f data/vmlinuz-*-x86_32 "$(CURDIR)/output/kernel-x86_32" ;\
	pushd "$${tmp}/" >/dev/null;\
	find . | cpio -H newc -o 2>/dev/null | gzip > "$(CURDIR)/output/initrd-x86_32";\
	popd >/dev/null

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

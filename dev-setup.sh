#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Setting up Tartarus development environment...${NC}"

# Create data directory
DATA_DIR="./data/firecracker"
mkdir -p "$DATA_DIR"
echo -e "${GREEN}Created data directory at $DATA_DIR${NC}"

# Download Hello World Kernel and Rootfs if not present
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
ROOTFS_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/rootfs/bionic.rootfs.ext4"

KERNEL_PATH="$DATA_DIR/vmlinux"
ROOTFS_PATH="$DATA_DIR/rootfs.img"

if [ ! -f "$KERNEL_PATH" ]; then
    echo -e "${YELLOW}Downloading kernel...${NC}"
    curl -L -o "$KERNEL_PATH" "$KERNEL_URL"
    echo -e "${GREEN}Kernel downloaded.${NC}"
else
    echo -e "${GREEN}Kernel already exists.${NC}"
fi

if [ ! -f "$ROOTFS_PATH" ]; then
    echo -e "${YELLOW}Downloading rootfs...${NC}"
    curl -L -o "$ROOTFS_PATH" "$ROOTFS_URL"
    echo -e "${GREEN}Rootfs downloaded.${NC}"
else
    echo -e "${GREEN}Rootfs already exists.${NC}"
fi

# Check for KVM
if [ -w "/dev/kvm" ]; then
    echo -e "${GREEN}/dev/kvm is accessible.${NC}"
else
    echo -e "${YELLOW}WARNING: /dev/kvm is not accessible or writable. The agent may fail to start VMs.${NC}"
    echo -e "${YELLOW}Try: sudo chmod 666 /dev/kvm${NC}"
fi

echo -e "${GREEN}Setup complete! You can now run 'docker-compose up --build'${NC}"

package typhon

import (
	"strings"
	"testing"
)

func TestAnalyzeStrace(t *testing.T) {
	input := `
execve("./test", ["./test"], 0x7ffd5d0e7040 /* 22 vars */) = 0
brk(NULL)                               = 0x1d36000
arch_prctl(0x3001 /* ARCH_??? */, 0x7ffe098ec2e0) = -1 EINVAL (Invalid argument)
access("/etc/ld.so.preload", R_OK)      = -1 ENOENT (No such file or directory)
openat(AT_FDCWD, "/etc/ld.so.cache", O_RDONLY|O_CLOEXEC) = 3
fstat(3, {st_mode=S_IFREG|0644, st_size=22238, ...}) = 0
mmap(NULL, 22238, PROT_READ, MAP_PRIVATE, 3, 0) = 0x7f8e83fdf000
close(3)                                = 0
openat(AT_FDCWD, "/lib/x86_64-linux-gnu/libc.so.6", O_RDONLY|O_CLOEXEC) = 3
read(3, "\177ELF\2\1\1\3\0\0\0\0\0\0\0\0\3\0>\0\1\0\0\0\320\203\2\0\0\0\0\0"..., 832) = 832
fstat(3, {st_mode=S_IFREG|0755, st_size=2029592, ...}) = 0
mmap(NULL, 8192, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0) = 0x7f8e83fdd000
+++ exited with 0 +++
`
	r := strings.NewReader(input)
	syscalls, err := AnalyzeStrace(r)
	if err != nil {
		t.Fatalf("AnalyzeStrace failed: %v", err)
	}

	expected := []string{"access", "arch_prctl", "brk", "close", "execve", "fstat", "mmap", "openat", "read"}

	if len(syscalls) != len(expected) {
		t.Errorf("Expected %d syscalls, got %d: %v", len(expected), len(syscalls), syscalls)
	}

	for i, s := range expected {
		if syscalls[i] != s {
			t.Errorf("Expected syscall %s at index %d, got %s", s, i, syscalls[i])
		}
	}
}

func TestAnalyzeStrace_WithResumed(t *testing.T) {
	input := `
futex(0x7f8e83fdd000, FUTEX_WAIT_PRIVATE, 2, NULL <unfinished ...>
+++ exited with 0 +++
<... futex resumed> )                   = -1 EAGAIN (Resource temporarily unavailable)
`
	r := strings.NewReader(input)
	syscalls, err := AnalyzeStrace(r)
	if err != nil {
		t.Fatalf("AnalyzeStrace failed: %v", err)
	}

	expected := []string{"futex"}
	if len(syscalls) != len(expected) || syscalls[0] != "futex" {
		t.Errorf("Expected %v, got %v", expected, syscalls)
	}
}

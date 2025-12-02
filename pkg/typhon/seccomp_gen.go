package typhon

import (
	"sort"
)

// SeccompProfileGenerator generates seccomp profiles based on templates and rules
type SeccompProfileGenerator struct {
	// Base syscalls allowed for all profiles
	baseSyscalls []string
	// Template-specific syscall additions
	templateSyscalls map[string][]string
}

// NewSeccompProfileGenerator creates a new generator with defaults
func NewSeccompProfileGenerator() *SeccompProfileGenerator {
	return &SeccompProfileGenerator{
		baseSyscalls: []string{
			"read", "write", "open", "openat", "close", "stat", "fstat", "lstat",
			"poll", "lseek", "mmap", "mprotect", "munmap", "brk", "rt_sigaction",
			"rt_sigprocmask", "rt_sigreturn", "ioctl", "pread64", "pwrite64",
			"readv", "writev", "access", "pipe", "select", "sched_yield",
			"mremap", "msync", "mincore", "madvise", "shmget", "shmat", "shmctl",
			"dup", "dup2", "dup3", "fcntl", "flock", "fsync", "fdatasync",
			"truncate", "ftruncate", "getdents", "getcwd", "chdir", "fchdir",
			"rename", "mkdir", "rmdir", "creat", "link", "unlink", "symlink",
			"readlink", "chmod", "fchmod", "chown", "fchown", "lchown", "umask",
			"gettimeofday", "getrlimit", "getrusage", "sysinfo", "times",
			"ptrace", "getuid", "syslog", "getgid", "setuid", "setgid",
			"geteuid", "getegid", "setpgid", "getppid", "getpgrp", "setsid",
			"setreuid", "setregid", "getgroups", "setgroups", "setresuid",
			"getresuid", "setresgid", "getresgid", "getpgid", "setfsuid",
			"setfsgid", "getsid", "capget", "capset", "rt_sigpending",
			"rt_sigtimedwait", "rt_sigqueueinfo", "rt_sigsuspend", "sigaltstack",
			"utime", "mknod", "uselib", "personality", "ustat", "statfs",
			"fstatfs", "sysfs", "getpriority", "setpriority", "sched_setparam",
			"sched_getparam", "sched_setscheduler", "sched_getscheduler",
			"sched_get_priority_max", "sched_get_priority_min", "sched_rr_get_interval",
			"mlock", "munlock", "mlockall", "munlockall", "vhangup", "modify_ldt",
			"pivot_root", "_sysctl", "prctl", "arch_prctl", "adjtimex",
			"setrlimit", "chroot", "sync", "acct", "settimeofday", "mount",
			"umount2", "swapon", "swapoff", "reboot", "sethostname", "setdomainname",
			"iopl", "ioperm", "create_module", "init_module", "delete_module",
			"get_kernel_syms", "query_module", "quotactl", "nfsservctl",
			"getpmsg", "putpmsg", "afs_syscall", "tuxcall", "security",
			"gettid", "readahead", "setxattr", "lsetxattr", "fsetxattr",
			"getxattr", "lgetxattr", "fgetxattr", "listxattr", "llistxattr",
			"flistxattr", "removexattr", "lremovexattr", "fremovexattr",
			"tkill", "time", "futex", "sched_setaffinity", "sched_getaffinity",
			"set_thread_area", "io_setup", "io_destroy", "io_getevents",
			"io_submit", "io_cancel", "get_thread_area", "lookup_dcookie",
			"epoll_create", "epoll_ctl_old", "epoll_wait_old", "remap_file_pages",
			"getdents64", "set_tid_address", "restart_syscall", "semtimedop",
			"fadvise64", "timer_create", "timer_settime", "timer_gettime",
			"timer_getoverrun", "timer_delete", "clock_settime", "clock_gettime",
			"clock_getres", "clock_nanosleep", "exit", "exit_group",
			"wait4", "waitid", "set_robust_list", "get_robust_list",
			"splice", "tee", "sync_file_range", "vmsplice", "move_pages",
			"utimensat", "epoll_pwait", "signalfd", "timerfd_create",
			"eventfd", "fallocate", "timerfd_settime", "timerfd_gettime",
			"accept", "accept4", "bind", "connect", "listen", "recvfrom",
			"recvmsg", "sendto", "sendmsg", "setsockopt", "getsockopt",
			"socket", "socketpair", "shutdown",
		},
		templateSyscalls: map[string][]string{
			"python-ds": {
				// Python Data Science specific needs (e.g. numpy, pandas might need specific ops)
				// Usually the base set covers most, but maybe some specialized ones
			},
			"nodejs": {
				// Node.js specific
			},
			"static": {
				// Static binaries might need less?
				// For now we just use base.
			},
			"minimal": {
				// Very restricted set
			},
		},
	}
}

// GenerateProfile generates a profile for a given template
func (g *SeccompProfileGenerator) GenerateProfile(templateID string, extraSyscalls []string) (*SeccompProfile, error) {
	// Start with base syscalls
	allowed := make(map[string]bool)
	for _, s := range g.baseSyscalls {
		allowed[s] = true
	}

	// Add template specific syscalls
	if extras, ok := g.templateSyscalls[templateID]; ok {
		for _, s := range extras {
			allowed[s] = true
		}
	}

	// Add extra syscalls from request
	for _, s := range extraSyscalls {
		allowed[s] = true
	}

	// Convert to list
	var syscallList []string
	for s := range allowed {
		syscallList = append(syscallList, s)
	}
	sort.Strings(syscallList)

	// Construct profile
	return &SeccompProfile{
		DefaultAction: "Errno", // Default deny with error
		Syscalls: []Syscall{
			{
				Names:  syscallList,
				Action: "Allow",
			},
		},
	}, nil
}

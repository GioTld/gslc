"""Inject one Linux x86_64 syscall outcome into a child compiler for acceptance tests."""
import ctypes
import os
import signal
import subprocess
import sys

PTRACE_TRACEME = 0
PTRACE_GETREGS = 12
PTRACE_SETREGS = 13
PTRACE_SYSCALL = 24
PTRACE_SETOPTIONS = 0x4200
PTRACE_O_TRACESYSGOOD = 1
PTRACE_O_EXITKILL = 0x100000
SYSCALL_STOP = signal.SIGTRAP | 0x80
SYS_WRITE, SYS_OPEN, SYS_CLOSE, SYS_FSYNC, SYS_RENAME = 1, 2, 3, 74, 82
OUTPUT_OPEN_FLAGS = os.O_WRONLY | os.O_CREAT | os.O_EXCL


class Registers(ctypes.Structure):
    _fields_ = [(name, ctypes.c_ulonglong) for name in
                "r15 r14 r13 r12 rbp rbx r11 r10 r9 r8 rax rcx rdx rsi rdi orig_rax rip cs eflags rsp ss fs_base gs_base ds es fs gs".split()]


libc = ctypes.CDLL(None, use_errno=True)
libc.ptrace.restype = ctypes.c_long
libc.ptrace.argtypes = [ctypes.c_ulong, ctypes.c_ulong, ctypes.c_void_p, ctypes.c_void_p]


def trace(request, pid=0, data=None):
    if libc.ptrace(request, pid, None, data) == -1:
        error = ctypes.get_errno()
        raise OSError(error, os.strerror(error))


def inject(fault, command):
    child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             preexec_fn=lambda: trace(PTRACE_TRACEME))
    try:
        _, status = os.waitpid(child.pid, 0)
        if not os.WIFSTOPPED(status):
            raise RuntimeError("compiler did not enter syscall tracing")
        trace(PTRACE_SETOPTIONS, child.pid, PTRACE_O_TRACESYSGOOD | PTRACE_O_EXITKILL)
        entering, applied, output_fd = True, False, None
        override, output_open, deliver = None, False, 0
        while True:
            trace(PTRACE_SYSCALL, child.pid, deliver)
            _, status = os.waitpid(child.pid, 0)
            if os.WIFEXITED(status) or os.WIFSIGNALED(status):
                child.returncode = os.waitstatus_to_exitcode(status)
                break
            stopped = os.WSTOPSIG(status)
            deliver = 0
            if stopped != SYSCALL_STOP:
                deliver = stopped
                continue
            regs = Registers()
            trace(PTRACE_GETREGS, child.pid, ctypes.byref(regs))
            if entering:
                output_open = regs.orig_rax == SYS_OPEN and regs.rsi == OUTPUT_OPEN_FLAGS
                wanted = {"short": SYS_WRITE, "interrupted": SYS_WRITE, "zero": SYS_WRITE,
                          "close": SYS_CLOSE, "sync": SYS_FSYNC, "rename": SYS_RENAME}[fault]
                if not applied and output_fd is not None and regs.orig_rax == wanted and (wanted == SYS_RENAME or regs.rdi == output_fd):
                    applied = True
                    if fault == "short":
                        regs.rdx = min(regs.rdx, 3)
                    else:
                        override = {"interrupted": -4, "zero": 0}.get(fault, -5)
                        # close must actually release the descriptor, even when reporting EIO.
                        if fault != "close":
                            regs.orig_rax = ctypes.c_ulonglong(-1).value
                    trace(PTRACE_SETREGS, child.pid, ctypes.byref(regs))
            else:
                if output_open:
                    output_fd = ctypes.c_longlong(regs.rax).value
                if override is not None:
                    regs.rax = ctypes.c_ulonglong(override).value
                    trace(PTRACE_SETREGS, child.pid, ctypes.byref(regs))
                    override = None
            entering = not entering
        stdout, stderr = child.communicate()
        if not applied:
            raise RuntimeError("requested output fault was not reached")
        sys.stdout.buffer.write(stdout)
        sys.stderr.buffer.write(stderr)
        return child.returncode
    finally:
        if child.returncode is None:
            child.kill()
            child.wait()


if __name__ == "__main__":
    sys.exit(inject(sys.argv[1], sys.argv[2:]))

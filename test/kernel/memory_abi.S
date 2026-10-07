.section .text
.global memory_abi_probe
memory_abi_probe:
    push %rbx
    sub $32, %rsp
    mov %rsp, %rbx
    mov %rbx, %rdi
    mov $255, %esi
    mov $16, %edx
    call memset
    cmp %rbx, %rax
    jne failed
    lea 16(%rbx), %rdi
    mov %rbx, %rsi
    mov $16, %edx
    call memcpy
    lea 16(%rbx), %rcx
    cmp %rcx, %rax
    jne failed
    mov %rbx, %rdi
    lea 16(%rbx), %rsi
    mov $16, %edx
    call memcmp
    test %eax, %eax
    jne failed
    movb $0, (%rbx)
    movb $128, 1(%rbx)
    lea 1(%rbx), %rdi
    mov %rbx, %rsi
    mov $15, %edx
    call memmove
    lea 1(%rbx), %rcx
    cmp %rcx, %rax
    jne failed
    cmpb $0, 1(%rbx)
    jne failed
    cmpb $128, 2(%rbx)
    jne failed
    mov %rbx, %rdi
    lea 1(%rbx), %rsi
    mov $15, %edx
    call memmove
    cmp %rbx, %rax
    jne failed
    cmpb $128, 1(%rbx)
    jne failed
    mov %rbx, %rdi
    lea 16(%rbx), %rsi
    mov $16, %edx
    call memcmp
    cmp $-255, %eax
    jne failed
    lea 16(%rbx), %rdi
    mov %rbx, %rsi
    mov $16, %edx
    call memcmp
    cmp $255, %eax
    jne failed
    xor %edi, %edi
    xor %esi, %esi
    xor %edx, %edx
    call memcmp
    test %eax, %eax
    jne failed
    jmp finished
failed:
    mov $1, %eax
finished:
    add $32, %rsp
    pop %rbx
    ret

.section .boot.bss,"aw",@nobits
.global runtime_status
runtime_status:
    .skip 4
.section .note.GNU-stack,"",@progbits

.section .multiboot2,"a"
.align 8
header_start:
.long 0xe85250d6
.long 0
.long header_end - header_start
.long -(0xe85250d6 + (header_end - header_start))
.short 0
.short 0
.long 8
header_end:

.section .multiboot1,"a"
.align 4
.long 0x1badb002
.long 3
.long -(0x1badb002 + 3)

.section .boot,"ax"
.code32
.global _start
_start:
    cli
    mov $boot_stack_top, %esp
    mov $page_directory, %edi
    mov $0x83, %eax
    mov $512, %ecx
1:
    mov %eax, (%edi)
    movl $0, 4(%edi)
    add $0x200000, %eax
    add $8, %edi
    loop 1b

    mov $page_directory, %eax
    or $3, %eax
    mov %eax, page_directory_pointer
    mov $page_directory_pointer, %eax
    or $3, %eax
    mov %eax, page_table_root

    mov %cr4, %eax
    or $0x20, %eax
    mov %eax, %cr4
    mov $page_table_root, %eax
    mov %eax, %cr3
    mov $0xc0000080, %ecx
    rdmsr
    or $0x100, %eax
    wrmsr
    mov %cr0, %eax
    or $0x80000000, %eax
    mov %eax, %cr0
    lgdt boot_gdt_pointer
    ljmp $0x08, $long_mode_start

.code64
long_mode_start:
    mov $0x10, %ax
    mov %ax, %ds
    mov %ax, %es
    mov %ax, %ss
    movabs $boot_stack_top, %rsp
    call kernel_main
2:
    cli
    hlt
    jmp 2b

.global interrupt_stub
interrupt_stub:
    push %rax
    push %rcx
    push %rdx
    push %rsi
    push %rdi
    push %r8
    push %r9
    push %r10
    push %r11
    call interrupt_gate
    pop %r11
    pop %r10
    pop %r9
    pop %r8
    pop %rdi
    pop %rsi
    pop %rdx
    pop %rcx
    pop %rax
    iretq

.align 8
boot_gdt:
    .quad 0
    .quad 0x00af9a000000ffff
    .quad 0x00af92000000ffff
boot_gdt_pointer:
    .short 23
    .long boot_gdt

.section .bss,"aw",@nobits
.align 4096
page_table_root:
    .skip 4096
page_directory_pointer:
    .skip 4096
page_directory:
    .skip 4096
.align 16
boot_stack:
    .skip 16384
boot_stack_top:
.align 16
.global kernel_gdt
kernel_gdt:
    .skip 24
.global kernel_gdtr
kernel_gdtr:
    .skip 10
.align 16
.global kernel_idt
kernel_idt:
    .skip 4096
.global kernel_idtr
kernel_idtr:
    .skip 10

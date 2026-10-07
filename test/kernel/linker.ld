ENTRY(_start)
SECTIONS {
    . = 1M;
    __kernel_start = .;
    .multiboot2 : ALIGN(8) { KEEP(*(.multiboot2)) }
    .multiboot1 : ALIGN(4) { KEEP(*(.multiboot1)) }
    .text : ALIGN(4K) { *(.boot) *(.text .text.*) }
    .rodata : ALIGN(4K) { *(.rodata .rodata.*) }
    .data : ALIGN(4K) { *(.data .data.*) }
    .bss : ALIGN(4K) { *(COMMON) *(.bss .bss.*) }
    . = ALIGN(4K);
    __kernel_end = .;
    /DISCARD/ : { *(.comment) *(.note*) *(.eh_frame*) }
}

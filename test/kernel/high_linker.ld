ENTRY(boot_entry)
KERNEL_BASE = 0xffffffff80000000;
SECTIONS {
    . = 1M;
    .multiboot2 : ALIGN(8) { KEEP(*(.multiboot2)) }
    .multiboot1 : ALIGN(4) { KEEP(*(.multiboot1)) }
    .boot : { *(.boot) }
    .boot.bss : ALIGN(4K) { *(.boot.bss) }
    . = KERNEL_BASE + 2M;
    .text : AT(ADDR(.text) - KERNEL_BASE) { *(.text .text.*) }
    . = ALIGN(4K);
    .rodata : AT(ADDR(.rodata) - KERNEL_BASE) { *(.rodata .rodata.*) }
    . = ALIGN(4K);
    .requests : AT(ADDR(.requests) - KERNEL_BASE) { KEEP(*(.requests)) }
    . = ALIGN(4K);
    .data : AT(ADDR(.data) - KERNEL_BASE) { *(.data .data.*) }
    . = ALIGN(4K);
    .bss : AT(ADDR(.bss) - KERNEL_BASE) { *(COMMON) *(.bss .bss.*) }
    /DISCARD/ : { *(.comment) *(.note*) *(.eh_frame*) }
}

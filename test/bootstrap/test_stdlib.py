"""Enforce the OS boundary of the standard library."""
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class LibraryBoundaryTests(unittest.TestCase):
    def test_core_is_os_independent(self):
        for path in (ROOT / "lib/core").glob("*.gsl"):
            text = path.read_text()
            with self.subTest(module=path.name):
                imports = re.findall(r'^import "([^"]+)"', text, re.MULTILINE)
                self.assertTrue(all(name.startswith("core/") for name in imports))
                self.assertNotRegex(text, r'\b(syscall|syscall_write|asm|asm_u64|asm_void)\s*\(')

    def test_std_uses_only_reserved_platform_modules(self):
        for path in (ROOT / "lib/std").glob("*.gsl"):
            text = path.read_text()
            with self.subTest(module=path.name):
                for name in re.findall(r'^import "([^"]+)"', text, re.MULTILINE):
                    self.assertTrue(name.startswith(("std/", "core/")) or name in
                                    {"platform/io", "platform/fs", "platform/process"})
                self.assertNotRegex(text, r'\b(syscall|syscall_write|asm|asm_u64|asm_void|linux_\w+)\s*\(')

    def test_hardware_and_deferred_modules_are_not_core(self):
        for name in ["io", "sync", "arch", "serial", "vga", "descriptors"]:
            self.assertFalse((ROOT / f"lib/core/{name}.gsl").exists())
        self.assertTrue((ROOT / "test/deferred/library/sync.gsl").is_file())
        self.assertTrue((ROOT / "lib/arch/x86_64/cpu.gsl").is_file())
        self.assertTrue((ROOT / "lib/devices/pc/serial.gsl").is_file())

"""Sanity checks for the independent arithmetic reference used by acceptance."""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))
from semantic_checks import ArithmeticTrap, evaluate, integer


class IntegerReferenceTests(unittest.TestCase):
    def test_signed_division_truncates_toward_zero(self):
        self.assertEqual(evaluate("/", -7, 3, 8, True), -2)
        self.assertEqual(evaluate("%", -7, 3, 8, True), -1)
        self.assertEqual(evaluate("/", 7, -3, 8, True), -2)
        self.assertEqual(evaluate("%", 7, -3, 8, True), 1)

    def test_width_and_sign_interpretation(self):
        self.assertEqual(integer(255, 8, True), -1)
        self.assertEqual(integer(-1, 64, False), 18446744073709551615)
        self.assertEqual(evaluate("+", 127, 1, 8, True), -128)
        self.assertEqual(evaluate("*", 255, 2, 8, False), 254)

    def test_shifts_and_unsigned_comparison(self):
        self.assertEqual(evaluate(">>", -128, 7, 8, True), -1)
        self.assertEqual(evaluate(">>", 128, 7, 8, False), 1)
        self.assertTrue(evaluate(">", 18446744073709551615, 1, 64, False))

    def test_partial_operations_trap(self):
        for op, a, b in [("/", 1, 0), ("%", 1, 0), ("/", -128, -1),
                         ("%", -128, -1), ("<<", 1, -1), (">>", 1, 8)]:
            with self.subTest(op=op, a=a, b=b), self.assertRaises(ArithmeticTrap):
                evaluate(op, a, b, 8, True)

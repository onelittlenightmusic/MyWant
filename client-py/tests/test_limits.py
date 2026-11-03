import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "engine", "core", "pysdk"))

from mywant import parallel  # noqa: E402


class ParallelTest(unittest.TestCase):
    def tearDown(self):
        os.environ.pop("MYWANT_PARALLEL", None)

    def test_unset_is_the_plugins_own(self):
        os.environ.pop("MYWANT_PARALLEL", None)
        self.assertEqual(parallel(5), 5)

    def test_the_server_limits_it(self):
        os.environ["MYWANT_PARALLEL"] = "1"
        self.assertEqual(parallel(5), 1)

    def test_never_more_than_asked_nor_less_than_one(self):
        os.environ["MYWANT_PARALLEL"] = "9"
        self.assertEqual(parallel(5), 5)
        os.environ["MYWANT_PARALLEL"] = "0"
        self.assertEqual(parallel(5), 1)
        os.environ["MYWANT_PARALLEL"] = "x"
        self.assertEqual(parallel(5), 5)


if __name__ == "__main__":
    unittest.main()

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "engine", "core", "pysdk"))

from mywant import fill_goto, fill_template, goto_template  # noqa: E402


class FillTemplateTest(unittest.TestCase):
    def test_walks_and_drops_notes(self):
        t = {"_comment": "note", "a": "{{x}}-{{y}}", "b": [{"c": "{{x}}"}, 3], "d": None}
        self.assertEqual(fill_template(t, {"x": "1", "y": 2}), {"a": "1-2", "b": [{"c": "1"}, 3], "d": None})


class FillGotoTest(unittest.TestCase):
    def test_reads_the_template(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "goto.json")
            with open(path, "w") as f:
                json.dump({"_comment": "n", "elements": [{"selector": "[value=\"{{v}}\"]", "say_key": "arrived"}]}, f)
            self.assertEqual(fill_goto({"v": "T"}, template=path),
                             {"elements": [{"selector": "[value=\"T\"]", "say_key": "arrived"}]})


class GotoTemplateTest(unittest.TestCase):
    def test_the_template_as_it_is_without_notes(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "goto.json")
            with open(path, "w") as f:
                json.dump({"_comment": "n", "elements": [{"selector": "[value=\"{{v}}\"]"}]}, f)
            self.assertEqual(goto_template(template=path), {"elements": [{"selector": "[value=\"{{v}}\"]"}]})


if __name__ == "__main__":
    unittest.main()

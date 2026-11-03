import importlib.util
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parents[1] / "runtime-benchmark.py"
SPEC = importlib.util.spec_from_file_location("runtime_benchmark", SCRIPT)
benchmark = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(benchmark)


class RuntimeBenchmarkSecurityTests(unittest.TestCase):
    def test_target_accepts_only_local_http_paths(self):
        self.assertEqual(
            benchmark.validate_target("https://panel.example", "/api/v1/health?scope=summary"),
            ("https", "panel.example", 443, True),
        )
        for path in ["//attacker.example/", "https://attacker.example/", "/\\\\attacker", "/api#fragment", "/api\nheader"]:
            with self.subTest(path=path), self.assertRaises(ValueError):
                benchmark.validate_target("https://panel.example", path)

    def test_cookie_header_respects_domain_path_secure_and_expiration(self):
        content = "\n".join(
            [
                ".example.com\tTRUE\t/\tTRUE\t2147483647\tsubdomain\tallowed",
                "api.example.com\tFALSE\t/api\tFALSE\t0\texact\tallowed",
                "other.example.com\tTRUE\t/\tFALSE\t0\tforeign\tblocked",
                ".example.com\tTRUE\t/private\tFALSE\t0\twrong_path\tblocked",
                ".example.com\tTRUE\t/\tFALSE\t1\texpired\tblocked",
                "..example.com\tTRUE\t/\tFALSE\t0\tinvalid\tblocked",
            ]
        )
        with tempfile.TemporaryDirectory() as directory:
            cookie_file = Path(directory) / "cookies.txt"
            cookie_file.write_text(content, encoding="utf-8")
            self.assertEqual(
                benchmark.read_cookie_header(cookie_file, "https", "api.example.com", "/api/v1"),
                "subdomain=allowed; exact=allowed",
            )

    def test_cookie_file_requires_https(self):
        with tempfile.TemporaryDirectory() as directory:
            cookie_file = Path(directory) / "cookies.txt"
            cookie_file.write_text("example.com\tFALSE\t/\tFALSE\t0\tsid\tsecret\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "requires an HTTPS target"):
                benchmark.read_cookie_header(cookie_file, "http", "example.com", "/")

    def test_result_target_redacts_query_and_brackets_ipv6(self):
        label = benchmark.safe_target_label("https://[2001:db8::1]:8443", "/api?token=secret")
        self.assertEqual(label, "https://[2001:db8::1]:8443/api")
        self.assertNotIn("secret", label)

    def test_requests_per_second_uses_completed_requests(self):
        self.assertEqual(benchmark.requests_per_second(3, 2), 1.5)
        self.assertEqual(benchmark.requests_per_second(3, 0), 0.0)


if __name__ == "__main__":
    unittest.main()

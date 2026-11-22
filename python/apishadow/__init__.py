"""apishadow — сравнение ответов основного и зеркального API."""

from .diff import VERSION, analyze_files, compare, compare_record, json_diff

__version__ = VERSION

__all__ = ["VERSION", "analyze_files", "compare", "compare_record", "json_diff"]

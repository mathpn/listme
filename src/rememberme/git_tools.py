"""
Git integration tools.
"""

import os
import re
import subprocess
from datetime import datetime


def blame_file(file_path: str) -> list[str]:
    """Return the git blame output for the given file."""
    blame = subprocess.getoutput(f"cd {os.path.dirname(file_path)} && git blame {file_path} -l")
    return blame.splitlines()


# TODO switch to --porcelain parsing, less error-prone
def parse_blame(blame: str) -> tuple[str, datetime]:
    match = re.match(
        "^.{40} .*\(\s*(.*?)\s+(\d{4}-[01]\d-[0-3]\d) [0-2]\d:[0-5]\d:[0-5]\d [+-][0-2]\d{3} .*\)",
        blame,
    )
    if not match:
        return "", datetime.today()
    if len(match.groups()) != 2:
        return "", datetime.today()
    name, date = match.groups()
    date = datetime.strptime(date, "%Y-%m-%d")
    return name.strip(), date


def blame_lines(file_path: str, lines: list[int]) -> list[tuple[str, datetime]]:
    """Return a list of (name, datetime) for each line to be blamed."""
    blames = blame_file(file_path)
    if blames[0].startswith("fatal"):
        return [("", datetime.today()) for _ in range(1 + max(lines))]
    return [parse_blame(blames[line - 1]) for line in lines]

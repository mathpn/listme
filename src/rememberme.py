"""
Draft of main script. This uses ripgrep on the background for speed.
"""

from collections import defaultdict
import re
import argparse
import subprocess


TAGS = ["TODO", "FIXME", "XXX", "NOTE", "BUG", "OPTIMIZE"]
REGEX = f"({'|'.join(TAGS)}) "
INLINE_REGEX = "^(#+|//+|<!--|--|/\*|\"\"\")|-->$"


# TODO summary per file
def parse_rg_output(output: str) -> dict[str, list]:
    lines = output.splitlines()
    by_file = defaultdict(list)
    for line in lines:
        match = re.search("(.*):(.*):(.*)", line)
        if not match:
            continue
        file, *content = match.groups()
        by_file[file].append(content)
    return by_file


# TODO colorful + emojis
def print_parsed_output(by_file: dict[str, list]) -> None:
    for file, contents in by_file.items():
        print(f"\n{file}")
        for content in contents:
            line = content[0] + ": " + re.sub(INLINE_REGEX, "", content[1]).strip()
            print(line)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("folder", type=str, default=".")
    args = parser.parse_args()

    # TODO better regex
    rg_output = subprocess.check_output(["./bin/rg", REGEX, args.folder, "-n"])
    rg_output = rg_output.decode('utf-8')
    by_file = parse_rg_output(rg_output)
    print_parsed_output(by_file)


if __name__ == '__main__':
    main()

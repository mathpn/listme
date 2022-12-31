from contextlib import redirect_stdout
import io

from rememberme.parser import main


def test_parser():
    with redirect_stdout(io.StringIO()) as out:
        main(["tests/generic_code.py", "-p"])
    out = out.getvalue().splitlines()
    print(out)
    assert out == [
        "generic_code.py",
        "        NOTE: this is a note in a multiline comment",
        "        TODO: type hints",
        "        HACK: this uses the Dijkstra algorithm",
        "        FIXME: don't forget to fix me",
        "        OPTIMIZE: this could be optimized",
        "        XXX: this has an unusual arranjement of # before the comment",
        "        BUG: I'm not a bug, don't believe them!",
        "",
    ]


if __name__ == "__main__":
    test_parser()

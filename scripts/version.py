"""Read the shared application VERSION, or an explicitly supplied package manifest."""
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else pathlib.Path(__file__).resolve().parents[1] / "VERSION"
text = path.read_text().strip()
if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", text):
    print(text)
else:
    matches = re.findall(r"^version[ \t]*=[ \t]*([0-9]+\.[0-9]+\.[0-9]+)[ \t]*$", text, re.MULTILINE)
    if len(matches) != 1:
        sys.exit("Invalid application version")
    print(matches[0])

import re
import sys

s = sys.stdin.read()
s = re.sub(r'0a', 'LF', s)
s = re.sub(r'0d', 'CR', s)
sys.stdout.write(s)
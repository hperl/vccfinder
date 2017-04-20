import sys
from tempfile import NamedTemporaryFile
from subprocess import check_output

with NamedTemporaryFile(suffix='.c') as f:
    f.write(sys.stdin.read())
    print check_output(["/usr/bin/rats", f.name])

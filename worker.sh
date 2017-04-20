#!/bin/bash

mkdir -p log
./github-data -log=import.log -log-level=info -name=$(hostname) -commit-threads=50 -commits-select=all


#!/bin/bash

OLDNAME=importunstableV57
NAME=importunstableV58
START=1
END=5

echo Starting $NAME

go build

for i in $(seq $START $END); do
	echo syncing security-research-crawler-$i
	ssh security-research-crawler-$i "screen -X -S $OLDNAME quit; rm -rf ./$OLDNAME; mkdir -p $NAME"
	scp github-data security-research-crawler-$i:./$NAME 1>/dev/null
	scp worker.sh security-research-crawler-$i:./$NAME 1>/dev/null
	#scp flawfinder.py security-research-crawler-$i:./$NAME 1>/dev/null
	ssh security-research-crawler-$i "./$NAME/github-data -self-test"
done

ssh security-research-crawler-1 "./$NAME/github-data -init-redis=true"

for i in $(seq 2 $END); do
	echo starting security-research-crawler-$i
	ssh security-research-crawler-$i "cd $NAME && screen -S $NAME -d -m ./worker.sh"
done

echo Finished $NAME

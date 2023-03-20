#!/bin/sh
# Watch the given JobID and when it finishes launches the next job

JOB_ID=$1
NEXT_JOB_PROCESS_ID=$2
NEXT_JOB_INPUTS=$3

STATUS="running"

while [ "$STATUS" = "running" ]
do
    sleep 5s
    # jq must be installed
    STATUS=$(curl "http://192.168.54.53:5050/jobs/${JOB_ID}" | jq -r '.status')
done

if [ "$STATUS" = "successful" ]
then
    NEXT_JOB_ID=$(
        formdata=$( jq -c -n --argjson inputs "$NEXT_JOB_INPUTS" '$ARGS.named' ) &&
        curl "http://192.168.54.53:5050/processes/${NEXT_JOB_PROCESS_ID}/execution" \
            --header 'Content-Type: application/json' \
            --data "$formdata" \
        | jq -r '.jobID')
    echo "$NEXT_JOB_ID"
else
    echo 'watchJob staus not successful'
    exit 1
fi
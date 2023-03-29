#!/bin/sh
# Watch the given JobID and when it finishes launches the next job

PARAMS=$1

WATCH_JOB_ID=$( jq -n -r --argjson data "$PARAMS" '$data.watchJobId')
NEXT_JOB_PROCESS_ID=$( jq -n -r --argjson data "$PARAMS" '$data.nextJobProcessId')
NEXT_JOB_INPUTS=$( jq -n -r --argjson data "$PARAMS" '$data.nextJobInputs')
JOB_ID=$( jq -n -r --argjson data "$PARAMS" '$data.jobID')

STATUS="running"

# check if WATCH_JOB_ID is correct
STATUSCODE=$(curl -s -o /dev/null -w "%{http_code}" "http://192.168.80.116:5050/jobs/${WATCH_JOB_ID}")

if test $STATUSCODE -ne 200; then
    echo 'watchJobId not found'
    exit 1
fi

while [ "$STATUS" = "running" ]
do
    sleep 5s
    # jq must be installed
    STATUS=$(curl -s "http://192.168.80.116:5050/jobs/${WATCH_JOB_ID}" | jq -r '.status')
done

if [ "$STATUS" = "successful" ]
then
    NEXT_JOB_ID=$(
        curl -s "http://192.168.80.116:5050/processes/${NEXT_JOB_PROCESS_ID}/execution" \
            --header 'Content-Type: application/json' \
            --data "$NEXT_JOB_INPUTS" \
        | jq -r '.jobID')
else
    echo 'watchJob did not succeed'
    exit 1
fi

jq -n -j --arg JOB_ID "$JOB_ID" '{nextJobId: $JOB_ID}' > ${JOB_ID}.json
aws s3 cp ${JOB_ID}.json s3://${S3_BUCKET}/results/ --content-type 'application/json'


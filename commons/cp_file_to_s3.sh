#!/bin/sh
# Copy a single file to S3
# Must pass path of the file and destination on s3

CONTENT_TYPE="application/text"
DATE_VALUE=`date -R`
FILE_PATH=$1
S3_PATH=$2

function copyToS3()
{
    FNAME=$(basename $FILE_PATH)
    echo "Start sending $FNAME to S3"
    RESOURCE="/${S3_BUCKET}/${S3_PATH}/{FNAME}"
    STRING_TO_SIGN="PUT\n\n${CONTENT_TYPE}\n${DATE_VALUE}\n${RESOURCE}"
    SIGNATURE_HASH=`echo -en ${STRING_TO_SIGN} | openssl sha1 -hmac ${AWS_SECRET_ACCESS_KEY} -binary | base64`

    curl -X PUT -T "${FILE_PATH}" \
        -H "Host: ${S3_BUCKET}.s3.amazonaws.com" \
        -H "Date: ${DATE_VALUE}" \
        -H "Content-Type: ${CONTENT_TYPE}" \
        -H "Authorization: AWS ${AWS_ACCESS_KEY_ID}:${SIGNATURE_HASH}" \
        https://${S3_BUCKET}.s3.amazonaws.com/${S3_PATH}/${NOW_DATE}/${FNAME}
    echo "Finished"
}

copyToS3
FROM curlimages/curl:7.88.1

USER root

RUN apk update \
    && apk add jq \
    && apk add --no-cache aws-cli

COPY call_next_job.sh /

RUN chmod +x call_next_job.sh

ENTRYPOINT ["./call_next_job.sh"]
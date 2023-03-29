FROM curlimages/curl:7.88.1

USER root

RUN apk update \
    && apk add jq \
    && apk add openssl

COPY call_next_job.sh /
RUN wget https://gist.githubusercontent.com/ar-siddiqui/820bf4a13eb20de38bf503b80c503add/raw/cp_file_to_s3.sh

RUN chmod +x call_next_job.sh
RUN chmod +x cp_file_to_s3.sh

ENTRYPOINT ["./call_next_job.sh"]
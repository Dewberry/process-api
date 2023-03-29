FROM osgeo/gdal:alpine-small-latest

USER root

RUN apk update \
    && apk add jq \
    && apk add --no-cache aws-cli

COPY entrypoint.sh /

RUN chmod +x entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]
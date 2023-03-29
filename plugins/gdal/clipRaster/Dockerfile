FROM osgeo/gdal:alpine-small-latest

USER root

RUN apk update \
    && apk add jq

COPY entrypoint.sh /

RUN chmod +x entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]
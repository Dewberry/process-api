FROM osgeo/gdal:alpine-small-latest

USER root

RUN apk update \
    && apk add --no-cache python3 py3-pip

COPY requirements.txt .
RUN pip3 install -r requirements.txt

ADD processes /app/
WORKDIR /app

ENTRYPOINT ["python3"]
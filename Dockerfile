FROM golang:1.23.9 AS builder

RUN apt update &&\
	  apt -y install gdal-bin gdal-data libgdal-dev

COPY . /src

WORKDIR /src

RUN go build -o hms-mutator

#-------------

FROM ubuntu:24.04 AS prod

ENV PATH=/root/.local/bin:$PATH

RUN apt update &&\
    apt -y install libssl-dev libbz2-dev libgdbm-dev uuid-dev libncurses-dev libffi-dev libgdbm-compat-dev sqlite3 lzma lzma-dev &&\
    apt -y install gdal-bin gdal-data libgdal-dev

COPY --from=builder /src/hms-mutator /app/hms-mutator
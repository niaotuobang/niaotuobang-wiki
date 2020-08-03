FROM golang:latest
WORKDIR /root/wiki
ENV GOPROXY https://goproxy.cn

COPY  . .
RUN go build cmd/web.go

CMD ["/root/wiki/web"]

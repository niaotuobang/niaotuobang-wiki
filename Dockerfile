FROM golang:latest

WORKDIR /niaotuobang/wiki

COPY go.mod .
RUN go mod download

COPY . .
RUN go build -o app main.go

ENTRYPOINT [ "/niaotuobang/wiki/app" ]

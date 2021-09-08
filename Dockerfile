FROM golang:1.17 as golang
WORKDIR /go/src/app
COPY go.* ./
RUN go mod download -x
COPY . .
RUN go install -v
ENTRYPOINT ["/go/bin/telegram-quezlet-bot"]
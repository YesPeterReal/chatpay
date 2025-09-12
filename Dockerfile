FROM golang:1.20
WORKDIR /app
COPY main.go .
RUN go mod init chatpay
RUN go get github.com/lib/pq
RUN go build -o chatpay .
EXPOSE 8080
CMD ["./chatpay"]
FROM golang:1.20
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -o chatpay .
EXPOSE 8080
CMD ["./chatpay"]
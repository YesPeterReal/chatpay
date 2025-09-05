FROM golang:1.25
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o ledger main.go
EXPOSE 8080
CMD ["./ledger"]